package stats

import (
	"testing"
	"time"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// TestBlocksAreWellFormed holds the table's own invariants: the card walks it in
// lockstep with its widgets, so a row without a key or a reader would render an
// empty line or panic on refresh.
func TestBlocksAreWellFormed(t *testing.T) {
	for _, b := range blocks {
		if b.title == "" {
			t.Error("a block carries no title")
		}
		if len(b.rows) == 0 {
			t.Errorf("block %q holds no rows", b.title)
		}
		for _, r := range b.rows {
			if r.key == "" {
				t.Errorf("block %q holds a row without a key", b.title)
			}
			if r.value == nil {
				t.Errorf("row %q/%q reads nothing", b.title, r.key)
			}
		}
	}
}

// TestEmptyPollHidesEverythingOptional is the state a tile opens in: a pipeline that
// has negotiated nothing. Every row that can hide reports no value, and the rows that
// hold their place report one.
func TestEmptyPollHidesEverythingOptional(t *testing.T) {
	v := View{}
	for _, b := range blocks {
		for _, r := range b.rows {
			got := r.value(v)
			if r.hides && got != "" {
				t.Errorf("row %q/%q reports %q on an empty poll, want nothing", b.title, r.key, got)
			}
		}
	}
}

// TestRowValues pins the readings a stream produces once its pipeline is up.
func TestRowValues(t *testing.T) {
	v := View{
		Stream: roster.Stream{Transport: "srt", Source: "srtsrc uri=srt://relay"},
		FPS:    "59.9",
		Stats: player.Stats{
			Codec:         "H.265 (Main 4:4:4 profile)",
			Profile:       "main-444",
			Level:         "5.1",
			VideoBytes:    3 * 1024 * 1024,
			VideoFrames:   120,
			Keyframes:     2,
			SinceKeyframe: 1500 * time.Millisecond,
			Width:         2560,
			Height:        1440,
			Format:        "Y444_10LE",
			Depth:         10,
			Subsampling:   "4:4:4",
			Colorimetry:   "bt709",
			FPSNum:        60,
			FPSDen:        1,
			Decoder:       "nvh265dec",
			Hardware:      true,
			Rendered:      118,
			Dropped:       2,
			Live:          true,
			LatencyMin:    200 * time.Millisecond,
			LatencyMax:    800 * time.Millisecond,
			Uptime:        90 * time.Second,
			AudioCodec:    "Opus",
			AudioFormat:   "F32LE",
			AudioRate:     48000,
			AudioChannels: 2,
			AudioBytes:    64 * 1024,
		},
		VideoRate: 42.5,
		AudioRate: 0.1,
	}
	want := map[string]string{
		"resolution": "2560×1440",
		"framerate":  "60/1",
		"profile":    "main-444 · L5.1",
		"format":     "Y444_10LE · 4:4:4 · 10 bit",
		"keyframes":  "2 · 1.5 s ago",
		"bitrate":    "42.5 Mbps · 3.0 MiB",
		"decoder":    "nvh265dec (hardware)",
		"frames":     "118 rendered · 2 dropped",
		"encoded":    "120 frames",
		"latency":    "200 ms / 800 ms · live",
		"uptime":     "1m30s",
		"fps":        "59.9",
	}
	for _, b := range blocks {
		for _, r := range b.rows {
			// The audio block repeats keys the video block has; only the video
			// readings are pinned here.
			if b.title == "audio" {
				continue
			}
			expected, ok := want[r.key]
			if !ok {
				continue
			}
			if got := r.value(v); got != expected {
				t.Errorf("row %q = %q, want %q", r.key, got, expected)
			}
		}
	}
}

func TestAudioBlockHidesWithoutAudio(t *testing.T) {
	var audio *block
	for i := range blocks {
		if blocks[i].title == "audio" {
			audio = &blocks[i]
		}
	}
	if audio == nil {
		t.Fatal("no audio block")
	}
	if audio.visible == nil {
		t.Fatal("the audio block is always visible")
	}
	if audio.visible(View{}) {
		t.Error("the audio block shows on a video-only stream")
	}
	if !audio.visible(View{Stats: player.Stats{AudioCodec: "Opus"}}) {
		t.Error("the audio block hides on a stream that carries audio")
	}
}

func TestAudioFormatText(t *testing.T) {
	cases := []struct {
		name string
		in   player.Stats
		want string
	}{
		{name: "nothing negotiated", in: player.Stats{}, want: ""},
		{name: "format alone", in: player.Stats{AudioFormat: "S16LE"}, want: "S16LE"},
		{
			name: "rate and channels",
			in:   player.Stats{AudioFormat: "F32LE", AudioRate: 48000, AudioChannels: 2},
			want: "F32LE 48 kHz stereo",
		},
		{
			name: "surround counts its channels",
			in:   player.Stats{AudioFormat: "F32LE", AudioRate: 44100, AudioChannels: 6},
			want: "F32LE 44.1 kHz 6ch",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := audioFormatText(c.in); got != c.want {
				t.Errorf("audioFormatText = %q, want %q", got, c.want)
			}
		})
	}
}

func TestByteSize(t *testing.T) {
	cases := []struct {
		in   uint64
		want string
	}{
		{in: 0, want: "0 B"},
		{in: 512, want: "512 B"},
		{in: 2048, want: "2.0 KiB"},
		{in: 5 * 1024 * 1024, want: "5.0 MiB"},
		{in: 3 * 1024 * 1024 * 1024, want: "3.0 GiB"},
	}
	for _, c := range cases {
		if got := byteSize(c.in); got != c.want {
			t.Errorf("byteSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestShortDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{in: 0, want: ""},
		{in: -time.Second, want: ""},
		{in: 200 * time.Millisecond, want: "200 ms"},
		{in: 2500 * time.Millisecond, want: "2.5 s"},
		{in: 90 * time.Second, want: "1m30s"},
	}
	for _, c := range cases {
		if got := shortDuration(c.in); got != c.want {
			t.Errorf("shortDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFramesTextFallsBackToPainted covers the window before the sink reports: the
// paintable's own count stands in, and it cannot tell a dropped frame apart.
func TestFramesTextFallsBackToPainted(t *testing.T) {
	if got := framesText(player.Stats{Frames: 30}); got != "30 painted" {
		t.Errorf("framesText = %q, want \"30 painted\"", got)
	}
	if got := framesText(player.Stats{}); got != "" {
		t.Errorf("framesText on an empty poll = %q, want nothing", got)
	}
}

func TestMbps(t *testing.T) {
	if got := mbps(1_000_000, 1); got != 8 {
		t.Errorf("mbps(1 MB, 1s) = %v, want 8", got)
	}
	if got := mbps(1_000_000, 0); got != 0 {
		t.Errorf("mbps over no interval = %v, want 0", got)
	}
}
