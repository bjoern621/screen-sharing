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

// audioChain is what an audio pad is fed into:
//
//	queue ! audioconvert ! level ! audioresample ! volume ! autoaudiosink
//
// The queue holds audio off the decode thread, as the video branch's does.
// Two elements are held onto: SetAudio drives volume, and audioconvert's input caps are the raw
// audio the receive state reports, answering to what the video branch reads off videoconvert.
//
// level ahead of volume rather than behind it is what makes the order worth stating. Behind, a
// muted stream would meter silent, leaving a reader who muted one unable to see it start making
// noise again, which is the question a meter beside a mute button answers.
// So the measurement is of what the stream carries, never of what the speakers were handed.
//
// A sink of its own ends the branch instead of a trip to the shell.
// Audio is small enough to play where it decodes, and shell and backend share a machine
// (docs/ipc-api.md), so a second channel would cross a process boundary to arrive at the same
// output device.
var audioChain = []string{"queue", "audioconvert", "level", "audioresample", "volume", "autoaudiosink"}

// audioCaps prefixes the media type of a pad this branch is built for.
const audioCaps = "audio/"

// LevelInterval is the level element's posting period, and so the fastest a meter moves.
//
// One constant and not two: the control service ticks its level stream at the rate the element
// posts at (docs/ipc-api.md). A faster tick would resend one measurement and a slower one would
// drop measurements already taken, and a meter is built from neither.
const LevelInterval = time.Second / 15

// levelMessage names the messages the level element posts under.
const levelMessage = "level"

// onDecodePad builds the audio branch where a decoder exposes an audio pad.
// The elements join an already-playing pipeline and sync to its state ahead of the link, the usual
// dynamic-pad order. Sound is all a failure costs, the video branch being untouched, so it is
// logged rather than failing the stream.
func (r *Receiver) onDecodePad(pad gst.Pad, onAudio func()) {
	if !isAudioPad(pad) {
		return
	}
	r.mu.Lock()
	already := r.volume != nil
	r.mu.Unlock()
	if already {
		// decodebin exposes a pad per elementary stream, and the first audio track is the one played:
		// a second would mix into it.
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

	// Whatever loudness was asked for before this branch existed lands here.
	// SetAudio keeps its argument with or without somewhere to write it, so a volume set on a decode
	// that had exposed no audio pad yet is not quietly dropped: an effect whose outcome turns on when
	// it arrived is not one a caller can repeat.
	r.applyAudio()

	logger.Infof("stream %q carries audio", r.name)
	if onAudio != nil {
		onAudio()
	}
}

// onSourcePad gives a track the launch line left unlinked a decoder of its own.
//
// What arrives here is what a source with a pad per track offers past the first, the line having
// placed the picture: an RTSP session's audio track would otherwise be received at the source and
// dropped there. The new decoder's own pads run through onDecodePad, so an audio pad becomes the
// branch whichever decoder produced it, syncing to the state of the pipeline it joins.
//
// Pads the line did link are left alone, which is what makes running this for every pad a source
// exposes safe.
//
// The track is all a failure costs: the picture already decodes through a decoder this never
// touches, so it is logged rather than failing the stream.
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

// SetAudio names how loud the stream plays and whether it plays at all.
//
// The request is held here with or without an audio branch, and written onto one the moment it is
// built. Repeating it and sending it early are equally safe: on a decode that has not negotiated
// its audio, on one that has, and on one already at that loudness, the state left behind is the
// same.
//
// Mute is not a volume of zero, because unmuting returns to the level the reader chose. Two fields
// are what remember it.
func (r *Receiver) SetAudio(volume float64, muted bool) {
	assert.Assert(volume >= 0 && volume <= 1, "volume is a fraction", volume)

	r.mu.Lock()
	r.wantVolume, r.wantMuted = volume, muted
	r.mu.Unlock()

	r.applyAudio()
}

// Audio is the loudness in force, and whether a branch exists to apply it to.
//
// It answers the request rather than the element, and the two cannot part company: applyAudio is
// the sole writer and writes this. An element read back answers nothing while there is no branch,
// which is exactly when a shell still has a slider to draw.
func (r *Receiver) Audio() (volume float64, muted bool, has bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.wantVolume, r.wantMuted, r.volume != nil
}

// applyAudio writes the held loudness onto the branch, and does nothing until there is one.
// Repeatable: values are written, nothing is toggled.
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

// Level is the loudest sample and the power average over the level element's last posted interval,
// in dBFS, and false while the stream carries no audio or has posted nothing.
//
// False and a silent reading are two facts, and they stay two all the way to the tile: no meter
// against an empty one.
func (r *Receiver) Level() (peakDB, rmsDB float64, ok bool) {
	if !r.hasLevel.Load() {
		return 0, 0, false
	}
	return math.Float64frombits(r.peakDB.Load()), math.Float64frombits(r.rmsDB.Load()), true
}

// onLevelMessage takes in one posting from the level element.
//
// An unreadable posting is dropped instead of stored as zero: zero dBFS is full scale, so a failed
// parse written through would draw as the loudest signal the format carries.
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

// loudest is the maximum across the channels of one of the level element's arrays.
//
// A figure per stream and not per channel, a meter being one bar and which side of a stereo pair
// led being nothing a tile asks. The maximum and not the mean, so a signal carried on one channel
// alone still reads as sound.
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

// isAudioPad reads the answer off the pad's caps. An unnegotiated pad carries nothing.
func isAudioPad(pad gst.Pad) bool {
	caps := pad.GetCurrentCaps()
	if caps == nil || caps.GetSize() == 0 {
		return false
	}
	return strings.HasPrefix(caps.GetStructure(0).GetName(), audioCaps)
}

// buildAudioChain makes, adds and links the audio elements, and is false where any step failed.
// Each element takes its factory's name behind an "a", which separates the audio queue from the
// video branch's.
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
	// Posting and its period are set on the level element before the branch plays, which puts the
	// first measurement one interval behind the audio rather than behind whenever something else got
	// round to configuring it.
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
