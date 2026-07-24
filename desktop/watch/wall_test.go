package watch

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestWallLayoutSingleTileFillsCanvas(t *testing.T) {
	tiles := wallLayout(1)
	if len(tiles) != 1 {
		t.Fatalf("got %d tiles, want 1", len(tiles))
	}
	if tiles[0] != (tile{x: 0, y: 0, w: wallWidth, h: wallHeight}) {
		t.Errorf("single tile = %+v, want full canvas", tiles[0])
	}
}

func TestWallLayoutCentersIncompleteLastRow(t *testing.T) {
	// Three streams: 2x2 grid with one tile in the last row, centered.
	tiles := wallLayout(3)
	if len(tiles) != 3 {
		t.Fatalf("got %d tiles, want 3", len(tiles))
	}
	w, h := wallWidth/2, wallHeight/2
	want := []tile{
		{x: 0, y: 0, w: w, h: h},
		{x: w, y: 0, w: w, h: h},
		{x: (wallWidth - w) / 2, y: h, w: w, h: h},
	}
	for i := range want {
		if tiles[i] != want[i] {
			t.Errorf("tile %d = %+v, want %+v", i, tiles[i], want[i])
		}
	}
}

func TestWallLayoutStaysOnCanvas(t *testing.T) {
	for n := 1; n <= 12; n++ {
		for i, tl := range wallLayout(n) {
			if tl.x < 0 || tl.y < 0 || tl.x+tl.w > wallWidth || tl.y+tl.h > wallHeight {
				t.Errorf("n=%d tile %d = %+v leaves the canvas", n, i, tl)
			}
		}
	}
}

func TestBuildWallArgsRTSP(t *testing.T) {
	args, err := BuildWallArgs(rtspStream(), []string{"alice", "bob"}, "rtsp")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"compositor",
		"sink_0::sizing-policy=keep-aspect-ratio",
		"sink_1::xpos=" + strconv.Itoa(wallWidth/2),
		"location=rtsp://relay.example:8554/alice",
		"location=rtsp://relay.example:8554/bob",
		"protocols=tcp",
		"comp.sink_1",
		`text="bob"`,
	} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
}

func TestBuildWallArgsSRT(t *testing.T) {
	args, err := BuildWallArgs(srtStream(), []string{"alice"}, "srt")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"srtsrc",
		"uri=srt://relay.example:8890",
		"streamid=read:alice",
		"latency=1200",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
	if joined := strings.Join(args, " "); strings.Contains(joined, "rcvbuf") {
		t.Errorf("ffmpeg-only srt options leaked into %v", args)
	}
}

func TestBuildWallArgsRejectsEmptyAndUnsupported(t *testing.T) {
	if _, err := BuildWallArgs(rtspStream(), nil, "rtsp"); err == nil {
		t.Fatal("expected error for an empty stream list")
	}
	if _, err := BuildWallArgs(rtspStream(), []string{"alice"}, "webrtc"); err == nil {
		t.Fatal("expected error for a transport without a GStreamer watch form")
	}
	if _, err := BuildWallArgs(rtspStream(), []string{"alice"}, "carrier-pigeon"); err == nil {
		t.Fatal("expected error for an unknown transport")
	}
}

func TestGstQuoteEscapes(t *testing.T) {
	if got := gstQuote(`a "b" \c`); got != `"a \"b\" \\c"` {
		t.Errorf("gstQuote = %s", got)
	}
}
