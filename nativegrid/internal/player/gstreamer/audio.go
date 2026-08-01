package gstreamer

import (
	"slices"
	"strings"

	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// audioChain is the branch an audio pad feeds:
//
//	queue ! audioconvert ! audioresample ! volume ! autoaudiosink
//
// The queue decouples audio from the decode thread like the video branch's does.
// Two of the elements are kept: volume drives the tile's control, and
// audioconvert's input caps are the raw audio the overlay reports, the
// counterpart of what the video branch reads off videoconvert.
var audioChain = []string{"queue", "audioconvert", "audioresample", "volume", "autoaudiosink"}

// audioCaps is the media type prefix of a pad the audio branch is built for.
const audioCaps = "audio/"

// onDecodePad builds the audio branch when the decoder exposes an audio pad.
// Elements are added to the already-playing pipeline and synced to its state
// before the pad links, the standard dynamic-pad dance. A failure only costs the
// sound: the video branch is untouched, so it is logged rather than failing the
// tile.
func (r *receiver) onDecodePad(pad gst.Pad, onAudio func()) {
	if !isAudioPad(pad) {
		return
	}
	r.mu.Lock()
	already := r.volume != nil
	r.mu.Unlock()
	if already {
		// decodebin exposes one pad per elementary stream, and only the first
		// audio track is played: a second one would mix into the first.
		logger.Debugf("stream %q already has an audio branch", r.name)
		return
	}

	branch, ok := r.buildAudioChain()
	if !ok {
		return
	}
	if ret := pad.Link(branch[0].GetStaticPad("sink")); ret != gst.PadLinkOK {
		logger.Warnf("stream %q audio pad link failed (%d), audio stays off", r.name, ret)
		return
	}

	r.mu.Lock()
	r.volume = branch[slices.Index(audioChain, "volume")]
	r.audioConvert = branch[slices.Index(audioChain, "audioconvert")]
	r.mu.Unlock()
	assert.IsNotNil(r.volume, "the audio chain carries a volume element")

	logger.Infof("stream %q carries audio", r.name)
	if onAudio != nil {
		onAudio()
	}
}

// isAudioPad reports whether a pad carries audio, off the caps it exposes. A pad
// that has not negotiated yet is not one.
func isAudioPad(pad gst.Pad) bool {
	caps := pad.GetCurrentCaps()
	if caps == nil || caps.GetSize() == 0 {
		return false
	}
	return strings.HasPrefix(caps.GetStructure(0).GetName(), audioCaps)
}

// buildAudioChain makes, adds and links the audio elements, and reports false
// when the build failed at any step. Every element is named after its factory
// with an "a" prefix, so an audio queue is told apart from the video branch's.
func (r *receiver) buildAudioChain() ([]gst.Element, bool) {
	branch := make([]gst.Element, 0, len(audioChain))
	for _, factory := range audioChain {
		e := gst.ElementFactoryMake(factory, "a"+factory)
		if e == nil {
			logger.Warnf("stream %q has no %s element, audio stays off", r.name, factory)
			return nil, false
		}
		branch = append(branch, e)
	}
	for _, e := range branch {
		r.pipeline.Add(e)
	}
	for i := range branch[:len(branch)-1] {
		if !branch[i].Link(branch[i+1]) {
			logger.Warnf("stream %q audio branch link failed at %s, audio stays off", r.name, audioChain[i])
			return nil, false
		}
	}
	for _, e := range branch {
		e.SyncStateWithParent()
	}
	return branch, true
}
