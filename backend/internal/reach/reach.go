// Package reach answers whether the relay this app is pointed at is answering, leg by leg.
//
// One row per listener, each dialled where the transport using it says it answers
// (transport.Listener), so what is measured is this deployment rather than a second list of ports.
// A leg this deployment addresses nowhere reads as Unaddressed, never as a cross: a relay binding
// what it is configured to bind is a relay behaving.
//
// Every answer here is another machine's, so nothing asserts on one.
// A refused connection, a missing listener, a name that does not resolve and an answer in the wrong
// protocol are Umgebungsfehler, each carried in the row it is about.
package reach

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/transport"
)

// Two legs no transport carries: the service every publisher asks for a token before reaching any
// listener, and the relay's own HTTP API.
const (
	legGroups = "groups"
	legAPI    = "api"
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
)

// Verdicts is every verdict a row can carry, and Reasons every reason a leg goes undialled for.
//
// Walked by whatever answers for all of them: the marks a report prints, the sentences it writes,
// and the contract's own enum (internal/wire).
// A value added above and left out of one is a failing test rather than a row nobody can draw.
var (
	Verdicts = []Verdict{Reachable, Unreachable, Unaddressed}
	Reasons  = []Reason{ReasonNoRelay, ReasonLoopbackOnly}
)

// Reason is why a leg is dialled nowhere.
type Reason int

const (
	ReasonNone Reason = iota
	// ReasonNoRelay: settings name no relay, so no leg has an address.
	ReasonNoRelay
	// ReasonLoopbackOnly: relay binds this listener to loopback, so it answers on the relay's own
	// machine alone (deploy/mediamtx-groups.yml).
	ReasonLoopbackOnly
)

// Endpoint is one leg and where this deployment addresses it.
type Endpoint struct {
	// Leg is the transport's registry name, or legGroups / legAPI.
	Leg string
	// Address is where the leg answers, as a reader would type it: "rtsps://relay:8322".
	// Empty exactly where Unaddressed names a reason.
	Address string
	// Unaddressed is why nothing is dialled, ReasonNone where something is.
	Unaddressed Reason
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
	// Unaddressed is the reason where Verdict is Unaddressed, and ReasonNone otherwise.
	Unaddressed Reason
	// Took is how long the probe waited, zero where nothing was dialled.
	Took time.Duration
}

// resolved is one row before it is probed.
type resolved struct {
	leg    string
	target target
	reason Reason
}

// target is where one leg is dialled.
type target struct {
	// url is the whole address, each probe reading what it needs off it.
	url string
	// insecure follows the relay's certificate: a relay reached directly on this network holds
	// the self-signed pair scripts/relay.sh draws, which nothing issued, so validating it opens
	// nothing (transport/tls.go).
	insecure bool
}

// Endpoints is where each leg is addressed, without dialling any of them.
func Endpoints(s settings.Settings) []Endpoint {
	rows := resolve(s)

	out := make([]Endpoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, Endpoint{Leg: row.leg, Address: row.target.url, Unaddressed: row.reason})
	}
	for _, e := range out {
		assert.Assert((e.Address == "") == (e.Unaddressed != ReasonNone),
			"a leg is either dialled somewhere or unaddressed for a reason", e.Leg, e.Address, e.Unaddressed)
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
		if row.reason != ReasonNone {
			results[i] = Result{Leg: row.leg, Verdict: Unaddressed, Unaddressed: row.reason}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = run(ctx, row.leg, row.target)
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

	rows := make([]resolved, 0, len(listeners)+2)
	for name, address := range listeners {
		rows = append(rows, resolved{leg: name, target: target{url: address, insecure: s.Relay.OnTrustedNetwork()}})
	}
	rows = append(rows, groupService(s.Relay), relayAPI(s.Relay))

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
	return resolved{leg: legGroups, target: target{url: base + "/jwks.json", insecure: r.OnTrustedNetwork()}}
}

// relayAPI is the relay's own HTTP API, bound to loopback wherever the relay runs
// (deploy/mediamtx-groups.yml), so nothing off that machine dials it and a check that did would
// cross a relay doing as it is told.
//
// Route is the one internal/relay reads paths from.
func relayAPI(r settings.Relay) resolved {
	if !r.OnThisMachine() {
		return resolved{leg: legAPI, reason: ReasonLoopbackOnly}
	}
	return resolved{leg: legAPI, target: target{url: fmt.Sprintf("http://%s:%d/v3/paths/list", r.Host, r.ApiPort)}}
}

// run is one row: the probe the address's protocol names, timed, carrying whatever came back.
func run(ctx context.Context, leg string, t target) Result {
	assert.IsNotNil(ctx, "a probe runs under a context, its whole bound being a deadline")
	assert.Assert(t.url != "", "a dialled leg names where", leg)

	probe, ok := probes[schemeOf(t.url)]
	assert.Assert(ok, "a leg is addressed in a protocol this speaks", leg, t.url)

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	began := time.Now()
	detail, err := probe(ctx, t)
	took := time.Since(began)
	if err != nil {
		return Result{Leg: leg, Address: t.url, Verdict: Unreachable, Detail: err.Error(), Took: took}
	}

	assert.Assert(detail != "", "a listener that answered says what it answered", leg)
	return Result{Leg: leg, Address: t.url, Verdict: Reachable, Detail: detail, Took: took}
}

// schemeOf is the protocol an address names.
// An address that will not parse is one this side built, so it is asserted rather than carried.
func schemeOf(address string) string {
	u, err := url.Parse(address)
	assert.Assert(err == nil, "a leg's address parses", address)
	return u.Scheme
}
