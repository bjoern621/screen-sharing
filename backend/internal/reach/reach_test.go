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
		legGroups: "https://relay.example/jwks.json",
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

// A relay this network reaches directly has no proxy in front of it, so every HTTP leg is a port of
// the relay's own (settings.Relay.HTTPOrigin).
func TestARelayOnThisNetworkIsDialledOnItsOwnPorts(t *testing.T) {
	s := settings.Defaults()
	s.Relay.Host = "192.168.1.9"

	want := map[string]string{
		legGroups: "http://192.168.1.9:9443/jwks.json",
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

// The relay answers its API to loopback alone (deploy/mediamtx-groups.yml), so a check dialling it
// from anywhere else would print a cross against a relay that is behaving.
func TestTheRelayApiIsDialledOnThisMachineAlone(t *testing.T) {
	for host, want := range map[string]Reason{
		"127.0.0.1":                    ReasonNone,
		"localhost":                    ReasonNone,
		"192.168.1.9":                  ReasonLoopbackOnly,
		"streamrelay.bjoernblessin.de": ReasonLoopbackOnly,
	} {
		s := settings.Defaults()
		s.Relay.Host = host

		e, ok := endpointFor(Endpoints(s), legAPI)
		if !ok {
			t.Fatalf("no %s row for relay %q", legAPI, host)
		}
		if e.Unaddressed != want {
			t.Errorf("relay %q leaves the API %v, want %v", host, e.Unaddressed, want)
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

// A transport left out of the check is a leg nobody is told about, which is the state this whole
// package exists to end.
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

// Every row carries either an address or the reason there is none, one row per leg, and never both
// answers or neither.
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

// A listener that is there and speaks something else is not the leg being checked, and saying so
// beats calling the port open: what a publish needs there is RTSP.
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

// Nothing on the port is the cross this check exists for, and the dial's own words are what a
// reader takes to a bug report.
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

// A listener that answers is the tick on the HTTP legs too, whatever status it answers with: what
// is asked is whether the server is there.
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

// A UDP port nothing is bound to swallows the request, so the answer is a wait rather than an
// error, and the check has to be the one that ends it.
//
// Ended here by the caller's own deadline rather than probeTimeout, run taking whichever comes
// first, so the suite waits a second for this instead of the five a reader waits.
func TestASilentUdpPortIsUnreachable(t *testing.T) {
	address := serveUDP(t, func([]byte) []byte { return nil })

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	r := run(ctx, "probe", target{url: "srt://" + address})
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

func resultFor(results []Result, leg string) (Result, bool) {
	for _, r := range results {
		if r.Leg == leg {
			return r, true
		}
	}
	return Result{}, false
}

// probeOne runs one address through the probe its scheme names, which is what Check does per row
// with the address the settings resolved.
func probeOne(t *testing.T, address string, insecure bool) Result {
	t.Helper()

	return run(t.Context(), "probe", target{url: address, insecure: insecure})
}

// serveTLS answers each connection with whatever handle writes, under a certificate nothing issued,
// which is the relay a development machine runs (scripts/relay.sh).
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
