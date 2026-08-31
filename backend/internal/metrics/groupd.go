package metrics

import (
	"maps"
	"net/http"
	"slices"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/groupsvc"
	"bjoernblessin.de/screenshare/internal/membership"
)

// Groups is the membership one scrape reads.
// An interface rather than the registry, so a scrape is tested without a relay behind it,
// the same seam the registry itself takes its relay through.
type Groups interface {
	Read() []membership.Reading
	Tallies() membership.Tallies
}

// Tokens is the service one scrape reads.
type Tokens interface {
	Tallies() groupsvc.Tallies
}

// Exporter answers a scrape off the two owners, holding nothing of its own.
type Exporter struct {
	groups Groups
	tokens Tokens
}

func NewExporter(groups Groups, tokens Tokens) *Exporter {
	assert.IsNotNil(groups, "a scrape reads membership from somewhere")
	assert.IsNotNil(tokens, "a scrape reads the token route from somewhere")

	return &Exporter{groups: groups, tokens: tokens}
}

// Handler answers one scrape.
// A read of both owners and nothing else,
// so a scrape that never comes costs nothing and one that comes twice reads the same registry twice.
func (e *Exporter) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		// A failed write is the scrape going away mid-answer,
		// the collector's business and not a condition this process acts on.
		if err := Render(w, e.Families()); err != nil {
			logger.Warnf("a scrape stopped reading part way through: %v", err)
		}
	})
}

// Families is everything one scrape carries, derived when it is asked for.
// Every family is declared whether or not anything holds a sample of it,
// so a consumer can tell a metric nothing holds from one this build does not export.
func (e *Exporter) Families() []Family {
	read := e.groups.Read()
	held := e.groups.Tallies()
	handed := e.tokens.Tallies()

	groups := Family{
		Name:    "groupd_groups",
		Help:    "Groups holding at least one live lease.",
		Type:    Gauge,
		Samples: []Sample{{Value: float64(len(read))}},
	}
	members := Family{
		Name: "groupd_members",
		Help: "Members holding a live lease, by group.",
		Type: Gauge,
	}
	publishing := Family{
		Name: "groupd_members_publishing",
		Help: "Members holding a publish connection on the relay, by group.",
		Type: Gauge,
	}
	// One series per member, which is what answers who is here.
	// The name is the one the member claimed, and the relay's id for them is not carried:
	// a scrape says who, and an id a keyed digest derives says nothing a reader of this can use.
	live := Family{
		Name: "groupd_member_live",
		Help: "One per member holding a live lease, under the name they claimed.",
		Type: Gauge,
	}

	for _, group := range read {
		pushing := 0
		for _, member := range group.Members {
			if member.Publishing {
				pushing++
			}
			live.Samples = append(live.Samples, Sample{
				Labels: []Label{
					{"group", group.Prefix},
					{"member", member.DisplayName},
					{"publishing", yesNo(member.Publishing)},
				},
				Value: 1,
			})
		}
		members.Samples = append(members.Samples, Sample{
			Labels: []Label{{"group", group.Prefix}},
			Value:  float64(len(group.Members)),
		})
		publishing.Samples = append(publishing.Samples, Sample{
			Labels: []Label{{"group", group.Prefix}},
			Value:  float64(pushing),
		})
	}

	return []Family{
		groups,
		members,
		publishing,
		live,
		counter("groupd_leases_stated_total", "Members arriving in a group. A refresh of a lease already held is not one.", held.Stated),
		counter("groupd_leases_released_total", "Members leaving a group by saying so.", held.Released),
		counter("groupd_leases_lapsed_total", "Leases that stopped being refreshed.", held.Lapsed),
		{
			Name: "groupd_tokens_issued_total",
			Help: "Relay access tokens handed out, by whether the request was answered.",
			Type: Counter,
			Samples: []Sample{
				{Labels: []Label{{"result", "issued"}}, Value: float64(handed.TokensIssued)},
				{Labels: []Label{{"result", "refused"}}, Value: float64(handed.TokensRefused)},
			},
		},
		keyed("groupd_kicks_total", "Connections enforcement closed, by transport.", "transport", held.Kicked),
		keyed("groupd_kicks_failed_total", "Closes the relay refused, by transport. Each is a member possibly still watching.", "transport", held.Refused),
		keyed("groupd_relay_lists_unread_total", "Relay connection lists a look could not read, by the relay's own name for the list.", "segment", held.Unread),
	}
}

// counter is a family carrying one reading and no dimension.
func counter(name, help string, count int64) Family {
	return Family{
		Name:    name,
		Help:    help,
		Type:    Counter,
		Samples: []Sample{{Value: float64(count)}},
	}
}

// keyed is a family carrying one reading per key, in one order.
// Sorted rather than ranged over: a map is read in no order,
// and a scrape whose lines move between reads is one nobody can diff.
func keyed(name, help, label string, counts map[string]int64) Family {
	family := Family{Name: name, Help: help, Type: Counter}
	for _, key := range slices.Sorted(maps.Keys(counts)) {
		family.Samples = append(family.Samples, Sample{
			Labels: []Label{{label, key}},
			Value:  float64(counts[key]),
		})
	}
	return family
}

// yesNo is how a state reads as a label, a label value being text and never a number.
func yesNo(state bool) string {
	if state {
		return "yes"
	}
	return "no"
}
