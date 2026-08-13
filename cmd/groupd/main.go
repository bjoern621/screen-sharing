// Command groupd runs the key, token and index service.
//
// It is a separate binary from the backend because it is a separate machine's:
// the backend runs on a publisher's desktop and this runs beside the relay,
// where the signing key lives and where the relay can fetch it.
// What they share is the path derivation, which is why both are in this repository
// (internal/group).
//
// Everything it needs is a signing key and somewhere to read the relay's stream list from.
// The rest it derives, which is why there is no database flag: possession of a group key is
// membership, and the prefix is that key's own digest.
package main

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/groupsvc"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/token"
)

// A key that cannot be read and an address that cannot be served are Umgebungsfehler,
// and both end this process through logger.Errorf.
// That is the one place a hard stop is the right answer to one: a service that reached neither has
// nothing left to serve, and the alternative is a process that is up and refusing every request.
func main() {
	listen := flag.String("listen", "127.0.0.1:9443", "address to serve on")
	keyPath := flag.String("key", "", "PEM file holding the signing key, drawn on first run where absent")
	relayHost := flag.String("relay-host", "127.0.0.1", "host the relay's API answers on")
	relayAPIPort := flag.Int("relay-api-port", 9997, "port the relay's API answers on")
	flag.Parse()

	signer, err := signerFrom(*keyPath)
	if err != nil {
		logger.Errorf("%v", err)
	}
	assert.IsNotNil(signer, "a serving instance holds the key it signs with")

	reader := &relayStreams{
		host:    *relayHost,
		apiPort: *relayAPIPort,
		// The relay checks its API the way it checks a stream, so this reads through with a token of its
		// own: signed here, granting the API action alone, handed to nobody.
		// Per call rather than held: a stored one expires while this process runs.
		client: relay.NewAuthorized(func() string { return apiToken(signer) }),
	}
	service := groupsvc.New(signer, reader)
	logger.Infof("serving groups on %s, signing with key %s", *listen, signer.KeyID())

	// Loopback by default and no TLS of its own: every leg is encrypted by the reverse proxy that
	// fronts this, the relay and the player page alike, and a second TLS terminator behind it would be
	// a second certificate to renew (docs/plan.md).
	server := &http.Server{Addr: *listen, Handler: service.Handler(), ReadHeaderTimeout: 5 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		logger.Errorf("serving: %v", err)
	}
}

// Validity of the credential the index reads through.
// Short: signed for one request, reaching nothing but the relay beside this process.
// Not shorter: the relay checks the window against its own clock, and two clocks agree to within
// seconds rather than exactly.
const apiWindow = time.Minute

// Signs the credential one read of the relay's API carries.
//
// A failed signature leaves an empty credential rather than stopping the process: the relay refuses
// the request, the index answers an empty listing, and keys and tokens go on being handed out -
// the half that does not depend on the relay being readable.
func apiToken(signer *token.Signer) string {
	signed, err := signer.Sign("index", token.APIPermissions(), time.Now(), apiWindow)
	if err != nil {
		logger.Warnf("cannot sign the credential the stream index reads through: %v", err)
		return ""
	}
	return signed
}

// signerFrom reads the signing key, drawing one and storing it where the file is not there yet.
//
// It is stored because a restart that drew a new key would invalidate every token in flight and
// every relay's cached JWKS at once.
// An empty path draws one and keeps it in memory, which is what a test deployment wants and what a
// real one must not have.
func signerFrom(path string) (*token.Signer, error) {
	if path == "" {
		logger.Warnf("no signing key file given, so this run draws one and forgets it on exit")
		return token.NewSigner()
	}

	stored, err := os.ReadFile(path)
	if err == nil {
		block, _ := pem.Decode(stored)
		if block == nil {
			return nil, fmt.Errorf("the signing key file %s carries no PEM block", path)
		}
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("reading the signing key %s: %v", path, err)
		}
		signing, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("the signing key %s is a %T, and tokens are signed with %s", path, key, token.Algorithm)
		}
		return token.SignerFor(signing), nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading the signing key %s: %v", path, err)
	}

	signer, err := token.NewSigner()
	if err != nil {
		return nil, err
	}
	if err := store(path, signer); err != nil {
		return nil, err
	}
	logger.Infof("drew a signing key and stored it in %s", path)
	return signer, nil
}

// store writes a freshly drawn key, readable by nobody else: it is what every token is signed with,
// so a key another user can read is every group's streams.
func store(path string, signer *token.Signer) error {
	assert.Assert(path != "", "a stored key is written to a named file")
	assert.IsNotNil(signer, "a stored key is a key")

	encoded, err := x509.MarshalPKCS8PrivateKey(signer.PrivateKey())
	if err != nil {
		return fmt.Errorf("encoding the signing key: %v", err)
	}
	block := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded})
	if err := os.WriteFile(path, block, 0o600); err != nil {
		return fmt.Errorf("writing the signing key %s: %v", path, err)
	}
	return nil
}

// relayStreams reads the relay's own path list, which is where the index's answer comes from.
// Which streams exist is the relay's fact and never this service's, so nothing here is written when
// a stream starts or stopped when one ends.
type relayStreams struct {
	host    string
	apiPort int
	client  *relay.Client
}

// Narrowed to what a member is told: the relay answers readers and byte counters beside each path,
// and the index is where that stops (internal/groupsvc, Stream).
func (r *relayStreams) Paths() []groupsvc.Stream {
	assert.IsNotNil(r.client, "a relay reader holds a client to read through")

	status := r.client.Fetch(r.host, r.apiPort)
	out := make([]groupsvc.Stream, 0, len(status.Paths))
	for _, p := range status.Paths {
		if !p.Ready {
			continue
		}
		out = append(out, groupsvc.Stream{Path: p.Name, Ready: p.Ready, Tracks: p.Tracks, Format: p.Format})
	}
	return out
}
