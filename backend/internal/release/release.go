// Package release answers whether a newer build of this app has been published.
//
// A beta runs on whatever its testers downloaded once, so a bug already fixed is reported again
// by somebody who cannot know their build predates the fix.
// The build a report came from is therefore worth stating beside the release that is current.
//
// Read from the project's own releases and from nothing else.
// The answer is one tag and one page to reach it at:
// what to do about it is the reader's, and nothing here downloads or replaces anything.
//
// Every failure on this path is an Umgebungsfehler.
// A machine with no route to the service runs unchanged and is told nothing,
// which is why a caller takes StateUnknown as the ordinary answer rather than as a fault.
package release

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// LatestURL is the release this project publishes, as its forge answers it.
const LatestURL = "https://api.github.com/repos/bjoern621/screen-sharing/releases/latest"

// Timeout bounds the read.
// A check nobody is waiting on still holds a goroutine and a connection,
// and the answer stops being worth having long before a stalled socket gives up.
const Timeout = 10 * time.Second

// State is what a build's version says about it against the published one.
type State int

const (
	// StateUnknown is every case the comparison cannot answer:
	// a build nobody stamped, a tag with no version in it, a service that did not answer.
	StateUnknown State = iota
	// StateCurrent is a build at or ahead of the published release.
	StateCurrent
	// StateBehind is a build the published release is newer than.
	StateBehind
)

// Latest is the published release.
type Latest struct {
	// Tag is the release's own name: "v0.5.0".
	Tag string `json:"tag_name"`
	// URL is the page a reader reaches it at.
	URL string `json:"html_url"`
}

// Fetch reads the published release off the project's forge.
func Fetch(ctx context.Context) (Latest, error) {
	assert.IsNotNil(ctx, "a release read runs under a context")

	return latestFrom(ctx, LatestURL)
}

// latestFrom reads one release answer, from an address a test can point elsewhere.
func latestFrom(ctx context.Context, url string) (Latest, error) {
	assert.IsNotNil(ctx, "a release read runs under a context")

	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Latest{}, err
	}
	// The forge serves a stable shape under this media type and reserves the right to change
	// what it answers without one.
	request.Header.Set("Accept", "application/vnd.github+json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return Latest{}, err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return Latest{}, fmt.Errorf("the release service answered %s", response.Status)
	}

	var latest Latest
	if err := json.NewDecoder(response.Body).Decode(&latest); err != nil {
		return Latest{}, fmt.Errorf("the release answer did not decode: %w", err)
	}
	if latest.Tag == "" {
		return Latest{}, fmt.Errorf("the release answer names no tag")
	}
	return latest, nil
}

// Compare answers what current is against latest.
//
// Either side that carries no version leaves the answer unknown.
// A build nobody stamped is the developer's own and is as often ahead of a release as behind it,
// so it is told nothing.
func Compare(current, latest string) State {
	running, ok := parse(current)
	if !ok {
		return StateUnknown
	}
	published, ok := parse(latest)
	if !ok {
		return StateUnknown
	}

	for i := range running {
		switch {
		case published[i] > running[i]:
			return StateBehind
		case published[i] < running[i]:
			return StateCurrent
		}
	}
	return StateCurrent
}

// parse reads a release into its three fields, and false where the string carries no version.
// The leading "v" a tag wears is optional, a tag carrying it and the VERSION file not.
func parse(version string) ([3]int, bool) {
	fields := strings.Split(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".")
	if len(fields) != 3 {
		return [3]int{}, false
	}

	var out [3]int
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
