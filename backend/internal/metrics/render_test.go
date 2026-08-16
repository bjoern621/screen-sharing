package metrics

import (
	"strings"
	"testing"
)

func TestRenderWritesHelpTypeAndSamples(t *testing.T) {
	page := render(t, []Family{{
		Name: "groupd_groups",
		Help: "Groups holding at least one live lease.",
		Type: Gauge,
		Samples: []Sample{
			{Value: 2},
		},
	}})

	for _, want := range []string{
		"# HELP groupd_groups Groups holding at least one live lease.",
		"# TYPE groupd_groups gauge",
		"groupd_groups 2",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("a scrape carries %q, and this one reads:\n%s", want, page)
		}
	}
}

// A family nobody holds a sample of still declares itself, so a consumer reading the page can tell a
// metric with nothing in it from one this build does not export.
func TestRenderDeclaresAFamilyWithNoSamples(t *testing.T) {
	page := render(t, []Family{{
		Name: "groupd_kicks_total",
		Help: "Connections enforcement closed.",
		Type: Counter,
	}})

	if !strings.Contains(page, "# TYPE groupd_kicks_total counter") {
		t.Errorf("an empty family declares its type, and the page reads:\n%s", page)
	}
	for _, line := range strings.Split(page, "\n") {
		if strings.HasPrefix(line, "groupd_kicks_total") {
			t.Errorf("an empty family carries no sample, and this one carries %q", line)
		}
	}
}

func TestRenderWritesLabelsInTheOrderGiven(t *testing.T) {
	page := render(t, []Family{{
		Name: "groupd_member_live",
		Help: "One per member holding a live lease.",
		Type: Gauge,
		Samples: []Sample{{
			Labels: []Label{{"group", "abc"}, {"member", "alice"}, {"publishing", "yes"}},
			Value:  1,
		}},
	}})

	want := `groupd_member_live{group="abc",member="alice",publishing="yes"} 1`
	if !strings.Contains(page, want) {
		t.Errorf("a sample reads %q, and the page reads:\n%s", want, page)
	}
}

// A display name is a label value and arrives from whatever the member typed, so the characters the
// format reserves are escaped rather than trusted.
func TestRenderEscapesLabelValues(t *testing.T) {
	page := render(t, []Family{{
		Name: "groupd_member_live",
		Help: "One per member holding a live lease.",
		Type: Gauge,
		Samples: []Sample{{
			Labels: []Label{{"member", "a\"b\\c\nd"}},
			Value:  1,
		}},
	}})

	want := `groupd_member_live{member="a\"b\\c\nd"} 1`
	if !strings.Contains(page, want) {
		t.Errorf("a label value is escaped as %q, and the page reads:\n%s", want, page)
	}
}

func TestRenderWritesAFractionWithoutExponent(t *testing.T) {
	page := render(t, []Family{{
		Name:    "groupd_groups",
		Help:    "Groups holding at least one live lease.",
		Type:    Gauge,
		Samples: []Sample{{Value: 0.5}},
	}})

	if !strings.Contains(page, "groupd_groups 0.5") {
		t.Errorf("a fraction reads as itself, and the page reads:\n%s", page)
	}
}

func render(t *testing.T, families []Family) string {
	t.Helper()

	var page strings.Builder
	if err := Render(&page, families); err != nil {
		t.Fatalf("rendering a scrape: %v", err)
	}
	return page.String()
}
