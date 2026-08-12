package wire

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
	"bjoernblessin.de/screenshare/internal/encoderate"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The running state has two shapes that no package below this one owns.
//
// A publish snapshot is assembled from the process supervisor, the retry timer and
// the held settings, and a viewer's identity is a pair the watch registry keys on.
// Each is read by the backend that produces it and by the control service that serves
// it, and giving either of those two the struct would make the other depend on a
// package it has no other use for. They live here instead, beside the conversion that
// is the only thing both of them do with them.

// PublishSnapshot is what is publishing, at one instant.
//
// It is the domain side of screenshare.v1.PublishState and nests the same way, which
// is what makes the conversion below total. The three rules the flat form could only
// assert - a running stream has settings, an attempt belongs to a retry, a retry
// belongs to a stream the user has not stopped - are all "the inner struct is nil"
// here, so a snapshot that breaks one cannot be built rather than being caught on the
// way past.
type PublishSnapshot struct {
	// Live is the stream in force, nil while nothing publishes. A pipeline waiting out
	// a retry backoff is live: it is still the stream the user asked for, and the one
	// call that stops a running pipeline stops it too.
	Live *LiveSnapshot
}

// LiveSnapshot is one stream the user asked for and has not stopped.
type LiveSnapshot struct {
	// Settings are what the running pipeline was built from. They are the stream the
	// viewers are watching, which is not necessarily what a form is showing.
	Settings settings.Settings
	// Pending reports that the settings the backend holds would build a different
	// pipeline than this one.
	Pending bool
	// Retry is set while the pipeline died on its own and a relaunch is waiting out a
	// backoff, and nil otherwise.
	Retry *RetrySnapshot
	// Preview is the local decode of this stream, nil where the backend is running
	// none. It is nested here because that is its whole lifetime: the pipeline goes up
	// with the publish child and down with it.
	Preview *PreviewSnapshot
}

// PreviewSnapshot is what the local preview of the running stream turned out to be.
//
// The publish child copies its already-encoded video to a loopback port and the backend
// decodes what arrives there, so nothing here crossed the relay and nothing here is
// keyed by a transport. Port is the number the kernel handed out for this run;
// everything after it is read off the running pipeline, exactly as ReceiveStream's
// fields are, because a chain falls back on a machine that cannot run its elements and a
// hardware decoder may download its own frames.
type PreviewSnapshot struct {
	Port         int
	Live         bool
	Chain        string
	DecodeMemory string
	RenderMemory string
	Decoder      string
	Hardware     bool
}

// RetrySnapshot is a relaunch waiting out a backoff. Attempt is which one is pending,
// counting from one, and Budget how many the backend will spend before it gives up.
type RetrySnapshot struct{ Attempt, Budget int }

// Publishing reports whether a stream is in force, which a pipeline waiting out its
// backoff still is. It is a method over the nil check rather than a field, so that
// "is anything publishing" has one answer and cannot fall out of step with the
// settings beside it the way a stored flag could.
func (p PublishSnapshot) Publishing() bool { return p.Live != nil }

// Retry is the relaunch pending on the live stream, nil where nothing is publishing
// and where a live pipeline is carrying frames. It saves a caller the two nil checks
// that would otherwise stand between it and an attempt count.
func (p PublishSnapshot) Retry() *RetrySnapshot {
	if p.Live == nil {
		return nil
	}
	return p.Live.Retry
}

// WatchKey identifies one open external viewer: a stream received over one
// transport. The stream name alone is not an identity, because the relay re-serves
// each stream on all its listeners and a stream can be watched over several
// transports at once.
type WatchKey struct{ StreamName, Transport string }

// PublishState carries a publish snapshot across.
//
// Nothing is asserted. Every invariant this function used to check is a nil pointer in
// the shape it reads and an absent message in the shape it writes, so there is no
// state left for it to reject: an idle snapshot has no settings to contradict, a live
// one cannot omit them, and an attempt count exists only inside a retry.
func PublishState(p PublishSnapshot) *screensharev1.PublishState {
	if p.Live == nil {
		return &screensharev1.PublishState{}
	}

	// The publish and relay groups, and not the viewer one: the running pipeline was
	// built from those two, and a live state carrying how this machine watches would
	// say it was built from a render chain.
	live := &screensharev1.PublishState_Live{
		Publish: PublishSettings(p.Live.Settings.Publish),
		Relay:   RelaySettings(p.Live.Settings.Relay),
		Pending: p.Live.Pending,
	}
	if r := p.Live.Retry; r != nil {
		live.Retry = &screensharev1.PublishState_Retry{
			Attempt: int32(r.Attempt),
			Budget:  int32(r.Budget),
		}
	}
	if v := p.Live.Preview; v != nil {
		live.Preview = &screensharev1.PublishState_Preview{
			Port:         uint32(v.Port),
			Live:         v.Live,
			Chain:        v.Chain,
			DecodeMemory: v.DecodeMemory,
			RenderMemory: v.RenderMemory,
			Decoder:      v.Decoder,
			Hardware:     v.Hardware,
		}
	}
	return &screensharev1.PublishState{Live: live}
}

// PublishStats carries one encoder progress sample across.
//
// A figure the run has no measurement for is left unset rather than sent as a zero,
// and proto3 presence is what says so. The mapping used to be a table of field names
// checked against the generated descriptor at load time, because a misspelt name would
// have hidden a figure instead of marking it absent with nothing on either side to say
// so; presence removes the spelling from the problem entirely.
//
// Dup and Drop cross as duplicated_frames and dropped_frames: the contract spells out
// what the short domain names abbreviate, and the counts they carry are the same two
// events ffmpeg.Stats defines - frames the encoder repeated to hold the output rate,
// and input frames discarded because they arrived faster than it. Those three counts
// carry no presence, because a run that has encoded no frames has encoded zero of them.
func PublishStats(s ffmpeg.Stats) *screensharev1.PublishStats {
	out := &screensharev1.PublishStats{
		FrameCount:       int64(s.Frame),
		DuplicatedFrames: int64(s.Dup),
		DroppedFrames:    int64(s.Drop),
	}

	measured(&out.Fps, s.Fps, s.Missing.Fps)
	measured(&out.CaptureFps, s.CaptureFps, s.Missing.CaptureFps)
	measured(&out.SizeKib, s.SizeKiB, s.Missing.SizeKiB)
	measured(&out.TimeSec, s.TimeSec, s.Missing.TimeSec)
	measured(&out.Speed, s.Speed, s.Missing.Speed)
	measured(&out.InstMbps, s.InstMbps, s.Missing.InstMbps)
	measured(&out.AvgMbps, s.AvgMbps, s.Missing.AvgMbps)
	return out
}

// measured sets one optional figure, and leaves it unset where the sample carries no
// measurement for it. It takes the destination by pointer so that adding a figure is
// one line at the call site rather than a four-line branch.
func measured(dst **float64, value float64, missing bool) {
	if missing {
		return
	}
	v := value
	*dst = &v
}

// RelayStatus carries one relay snapshot across.
//
// An unreachable relay converts to a snapshot rather than to a failure. It is an
// environment condition the screen has to say something about, and a call that
// failed instead would leave the shell with nothing to say it with.
func RelayStatus(s relay.Status) *screensharev1.RelayStatus {
	paths := make([]*screensharev1.RelayPath, 0, len(s.Paths))
	for _, path := range s.Paths {
		paths = append(paths, RelayPath(path))
	}

	assert.Assert(len(paths) == len(s.Paths), "a snapshot carries a path per path the relay reported", len(paths), len(s.Paths))
	return &screensharev1.RelayStatus{
		Reachable: s.Reachable,
		Error:     s.Error,
		Paths:     paths,
	}
}

// RelayPath carries one live stream across.
func RelayPath(p relay.Path) *screensharev1.RelayPath {
	assert.Assert(p.Readers >= 0, "a path counts the readers it is serving", p.Readers)
	assert.Assert(len(p.Roster) == p.Readers,
		"a path's roster names every reader it counts", len(p.Roster), p.Readers)

	roster := make([]*screensharev1.RelayReader, 0, len(p.Roster))
	for _, reader := range p.Roster {
		roster = append(roster, RelayReader(reader))
	}

	return &screensharev1.RelayPath{
		Name:         p.Name,
		Ready:        p.Ready,
		Tracks:       p.Tracks,
		Format:       p.Format,
		Readers:      int32(p.Readers),
		ReaderRoster: roster,
		InMbps:       p.InMbps,
	}
}

// RelayReader carries one reader across.
//
// Every figure is copied through its presence rather than through its value, so a leg
// that reports no round trip crosses as a message with no rtt_ms and not as one with a
// zero. optional pointers on both sides means the copy is the pointer itself, which is
// the one shape that cannot lose the distinction on the way.
//
// Nothing here asserts on the strings. They are the relay's words rather than this code's,
// and a relay that answered with a reader carrying no type is an Umgebungsfehler the app
// has to survive - an empty one crosses as empty and reads as unknown, which is what an
// empty format on the path beside it already means (docs/development-principles.md,
// "Contracts").
func RelayReader(r relay.Reader) *screensharev1.RelayReader {
	return &screensharev1.RelayReader{
		Type:            r.Type,
		Id:              r.ID,
		Transport:       r.Transport,
		RemoteAddr:      optional(r.RemoteAddr),
		Joined:          optional(r.Joined),
		BytesSent:       r.BytesSent,
		RttMs:           r.RttMs,
		LossPercent:     r.LossPercent,
		PacketsSent:     r.PacketsSent,
		PacketsLost:     r.PacketsLost,
		PacketsDropped:  r.PacketsDropped,
		FramesDiscarded: r.FramesDiscarded,
	}
}

// optional carries a string the relay may not have stated at all. An empty one is
// absence and not a measured empty string: the fields it is used on are an address and a
// timestamp, and neither has a meaningful empty value to tell apart from a missing one.
func optional(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

// ViewerState carries the open external viewers across. A nil or empty slice converts
// to an empty list, which is what "no viewer is open" looks like on the wire.
func ViewerState(keys []WatchKey) *screensharev1.ViewerState {
	out := make([]*screensharev1.WatchKey, 0, len(keys))
	for _, key := range keys {
		assert.Assert(key.StreamName != "" && key.Transport != "",
			"a viewer is identified by a stream and the transport it is received over", key.StreamName, key.Transport)
		out = append(out, WatchKeyMessage(key))
	}

	assert.Assert(len(out) == len(keys), "a key per open viewer", len(out), len(keys))
	return &screensharev1.ViewerState{Viewers: out}
}

// ReceiveStream is one running decode, at one instant: what it is, and what the
// pipeline behind it turned out to be.
//
// Everything after the first two fields is reported rather than asked for. A chain
// falls back on a machine that cannot run its elements and a hardware decoder may
// download its own frames, so what is here is a fact about the run rather than a copy
// of the viewer settings it was opened on.
type ReceiveStream struct {
	// Stream is the stream name and the leg it is received over, which together are
	// the identity, for the reason WatchKey exists.
	Stream WatchKey
	// Live is whether a decoded frame has left the pipeline. Until it has, nothing
	// downstream of the decoder has negotiated and the fields below are empty.
	Live bool
	// Chain is the render chain the pipeline was built with, which is not always the
	// one the viewer settings asked for.
	Chain string
	// DecodeMemory and RenderMemory are the memory features the decoder's output pad
	// and the sink's input pad carried, which together are the evidence of a download
	// or an upload between decode and display.
	DecodeMemory string
	RenderMemory string
	// Decoder is the element the pipeline picked, and Hardware whether it ran on
	// silicon. Hardware says where the decoding ran and nothing about where the frames
	// went afterwards, which DecodeMemory is what answers.
	Decoder  string
	Hardware bool
	// HasAudio is whether the decoder exposed an audio pad and the branch was built.
	// Until it did there is nothing to set a volume on, and a tile draws no meter
	// rather than one that measures nothing.
	HasAudio bool
	// Volume and Muted are the loudness in force, held by the receiver whether or not
	// the branch exists. They are reported rather than remembered by whoever set them,
	// which is what lets two shells agree about one decode's loudness.
	Volume float64
	Muted  bool
	// Transfer is the transfer characteristic the decoded frames carry, as GStreamer
	// names it, and HDR the verdict on it. The verdict decides whether a viewer is
	// offered anything at all; the characteristic is what a reader is shown.
	Transfer string
	HDR      bool
	// ToneMap is whether the pipeline was built with the rung that rolls an HDR stream
	// down, which is what ran rather than what was asked for.
	ToneMap bool
	// CanToneMap is whether this machine has an element that rolls the range down, and
	// ToneMapMissing the first one it needs and does not register. The string is empty
	// both where the machine can and where the platform declares no rung at all, which
	// the boolean is what tells apart.
	CanToneMap     bool
	ToneMapMissing string
}

// AudioLevel is how loud one decode is right now, measured before its volume element
// so that a muted stream still reports what it is carrying.
type AudioLevel struct {
	Stream WatchKey
	// PeakDB is the loudest sample of the interval and RMSDB its power average, in
	// decibels relative to full scale. Silence is negative infinity rather than a
	// floor chosen here, because a floor is a drawing decision and this is a
	// measurement.
	PeakDB float64
	RMSDB  float64
}

// AudioLevels carries one instant's levels across. A decode carrying no audio has no
// entry, which is a different fact from a silent one and is drawn differently.
func AudioLevels(levels []AudioLevel) *screensharev1.AudioLevels {
	out := make([]*screensharev1.AudioLevel, 0, len(levels))
	for _, l := range levels {
		assert.Assert(l.Stream.StreamName != "" && l.Stream.Transport != "",
			"a level belongs to a decode identified by a stream and a transport",
			l.Stream.StreamName, l.Stream.Transport)
		out = append(out, &screensharev1.AudioLevel{
			Stream: WatchKeyMessage(l.Stream),
			PeakDb: l.PeakDB,
			RmsDb:  l.RMSDB,
		})
	}

	assert.Assert(len(out) == len(levels), "a message per metered decode", len(out), len(levels))
	return &screensharev1.AudioLevels{Levels: out}
}

// ReceiveState carries the running decodes across. A nil or empty slice converts to an
// empty list, which is what "nothing is being decoded" looks like on the wire.
func ReceiveState(streams []ReceiveStream) *screensharev1.ReceiveState {
	out := make([]*screensharev1.ReceiveStream, 0, len(streams))
	for _, r := range streams {
		assert.Assert(r.Stream.StreamName != "" && r.Stream.Transport != "",
			"a decode is identified by a stream and the transport it is received over",
			r.Stream.StreamName, r.Stream.Transport)
		out = append(out, &screensharev1.ReceiveStream{
			Stream:         WatchKeyMessage(r.Stream),
			Live:           r.Live,
			Chain:          r.Chain,
			DecodeMemory:   r.DecodeMemory,
			RenderMemory:   r.RenderMemory,
			Decoder:        r.Decoder,
			Hardware:       r.Hardware,
			HasAudio:       r.HasAudio,
			Volume:         r.Volume,
			Muted:          r.Muted,
			Transfer:       r.Transfer,
			Hdr:            r.HDR,
			ToneMap:        r.ToneMap,
			CanToneMap:     r.CanToneMap,
			ToneMapMissing: r.ToneMapMissing,
		})
	}

	assert.Assert(len(out) == len(streams), "a message per running decode", len(out), len(streams))
	return &screensharev1.ReceiveState{Streams: out}
}

// ReceiveStatValue is one counter an element keeps, under the element's own name for
// it. Nothing here is labelled: a key is the element's word and a label is a reader's
// (api/proto/screenshare/v1/text.proto).
type ReceiveStatValue struct {
	Key   string
	Value float64
}

// ReceiveStatGroup is one element's counters, with the factory saying what kind of
// element is counting and the name telling two of a kind apart.
type ReceiveStatGroup struct {
	Factory string
	Element string
	Values  []ReceiveStatValue
}

// ReceiveStreamStats is one sample of one running decode.
//
// It is a sample and ReceiveStream is a state, which is why they are two shapes for
// one decode. What a decode is settles when the pipeline negotiates and is announced
// when it moves; what it is doing has to be read off the running pipeline on a clock.
//
// The three rates are pointers because "not measured yet" is a state a sample has to
// be able to report: they are deltas between two samples, and the first sample of a run
// has no previous one to subtract. Absent is not zero, and a zero here would say a
// decode is receiving nothing.
type ReceiveStreamStats struct {
	Stream WatchKey

	// What arrives, off the video decoder's input pad.
	Codec         string
	Profile       string
	Level         string
	VideoBytes    uint64
	VideoFrames   uint64
	Keyframes     uint64
	SinceKeyframe *float64
	VideoMbps     *float64
	VideoFPS      *float64

	// The decoded picture, off the video decoder's output pad.
	Width          int
	Height         int
	PixelFormat    string
	Depth          int
	Subsampling    string
	Colorimetry    string
	Transfer       string
	ChromaSite     string
	PixelAspect    string
	Interlace      string
	FPSNum, FPSDen int

	// Decode and render.
	Decoder           string
	Hardware          bool
	DecodeMemory      string
	RenderMemory      string
	Chain             string
	ToneMap           bool
	RenderFormat      string
	RenderColorimetry string
	RenderWidth       int
	RenderHeight      int
	Frames            uint64
	Rendered          uint64
	Dropped           uint64
	RenderFPS         *float64

	// Timing.
	Live       bool
	LatencyMin *float64
	LatencyMax *float64
	Position   *float64
	Uptime     float64

	// Audio, empty until the branch is built.
	AudioCodec    string
	AudioDecoder  string
	AudioFormat   string
	AudioRate     int
	AudioChannels int
	AudioBytes    uint64
	AudioKbps     *float64

	Groups []ReceiveStatGroup
}

// ReceiveStats carries one sample of every running decode across. A nil or empty slice
// converts to an empty list, which is what a tick with nothing decoding looks like.
func ReceiveStats(streams []ReceiveStreamStats) *screensharev1.ReceiveStats {
	out := make([]*screensharev1.ReceiveStreamStats, 0, len(streams))
	for _, s := range streams {
		assert.Assert(s.Stream.StreamName != "" && s.Stream.Transport != "",
			"a sample belongs to a decode identified by a stream and the transport it is received over",
			s.Stream.StreamName, s.Stream.Transport)

		msg := &screensharev1.ReceiveStreamStats{
			Stream: WatchKeyMessage(s.Stream),

			CodecDescription: s.Codec,
			Profile:          s.Profile,
			Level:            s.Level,
			VideoBytes:       s.VideoBytes,
			VideoFrames:      s.VideoFrames,
			Keyframes:        s.Keyframes,

			Width:       int32(s.Width),
			Height:      int32(s.Height),
			PixelFormat: s.PixelFormat,
			Depth:       int32(s.Depth),
			Subsampling: s.Subsampling,
			Colorimetry: s.Colorimetry,
			Transfer:    s.Transfer,
			ChromaSite:  s.ChromaSite,
			PixelAspect: s.PixelAspect,
			Interlace:   s.Interlace,
			FpsNum:      int32(s.FPSNum),
			FpsDen:      int32(s.FPSDen),

			Decoder:           s.Decoder,
			Hardware:          s.Hardware,
			DecodeMemory:      s.DecodeMemory,
			RenderMemory:      s.RenderMemory,
			Chain:             s.Chain,
			ToneMap:           s.ToneMap,
			RenderFormat:      s.RenderFormat,
			RenderColorimetry: s.RenderColorimetry,
			RenderWidth:       int32(s.RenderWidth),
			RenderHeight:      int32(s.RenderHeight),
			Frames:            s.Frames,
			Rendered:          s.Rendered,
			Dropped:           s.Dropped,

			Live:      s.Live,
			UptimeSec: s.Uptime,

			AudioCodecDescription: s.AudioCodec,
			AudioDecoder:          s.AudioDecoder,
			AudioFormat:           s.AudioFormat,
			AudioRate:             int32(s.AudioRate),
			AudioChannels:         int32(s.AudioChannels),
			AudioBytes:            s.AudioBytes,

			Groups: receiveStatGroups(s.Groups),
		}

		msg.SinceKeyframeSec = s.SinceKeyframe
		msg.VideoMbps = s.VideoMbps
		msg.VideoFps = s.VideoFPS
		msg.RenderFps = s.RenderFPS
		msg.LatencyMinMs = s.LatencyMin
		msg.LatencyMaxMs = s.LatencyMax
		msg.PositionSec = s.Position
		msg.AudioKbps = s.AudioKbps

		out = append(out, msg)
	}

	assert.Assert(len(out) == len(streams), "a message per sampled decode", len(out), len(streams))
	return &screensharev1.ReceiveStats{Streams: out}
}

// receiveStatGroups carries the transport's own counters across, in the order the
// pipeline holds the elements. A group with no values never reaches here: the receiver
// leaves out an element that answers none of the keys it is read for.
func receiveStatGroups(groups []ReceiveStatGroup) []*screensharev1.ReceiveStatGroup {
	out := make([]*screensharev1.ReceiveStatGroup, 0, len(groups))
	for _, g := range groups {
		assert.Assert(g.Factory != "" && len(g.Values) > 0,
			"a reported group names its factory and carries at least one counter", g.Factory, len(g.Values))

		values := make([]*screensharev1.ReceiveStatValue, 0, len(g.Values))
		for _, v := range g.Values {
			assert.Assert(v.Key != "", "a counter carries the element's own name for it", g.Element)
			values = append(values, &screensharev1.ReceiveStatValue{Key: v.Key, Value: v.Value})
		}

		out = append(out, &screensharev1.ReceiveStatGroup{
			Factory: g.Factory,
			Element: g.Element,
			Values:  values,
		})
	}

	assert.Assert(len(out) == len(groups), "a message per counting element", len(out), len(groups))
	return out
}

// PreviewedMonitor is one monitor the backend is reading into a picture the frame
// channel can hand over.
//
// It is a much shorter row than ReceiveStream, and what is missing is what a decode has
// and a screen capture does not: nothing encoded these frames, so there is no decoder to
// name, and nothing carried them, so there is no leg.
type PreviewedMonitor struct {
	// Monitor is the index the output is enumerated under, which is the whole identity.
	Monitor int
	// Live is whether a frame has left the pipeline. Until it has, the capture element
	// is still opening the screen.
	Live bool
}

// MonitorPreviewState carries the running monitor previews across. A nil or empty slice
// converts to an empty list, which is what "no screen is being previewed" looks like on
// the wire.
func MonitorPreviewState(monitors []PreviewedMonitor) *screensharev1.MonitorPreviewState {
	out := make([]*screensharev1.PreviewedMonitor, 0, len(monitors))
	for _, m := range monitors {
		out = append(out, &screensharev1.PreviewedMonitor{
			Monitor: int32(m.Monitor),
			Live:    m.Live,
		})
	}

	assert.Assert(len(out) == len(monitors), "a message per running preview", len(out), len(monitors))
	return &screensharev1.MonitorPreviewState{Monitors: out}
}

// WatchKeyMessage carries one viewer's identity across. It is one function because the
// identity travels on four surfaces - the state, the exit event, and the two calls that
// open and close a viewer - and a pair spelled out at each of them is how a name and a
// stream_name end up naming one thing.
func WatchKeyMessage(key WatchKey) *screensharev1.WatchKey {
	return &screensharev1.WatchKey{StreamName: key.StreamName, Transport: key.Transport}
}

// WatchKeyOf reads a viewer's identity back off the contract. A message that arrived
// without one converts to the zero key, which the control service refuses with
// INVALID_ARGUMENT rather than acting on.
func WatchKeyOf(m *screensharev1.WatchKey) WatchKey {
	return WatchKey{StreamName: m.GetStreamName(), Transport: m.GetTransport()}
}

// TestStreamState carries the count of live synthetic publishers across.
func TestStreamState(running int) *screensharev1.TestStreamState {
	assert.Assert(running >= 0, "a count of live publishers is not negative", running)
	return &screensharev1.TestStreamState{RunningCount: int32(running)}
}

// EncodeRate carries the encode-capacity bracket across.
func EncodeRate(r encoderate.Rate) *screensharev1.EncodeRate {
	assert.Assert(r.LowFps <= r.HighFps, "the harder content codes no faster than the easier", r.LowFps, r.HighFps)

	return &screensharev1.EncodeRate{
		LowFps:      r.LowFps,
		HighFps:     r.HighFps,
		LowBounded:  r.LowBounded,
		HighBounded: r.HighBounded,
	}
}

// ExitInfo carries how a child process ended across. An empty message is a clean
// exit, which is why nothing here refuses one.
func ExitInfo(message, logPath string) *screensharev1.ExitInfo {
	return &screensharev1.ExitInfo{Message: message, LogPath: logPath}
}
