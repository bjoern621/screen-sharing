package moq

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// validFingerprint is a well-formed SHA-256 hex digest, the shape the relay
// answers a fingerprint request with.
const validFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// tlsRelay starts a TLS test server answering /fingerprint with body, and returns
// the host and port to reach it on. httptest signs with its own throwaway CA, so
// the certificate does not verify against the system roots - which is the case
// this package exists for.
func tlsRelay(t *testing.T, body string, status int) (string, int) {
	t.Helper()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/fingerprint") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	host, portText, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
	if err != nil {
		t.Fatalf("test server URL %q: %v", srv.URL, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("test server port %q: %v", portText, err)
	}
	return host, port
}

func TestFetchPinsAnUnverifiedCertificate(t *testing.T) {
	host, port := tlsRelay(t, validFingerprint, http.StatusOK)

	cert, err := Fetch(context.Background(), host, port, "alice")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if cert.Fingerprint != validFingerprint {
		t.Errorf("Fingerprint = %q, want %q", cert.Fingerprint, validFingerprint)
	}
	// The point of the field: a self-signed relay still yields a pin, and says
	// that it proved nothing.
	if cert.Verified {
		t.Error("a certificate that does not chain to a trusted root must report Verified false")
	}
}

func TestFetchLowercasesAndTrims(t *testing.T) {
	host, port := tlsRelay(t, "  "+strings.ToUpper(validFingerprint)+"\n", http.StatusOK)

	cert, err := Fetch(context.Background(), host, port, "alice")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// The page compares nothing, but it parses the string two characters at a
	// time, so whitespace and case have to be gone before it gets there.
	if cert.Fingerprint != validFingerprint {
		t.Errorf("Fingerprint = %q, want %q", cert.Fingerprint, validFingerprint)
	}
}

// A listener that is not the relay's MoQ server can answer 200 with anything. The
// refusal belongs here, where it can name the reason, rather than reaching the
// page as a WebTransport failure with no stated cause.
func TestFetchRejectsAMalformedFingerprint(t *testing.T) {
	for name, body := range map[string]string{
		"too short":    "abcd",
		"not hex":      strings.Repeat("z", sha256HexLen),
		"an HTML page": "<!doctype html><html><head><title>hi</title></head></html>",
		"empty":        "",
	} {
		t.Run(name, func(t *testing.T) {
			host, port := tlsRelay(t, body, http.StatusOK)

			if _, err := Fetch(context.Background(), host, port, "alice"); err == nil {
				t.Errorf("Fetch accepted %q as a fingerprint", body)
			}
		})
	}
}

func TestFetchReportsAnErrorStatus(t *testing.T) {
	host, port := tlsRelay(t, "", http.StatusNotFound)

	_, err := Fetch(context.Background(), host, port, "alice")
	if err == nil {
		t.Fatal("Fetch must report a relay that answered an error status")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error %q does not name the status the relay answered", err)
	}
}

// A relay with moq disabled is an environment condition the grid reports on the
// tile, not a defect: nothing is listening and Fetch has to say so rather than
// hang or panic.
func TestFetchReportsAnAbsentListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	if _, err := Fetch(context.Background(), "127.0.0.1", port, "alice"); err == nil {
		t.Error("Fetch must report a listener that is not there")
	}
}

func TestURLs(t *testing.T) {
	if got, want := FingerprintURL("relay.example", 8892, "bob"), "https://relay.example:8892/bob/fingerprint"; got != want {
		t.Errorf("FingerprintURL = %q, want %q", got, want)
	}
	// The WebTransport endpoint is the same host and port over UDP: MediaMTX
	// defaults its HTTP/2 and HTTP/3 listeners to one number.
	if got, want := WatchURL("relay.example", 8892, "bob"), "https://relay.example:8892/bob/moq"; got != want {
		t.Errorf("WatchURL = %q, want %q", got, want)
	}
}
