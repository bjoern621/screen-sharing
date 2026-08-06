// Package moq reaches the relay's Media-over-QUIC listener on behalf of the web
// grid.
//
// MoQ rides on WebTransport, which refuses a plain listener: the relay serves it
// over HTTPS or not at all. That leaves the certificate as the one thing a viewer
// has to settle before it can subscribe, and it is settled here rather than in the
// page because the page cannot.
//
// A browser reading the relay's own MoQ page has already accepted the certificate
// to load the page. The web grid has not: it runs on the app's own origin, so a
// self-signed relay certificate fails the fingerprint fetch in the webview before
// any of it reaches WebTransport. This package makes that request from Go, where
// the verification decision is ours to make, and hands the page a fingerprint it
// pins through serverCertificateHashes.
//
// The pin is what makes the unverified case honest rather than merely permissive.
// Fetch reports which of the two happened, so a caller can say whether the relay
// proved its identity or was taken on trust.
//
// The certificate this reports is the HTTP/3 listener's, not the HTTP/2 one whose
// TLS the request itself rode on, and the two are genuinely different: measured
// against a v1.20.0 relay, the TCP listener presents moqServerCert verbatim while
// the fingerprint endpoint answers with something else. That is the useful half.
// WebTransport connects to the HTTP/3 listener, so the endpoint describes the peer
// the page will meet, and MediaMTX generating a separate certificate for it is what
// lets the pin satisfy WebTransport's rule that a pinned certificate be ECDSA and
// live at most 14 days - which moqServerCert, generated for 3650, could never meet.
//
// A short-lived certificate is one that rotates, so a fingerprint is only good
// until it does. Fetch runs per sink, so every new tile pins afresh; what it cannot
// help is a reader retrying inside a tile that has been open longer than the
// certificate lived, which fails until the tile is reconnected.
package moq

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// fetchTimeout bounds one fingerprint request. It is a single small GET against
// the relay, so a wait past this is a listener that is not there rather than a
// slow one, and the grid tile should say so instead of holding its loading state.
const fetchTimeout = 3 * time.Second

// sha256HexLen is the length of the fingerprint the relay writes: SHA-256 of the
// certificate's DER bytes, hex-encoded, no separators.
const sha256HexLen = 64

// Cert is the relay's Media-over-QUIC certificate as the web grid needs it.
//
// Verified says whether the certificate chained to a root this machine trusts.
// It is not a precondition for subscribing - an unverified certificate is still
// pinned by Fingerprint, which is what the relay's own page does - but it is the
// difference between a relay that proved its identity and one taken on trust, and
// only the caller can decide what to tell the user about that.
type Cert struct {
	// Fingerprint is the SHA-256 of the certificate, hex-encoded lowercase. It is
	// the value the page hands WebTransport as a serverCertificateHashes entry.
	Fingerprint string `json:"fingerprint"`
	Verified    bool   `json:"verified"`
}

// Fetch reads the relay's Media-over-QUIC certificate fingerprint.
//
// Verification is attempted first and the unverified retry happens only after it
// fails, so a relay carrying a real certificate is held to it and a self-signed
// one still yields a pin. Ordering them the other way would silently downgrade
// every relay to trust-on-first-use, including the ones that did not need to be.
//
// An unreachable listener is an environment failure and comes back as an error:
// the relay may simply be running without moq enabled, which the grid reports on
// the tile rather than treating as a defect.
func Fetch(ctx context.Context, host string, port int, streamName string) (Cert, error) {
	assert.Assert(host != "", "a fingerprint request names a relay host")
	assert.Assert(port > 0, "a fingerprint request names a listener port", port)
	assert.Assert(streamName != "", "a fingerprint request names a stream", streamName)

	url := FingerprintURL(host, port, streamName)

	fingerprint, verifiedErr := get(ctx, url, true)
	if verifiedErr == nil {
		return Cert{Fingerprint: fingerprint, Verified: true}, nil
	}

	fingerprint, err := get(ctx, url, false)
	if err != nil {
		// The unverified attempt is the one that says whether the listener is
		// there at all, so its error is the one worth reporting.
		return Cert{}, err
	}
	return Cert{Fingerprint: fingerprint, Verified: false}, nil
}

// FingerprintURL is the endpoint serving the certificate fingerprint for a
// stream. MediaMTX answers it on the TCP half of the MoQ listener, the same port
// number the WebTransport endpoint uses over UDP.
func FingerprintURL(host string, port int, streamName string) string {
	return fmt.Sprintf("https://%s:%d/%s/fingerprint", host, port, streamName)
}

// WatchURL is the WebTransport endpoint a reader subscribes on. It is the watch
// leg's address and has no publish counterpart here, since no engine this app
// drives publishes MoQ.
func WatchURL(host string, port int, streamName string) string {
	return fmt.Sprintf("https://%s:%d/%s/moq", host, port, streamName)
}

// get performs one fingerprint request and validates what came back.
//
// A transport is built per call rather than shared, because the two calls differ
// in exactly the setting a shared client would have to hold constant.
func get(ctx context.Context, url string, verify bool) (string, error) {
	client := &http.Client{
		Timeout: fetchTimeout,
		Transport: &http.Transport{
			//nolint:gosec // The unverified pass exists so a self-signed relay can
			// still be pinned; the fingerprint it returns is what the page then
			// requires the peer to present. See the package comment.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: !verify},
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("relay answered %s for the MoQ certificate fingerprint", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, sha256HexLen*2))
	if err != nil {
		return "", err
	}

	fingerprint := strings.TrimSpace(string(body))
	// A listener that is not MediaMTX's MoQ server can answer 200 with anything.
	// Rejecting it here keeps a body the page could not pin from reaching it as a
	// WebTransport failure with no stated cause.
	if len(fingerprint) != sha256HexLen {
		return "", fmt.Errorf("relay returned a %d-character MoQ fingerprint, not the %d of a SHA-256 hash",
			len(fingerprint), sha256HexLen)
	}
	if _, err := hex.DecodeString(fingerprint); err != nil {
		return "", fmt.Errorf("relay returned a non-hexadecimal MoQ fingerprint")
	}

	return strings.ToLower(fingerprint), nil
}
