// Package applink is the address a stream of this app has outside it.
//
// One shape, "mirrorme://watch/<group id>/<stream>", which the desktop hands to this app rather than
// to a browser (packaging/linux/mirrorme.desktop).
// The group id is the public digest every path on the relay already carries,
// and holding it opens nothing: a link is followed by a member of that group and by nobody else
// (docs/auth-flow.md).
package applink

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
)

// Scheme is what the desktop registers this app as the handler for.
const Scheme = "mirrorme"

// watchHost is the one action a link states.
// In the host rather than in the path so a second action is a second word at the same depth.
const watchHost = "watch"

// Watch names one stream inside one group.
type Watch struct {
	// Group is the public group id, the digest a prefix carries before the separator.
	Group string
	// Stream is the stream's own name inside that group, which carries a slash of its own:
	// "bob/monitor-0".
	Stream string
}

// FormatWatch is the link naming one stream of one group.
func FormatWatch(group, stream string) string {
	assert.Assert(group != "", "a link names the group its stream lives in", stream)
	assert.Assert(stream != "", "a link names the stream it opens", group)

	segments := make([]string, 0, 3)
	segments = append(segments, url.PathEscape(group))
	for _, part := range strings.Split(stream, "/") {
		segments = append(segments, url.PathEscape(part))
	}

	link := Scheme + "://" + watchHost + "/" + strings.Join(segments, "/")

	// The producing side fails where the pair cannot be read back,
	// rather than the machine that was handed the link.
	parsed, err := Parse(link)
	assert.Assert(err == nil, "a formatted link parses", link)
	assert.Assert(parsed.Group == group && parsed.Stream == stream,
		"a formatted link carries the pair it was made from", parsed.Group, parsed.Stream)
	return link
}

// Parse reads a link.
//
// Every refusal is an Umgebungsfehler: what arrives here was typed, pasted,
// or handed over by another program.
func Parse(raw string) (Watch, error) {
	if raw == "" {
		return Watch{}, errors.New("a link names a stream to open, and this one is empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return Watch{}, fmt.Errorf("this is not a link this app opens: %w", err)
	}
	if parsed.Scheme != Scheme {
		return Watch{}, fmt.Errorf("this app opens %s:// links, and this one is %q", Scheme, parsed.Scheme)
	}
	if parsed.Host != watchHost {
		return Watch{}, fmt.Errorf("this app opens %s://%s links, and this one names %q", Scheme, watchHost, parsed.Host)
	}

	// Split before unescaping, so a segment carrying an escaped separator stays one segment.
	// A display name is part of a stream's own name and a person types it, slash included.
	trimmed := strings.Trim(parsed.EscapedPath(), "/")
	group, stream, found := strings.Cut(trimmed, "/")
	if !found || group == "" || stream == "" {
		return Watch{}, errors.New("a link names a group and a stream inside it")
	}

	return unescapeWatch(group, stream)
}

// unescapeWatch reads the pair back, segment by segment.
func unescapeWatch(group, stream string) (Watch, error) {
	read, err := url.PathUnescape(group)
	if err != nil {
		return Watch{}, fmt.Errorf("the group this link names does not decode: %w", err)
	}

	parts := strings.Split(stream, "/")
	for i, part := range parts {
		if parts[i], err = url.PathUnescape(part); err != nil {
			return Watch{}, fmt.Errorf("the stream this link names does not decode: %w", err)
		}
	}

	return Watch{Group: read, Stream: strings.Join(parts, "/")}, nil
}
