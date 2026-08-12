package publish

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/cursor"
	"bjoernblessin.de/screenshare/internal/portal"
	"bjoernblessin.de/screenshare/internal/rules"
)

// What each capture backend can do with the pointer.
//
// It is a table for the reason captureNeeds beside it is one: the same fact was being
// answered by whoever asked, and until this file existed nobody asked at all. Every
// backend drew the pointer unconditionally - ximagesrc with show-pointer=true,
// d3d11screencapturesrc with show-cursor=true, avfvideosrc with capture-screen-cursor,
// the ffmpeg backends on their own defaults - except kmsgrab, which passed nothing and
// drew none. That difference was in no table, stated nowhere, and visible only by
// running both.
//
// The axis is the capture backend rather than the codec, which is what the old gap
// mechanism could not express: a gap names a codec, and what the pointer does has
// nothing to do with the encoder. These are the first rules in the repository authored
// as rules rather than converted into them (docs/domain-model.md, "One evaluator").

// cursorServes is the modes each capture backend serves, in Modes order.
//
// A backend absent from the map is a bug rather than a backend that serves nothing,
// which init asserts against the registry: a new backend that forgot this table would
// otherwise offer every mode and pass a flag nothing reads.
var cursorServes = map[string][]string{
	// The two ffmpeg Windows grabbers take draw_mouse, and the GStreamer one takes
	// show-cursor. None of the three reports a pointer position of its own.
	"ddagrab":               {cursor.Embedded, cursor.Hidden},
	"gdigrab":               {cursor.Embedded, cursor.Hidden},
	"d3d11screencapturesrc": {cursor.Embedded, cursor.Hidden},

	// The X11 pair: draw_mouse on the ffmpeg side, show-pointer on the GStreamer one.
	// X11 hands the pointer over as part of the image, and it also answers any client that
	// asks where the pointer is, which is what the metadata mode reads there
	// (internal/pointer). The GStreamer one alone serves it, because the position is
	// reported by the publish child and the ffmpeg engine's child is ffmpeg.
	"x11grab":   {cursor.Embedded, cursor.Hidden},
	"ximagesrc": {cursor.Embedded, cursor.Hidden, cursor.Metadata},

	// kmsgrab reads the scanout's primary plane, and on every driver here the pointer is
	// a hardware plane of its own that the scanout composes at display time. There is
	// nothing on that path to draw it into the frames, so the capture is cursorless
	// whatever it is asked for, and hidden is the only mode that describes what it does.
	"kmsgrab": {cursor.Hidden},

	// The portal is the one backend that reports a pointer position instead of drawing
	// it, which is what cursor_mode's metadata value selects (internal/portal).
	"portal": {cursor.Embedded, cursor.Hidden, cursor.Metadata},

	// macOS: -capture_cursor on the ffmpeg side, capture-screen-cursor on the GStreamer
	// one.
	"avfoundation": {cursor.Embedded, cursor.Hidden},
	"avfvideosrc":  {cursor.Embedded, cursor.Hidden},
}

// The table describes the same set of backends the registry does, so a row in one and
// not the other is a backend whose pointer behaviour nobody stated or a table entry for
// a backend that does not exist. Both are bugs in this package, so they fail at load.
func init() {
	assert.Assert(len(cursorServes) == len(captureBackends),
		"a capture backend states what it does with the pointer exactly once",
		len(cursorServes), len(captureBackends))
	for name, serves := range cursorServes {
		_, ok := captureBackends[name]
		assert.Assert(ok, "a backend with a pointer row is one the registry carries", name)
		assert.Assert(len(serves) > 0, "a capture backend serves at least one pointer mode", name)
		for _, mode := range serves {
			assert.Assert(cursor.Known(mode), "a served pointer mode is one the settings name", name, mode)
		}
	}
	rules.Register(cursorRules()...)
}

// cursorRules is what each backend cannot do with the pointer, plus the one limit that
// is this app's rather than any backend's.
func cursorRules() []rules.Rule {
	var out []rules.Rule
	for _, name := range Captures() {
		for _, mode := range cursor.Modes {
			if serves(name, mode) {
				continue
			}
			out = append(out, rules.Rule{
				When:    map[string]rules.Match{rules.AxisCapture: rules.OneOf(name)},
				Verdict: rules.Refuse,
				Field:   rules.AxisCursor,
				Values:  rules.OneOf(mode),
				Reason:  cursorRefusal(name, mode),
			})
		}
	}

	// The portal reports a pointer position and nothing here reads it yet: the position
	// rides in the cursor metadata PipeWire carries beside each frame, which the publish
	// child would have to take off the stream itself rather than through pipewiresrc. The
	// refusal is stated separately from the backend rows above so a reader on the portal -
	// where the capture does report a pointer - learns what is actually missing, and it
	// binds on that backend alone now that the X11 one reads a position of its own.
	out = append(out, rules.Rule{
		When:    map[string]rules.Match{rules.AxisCapture: rules.OneOf("portal")},
		Verdict: rules.Refuse,
		Field:   rules.AxisCursor,
		Values:  rules.OneOf(cursor.Metadata),
		Reason:  screensharev1.TextCode_TEXT_CODE_CURSOR_METADATA_NOT_CARRIED,
	})

	// Where the mode is offered, it says how far the position travels. It leaves the capture,
	// crosses the control contract and reaches this machine's own screens, which is what makes
	// the choice usable and what a publisher can see for themselves on the preview; what does
	// not exist yet is a leg carrying it over the relay, so somebody watching from another
	// machine sees no pointer at all.
	//
	// A note and not a refusal, because the mode does what it says on the machine that picks
	// it, and the thing it does not do is a fact about what viewers receive rather than about
	// this capture. It binds wherever the mode is not already refused.
	out = append(out, rules.Rule{
		Verdict: rules.Note,
		Field:   rules.AxisCursor,
		Values:  rules.OneOf(cursor.Metadata),
		Reason:  screensharev1.TextCode_TEXT_CODE_CURSOR_METADATA_LOCAL_ONLY,
	})
	return out
}

// cursorRefusal is why a backend does not serve a mode.
//
// Two facts rather than one sentence with the backend's name in it, because they send a
// reader to different places: a capture that cannot draw the pointer at all is a
// property of how it reads the screen, and a capture with no pointer position to report
// is a property of what it reports beside the picture.
func cursorRefusal(capture, mode string) screensharev1.TextCode {
	if mode == cursor.Metadata {
		return screensharev1.TextCode_TEXT_CODE_CAPTURE_HAS_NO_CURSOR_METADATA
	}
	assert.Assert(capture == "kmsgrab",
		"the only backend that cannot draw the pointer is the scanout one", capture, mode)
	return screensharev1.TextCode_TEXT_CODE_KMSGRAB_HAS_NO_CURSOR_PLANE
}

// serves reports whether a capture backend offers a pointer mode.
func serves(capture, mode string) bool {
	for _, m := range cursorServes[capture] {
		if m == mode {
			return true
		}
	}
	return false
}

// gstBool is how a GStreamer element spells a boolean property value. One helper because
// three elements set the same fact through three differently named properties, and a
// literal typed at each site is one that can be spelled "1" at one of them.
func gstBool(on bool) string {
	if on {
		return "true"
	}
	return "false"
}

// portalCursor is the pointer setting as the ScreenCast interface's own cursor_mode.
//
// The portal is the one backend whose three modes are the settings' three, which is why
// this is a mapping rather than a boolean: its metadata mode sends the pointer position
// out of band instead of drawing it, and that is exactly the third value.
func portalCursor(mode string) portal.CursorMode {
	switch mode {
	case cursor.Embedded:
		return portal.CursorEmbedded
	case cursor.Hidden:
		return portal.CursorHidden
	case cursor.Metadata:
		return portal.CursorMetadata
	default:
		assert.Never("a portal capture names a pointer mode the settings carry", mode)
		return portal.CursorEmbedded
	}
}

// CursorServed reports whether the capture backend serves this pointer mode, for a
// caller choosing a value rather than greying one. What a form shows is the rules'
// answer; this is the same table asked directly, for the repair that has to land on a
// mode the backend runs.
func CursorServed(capture, mode string) bool {
	assert.Assert(cursor.Known(mode), "a pointer question names a mode the settings carry", mode)
	return serves(capture, mode)
}
