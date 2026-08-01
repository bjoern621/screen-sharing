package stats

import (
	"testing"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
)

// TestPathsCoverEveryPair is what the lookup's assert relies on: the table answers
// for all four combinations of the two ends, so a poll can never reach a pair the
// table has no verdict for.
func TestPathsCoverEveryPair(t *testing.T) {
	if len(paths) != 4 {
		t.Errorf("the path table holds %d verdicts, want one per pair of ends", len(paths))
	}
	for _, decode := range []bool{true, false} {
		for _, sink := range []bool{true, false} {
			p, ok := paths[memoryPair{decodeOnDevice: decode, sinkOnDevice: sink}]
			if !ok {
				t.Errorf("no render path for decode=%t sink=%t", decode, sink)
				continue
			}
			if p.label == "" {
				t.Errorf("the path for decode=%t sink=%t reads as nothing", decode, sink)
			}
			if p.tip == "" {
				t.Errorf("the path for decode=%t sink=%t explains nothing", decode, sink)
			}
		}
	}
}

// TestPathOfReadsTheTwoEnds pins the verdict each pair of negotiated memories gets,
// which is the reading the overlay's path row is.
func TestPathOfReadsTheTwoEnds(t *testing.T) {
	const gl = "memory:GLMemory"
	cases := []struct {
		name         string
		decode, sink string
		want         string
	}{
		{name: "GPU throughout", decode: gl, sink: gl, want: "no download"},
		{name: "hidden download", decode: gl, sink: player.MemorySystem, want: "downloaded before the sink"},
		{name: "upload for the sink", decode: player.MemorySystem, sink: gl, want: "uploaded before the sink"},
		{name: "system memory", decode: player.MemorySystem, sink: player.MemorySystem, want: "system memory throughout"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := player.Stats{DecodeMemory: c.decode, RenderMemory: c.sink}
			p, ok := pathOf(s)
			if !ok {
				t.Fatal("two negotiated ends yield no verdict")
			}
			if p.label != c.want {
				t.Errorf("path = %q, want %q", p.label, c.want)
			}
			if got := pathText(s); got != c.want {
				t.Errorf("path row = %q, want %q", got, c.want)
			}
			if pathTip(s) != p.tip {
				t.Error("the path row explains something other than the verdict it shows")
			}
		})
	}
}

// TestPathOfWaitsForBothEnds is the state a tile opens in: one end negotiated and
// the other not. An unknown memory is not a claim that the frames stayed in system
// memory, so the row shows nothing rather than a verdict off half the evidence.
func TestPathOfWaitsForBothEnds(t *testing.T) {
	cases := []player.Stats{
		{},
		{DecodeMemory: "memory:GLMemory"},
		{RenderMemory: player.MemorySystem},
	}
	for _, s := range cases {
		if _, ok := pathOf(s); ok {
			t.Errorf("a poll with decode=%q sink=%q yields a verdict", s.DecodeMemory, s.RenderMemory)
		}
		if got := pathText(s); got != "" {
			t.Errorf("path row = %q on half the evidence, want nothing", got)
		}
		if got := pathTip(s); got != "" {
			t.Errorf("path tip = %q on half the evidence, want nothing", got)
		}
	}
}

// TestMemoryTextIsBothFeaturesVerbatim keeps the evidence row an evidence row: the
// features as the caps carry them, in decode-then-sink order, and nothing while
// either end is missing.
func TestMemoryTextIsBothFeaturesVerbatim(t *testing.T) {
	s := player.Stats{DecodeMemory: "memory:D3D11Memory", RenderMemory: player.MemorySystem}
	if got := memoryText(s); got != "memory:D3D11Memory → memory:SystemMemory" {
		t.Errorf("memoryText = %q", got)
	}
	if got := memoryText(player.Stats{DecodeMemory: "memory:GLMemory"}); got != "" {
		t.Errorf("memoryText with one end = %q, want nothing", got)
	}
}

// TestChainTextNamesTheColourClaim covers the row a chain choice is read back off:
// a chain that states its colour and one that does not read differently, and a
// pipeline that has not reported one reads as nothing.
func TestChainTextNamesTheColourClaim(t *testing.T) {
	if got := chainText(player.Stats{Chain: "cpu", ChainExact: true}); got != "cpu (colour stated)" {
		t.Errorf("chainText = %q", got)
	}
	if got := chainText(player.Stats{Chain: "d3d11"}); got != "d3d11 (colour unstated)" {
		t.Errorf("chainText = %q", got)
	}
	if got := chainText(player.Stats{}); got != "" {
		t.Errorf("chainText without a chain = %q, want nothing", got)
	}
}

// TestRowTipFollowsItsValue is the contract the card's render pass carries: a row
// whose reading is a verdict explains the verdict it shows, and falls back to its
// own explanation while there is none.
func TestRowTipFollowsItsValue(t *testing.T) {
	var pathRow *row
	for bi := range blocks {
		for ri := range blocks[bi].rows {
			if blocks[bi].rows[ri].key == "path" {
				pathRow = &blocks[bi].rows[ri]
			}
		}
	}
	if pathRow == nil {
		t.Fatal("no path row")
	}
	empty := View{}
	if got := pathRow.tipAt(empty); got != pathRow.tip {
		t.Errorf("the path row on an empty poll explains %q, want its own explanation", got)
	}
	settled := View{Stats: player.Stats{
		DecodeMemory: "memory:GLMemory",
		RenderMemory: player.MemorySystem,
	}}
	if got := pathRow.tipAt(settled); got == pathRow.tip || got == "" {
		t.Errorf("the path row on a settled poll explains %q, want the verdict's own reading", got)
	}
}
