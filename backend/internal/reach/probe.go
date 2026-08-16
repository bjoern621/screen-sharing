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
// A port with nothing behind it drops the packets rather than refusing them, so this is what a
// closed listener costs, and every leg is dialled at once so a whole check costs it once.
const probeTimeout = 5 * time.Second

// probes is one entry per URL scheme, since what a listener answers follows the protocol its
// address names and not the leg that carries it.
//
// A scheme with no entry here is a transport addressing its listener in a protocol nothing can
// speak to, which run asserts rather than reports.
var probes = map[string]func(context.Context, target) (string, error){
	"http":  probeHTTP,
	"https": probeHTTP,
	"rtsps": probeRTSP,
	"rtmps": probeTLS,
	"srt":   probeSRT,
}

// probeHTTP fetches the address and answers the status line.
//
// Any status counts as the listener answering, including a refusal: what is asked here is whether
// the server is there, and a relay serving no such path answers 404 over a listener that is up.
func probeHTTP(ctx context.Context, t target) (string, error) {
	assert.Assert(t.url != "", "an HTTP probe names what it fetches")

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, t.url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: t.insecure}}}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	return response.Status, nil
}

// probeTLS opens the connection and completes the handshake, which is as far as a check goes on a
// leg whose next move is a publish.
// What comes back is the certificate the listener presented, that being the other thing a reader
// asks about a leg that terminates TLS itself.
func probeTLS(ctx context.Context, t target) (string, error) {
	c, err := dialTLS(ctx, t)
	if err != nil {
		return "", err
	}
	defer c.Close()

	return certificateOf(c), nil
}

// probeRTSP asks the listener for its options, the one request an RTSP server answers before a
// session exists, and answers the status line it replied with.
//
// A listener answering something else is not this leg, and saying so beats calling the port open:
// what a publish needs there is RTSP.
func probeRTSP(ctx context.Context, t target) (string, error) {
	c, err := dialTLS(ctx, t)
	if err != nil {
		return "", err
	}
	defer c.Close()

	if deadline, ok := ctx.Deadline(); ok {
		if err := c.SetDeadline(deadline); err != nil {
			return "", err
		}
	}
	if _, err := fmt.Fprintf(c, "OPTIONS %s RTSP/1.0\r\nCSeq: 1\r\n\r\n", t.url); err != nil {
		return "", err
	}

	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "RTSP/") {
		return "", fmt.Errorf("answered %q, which is no RTSP listener", line)
	}
	return line, nil
}

// dialTLS opens the address's host and completes the handshake.
// The server name the certificate is measured against is the address's own, which the dialer takes
// off it.
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
