// Command groupd serves keys, tokens and the stream index.
//
// A binary of its own because it is a separate machine's: the backend runs on a publisher's
// desktop, this runs beside the relay, where the signing key lives and where the relay fetches it.
// Shared with the backend is the path derivation, which is why both live in this repository
// (internal/group).
//
// A signing key and somewhere to read the relay's stream list are all it takes.
// Nothing else is stored: possession of a group key is membership, and the prefix is that key's own
// digest.
package main

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
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

// main serves keys, tokens and the index on the address given, or prints one operator credential and
// exits where -api-token asks for it.
//
// An unreadable key and an unservable address are Umgebungsfehler, and both end this process
// through logger.Errorf.
// A hard stop is the right answer to either: a service that reached neither has nothing left to
// serve, and the alternative is a process that is up and refusing every request.
func main() {
	listen := flag.String("listen", "127.0.0.1:9443", "address to serve on")
	keyPath := flag.String("key", "", "PEM file holding the signing key, drawn on first run where absent")
	relayHost := flag.String("relay-host", "127.0.0.1", "host the relay's API answers on")
	relayAPIPort := flag.Int("relay-api-port", 9997, "port the relay's API answers on")
	operatorWindow := flag.Duration("api-token", 0, "print a token granting the relay's API for this long and exit, for an operator reading that API directly")
	flag.Parse()

	// Checked before the key is read, because reading it is what would draw one.
	if *operatorWindow > 0 {
		if err := operatorKeyPresent(*keyPath); err != nil {
			logger.Errorf("%v", err)
		}
	}

	signer, err := signerFrom(*keyPath)
	if err != nil {
		logger.Errorf("%v", err)
	}
	assert.IsNotNil(signer, "a serving instance holds the key it signs with")

	if *operatorWindow > 0 {
		printOperatorToken(signer, *operatorWindow)
		return
	}

	reader := &relayStreams{
		host:    *relayHost,
		apiPort: *relayAPIPort,
		// The relay checks its API the way it checks a stream, so this reads through with a token of its
		// own: signed here, granting the API action alone, handed to nobody.
		// Minted per call, because a stored one expires while this process runs.
		client: relay.NewAuthorized(func() string { return apiToken(signer) }),
	}
	service := groupsvc.New(signer, reader)
	logger.Infof("serving groups on %s, signing with key %s", *listen, signer.KeyID())

	// Loopback by default and no TLS of its own: the reverse proxy in front encrypts every leg to
	// this, to the relay and to the player page, and a second terminator behind it would be a second
	// certificate to renew (docs/plan.md).
	//
	// Every phase is bounded, not the headers alone.
	// The routes are small JSON over a local hop, so a request that has not finished in this long is
	// a caller that stopped rather than a slow one, and an unbounded phase holds a connection and a
	// goroutine for as long as it cares to.
	// The write bound is the loosest of the three because GET /streams reads the relay's API first,
	// and that read carries a timeout of its own.
	server := &http.Server{
		Addr:              *listen,
		Handler:           service.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		logger.Errorf("serving: %v", err)
	}
}

// apiWindow bounds the credential the index reads through.
// Short: signed for one request, reaching nothing but the relay beside this process.
// Not shorter: the relay checks the window against its own clock, and two clocks agree to within
// seconds rather than exactly.
const apiWindow = time.Minute

// apiToken signs the credential one read of the relay's API carries.
//
// A failed signature yields an empty credential rather than stopping the process: the relay refuses
// the request, the index answers an empty listing, and keys and tokens go on being handed out,
// the half that does not depend on the relay being readable.
func apiToken(signer *token.Signer) string {
	signed, err := signer.Sign("index", token.APIPermissions(), time.Now(), apiWindow)
	if err != nil {
		logger.Warnf("cannot sign the credential the stream index reads through: %v", err)
		return ""
	}
	return signed
}

// printOperatorToken writes a credential for the relay's own API to stdout.
//
// The relay grants its API, its metrics and its playback endpoints to nothing a group token carries
// (deploy/mediamtx-groups.yml), so reading them takes a token signed with this key and asked for
// here.
// Printed rather than served, and there is no route that answers it: what makes a caller an operator
// is standing at the shell of the machine the key is on, and a route would make it holding a group
// key like everybody else.
//
// The window is the caller's, where the index's is fixed at apiWindow.
// A person reading the API works in sessions rather than in single requests, and a credential that
// outlives the session is one signed again.
func printOperatorToken(signer *token.Signer, window time.Duration) {
	assert.IsNotNil(signer, "a printed token is signed with the key this process read")
	assert.Assert(window > 0, "a printed token carries a window with something in it", window)

	signed, err := signer.Sign("operator", token.APIPermissions(), time.Now(), window)
	if err != nil {
		logger.Errorf("cannot sign a token for the relay's API: %v", err)
	}
	fmt.Println(signed)
}

// operatorKeyPresent refuses to print a token where the signing key is not already on disk.
//
// The relay authenticates against the public half of whichever key this service publishes, so a
// token signed with one drawn for this run is refused there with a 401 naming nothing, and the run
// leaves a second private key beside whichever one is real.
// The serving path draws on purpose, a first start having nothing to read; this one cannot, because
// what it prints is worth something only against a key the relay already trusts.
func operatorKeyPresent(path string) error {
	if path == "" {
		return errors.New("an operator token is signed with the key the relay publishes, so -api-token needs -key naming the file holding it")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("no signing key at %s, and an operator token is signed with the key the relay already publishes rather than one drawn now", path)
	}
	return nil
}

// signerFrom reads the signing key, drawing one and storing it where the file is absent.
//
// Stored because a restart that drew a new key would invalidate every token in flight and every
// relay's cached JWKS at once.
// An empty path draws one and keeps it in memory, which a test deployment wants and a real one must
// not have.
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

// store writes a freshly drawn key readable by nobody else.
// Every token is signed with it, so a key another user can read is every group's streams.
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
// a stream starts or cleared when one ends.
type relayStreams struct {
	host    string
	apiPort int
	client  *relay.Client
}

// Paths narrows the relay's answer to what a member is told: the readers and byte counters it
// carries beside each path stop here (internal/groupsvc, Stream).
func (r *relayStreams) Paths() []groupsvc.Stream {
	assert.IsNotNil(r.client, "a relay reader holds a client to read through")

	// A path that carries nothing yet crosses too, with Ready false.
	// Dropping it here would leave the field with one value it could ever hold, and a viewer waiting
	// on a publisher that has connected but not established a track would see the row vanish from the
	// index while a relay read directly shows it starting.
	// The relay configures one wildcard path, so a path it lists at all is one something connected to
	// rather than one an operator wrote down (deploy/mediamtx-groups.yml).
	status := r.client.Fetch(r.host, r.apiPort)
	out := make([]groupsvc.Stream, 0, len(status.Paths))
	for _, p := range status.Paths {
		out = append(out, groupsvc.Stream{Path: p.Name, Ready: p.Ready, Tracks: p.Tracks, Format: p.Format})
	}
	return out
}
