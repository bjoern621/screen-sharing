package receive

import "bjoernblessin.de/go-utils/util/logger"

// Tone mapping is the one thing a viewer decides about an HDR stream.
//
// A stream that carries one of the BT.2100 curves carries more range than a standard display
// shows. Drawn as it is, it is wrong in the direction the curve leans: the frames reach the
// window labelled sRGB and carrying PQ samples, because every chain here converts matrix and
// range and applies no transfer function at all. Rolling that range down into the display's
// is a conversion of its own, and the element that does it is the one thing this app cannot
// supply itself.
//
// It is a choice rather than something done to every HDR stream, and per tile rather than
// per machine. A reader comparing what a stream carries against what a display shows wants
// the untouched picture; a reader watching it wants the mapped one; and both can be on
// screen at once, watching the same stream through two tiles.
//
// It is not a setting either, and that is deliberate: a preference kept per stream path
// would outlive the stream it was made about, so a path that stops carrying HDR would carry
// a stale choice nobody can find. The choice lives with the decode, which is exactly as long
// as it means anything.

// toneMapRung is what a decode rolls an HDR stream down with: the launch-line fragment that
// goes between the decoder and the chain, and the factories it is built from.
//
// One rung for every chain rather than a column on each row. It takes whatever the decoder
// produced and hands on a standard-range picture, which is the input every chain already
// converts, so which rung this machine has is the platform's answer and not the row's.
type toneMapRung struct {
	needs    []string
	elements []string
}

// missing names the first factory the rung needs and this GStreamer does not register, and
// "" for a rung that can be built. A rung with no elements at all is a platform that
// declares none, which has nothing to be missing.
func (r toneMapRung) missing() string {
	if len(r.elements) == 0 {
		return ""
	}
	return missingFactory(r.needs)
}

// available reports whether a decode asked to tone-map would be built with a rung that does.
func (r toneMapRung) available() bool {
	return len(r.elements) > 0 && r.missing() == ""
}

// unavailable says why a decode that asked to tone-map was built without a rung, for the
// line that reports the fallback in the log. What a reader is shown is written from the
// ToneMap report instead, because the words on screen are the shell's.
func (r toneMapRung) unavailable() string {
	if len(r.elements) == 0 {
		return "no element on this platform rolls an HDR stream down"
	}
	return "this GStreamer registers no " + r.missing()
}

// WillToneMap is whether a decode asked for tone mapping is built with the rung.
//
// It is what a caller compares a running decode against, rather than its own request. A
// machine with no rung builds without one, so a caller holding the request itself against
// what ran would find the two different every time and rebuild the same pipeline forever.
func WillToneMap(want bool) bool {
	// The registry decides what runs and it exists after the library is up, which a caller
	// asking before it has opened anything has not done.
	initGStreamer()

	return want && toneMapping.available()
}

// toneMapFor is WillToneMap with the line that says a rung was dropped.
//
// A rung this machine does not have is dropped rather than refused, for the reason a chain
// falls back: what is missing is an element of this GStreamer, and a stream drawn in the
// range it was coded in is a picture where a refusal is none. The answer is what the receive
// state then reports, so a tile shows what ran rather than what was asked for.
func toneMapFor(stream string, want bool) bool {
	if got := WillToneMap(want); got == want {
		return got
	}
	logger.Warnf("stream %q was asked to be tone-mapped and %s, so it is drawn as it arrives",
		stream, toneMapping.unavailable())
	return false
}

// ToneMap is what this machine can do about an HDR stream, as a viewer offers the choice.
//
// Nothing above this package names GStreamer, so which element rolls the range down stays
// here and only the reading of it crosses.
type ToneMap struct {
	// Available is whether a decode that asks to tone-map is built with a rung that does. A
	// decode that asks anyway on a machine that cannot is built without one and reports so,
	// which is the same fallback a render chain makes.
	Available bool
	// MissingElement names the first factory the platform's rung needs and this GStreamer
	// does not register. It is empty both on a machine that can tone-map and on a platform
	// that declares no rung at all, which Available is what tells apart: there is nothing to
	// install where nothing was declared.
	MissingElement string
}

// ToneMapping is what this machine offers a viewer that is watching an HDR stream.
func ToneMapping() ToneMap {
	// The registry answers what runs only once the library is up, and a form can ask what it
	// may offer before a stream is ever received.
	initGStreamer()

	return ToneMap{
		Available:      toneMapping.available(),
		MissingElement: toneMapping.missing(),
	}
}
