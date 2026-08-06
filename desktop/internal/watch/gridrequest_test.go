package watch

import "testing"

func TestParseGridRequest(t *testing.T) {
	r, err := ParseGridRequest(`{"stream":"alice","transport":"rtsp","options":{"rtspWatchLatencyMs":"400"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Stream != "alice" || r.Transport != "rtsp" {
		t.Errorf("request = %+v, want alice over rtsp", r)
	}
	if got := r.Options["rtspWatchLatencyMs"]; got != "400" {
		t.Errorf("option = %q, want 400", got)
	}
}

// The window logs to stderr and writes nothing but requests here, so a line
// that is not one came from somewhere else and moves no stream.
func TestParseGridRequestRefusesAnythingElse(t *testing.T) {
	for _, line := range []string{"", "gst warning: something", "{}", `{"transport":"rtsp"}`} {
		if _, err := ParseGridRequest(line); err == nil {
			t.Errorf("%q was read as a request", line)
		}
	}
}
