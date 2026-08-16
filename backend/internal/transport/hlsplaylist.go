package transport

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/settings"
)

// Resolving what a receive pipeline opens for HLS, the one watch leg whose address settings do not
// hold.
//
// The master playlist names a media playlist per rendition, under a session MediaMTX mints per
// reader, and that address is answered 401 without it.
//
// Two renditions are one too many. hlsdemux2 stalls before the first frame of a master naming an
// audio group, where libavformat plays the same stream, so what is opened is the video media
// playlist and the leg carries no audio.
//
// A just-started muxer serves EXT-X-GAP placeholders in place of segments. RFC 8216 has a client
// skip them, hlsdemux2 downloads one and fails the pipeline, so a source is handed over only once a
// segment that is not a gap is in the playlist.

// hlsResolveTimeout bounds the whole resolve: the master read, and the wait for the media playlist
// to carry a segment.
// Longer than one segment (the relay cuts at a second) so the ordinary cold start is waited out,
// short enough that a relay serving nothing is a refusal a reader sees rather than a tile that hangs.
const hlsResolveTimeout = 15 * time.Second

// hlsFetchTimeout is one playlist read. Several fit inside hlsResolveTimeout.
const hlsFetchTimeout = 3 * time.Second

// hlsResolvePoll is the wait between reads of a playlist that carries no segment yet.
// The same session is read each time, so polling costs the relay a request and never a new reader.
const hlsResolvePoll = 500 * time.Millisecond

const (
	hlsStreamInfTag = "#EXT-X-STREAM-INF"
	hlsGapTag       = "#EXT-X-GAP"
)

// hlsMediaSource is the address a receive pipeline opens, resolved from the master playlist at
// masterURL and carrying the credential in the form souphttpsrc sends.
func hlsMediaSource(s settings.Settings, masterURL string) (string, error) {
	assert.Assert(masterURL != "", "a playlist resolve names the master it reads")

	client := &http.Client{Timeout: hlsFetchTimeout}

	master, err := hlsFetch(client, s, masterURL)
	if err != nil {
		return "", err
	}
	variant, ok := hlsVariantURI(master)
	if !ok {
		return "", fmt.Errorf("the relay's playlist at %s names no video rendition", masterURL)
	}
	mediaURL, err := hlsResolveRef(masterURL, variant)
	if err != nil {
		return "", err
	}

	deadline := time.Now().Add(hlsResolveTimeout)
	for {
		media, err := hlsFetch(client, s, mediaURL)
		if err != nil {
			return "", err
		}
		if hlsHasSegment(media) {
			return hlsWithCredential(s, mediaURL)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("the relay's playlist at %s carried no segment within %s", mediaURL, hlsResolveTimeout)
		}
		time.Sleep(hlsResolvePoll)
	}
}

// hlsFetch reads one playlist, with the credential in the header the relay's HTTP servers read
// (credential.go).
func hlsFetch(client *http.Client, s settings.Settings, addr string) (string, error) {
	assert.IsNotNil(client, "a playlist read runs on a client")

	req, err := http.NewRequest(http.MethodGet, addr, nil)
	if err != nil {
		return "", fmt.Errorf("the relay address %s is not one that can be read: %w", addr, err)
	}
	if name, value, ok := CredentialHeader(s); ok {
		req.Header.Set(name, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("the relay did not answer %s: %s", addr, Redact(s, err.Error()))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the relay answered %s with %s", addr, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("the relay's answer at %s broke off: %w", addr, err)
	}
	return string(body), nil
}

// hlsVariantURI is the URI under the first EXT-X-STREAM-INF, the video rendition.
// An audio group carries its URI inside an EXT-X-MEDIA tag, so it is never what this returns.
func hlsVariantURI(playlist string) (string, bool) {
	announced := false
	for _, line := range strings.Split(playlist, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case strings.HasPrefix(line, hlsStreamInfTag):
			announced = true
		case strings.HasPrefix(line, "#"):
		case announced:
			return line, true
		}
	}
	return "", false
}

// hlsHasSegment reports whether the playlist carries a media segment that is not a gap.
// A tag block applies to the URI under it and to no later one, so EXT-X-GAP is carried one line and
// cleared there.
func hlsHasSegment(playlist string) bool {
	gap := false
	for _, line := range strings.Split(playlist, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case strings.HasPrefix(line, hlsGapTag):
			gap = true
		case strings.HasPrefix(line, "#"):
		case !gap:
			return true
		default:
			gap = false
		}
	}
	return false
}

// hlsResolveRef resolves a playlist's own reference against the address it was read from, the
// renditions being named relative to the master and under a query.
func hlsResolveRef(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("the relay address %s is not one that can be read: %w", base, err)
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("the relay's playlist at %s names the rendition %q, which is not an address: %w", base, ref, err)
	}
	return b.ResolveReference(r).String(), nil
}

// hlsWithCredential puts the token where souphttpsrc sends it from.
//
// A launch line sets no header, and the element is built by urisourcebin rather than by this side, so
// the one form that reaches the relay is the Basic pair it builds out of the address itself. The
// segment requests inherit it, being resolved against this address.
func hlsWithCredential(s settings.Settings, addr string) (string, error) {
	token, ok := credentialToken(s)
	if !ok {
		return addr, nil
	}
	u, err := url.Parse(addr)
	if err != nil {
		return "", fmt.Errorf("the relay address %s is not one that can be read: %w", addr, err)
	}
	u.User = url.UserPassword(browserAuthUser, token)
	return u.String(), nil
}
