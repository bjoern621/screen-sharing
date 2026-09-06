package reach

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// probeTimeout bounds one leg.
// A port with nothing behind it drops packets rather than refusing them, so a closed listener costs
// the whole timeout.
// Legs are dialled at once, so a check pays it once.
const probeTimeout = 5 * time.Second

// answer is what one probe got back.
type answer struct {
	// detail is the listener's own words: a status line, a certificate, a handshake.
	detail string
	// version is what the listener named for itself, empty where it named none.
	version string
}

// probes is one entry per URL scheme, since what a listener answers follows the protocol
// its address names, not the leg carrying it.
//
// A scheme with no entry is a transport addressing its listener in a protocol nothing speaks,
// which run asserts rather than reports.
var probes = map[string]func(context.Context, target) (answer, error){
	"http":  probeHTTP,
	"https": probeHTTP,
	"rtsps": probeRTSP,
	"rtmps": probeTLS,
	"srt":   probeSRT,
}

// probeHTTP makes the leg's own request and answers the status line, with the version the server
// named.
//
// A route the relay serves answers 2xx where the request carries the credential the settings hold.
// Any status counts as the listener answering: the question is whether the server is there, and
// a reader holding no token is answered 401 over a listener that is up.
// A route held to a success is the exception, its own service owning it and taking no credential
// (target.wantOK).
func probeHTTP(ctx context.Context, t target) (answer, error) {
	assert.Assert(t.url != "", "an HTTP probe names what it fetches")
	assert.Assert(t.method != "", "an HTTP probe names what it asks", t.url)

	request, err := http.NewRequestWithContext(ctx, t.method, t.url, nil)
	if err != nil {
		return answer{}, err
	}
	if t.credential.name != "" {
		request.Header.Set(t.credential.name, t.credential.value)
	}

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: t.insecure}}}
	response, err := client.Do(request)
	if err != nil {
		return answer{}, err
	}
	defer response.Body.Close()

	if t.wantOK && response.StatusCode/100 != 2 {
		return answer{}, fmt.Errorf("answered %q, so the service is missing or older than this check", response.Status)
	}
	return answer{detail: response.Status, version: versionOf(response.Header.Get("Server"))}, nil
}

// versionOf is the version a server named itself with: "groupd/0.6.1" is "0.6.1".
//
// The Server header rather than a route of its own, so one request answers both questions and
// every route carries the answer.
// A name with no version after it is a server that names none, MediaMTX being one.
func versionOf(server string) string {
	product := strings.Fields(server)
	if len(product) == 0 {
		return ""
	}

	_, version, _ := strings.Cut(product[0], "/")
	return version
}

// probeTLS opens the connection and completes the handshake, as far as a check goes on a leg whose
// next move is a publish.
// Answers the certificate the listener presented, the other thing a reader asks about a leg
// that terminates TLS itself.
func probeTLS(ctx context.Context, t target) (answer, error) {
	c, err := dialTLS(ctx, t)
	if err != nil {
		return answer{}, err
	}
	defer c.Close()

	return answer{detail: certificateOf(c)}, nil
}

// probeRTSP asks the listener for its options, the one request an RTSP server answers before
// a session exists, and answers the status line it replied with.
//
// A listener answering something else is not this leg: a publish there needs RTSP, so the row says
// so rather than calling the port open.
func probeRTSP(ctx context.Context, t target) (answer, error) {
	c, err := dialTLS(ctx, t)
	if err != nil {
		return answer{}, err
	}
	defer c.Close()

	if deadline, ok := ctx.Deadline(); ok {
		if err := c.SetDeadline(deadline); err != nil {
			return answer{}, err
		}
	}
	if _, err := fmt.Fprintf(c, "OPTIONS %s RTSP/1.0\r\nCSeq: 1\r\n\r\n", t.url); err != nil {
		return answer{}, err
	}

	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return answer{}, err
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "RTSP/") {
		return answer{}, fmt.Errorf("answered %q, which is no RTSP listener", line)
	}
	return answer{detail: line}, nil
}

// dialTLS opens the address's host and completes the handshake.
// Certificate is measured against the address's own server name, which the dialer takes off it.
func dialTLS(ctx context.Context, t target) (*tls.Conn, error) {
	u, err := url.Parse(t.url)
	assert.Assert(err == nil, "a leg's address parses", t.url)

	dialer := tls.Dialer{Config: &tls.Config{InsecureSkipVerify: t.insecure}}
	c, err := dialer.DialContext(ctx, "tcp", u.Host)
	if err != nil {
		return nil, err
	}
	return c.(*tls.Conn), nil
}

// certificateOf is what the listener presented, as a reader checks a certificate: who it is for and
// how long it lasts.
func certificateOf(c *tls.Conn) string {
	certs := c.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return "handshake answered, no certificate"
	}

	name := certs[0].Subject.CommonName
	if len(certs[0].DNSNames) > 0 {
		name = certs[0].DNSNames[0]
	}
	return fmt.Sprintf("certificate for %s, until %s", name, certs[0].NotAfter.Format(time.DateOnly))
}
