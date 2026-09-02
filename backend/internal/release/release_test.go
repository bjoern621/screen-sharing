package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The comparison decides whether a user is told anything at all,
// so it answers on the two shapes a build carries: the stamp a release writes, and "dev".
func TestWhichBuildIsNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            State
	}{
		{"0.4.0", "0.4.0", StateCurrent},
		{"0.4.0", "v0.4.0", StateCurrent},
		{"v0.4.0", "0.4.0", StateCurrent},
		{"0.4.0", "0.5.0", StateBehind},
		{"0.4.0", "0.4.1", StateBehind},
		{"0.9.0", "0.10.0", StateBehind},
		{"1.0.0", "0.99.9", StateCurrent},
		// A build nobody stamped is every developer's, and it is newer than any release
		// as often as it is older, so it is told nothing.
		{"dev", "0.5.0", StateUnknown},
		{"0.4.0", "", StateUnknown},
		{"", "0.5.0", StateUnknown},
		// A tag nothing can be read out of says nothing about this build.
		{"0.4.0", "nightly", StateUnknown},
	}

	for _, tc := range cases {
		if got := Compare(tc.current, tc.latest); got != tc.want {
			t.Errorf("Compare(%q, %q) is %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

// The tag is read off the release the project publishes,
// which is the one a download link points at.
func TestTheLatestTagIsReadOffTheAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v0.5.0","html_url":"https://example.invalid/releases/v0.5.0"}`))
	}))
	defer server.Close()

	latest, err := latestFrom(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("reading the latest release: %v", err)
	}
	if latest.Tag != "v0.5.0" {
		t.Errorf("the tag reads %q, want the one the answer carries", latest.Tag)
	}
	if latest.URL == "" {
		t.Error("the release names no page to reach it at")
	}
}

// A service that is not there is an Umgebungsfehler:
// the app runs, and what it cannot say is whether a newer build exists.
func TestAnUnreachableServiceIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	if _, err := latestFrom(context.Background(), server.URL); err == nil {
		t.Error("a refused read answered no error, so a caller would treat it as a release")
	}
}

// Nothing that is not a release is read as one.
// The answer is JSON from a service this app does not run,
// so a body that is not what it expects leaves rather than half-decoding.
func TestAnAnswerThatIsNotAReleaseIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	if _, err := latestFrom(context.Background(), server.URL); err == nil {
		t.Error("a body that is not JSON answered no error")
	}
}
