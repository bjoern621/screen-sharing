package receive

import (
	"math"
	"slices"
	"strings"
	"time"

	"github.com/go-gst/go-glib/pkg/gobject/v2"
	"github.com/go-gst/go-gst/pkg/gst"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// audioChain is the branch an audio pad feeds:
//
//	queue ! audioconvert ! level ! audioresample ! volume ! autoaudiosink
//
// The queue decouples audio from the decode thread like the video branch's does.
// Two of the elements are kept: volume drives SetAudio, and audioconvert's input
// caps are the raw audio the receive state reports, the counterpart of what the
// video branch reads off videoconvert.
//
// level sits before volume and not after it, which is the whole reason its position
// is worth stating. Measured after the volume element, a muted stream would meter as
// silent, and a reader who muted one stream could not see that it had started making
// noise again - which is the question a meter beside a mute button exists to answer.
// What it measures is therefore what the stream is carrying, never what the speakers
// were given.
//
// The branch ends in a sink of its own rather than travelling to the shell.
// Audio is small enough to play where it is decoded, and the backend runs on the
// machine the shell is on (docs/ipc-api.md), so a second channel would carry the
// samples across a process boundary to reach the same output device.
var audioChain = []string{"queue", "audioconvert", "level", "audioresample", "volume", "autoaudiosink"}

// audioCaps is the media type prefix of a pad the audio branch is built for.
const audioCaps = "audio/"

// LevelInterval is how often the level element posts a measurement, and so how fast
// a meter can move.
//
// One constant rather than two, because the control service ticks its level stream at
// the same rate it is posted at (docs/ipc-api.md). A cadence faster than this would
// send the same measurement twice and a slower one would drop measurements that were
// taken, and neither is a thing a meter should be made of.
const LevelInterval = time.Second / 15

// levelMessage is the name the level element posts its measurements under.
const levelMessage = "level"

// onDecodePad builds the audio branch when the decoder exposes an audio pad.
// Elements are added to the already-playing pipeline and synced to its state
// before the pad links, the standard dynamic-pad dance. A failure only costs the
// sound: the video branch is untouched, so it is logged rather than failing the
// stream.
func (r *Receiver) onDecodePad(pad gst.Pad, onAudio func()) {
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

	// The loudness a caller asked for before this branch existed is written now.
	// SetAudio holds what it was asked for whether or not there is anything to write
	// it onto, so that a volume set on a decode that had not yet exposed an audio pad
	// is not silently lost - an effect whose result depends on when it arrived is not
	// one a caller can repeat.
	r.applyAudio()

	logger.Infof("stream %q carries audio", r.name)
	if onAudio != nil {
		onAudio()
	}
}

// onSourcePad decodes a track the launch line left unlinked, into a decoder of its own.
//
// The pads that reach here are the ones a source with a pad per track offers beyond the first: the
// picture is placed by the line and this is what the audio track of an RTSP session would otherwise
// be, received and dropped at the source. What the new decoder exposes goes through onDecodePad,
// so an audio pad becomes the branch whatever decoder produced it, and the state of the pipeline it
// is added to is what it is synced to.
//
// A pad the line did link is left alone, which is what makes this safe to run for every pad the
// source exposes.
//
// A failure costs the track and nothing else: the picture is already decoding through a decoder
// this one never touches, so it is logged rather than failing the stream.
func (r *Receiver) onSourcePad(pad gst.Pad, onAudio func()) {
	if pad.IsLinked() {
		return
	}

	dec := gst.ElementFactoryMake("decodebin", "dec-"+pad.GetName())
	if dec == nil {
		logger.Warnf("stream %q has no decodebin element, the track beside the picture stays off", r.name)
		return
	}
	r.pipeline.Add(dec)
	dec.Connect("pad-added", func(_ gst.Element, decoded gst.Pad) {
		r.onDecodePad(decoded, onAudio)
	})
	dec.SyncStateWithParent()

	if ret := pad.Link(dec.GetStaticPad("sink")); ret != gst.PadLinkOK {
		logger.Warnf("stream %q could not decode the track beside the picture (%d)", r.name, ret)
	}
}

// SetAudio sets how loud the stream plays and whether it plays at all.
//
// What it asks for is held here whether or not the audio branch exists yet, and
// written onto the branch the moment one is built. That is what makes it safe to
// repeat and safe to send early: the same call on a decode that has not negotiated
// its audio, on one that has, and on one that is already at that loudness all leave
// the same state behind.
//
// Mute is separate from a volume of zero because unmuting has to return to the level
// the reader chose. Two fields rather than one is the only way to remember it.
func (r *Receiver) SetAudio(volume float64, muted bool) {
	assert.Assert(volume >= 0 && volume <= 1, "volume is a fraction", volume)

	r.mu.Lock()
	r.wantVolume, r.wantMuted = volume, muted
	r.mu.Unlock()

	r.applyAudio()
}

// Audio is the loudness in force and whether there is a branch to apply it to.
//
// The loudness is what was asked for rather than what the element reports, and the
// two cannot disagree: applyAudio is the only writer, and it writes exactly this.
// Reading the element back would answer nothing while the branch does not exist,
// which is precisely when a shell still has to be able to draw the slider it moved.
func (r *Receiver) Audio() (volume float64, muted bool, has bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.wantVolume, r.wantMuted, r.volume != nil
}

// applyAudio writes the held loudness onto the branch, and does nothing while there
// is no branch. Safe to run twice: it writes values rather than toggling anything.
func (r *Receiver) applyAudio() {
	r.mu.Lock()
	vol, volume, muted := r.volume, r.wantVolume, r.wantMuted
	r.mu.Unlock()

	if vol == nil {
		return
	}
	vol.SetObjectProperty("volume", volume)
	vol.SetObjectProperty("mute", muted)
}

// Level is the loudest sample and the power average of the last interval the level
// element posted, in decibels relative to full scale, and false while the stream
// carries no audio or has posted nothing yet.
//
// False and a silent reading are different facts and stay different all the way to
// the tile: one draws no meter, the other draws an empty one.
func (r *Receiver) Level() (peakDB, rmsDB float64, ok bool) {
	if !r.hasLevel.Load() {
		return 0, 0, false
	}
	return math.Float64frombits(r.peakDB.Load()), math.Float64frombits(r.rmsDB.Load()), true
}

// onLevelMessage records one posting of the level element.
//
// A posting that cannot be read is dropped rather than stored as a zero: zero dBFS
// is full scale, so a failed parse written through would draw as the loudest signal
// the format can carry.
func (r *Receiver) onLevelMessage(msg *gst.Message) {
	st := msg.GetStructure()
	if st == nil {
		return
	}
	peak, ok := loudest(st, "peak")
	if !ok {
		return
	}
	rms, ok := loudest(st, "rms")
	if !ok {
		return
	}

	r.peakDB.Store(math.Float64bits(peak))
	r.rmsDB.Store(math.Float64bits(rms))
	r.hasLevel.Store(true)
}

// loudest is the maximum over the channels of one of the level element's arrays.
//
// One figure per stream rather than one per channel, because a meter is one bar and
// which side of a stereo pair was louder is not a thing a tile asks. The maximum
// rather than the average of the two, so a signal on one channel alone still reads
// as sound.
func loudest(st *gst.Structure, field string) (float64, bool) {
	values, ok := st.GetValue(field).(gobject.ValueArray)
	if !ok || len(values) == 0 {
		return 0, false
	}

	out := math.Inf(-1)
	for _, v := range values {
		db, isFloat := v.(float64)
		if !isFloat {
			return 0, false
		}
		out = math.Max(out, db)
	}
	return out, true
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
func (r *Receiver) buildAudioChain() ([]gst.Element, bool) {
	branch := make([]gst.Element, 0, len(audioChain))
	for _, factory := range audioChain {
		e := gst.ElementFactoryMake(factory, "a"+factory)
		if e == nil {
			logger.Warnf("stream %q has no %s element, audio stays off", r.name, factory)
			return nil, false
		}
		branch = append(branch, e)
	}
	// The level element is told to post and how often before the branch plays, so the
	// first measurement arrives one interval after the audio does rather than after
	// something else has got around to configuring it.
	lvl := branch[slices.Index(audioChain, levelMessage)]
	lvl.SetObjectProperty("post-messages", true)
	lvl.SetObjectProperty("interval", uint64(LevelInterval.Nanoseconds()))

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
