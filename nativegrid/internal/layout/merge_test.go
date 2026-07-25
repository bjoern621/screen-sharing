package layout

import (
	"slices"
	"testing"
)

func TestMergeOrder(t *testing.T) {
	cases := []struct {
		name         string
		saved, shown []string
		want         []string
	}{
		{
			name:  "first run has nothing remembered",
			saved: nil,
			shown: []string{"a", "b"},
			want:  []string{"a", "b"},
		},
		{
			name:  "the shown order wins over the remembered one",
			saved: []string{"a", "b", "c"},
			shown: []string{"c", "a", "b"},
			want:  []string{"c", "a", "b"},
		},
		{
			name:  "an absent stream keeps the slot behind the one it followed",
			saved: []string{"a", "b", "c"},
			shown: []string{"c", "a"},
			want:  []string{"c", "a", "b"},
		},
		{
			name:  "an absent stream that led the order still leads it",
			saved: []string{"a", "b", "c"},
			shown: []string{"c", "b"},
			want:  []string{"a", "c", "b"},
		},
		{
			name:  "a stream never seen before goes where it is shown",
			saved: []string{"a", "b"},
			shown: []string{"new", "a", "b"},
			want:  []string{"new", "a", "b"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := MergeOrder(c.saved, c.shown)
			if !slices.Equal(got, c.want) {
				t.Errorf("MergeOrder(%v, %v) = %v, want %v", c.saved, c.shown, got, c.want)
			}
		})
	}
}
