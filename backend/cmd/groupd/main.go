// Command groupd serves group keys, relay access tokens, the stream index,
// and the membership those are enforced against.
//
// A binary of its own because it is a separate machine's:
// the backend runs on a publisher's desktop, this runs beside the relay,
// where the signing key lives and where the relay fetches it.
// Shared with the backend is the path derivation, so both live in this repository (internal/group).
//
// A signing key and a reachable relay API are all it takes.
// The signing key is the one thing on disk.
// Presence leases are held in memory alone: a restart forgets them,
// and every live app states its own again within one refresh interval (internal/membership).
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
	"bjoernblessin.de/screenshare/internal/membership"
	"bjoernblessin.de/screenshare/internal/metrics"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/token"
)

// main serves keys, tokens and the index on the address given,
// or prints one operator credential and exits where -api-token asks for it.
//
// An unreadable key and an unservable address are Umgebungsfehler,
// and both end this process through logger.Errorf:
// a service that reached neither has nothing left to serve,
// and the alternative is a process that is up and refusing every request.
func main() {
	listen := flag.String("listen", "127.0.0.1:9443", "address to serve on")
	scrapeListen := flag.String("metrics", "", "address to serve the Prometheus scrape on, off where empty")
	keyPath := flag.String("key", "", "PEM file holding the signing key, drawn on first run where absent")
	relayHost := flag.String("relay-host", "127.0.0.1", "host the relay's API answers on")
	relayAPIPort := flag.Int("relay-api-port", 9997, "port the relay's API answers on")
	operatorWindow := flag.Duration("api-token", 0, "print a token granting the relay's API for this long and exit, for an operator reading that API directly")
	flag.Parse()

	// Before the key is read, because reading it would draw one.
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

	// The relay checks its API the way it checks a stream,
	// so this reads through with a token of its own:
	// signed here, granting the API action alone, handed to nobody.
	// Minted per call, a stored one expiring while this process runs.
	client := relay.NewAuthorized(func() string { return apiToken(signer) })
	reader := &relayStreams{host: *relayHost, apiPort: *relayAPIPort, client: client}

	// One client for all three: the index reading what is live,
	// enforcement closing what a member whose lease lapsed still holds,
	// and the SRT keys riding the same credential.
	// None reaches the relay as anything a group token could.
	enforcer := relayConnections{host: *relayHost, apiPort: *relayAPIPort, client: client}
	members := membership.New(enforcer)
	go reap(members)

	srtKeys := relayKeys{host: *relayHost, apiPort: *relayAPIPort, client: client}
	service := groupsvc.New(signer, reader, members, srtKeys)
	logger.Infof("serving groups on %s, signing with key %s", *listen, signer.KeyID())

	// A listener of its own, off unless a deployment asks for one.
	// It carries who is in which group,
	// so where it binds and who may reach it is the deployment's decision,
	// as the relay's own metrics listener is (deploy/mediamtx-groups.yml).
	if *scrapeListen != "" {
		go serveScrape(*scrapeListen, metrics.NewExporter(members, service))
	}

	// Loopback by default and no TLS of its own:
	// the reverse proxy in front encrypts every leg to this, to the relay and to the player page,
	// and a second terminator behind it would be a second certificate to renew (docs/plan.md).
	//
	// Every phase is bounded, not the headers alone.
	// The routes are small JSON over a local hop,
	// so a request outlasting these bounds is a caller that stopped rather than a slow one,
	// and an unbounded phase holds a connection and a goroutine indefinitely.
	// The write bound is the loosest because GET /streams reads the relay's API first,
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

// serveScrape answers scrapes for as long as this process runs.
//
// A listener that will not come up is an Umgebungsfehler and leaves the service running:
// a held port costs the deployment its readings,
// where stopping over one costs every member their group.
func serveScrape(listen string, exporter *metrics.Exporter) {
	assert.Assert(listen != "", "a scrape is served somewhere")
	assert.IsNotNil(exporter, "a scrape is answered by an exporter")

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", exporter.Handler())

	logger.Infof("serving the scrape on %s", listen)
	server := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		logger.Warnf("the scrape listener on %s stopped: %v", listen, err)
	}
}

// reapEvery is how often lapsed leases are swept.
//
// Well inside membership.Lease, so a member who stopped refreshing loses what they hold soon after
// the lease runs out, even where nobody else's call notices first.
const reapEvery = 5 * time.Second

// reap closes what lapsed leases still hold, for as long as this process runs.
//
// The only timer in the service.
// A relay that would not answer a list is an Umgebungsfehler:
// the run is left partial, reported, and the next tick tries again.
func reap(members *membership.Registry) {
	assert.IsNotNil(members, "a sweep runs against the registry holding the leases")

	ticker := time.NewTicker(reapEvery)
	defer ticker.Stop()

	for tick := range ticker.C {
		for _, swept := range members.Reap(tick) {
			if len(swept.Kicked) > 0 {
				logger.Infof("closed %d connections under %s after a lease lapsed", len(swept.Kicked), swept.Prefix)
			}
			for _, refused := range swept.Failed {
				logger.Warnf("the relay would not close %s under %s: %s", refused.Stream, swept.Prefix, refused.Reason)
			}
			for _, unread := range swept.Unread {
				logger.Warnf("the relay's %s list would not answer a sweep of %s: %s", unread.Segment, swept.Prefix, unread.Reason)
			}
		}
	}
}

// apiWindow bounds the credential the index reads through.
// Short: signed for one request, reaching nothing but the relay beside this process.
// Not shorter: the relay checks the window against its own clock,
// and two clocks agree to within seconds rather than exactly.
const apiWindow = time.Minute

// apiToken signs the credential one read of the relay's API carries.
//
// A failed signature yields an empty credential rather than stopping the process:
// the relay refuses the request, the index answers an empty listing,
// and keys and tokens go on being handed out, those not depending on the relay being readable.
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
// The relay grants its API, metrics and playback endpoints to nothing a group token carries
// (deploy/mediamtx-groups.yml), so reading them takes a token signed with this key.
// Printed rather than served by a route:
// what makes a caller an operator is standing at the shell of the machine the key is on,
// where a route would make it holding a group key like everybody else.
//
// The window is the caller's, where the index's is fixed at apiWindow.
// A person reading the API works in sessions rather than single requests,
// and a credential that outlives the session is one signed again.
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
// The relay authenticates against the public half of whichever key this service publishes,
// so a token signed with one drawn for this run is refused there with a 401 naming nothing,
// and the run leaves a second private key beside whichever one is real.
// The serving path draws on purpose, a first start having nothing to read;
// this one cannot, what it prints being worth something only against a key the relay already trusts.
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
// Stored because a restart that drew another key would invalidate every token in flight
// and every relay's cached JWKS at once.
// An empty path draws one and keeps it in memory,
// which a test deployment wants and a real one must not.
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

// relayStreams reads the relay's own path list, which the index's answer comes from.
// Which streams exist is the relay's fact,
// so nothing here is written when a stream starts or cleared when one ends.
type relayStreams struct {
	host    string
	apiPort int
	client  *relay.Client
}

// Paths narrows the relay's answer to what a member is told:
// the reader roster beside each path stops here, and its length crosses
// (internal/groupsvc, Stream).
func (r *relayStreams) Paths() []groupsvc.Stream {
	assert.IsNotNil(r.client, "a relay reader holds a client to read through")

	// A path carrying nothing crosses too, with Ready false.
	// Dropping it would leave Ready with one value it could ever hold,
	// and a viewer waiting on a publisher that connected without establishing a track
	// would see the row vanish from the index while a direct relay read shows it starting.
	// The relay configures one wildcard path,
	// so a path it lists is one something connected to rather than one an operator wrote down
	// (deploy/mediamtx-groups.yml).
	status := r.client.Fetch(r.host, r.apiPort)
	out := make([]groupsvc.Stream, 0, len(status.Paths))
	for _, p := range status.Paths {
		out = append(out, groupsvc.Stream{
			Path: p.Name, Ready: p.Ready, Tracks: p.Tracks, Format: p.Format,
			InMbps: p.InMbps, Readers: p.Readers,
		})
	}
	return out
}

// relayConnections is the relay this service enforces membership against.
//
// The client is addressed by host and port per call, where the registry holds a relay rather than
// an address, so the address this process was pointed at is bound in here.
type relayConnections struct {
	host    string
	apiPort int
	client  *relay.Client
}

func (c relayConnections) Sessions() ([]relay.Session, []relay.Unread) {
	return c.client.Sessions(c.host, c.apiPort)
}

func (c relayConnections) Kick(segment, id string) error {
	return c.client.Kick(c.host, c.apiPort, segment, id)
}

// relayKeys is the relay this service writes SRT path keys into,
// bound to the one address the process was pointed at the way relayConnections is.
type relayKeys struct {
	host    string
	apiPort int
	client  *relay.Client
}

func (k relayKeys) Ensure(prefix, passphrase string) error {
	return k.client.EnsureSRTKeys(k.host, k.apiPort, prefix, passphrase)
}
