// Package reach answers whether the relay this app is pointed at is answering, leg by leg.
//
// One row per listener, each dialled where the transport using it says it answers
// (transport.Listener), so what is measured is this deployment rather than a second list of ports.
// The dial goes to a route that listener's own server owns where the transport names one
// (transport.Probed), one name fronting several of them.
// A leg this deployment addresses nowhere reads as Unaddressed, and one whose answer nothing here
// needs reads as Unused, never as a cross: a relay binding what it is configured to bind is a relay
// behaving.
//
// Every answer here is another machine's, so nothing asserts on one.
// A refused connection, a missing listener, a name that does not resolve and an answer in the wrong
// protocol are Umgebungsfehler, each carried in the row it is about.
package reach

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// The two legs no transport carries: the service every publisher asks for a token before reaching
// any listener, and the manager a relay in Discord mode asks instead (docs/discord-mode.md).
const (
	legGroups  = "groups"
	legDiscord = "discord"
)

// Verdict is what one leg's probe found.
type Verdict int

const (
	// Reachable: the listener answered, in the protocol its address names.
	Reachable Verdict = iota + 1
	// Unreachable: nothing answered, or what answered speaks something else.
	Unreachable
	// Unaddressed: this deployment sends nothing here, so nothing was dialled.
	Unaddressed
	// Unused: the listener answered, and nothing this machine does uses it.
	Unused
)

// Verdicts is every verdict a row can carry, and Reasons every reason nothing here uses a leg.
//
// Walked by whatever answers for all of them: the marks a report prints, the sentences it writes,
// and the contract's own enum (internal/wire).
// A value added above and left out of one is a failing test rather than a row nobody can draw.
var (
	Verdicts = []Verdict{Reachable, Unreachable, Unaddressed, Unused}
	Reasons  = []Reason{ReasonNoRelay, ReasonDiscordOff}
)

// Reason is why nothing here uses a leg, whether it was dialled or not.
type Reason int

const (
	ReasonNone Reason = iota
	// ReasonNoRelay: settings name no relay, so no leg has an address.
	ReasonNoRelay
	// ReasonDiscordOff: Discord mode is off, so nothing this machine does reaches the manager,
	// whatever it answers.
	ReasonDiscordOff
)

// Endpoint is one leg and where this deployment addresses it.
type Endpoint struct {
	// Leg is the transport's registry name, or legGroups.
	Leg string
	// Address is where the leg answers, as a reader would type it: "rtsps://relay:8322".
	// Empty where no address could be built.
	Address string
	// Unused is why nothing here uses the leg, ReasonNone where something does.
	// An address beside one is a leg dialled anyway, the relay being what a check asks about.
	Unused Reason
}

// Result is one leg's answer.
type Result struct {
	Leg     string
	Address string
	Verdict Verdict
	// Detail is what the listener answered, or the dial's own failure, unedited so it reaches a bug
	// report as it stands.
	// Empty where nothing was dialled.
	Detail string
	// Unused is why nothing here uses the leg, under Unaddressed and Unused, ReasonNone otherwise.
	Unused Reason
	// Version is what the listener named for itself, empty where it named none.
	// A relay's own services carry one and MediaMTX carries none, so a row is answered either way.
	Version string
	// Took is how long the probe waited, zero where nothing was dialled.
	Took time.Duration
}

// resolved is one row before it is probed.
type resolved struct {
	leg string
	// address is the listener the row is about, which is the field a wrong answer is corrected in.
	address string
	// target is where the probe dials, a route inside that listener where the transport names one
	// (transport.Probed).
	// Empty exactly where a leg goes undialled.
	target target
	// reason is why nothing here uses the leg, ReasonNone where something does.
	reason Reason
}

// target is where one leg is dialled.
type target struct {
	// url is the whole address, each probe reading what it needs off it.
	url string
	// method is the request an HTTP probe makes, ignored by the legs carrying no HTTP.
	method string
	// credential is the header an HTTP probe sends, empty where the settings hold no token.
	// Without one the relay answers 401 over a listener that is up, which is an answer about
	// the credential rather than about the route (transport.CredentialHeader).
	credential header
	// wantOK holds a route to a 2xx, for the two services that own their routes and answer them
	// without a credential: anything else there is the service missing rather than a reader refused.
	wantOK bool
	// insecure follows the relay's certificate: a relay reached directly on this network holds
	// the self-signed pair deploy/relay.sh draws, which nothing issued, so validating it opens
	// nothing (transport/tls.go).
	insecure bool
}

// header is one request header, both halves empty where there is none.
type header struct {
	name  string
	value string
}

// Endpoints is where each leg is addressed, without dialling any of them.
func Endpoints(s settings.Settings) []Endpoint {
	rows := resolve(s)

	out := make([]Endpoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, Endpoint{Leg: row.leg, Address: row.address, Unused: row.reason})
	}
	for _, e := range out {
		assert.Assert(e.Address != "" || e.Unused != ReasonNone,
			"a leg with no address says why nothing here uses it", e.Leg, e.Unused)
	}
	return out
}

// Check dials every leg this deployment addresses and answers one row each.
//
// All at once, so a check costs the slowest leg rather than their sum, and a missing listener costs
// probeTimeout.
// Nothing is left out on a failure: a report covers a whole relay, so a leg that could not be
// dialled is a row saying so.
func Check(ctx context.Context, s settings.Settings) []Result {
	assert.IsNotNil(ctx, "a check runs under a context, its whole bound being a deadline")

	rows := resolve(s)
	results := make([]Result, len(rows))

	var wg sync.WaitGroup
	for i, row := range rows {
		if row.target.url == "" {
			assert.Assert(row.reason != ReasonNone, "an undialled leg says why", row.leg)

			results[i] = Result{Leg: row.leg, Verdict: Unaddressed, Unused: row.reason}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = run(ctx, row)
		}()
	}
	wg.Wait()

	for i, r := range results {
		assert.Assert(r.Leg == rows[i].leg, "every leg answers in its own row", rows[i].leg, r.Leg)
		assert.Assert(r.Verdict != 0, "every leg is answered", r.Leg)
	}
	return results
}

// resolve is one row per leg, in the order a report walks them.
//
// Sorted by name, keeping the order a property of the legs rather than a second list beside
// the registry.
func resolve(s settings.Settings) []resolved {
	listeners := transport.Listeners(s)
	probes := transport.Probes(s)
	credential := relayCredential(s)

	rows := make([]resolved, 0, len(listeners)+1)
	for name, address := range listeners {
		probe, ok := probes[name]
		assert.Assert(ok, "every listener names where a check dials it", name)

		rows = append(rows, resolved{
			leg:     name,
			address: address,
			target: target{
				url:        probe.URL,
				method:     probe.Method,
				credential: credential,
				insecure:   s.Relay.OnTrustedNetwork(),
			},
		})
	}
	rows = append(rows, groupService(s.Relay), discordService(s.Relay))

	// A relay nobody named leaves every leg without an address, whatever the ports beside it would
	// otherwise build.
	if s.Relay.Host == "" {
		for i := range rows {
			rows[i] = resolved{leg: rows[i].leg, reason: ReasonNoRelay}
		}
	}

	slices.SortFunc(rows, func(a, b resolved) int { return strings.Compare(a.leg, b.leg) })
	return rows
}

// groupService is where keys, tokens, membership and the stream index are answered
// (settings.Relay.GroupService).
//
// Dialled on the key route rather than the root: it takes no credential and is the service's own,
// so an answer there is groupd and not the terminator in front of it (internal/groupsvc).
func groupService(r settings.Relay) resolved {
	base, ok := r.GroupService()
	if !ok {
		return resolved{leg: legGroups, reason: ReasonNoRelay}
	}
	return resolved{
		leg:     legGroups,
		address: base,
		// No credential: the key set is what a token is verified against, so it is public
		// by construction and is answered before anybody holds one.
		target: target{url: base + "/jwks.json", method: http.MethodGet, wantOK: true, insecure: r.OnTrustedNetwork()},
	}
}

// discordService is the manager a relay in Discord mode asks for presence, tokens and the paths
// a voice channel's group gets (settings.Relay.DiscordService).
//
// Dialled on the route it answers a check with, which takes no credential and starts no consent
// flow (internal/discordapi).
// Dialled with the mode off as well: what a check asks is whether the relay is whole, and the mode
// is this machine's setting rather than the relay's.
// The mode off leaves the answer standing and the leg unused,
// the stored group key being what a token is traded for then.
func discordService(r settings.Relay) resolved {
	base, ok := r.DiscordService()
	if !ok {
		return resolved{leg: legDiscord, reason: ReasonNoRelay}
	}

	row := resolved{
		leg:     legDiscord,
		address: base,
		target:  target{url: base + "/health", method: http.MethodGet, wantOK: true, insecure: r.OnTrustedNetwork()},
	}
	if !r.DiscordMode {
		row.reason = ReasonDiscordOff
	}
	return row
}

// relayCredential is the header the HTTP legs are dialled with, empty where the settings hold
// no token.
//
// The token is the caller's, traded for the group key before a check runs (internal/app,
// checkRelay): a relay refuses a reader holding none, which is a row about the credential rather
// than about the listener.
func relayCredential(s settings.Settings) header {
	name, value, ok := transport.CredentialHeader(s)
	if !ok {
		return header{}
	}
	return header{name: name, value: value}
}

// run is one row: the probe the route's protocol names, timed, carrying whatever came back.
func run(ctx context.Context, row resolved) Result {
	assert.IsNotNil(ctx, "a probe runs under a context, its whole bound being a deadline")
	assert.Assert(row.target.url != "", "a dialled leg names where", row.leg)

	probe, ok := probes[schemeOf(row.target.url)]
	assert.Assert(ok, "a leg is addressed in a protocol this speaks", row.leg, row.target.url)

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	began := time.Now()
	a, err := probe(ctx, row.target)
	took := time.Since(began)
	// A leg that did not answer carries no reason: what the row is about is the silence, whether
	// anything here would have used the answer or not.
	if err != nil {
		return Result{Leg: row.leg, Address: row.address, Verdict: Unreachable, Detail: err.Error(), Took: took}
	}

	assert.Assert(a.detail != "", "a listener that answered says what it answered", row.leg)
	verdict := Reachable
	if row.reason != ReasonNone {
		verdict = Unused
	}
	return Result{
		Leg:     row.leg,
		Address: row.address,
		Verdict: verdict,
		Detail:  a.detail,
		Unused:  row.reason,
		Version: a.version,
		Took:    took,
	}
}

// schemeOf is the protocol an address names.
// An address that will not parse is one this side built, so it is asserted rather than carried.
func schemeOf(address string) string {
	u, err := url.Parse(address)
	assert.Assert(err == nil, "a leg's address parses", address)
	return u.Scheme
}
