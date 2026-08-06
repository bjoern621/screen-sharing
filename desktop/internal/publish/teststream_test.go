package publish

import (
	"slices"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

func TestBuildTestStreamArgs(t *testing.T) {
	// Transport srt on purpose: test streams must publish over RTSP anyway.
	s := settings.Stream{
		Name:                "nixos",
		RelayHost:           "relay.example",
		RtspPort:            8554,
		Transport:           "srt",
		RtspPublishProtocol: "tcp",
	}
	args, err := BuildTestStreamArgs(s, "test-1", "ball")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"videotestsrc",
		"is-live=true",
		"pattern=ball",
		"x264enc",
		"protocols=tcp",
		"location=rtsp://relay.example:8554/test-1",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in %v", want, args)
		}
	}
}

func TestTestPatternCycles(t *testing.T) {
	if TestPattern(0) != TestPattern(len(testPatterns)) {
		t.Error("TestPattern must wrap around")
	}
	seen := map[string]bool{}
	for i := range len(testPatterns) {
		p := TestPattern(i)
		if seen[p] {
			t.Errorf("pattern %q handed out twice within one cycle", p)
		}
		seen[p] = true
	}
}
