package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/groupsvc"
	"bjoernblessin.de/screenshare/internal/membership"
)

// held is the membership a scrape reads, stated rather than served.
type held struct {
	read    []membership.Reading
	tallies membership.Tallies
}

func (h held) Read() []membership.Reading  { return h.read }
func (h held) Tallies() membership.Tallies { return h.tallies }

// handed is the token route a scrape reads.
type handed groupsvc.Tallies

func (h handed) Tallies() groupsvc.Tallies { return groupsvc.Tallies(h) }

func scrape(t *testing.T, groups Groups, tokens Tokens) string {
	t.Helper()

	r := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	NewExporter(groups, tokens).Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("a scrape answered %d", w.Code)
	}
	return w.Body.String()
}

func carries(t *testing.T, page string, lines ...string) {
	t.Helper()

	for _, want := range lines {
		if !strings.Contains(page, want) {
			t.Errorf("a scrape carries %q, and this one reads:\n%s", want, page)
		}
	}
}

func TestAScrapeCountsGroupsAndTheirMembers(t *testing.T) {
	page := scrape(t, held{read: []membership.Reading{
		{Prefix: "aaa/", Members: []membership.Member{
			{MemberID: "one", DisplayName: "Björn", Publishing: true},
			{MemberID: "two", DisplayName: "Alice"},
		}},
		{Prefix: "bbb/", Members: []membership.Member{
			{MemberID: "three", DisplayName: "Carol"},
		}},
	}}, handed{})

	carries(t, page,
		"groupd_groups 2",
		`groupd_members{group="aaa/"} 2`,
		`groupd_members{group="bbb/"} 1`,
		`groupd_members_publishing{group="aaa/"} 1`,
		`groupd_members_publishing{group="bbb/"} 0`,
	)
}

// The question the dashboard's table asks is who, so a member is a series of its own under the name
// it claimed.
func TestAScrapeNamesEveryLiveMember(t *testing.T) {
	page := scrape(t, held{read: []membership.Reading{
		{Prefix: "aaa/", Members: []membership.Member{
			{MemberID: "one", DisplayName: "Björn", Publishing: true},
			{MemberID: "two", DisplayName: "Alice"},
		}},
	}}, handed{})

	carries(t, page,
		`groupd_member_live{group="aaa/",member="Björn",publishing="yes"} 1`,
		`groupd_member_live{group="aaa/",member="Alice",publishing="no"} 1`,
	)
}

// The member id is what the relay logs a connection under and is a keyed digest of the secret behind
// it. A scrape is read by an operator and says who is here, which the display name already answers.
func TestAScrapeCarriesNoMemberID(t *testing.T) {
	page := scrape(t, held{read: []membership.Reading{
		{Prefix: "aaa/", Members: []membership.Member{{MemberID: "a-derived-id", DisplayName: "Björn"}}},
	}}, handed{})

	if strings.Contains(page, "a-derived-id") {
		t.Errorf("a scrape carried a member id:\n%s", page)
	}
}

func TestAScrapeCarriesEveryTally(t *testing.T) {
	page := scrape(t, held{tallies: membership.Tallies{
		Stated:   7,
		Released: 3,
		Lapsed:   2,
		Kicked:   map[string]int64{"srt": 4, "hls": 1},
		Refused:  map[string]int64{"srt": 1},
		Unread:   map[string]int64{"srtconns": 5},
	}}, handed{TokensIssued: 9, TokensRefused: 2})

	carries(t, page,
		"groupd_leases_stated_total 7",
		"groupd_leases_released_total 3",
		"groupd_leases_lapsed_total 2",
		`groupd_kicks_total{transport="hls"} 1`,
		`groupd_kicks_total{transport="srt"} 4`,
		`groupd_kicks_failed_total{transport="srt"} 1`,
		`groupd_relay_lists_unread_total{segment="srtconns"} 5`,
		`groupd_tokens_issued_total{result="issued"} 9`,
		`groupd_tokens_issued_total{result="refused"} 2`,
	)
}

// A map is read in no order, and a scrape whose lines move between reads is one a consumer cannot
// diff.
func TestAScrapeReadsTheSameTwice(t *testing.T) {
	groups := held{tallies: membership.Tallies{
		Kicked: map[string]int64{"srt": 1, "hls": 1, "rtsp": 1, "webrtc": 1, "rtmp": 1, "moq": 1},
	}}

	first := scrape(t, groups, handed{})
	for range 8 {
		if again := scrape(t, groups, handed{}); again != first {
			t.Fatalf("two scrapes of one reading differ:\n%s\n---\n%s", first, again)
		}
	}
}

// A group nobody holds a lease in is absent rather than zero, so an empty service still declares
// every family it exports.
func TestAnEmptyServiceStillDeclaresItsFamilies(t *testing.T) {
	page := scrape(t, held{}, handed{})

	carries(t, page,
		"groupd_groups 0",
		"# TYPE groupd_members gauge",
		"# TYPE groupd_member_live gauge",
		"# TYPE groupd_kicks_total counter",
	)
	if strings.Contains(page, "groupd_members{") {
		t.Errorf("no group holds a lease, and the scrape names one:\n%s", page)
	}
}

func TestAScrapeAnswersAsPrometheusText(t *testing.T) {
	r := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	NewExporter(held{}, handed{}).Handler().ServeHTTP(w, r)

	if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Errorf("a scrape answers as text, and this one answers %q", got)
	}
}
