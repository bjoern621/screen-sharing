package reach

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// A check dials what a stream dials.
// The address per leg is the transport's own, so a listener this deployment does not bind answers
// here instead of in a publish that waits out its connect window (transport.Listener).
func TestEveryLegIsDialledWhereThisRelayAddressesIt(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"

	want := map[string]string{
		legGroups: "https://relay.example",
		"srt":     "srt://relay.example:8890",
		"rtsp":    "rtsps://relay.example:8322",
		"rtmp":    "rtmps://relay.example:1936",
		"hls":     "https://relay.example",
		"webrtc":  "https://relay.example",
		"moq":     "https://relay.example:8892",
	}
	for _, e := range Endpoints(s) {
		address, stated := want[e.Leg]
		if !stated {
			continue
		}
		if e.Address != address {
			t.Errorf("%s is dialled at %q, want %q", e.Leg, e.Address, address)
		}
	}
}

// A relay this network reaches directly has no proxy in front of it, so every HTTP leg is a port
// of the relay's own (settings.Relay.HTTPOrigin).
func TestARelayOnThisNetworkIsDialledOnItsOwnPorts(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "192.168.1.9"

	want := map[string]string{
		legGroups: "http://192.168.1.9:9443",
		"hls":     "http://192.168.1.9:8888",
		"webrtc":  "http://192.168.1.9:8889",
		// RTSP, RTMP and MoQ terminate TLS at the relay wherever it runs, so no proxy moves them.
		"rtsp": "rtsps://192.168.1.9:8322",
		"moq":  "https://192.168.1.9:8892",
	}
	for _, e := range Endpoints(s) {
		address, stated := want[e.Leg]
		if !stated {
			continue
		}
		if e.Address != address {
			t.Errorf("%s is dialled at %q, want %q", e.Leg, e.Address, address)
		}
	}
}

// The relay's own HTTP API is no leg of this check.
// What is live comes off the group service's index, and nothing this app runs dials the API
// (internal/app, groups.go).
func TestTheRelayApiIsNoLeg(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost", "192.168.1.9", "streamrelay.bjoernblessin.de"} {
		s := settings.Defaults()
		s.Relay.Host = host

		if e, ok := endpointFor(Endpoints(s), "api"); ok {
			t.Errorf("relay %q checks its API at %q", host, e.Address)
		}
	}
}

// A relay nobody named is no address to dial, on any leg.
func TestNoRelayNamedDialsNothing(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = ""

	for _, e := range Endpoints(s) {
		if e.Unaddressed != ReasonNoRelay {
			t.Errorf("%s reads %v with no relay named, want %v", e.Leg, e.Unaddressed, ReasonNoRelay)
		}
		if e.Address != "" {
			t.Errorf("%s is dialled at %q with no relay named", e.Leg, e.Address)
		}
	}
}

// A transport left out of the check is a leg nobody is told about.
func TestEveryTransportIsChecked(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"

	endpoints := Endpoints(s)
	for name := range transport.Listeners(s) {
		if _, ok := endpointFor(endpoints, name); !ok {
			t.Errorf("transport %q is checked nowhere", name)
		}
	}
}

// One row per leg, carrying either an address or the reason there is none,
// exactly one of the two.
func TestEveryLegAnswersOnceEitherWay(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"

	seen := map[string]bool{}
	for _, e := range Endpoints(s) {
		if seen[e.Leg] {
			t.Errorf("leg %q is checked twice", e.Leg)
		}
		seen[e.Leg] = true
		if (e.Address == "") == (e.Unaddressed == ReasonNone) {
			t.Errorf("leg %q is dialled at %q and unaddressed for %v", e.Leg, e.Address, e.Unaddressed)
		}
	}
	if want := len(transport.Listeners(s)) + 2; len(seen) != want {
		t.Errorf("%d legs answered, want %d", len(seen), want)
	}
}

// A listener speaking its own protocol is the tick, and what it said is what the row carries.
func TestAListenerAnsweringRtspIsReachable(t *testing.T) {
	address := serveTLS(t, func(c net.Conn) {
		buf := make([]byte, 256)
		if _, err := c.Read(buf); err != nil {
			return
		}
		fmt.Fprint(c, "RTSP/1.0 200 OK\r\nCSeq: 1\r\n\r\n")
	})

	r := probeOne(t, "rtsps://"+address, true)
	if r.Verdict != Reachable {
		t.Fatalf("an answering RTSP listener reads %v: %s", r.Verdict, r.Detail)
	}
	if !strings.Contains(r.Detail, "RTSP/1.0 200 OK") {
		t.Errorf("the row says %q, want the listener's own status line", r.Detail)
	}
}

// A listener speaking something else is not the leg being checked: a publish there needs RTSP.
func TestAListenerThatIsNotRtspIsUnreachable(t *testing.T) {
	address := serveTLS(t, func(c net.Conn) {
		buf := make([]byte, 256)
		if _, err := c.Read(buf); err != nil {
			return
		}
		fmt.Fprint(c, "HTTP/1.1 400 Bad Request\r\n\r\n")
	})

	r := probeOne(t, "rtsps://"+address, true)
	if r.Verdict != Unreachable {
		t.Fatalf("an HTTP answer on the RTSP port reads %v, want %v", r.Verdict, Unreachable)
	}
	if !strings.Contains(r.Detail, "HTTP/1.1 400") {
		t.Errorf("the row says %q, want what the listener answered", r.Detail)
	}
}

// Nothing on the port is the cross this check exists for, and the dial's own words go to a bug
// report.
func TestAPortWithNoListenerIsUnreachable(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("taking a port to close: %v", err)
	}
	address := l.Addr().String()
	l.Close()

	r := probeOne(t, "rtsps://"+address, true)
	if r.Verdict != Unreachable {
		t.Fatalf("a closed port reads %v, want %v", r.Verdict, Unreachable)
	}
	if r.Detail == "" {
		t.Error("a cross says nothing about why")
	}
}

// A listener that answers is the tick on the HTTP legs too, whatever the status: the question
// is whether the server is there.
func TestAnHttpListenerRefusingTheRouteIsStillReachable(t *testing.T) {
	address := serveTCP(t, func(c net.Conn) {
		buf := make([]byte, 512)
		if _, err := c.Read(buf); err != nil {
			return
		}
		fmt.Fprint(c, "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n")
	})

	r := probeOne(t, "http://"+address, false)
	if r.Verdict != Reachable {
		t.Fatalf("a 404 from a listener that is up reads %v: %s", r.Verdict, r.Detail)
	}
	if !strings.Contains(r.Detail, "404") {
		t.Errorf("the row says %q, want the status the listener answered", r.Detail)
	}
}

// SRT is UDP, where opening a socket reaches nothing, so the check is the handshake every listener
// answers before a stream id or a passphrase is sent.
func TestAnSrtListenerAnsweringInductionIsReachable(t *testing.T) {
	address := serveUDP(t, func(request []byte) []byte {
		if len(request) < srtPacketBytes {
			return nil
		}
		response := make([]byte, srtPacketBytes)
		binary.BigEndian.PutUint32(response[0:], srtControlHandshake)
		binary.BigEndian.PutUint32(response[16:], 5)
		binary.BigEndian.PutUint32(response[36:], srtInduction)
		return response
	})

	r := probeOne(t, "srt://"+address, false)
	if r.Verdict != Reachable {
		t.Fatalf("an answering SRT listener reads %v: %s", r.Verdict, r.Detail)
	}
	if !strings.Contains(r.Detail, "version 5") {
		t.Errorf("the row says %q, want the version the listener answered with", r.Detail)
	}
}

// A UDP port nothing is bound to swallows the request, so the answer is a wait rather than an error
// and the check ends it.
//
// Ended here by the caller's own deadline rather than probeTimeout, run taking whichever comes
// first, so the suite waits a second instead of five.
func TestASilentUdpPortIsUnreachable(t *testing.T) {
	address := serveUDP(t, func([]byte) []byte { return nil })

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	r := run(ctx, resolved{leg: "probe", address: "srt://" + address, target: target{url: "srt://" + address, method: http.MethodGet}})
	if r.Verdict != Unreachable {
		t.Fatalf("a silent UDP port reads %v, want %v", r.Verdict, Unreachable)
	}
	if r.Detail == "" {
		t.Error("a cross says nothing about why")
	}
}

// Something else on the SRT port is not the leg answering, and a datagram back is no handshake.
func TestAUdpPortAnsweringSomethingElseIsUnreachable(t *testing.T) {
	address := serveUDP(t, func([]byte) []byte { return []byte("hello") })

	r := probeOne(t, "srt://"+address, false)
	if r.Verdict != Unreachable {
		t.Fatalf("a non-SRT answer reads %v, want %v", r.Verdict, Unreachable)
	}
}

// The request is what an SRT listener answers: a control handshake, version 4, induction.
// A field in the wrong place is a probe every relay ignores, which reads as a relay that is down.
func TestTheInductionRequestIsWhatAnSrtListenerAnswers(t *testing.T) {
	p := inductionRequest()

	if len(p) != srtPacketBytes {
		t.Fatalf("the request is %d bytes, want %d", len(p), srtPacketBytes)
	}
	for _, c := range []struct {
		what string
		at   int
		want uint32
	}{
		{"control and handshake", 0, srtControlHandshake},
		{"destination socket id", 12, 0},
		{"version", 16, 4},
		{"socket type", 20, srtDatagram},
		{"handshake type", 36, srtInduction},
		{"syn cookie", 44, 0},
	} {
		if got := binary.BigEndian.Uint32(p[c.at:]); got != c.want {
			t.Errorf("%s is %#x, want %#x", c.what, got, c.want)
		}
	}
}

// Check answers a row per leg whatever the network did, so a report prints a whole relay rather
// than the legs that happened to finish.
//
// A leg addressed nowhere is neither a tick nor a cross: nothing was dialled, and a cross would
// report a relay as broken for binding what it is configured to bind.
func TestCheckAnswersARowPerLeg(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = ""

	rows := Check(t.Context(), s)
	if want := len(transport.Listeners(s)) + 2; len(rows) != want {
		t.Fatalf("%d rows, want %d", len(rows), want)
	}
	for _, r := range rows {
		if r.Verdict != Unaddressed {
			t.Errorf("%s reads %v with no relay named, want %v", r.Leg, r.Verdict, Unaddressed)
		}
		if r.Unaddressed != ReasonNoRelay {
			t.Errorf("%s is unaddressed for %v, want %v", r.Leg, r.Unaddressed, ReasonNoRelay)
		}
		if r.Took != 0 {
			t.Errorf("%s waited %s on a leg nothing dialled", r.Leg, r.Took)
		}
	}
}

func endpointFor(endpoints []Endpoint, leg string) (Endpoint, bool) {
	for _, e := range endpoints {
		if e.Leg == leg {
			return e, true
		}
	}
	return Endpoint{}, false
}

// probeOne runs one address through the probe its scheme names, as Check does per row.
func probeOne(t *testing.T, address string, insecure bool) Result {
	t.Helper()

	return run(t.Context(), resolved{leg: "probe", address: address, target: target{url: address, method: http.MethodGet, insecure: insecure}})
}

// serveTLS answers each connection with whatever handle writes, under a certificate nothing issued,
// as a development relay runs (deploy/relay.sh).
func serveTLS(t *testing.T, handle func(net.Conn)) string {
	t.Helper()

	l, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{selfSigned(t)}})
	if err != nil {
		t.Fatalf("serving TLS: %v", err)
	}
	return accept(t, l, handle)
}

func serveTCP(t *testing.T, handle func(net.Conn)) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("serving TCP: %v", err)
	}
	return accept(t, l, handle)
}

func accept(t *testing.T, l net.Listener, handle func(net.Conn)) string {
	t.Helper()

	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				handle(c)
			}()
		}
	}()
	return l.Addr().String()
}

// serveUDP answers each datagram with whatever answer returns, and swallows it where that is nil.
func serveUDP(t *testing.T, answer func([]byte) []byte) string {
	t.Helper()

	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("serving UDP: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := c.ReadFrom(buf)
			if err != nil {
				return
			}
			if response := answer(buf[:n]); response != nil {
				c.WriteTo(response, from)
			}
		}
	}()
	return c.LocalAddr().String()
}

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("drawing a key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "relay.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("drawing a certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// A leg is dialled on the route its transport names, and the row stays about the listener.
// What a reader corrects is the address and the port, so that is what the row carries, and where
// the check reached inside it is the transport's own affair (transport.Probed).
func TestALegIsDialledOnItsOwnRoute(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"

	probes := transport.Probes(s)
	listeners := transport.Listeners(s)
	for _, row := range resolve(s) {
		route, ok := probes[row.leg]
		if !ok {
			continue
		}
		if row.target.url != route.URL || row.target.method != route.Method {
			t.Errorf("%s is dialled with %s %q, want %s %q",
				row.leg, row.target.method, row.target.url, route.Method, route.URL)
		}
		if row.address != listeners[row.leg] {
			t.Errorf("%s reads as %q, want its listener %q", row.leg, row.address, listeners[row.leg])
		}
	}
}

// A listener naming its version has it read off the row, which is what tells a relay behind
// the app it serves from one running what this build expects.
func TestAListenerNamingItsVersionCarriesItOnTheRow(t *testing.T) {
	address := serveTCP(t, func(c net.Conn) {
		buf := make([]byte, 512)
		if _, err := c.Read(buf); err != nil {
			return
		}
		fmt.Fprint(c, "HTTP/1.1 200 OK\r\nServer: groupd/0.6.1\r\nContent-Length: 0\r\n\r\n")
	})

	r := probeOne(t, "http://"+address, false)
	if r.Version != "0.6.1" {
		t.Errorf("the row reads version %q, want the one the listener named", r.Version)
	}
}

// A listener naming itself and no version leaves the row without one, rather than reading
// its name as a number.
func TestAListenerNamingNoVersionCarriesNone(t *testing.T) {
	address := serveTCP(t, func(c net.Conn) {
		buf := make([]byte, 512)
		if _, err := c.Read(buf); err != nil {
			return
		}
		fmt.Fprint(c, "HTTP/1.1 401 Unauthorized\r\nServer: mediamtx\r\nContent-Length: 0\r\n\r\n")
	})

	r := probeOne(t, "http://"+address, false)
	if r.Version != "" {
		t.Errorf("the row reads version %q from a listener that named none", r.Version)
	}
	if r.Verdict != Reachable {
		t.Errorf("a listener that answered reads %v, want %v", r.Verdict, Reachable)
	}
}

// A leg nothing dialled names no version either: nothing answered to name one.
func TestAnUndialledLegCarriesNoVersion(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = ""

	for _, r := range Check(t.Context(), s) {
		if r.Version != "" {
			t.Errorf("%s reads version %q with no relay named", r.Leg, r.Version)
		}
	}
}

// The HTTP legs carry the token the settings hold, which is what makes a relay answer the route
// rather than the credential.
// The key set is the exception: it verifies a token and cannot make one, so it is public.
func TestTheHttpLegsCarryTheRelayToken(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"
	s.Relay.Token = "a-token"

	for _, row := range resolve(s) {
		// The key set verifies a token and cannot make one, and the manager takes a link secret
		// in a body: neither route reads a relay token.
		if row.leg == legGroups || row.leg == legDiscord {
			if row.target.credential.value != "" {
				t.Errorf("%s is dialled with a relay token", row.leg)
			}
			continue
		}
		if !strings.Contains(row.target.credential.value, s.Relay.Token) {
			t.Errorf("%s is dialled with %q, want the token the settings hold", row.leg, row.target.credential.name)
		}
	}
}

// Settings holding no token dial every leg without one, rather than a header naming nothing.
func TestNoTokenDialsWithNoCredential(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"

	for _, row := range resolve(s) {
		if row.target.credential.name != "" {
			t.Errorf("%s is dialled with a %q header on settings holding no token", row.leg, row.target.credential.name)
		}
	}
}

// The token reaches the listener, so a route the relay serves is answered as a reader.
func TestAProbeSendsTheTokenItWasGiven(t *testing.T) {
	var sent string
	address := serveTCP(t, func(c net.Conn) {
		buf := make([]byte, 1024)
		n, err := c.Read(buf)
		if err != nil {
			return
		}
		sent = string(buf[:n])
		fmt.Fprint(c, "HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n")
	})

	r := run(t.Context(), resolved{
		leg:     "webrtc",
		address: "http://" + address,
		target: target{
			url:        "http://" + address + "/group/mirrorme-check/whep",
			method:     "OPTIONS",
			credential: header{name: "Authorization", value: "Bearer a-token"},
		},
	})

	if r.Verdict != Reachable {
		t.Fatalf("a listener that answered reads %v: %s", r.Verdict, r.Detail)
	}
	if !strings.Contains(sent, "OPTIONS /group/mirrorme-check/whep") {
		t.Errorf("the listener was asked %q, want the leg's own request", strings.SplitN(sent, "\r\n", 2)[0])
	}
	if !strings.Contains(sent, "Authorization: Bearer a-token") {
		t.Error("the request carries no credential")
	}
}

// Discord mode reaches the manager beside the relay for presence, tokens and the paths a group
// gets, so a check of a relay in that mode says whether it answers (docs/discord-mode.md).
func TestTheDiscordManagerIsCheckedInDiscordMode(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"
	s.Relay.DiscordMode = true

	e, ok := endpointFor(Endpoints(s), legDiscord)
	if !ok {
		t.Fatalf("the manager is checked nowhere, so nothing says whether Discord mode has one")
	}
	if e.Address != "https://relay.example/discord" {
		t.Errorf("the manager is checked at %q", e.Address)
	}
	if e.Unaddressed != ReasonNone {
		t.Errorf("the manager reads %v with Discord mode on", e.Unaddressed)
	}
}

// Discord mode off leaves the manager undialled, and says which case that is:
// nothing this machine does reaches it, so a cross against it would send a reader after a break
// that is not there.
func TestTheDiscordManagerIsUndialledWithTheModeOff(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "relay.example"

	e, ok := endpointFor(Endpoints(s), legDiscord)
	if !ok {
		t.Fatalf("the manager has no row at all with the mode off")
	}
	if e.Unaddressed != ReasonDiscordOff {
		t.Errorf("the manager reads %v with the mode off, want %v", e.Unaddressed, ReasonDiscordOff)
	}
	if e.Address != "" {
		t.Errorf("the manager is dialled at %q with the mode off", e.Address)
	}
}
