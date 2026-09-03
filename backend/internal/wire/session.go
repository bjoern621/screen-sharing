package wire

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
	"bjoernblessin.de/screenshare/internal/encoderate"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
)

// Two running-state shapes with no owner below this package.
//
// A publish snapshot is assembled from the process supervisor, the retry timer and the held
// settings.
// A viewer's identity is the pair the watch registry keys on.
// Both are read by the backend that produces them and by the control service that serves them,
// so a home in either would make the other import a package it has no other use for.

// PublishSnapshot is what is publishing, at one instant,
// and the domain side of screenshare.v1.PublishState.
//
// It nests the way that message does, which makes the conversion below total.
// "A running stream has settings", "an attempt belongs to a retry" and "a retry belongs to a stream
// the user has not stopped" are each a nil inner struct here,
// so a snapshot breaking one cannot be built rather than being caught on the way past.
type PublishSnapshot struct {
	// Live is the stream in force, nil while nothing publishes.
	// A pipeline waiting out a retry backoff is live:
	// still the stream the user asked for, and stopped by the same call.
	Live *LiveSnapshot
}

// LiveSnapshot is one stream the user asked for and has not stopped.
type LiveSnapshot struct {
	// Settings are what the running pipeline was built from,
	// so they are what the viewers are watching rather than what a form is showing.
	Settings settings.Settings
	// Pending reports that the settings the backend holds would build a different pipeline.
	Pending bool
	// Retry is a relaunch waiting out a backoff after the pipeline died on its own, nil otherwise.
	Retry *RetrySnapshot
	// Preview is the local decode of this stream, nil where the backend runs none.
	// Nested here because that is its whole lifetime:
	// the pipeline goes up with the publish child and down with it.
	Preview *PreviewSnapshot
	// RateCeilingMbps is the rate this encoder is held to, nil where the encode is bounded by nothing.
	// A reading rather than a settings field:
	// which figure bounds an encode, and whether one does at all,
	// is the mode's and the element's answer (publish.RateCeilingMbps).
	RateCeilingMbps *float64
}

// PreviewSnapshot is what the local preview of the running stream turned out to be.
//
// The publish child copies its already-encoded video to a loopback port and the backend decodes
// what arrives there, so nothing here crossed the relay and nothing here is keyed by a transport.
// Port is what the kernel handed out for this run.
// Everything after it is read off the running pipeline, as ReceiveStream's fields are:
// a chain falls back on a machine that cannot run its elements,
// and a hardware decoder may download its own frames.
type PreviewSnapshot struct {
	Port         int
	Live         bool
	Chain        string
	DecodeMemory string
	RenderMemory string
	Decoder      string
	Hardware     bool
}

// RetrySnapshot is a relaunch waiting out a backoff.
type RetrySnapshot struct {
	// Attempt is the pending one, counting from one,
	// and Budget how many the backend spends before it gives up.
	Attempt, Budget int
	// Cause is this app's statement about what ended the pipeline, nil where nothing here names it.
	// Message is that pipeline's own last words.
	Cause   *screensharev1.Text
	Message string
}

// Publishing reports whether a stream is in force,
// which a pipeline waiting out its backoff still is.
// A method over the nil check rather than a field,
// so the answer cannot fall out of step with the settings beside it the way a stored flag could.
func (p PublishSnapshot) Publishing() bool { return p.Live != nil }

// Retry is the relaunch pending on the live stream,
// nil where nothing publishes and where a live pipeline is carrying frames.
// Saves a caller the two nil checks between it and an attempt count.
func (p PublishSnapshot) Retry() *RetrySnapshot {
	if p.Live == nil {
		return nil
	}
	return p.Live.Retry
}

// StreamRef identifies one open external viewer: a stream received over one transport.
// The stream name alone is not an identity: the relay re-serves each stream on all its listeners,
// and one stream can be watched over several transports at once.
type StreamRef struct{ StreamName, Transport string }

// PublishState carries a publish snapshot across.
//
// Nothing is asserted: every invariant is a nil pointer in the shape read and an absent message
// in the shape written, so no state is left to reject.
// An idle snapshot has no settings to contradict, a live one cannot omit them,
// and an attempt count exists only inside a retry.
func PublishState(p PublishSnapshot) *screensharev1.PublishState {
	if p.Live == nil {
		return &screensharev1.PublishState{}
	}

	// The publish and relay groups, and not the viewer one:
	// the running pipeline was built from those two,
	// and a live state carrying how this machine watches would claim a render chain built it.
	live := &screensharev1.PublishState_Live{
		Publish:         PublishSettings(p.Live.Settings.Publish),
		Relay:           RelaySettings(p.Live.Settings.Relay),
		Pending:         p.Live.Pending,
		RateCeilingMbps: p.Live.RateCeilingMbps,
		StreamName:      p.Live.Settings.StreamName(),
	}
	if r := p.Live.Retry; r != nil {
		live.Retry = &screensharev1.PublishState_Retry{
			Attempt: int32(r.Attempt),
			Budget:  int32(r.Budget),
			Cause:   r.Cause,
			Message: r.Message,
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
// and proto3 presence is what says so.
//
// Dup and Drop cross as duplicated_frames and dropped_frames,
// the contract spelling out what the domain abbreviates:
// frames the encoder repeated to hold the output rate,
// and input frames discarded for arriving faster than it took them.
// The frame counts carry no presence, a run that has encoded no frames having encoded zero of them.
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
	measured(&out.TransitMs, s.TransitMs, s.Missing.TransitMs)
	return out
}

// measured sets one optional figure, and leaves it unset where the sample carries no measurement.
// The destination is a pointer,
// so a figure added to the sample costs one line here rather than a branch.
func measured(dst **float64, value float64, missing bool) {
	if missing {
		return
	}
	v := value
	*dst = &v
}

// RelayStatus carries one relay snapshot across.
//
// An unreachable relay converts to a snapshot and not to a failure:
// an environment condition the screen has to say something about,
// and a failed call would leave the shell nothing to say it with.
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
//
// A roster either names every reader the path counts or names none of them,
// the two sources answering different amounts of one truth:
// the relay's own API reads the count and the roster off one array,
// and the group index answers the count with the roster left at the service
// (internal/relay, Status.FromIndex).
func RelayPath(p relay.Path) *screensharev1.RelayPath {
	assert.Assert(p.Readers >= 0, "a path counts the readers it is serving", p.Readers)
	assert.Assert(len(p.Roster) == p.Readers || len(p.Roster) == 0,
		"a path's roster names every reader it counts or none of them", len(p.Roster), p.Readers)

	roster := make([]*screensharev1.RelayReader, 0, len(p.Roster))
	for _, reader := range p.Roster {
		roster = append(roster, RelayReader(reader))
	}

	return &screensharev1.RelayPath{
		Name:         p.Name,
		OwnName:      p.OwnName,
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
// Every figure crosses through its presence rather than through its value,
// so a leg the relay times no round trip on arrives with rtt_ms absent instead of zero.
// SRT is the only leg it times and states a loss rate on.
// optional pointers on both sides make the copy the pointer itself,
// the one shape that cannot lose the distinction on the way.
//
// The strings are the relay's words rather than this code's, so none of them is asserted:
// a reader that arrived with no type is an Umgebungsfehler,
// and an empty one crosses as empty and reads as unknown,
// which is what an empty format on the path beside it already means
// (docs/development-principles.md, "Contracts").
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

// optional carries a string the relay may not have stated at all.
// Empty is absence and not a measured empty string:
// used on a host:port and on an RFC 3339 timestamp, neither of which has a meaningful empty value.
func optional(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

// ViewerState carries the open external viewers across.
// A nil or empty slice crosses as an empty list,
// which is what "no viewer is open" looks like on the wire.
func ViewerState(refs []StreamRef) *screensharev1.ViewerState {
	out := make([]*screensharev1.StreamRef, 0, len(refs))
	for _, ref := range refs {
		assert.Assert(ref.StreamName != "" && ref.Transport != "",
			"a viewer is identified by a stream and the transport it is received over", ref.StreamName, ref.Transport)
		out = append(out, StreamRefMessage(ref))
	}

	assert.Assert(len(out) == len(refs), "a message per open viewer", len(out), len(refs))
	return &screensharev1.ViewerState{Viewers: out}
}

// ReceiveStream is one running decode at one instant:
// what it is, and what the pipeline behind it turned out to be.
//
// Everything after the first two fields is reported rather than asked for.
// A chain falls back on a machine that cannot run its elements and a hardware decoder may download
// its own frames, so this is a fact about the run and not a copy of the viewer settings it
// was opened on.
type ReceiveStream struct {
	// Stream is the stream name and the leg it is received over,
	// which together are the identity, for the reason StreamRef exists.
	Stream StreamRef
	// Live is whether a decoded frame has left the pipeline.
	// Until it has, nothing downstream of the decoder has negotiated and the fields below are empty.
	Live bool
	// Chain is the render chain the pipeline was built with,
	// which is not always the one the viewer settings asked for.
	Chain string
	// DecodeMemory and RenderMemory are the memory features the decoder's output pad and the sink's
	// input pad carried, read together as the evidence of a download or an upload in between.
	DecodeMemory string
	RenderMemory string
	// Decoder is the element the pipeline picked and Hardware whether it ran on silicon.
	// Hardware says where the decoding ran and nothing about where the frames went afterwards,
	// which is DecodeMemory's answer.
	Decoder  string
	Hardware bool
	// HasAudio is whether the decoder exposed an audio pad and the branch was built.
	// Until it did there is nothing to set a volume on,
	// and a tile draws no meter rather than one that measures nothing.
	HasAudio bool
	// Volume and Muted are the loudness in force,
	// held by the receiver whether or not the branch exists.
	// Reported rather than remembered by whoever set them,
	// which is what lets two shells agree about one decode's loudness.
	Volume float64
	Muted  bool
	// Transfer is the transfer characteristic the decoded frames carry, in GStreamer's spelling,
	// and HDR the verdict on it.
	// The verdict decides whether a viewer is offered anything at all.
	// The characteristic is what a reader is shown.
	Transfer string
	HDR      bool
	// ToneMap is whether the pipeline was built with the rung that rolls an HDR stream down,
	// so it reports what ran and not what was asked for.
	ToneMap bool
	// CanToneMap is whether this machine has an element that rolls the range down,
	// and ToneMapMissing the first one it needs and does not register.
	// The string is empty both where the machine can and where the platform declares no rung at all,
	// and the boolean is what tells the two apart.
	CanToneMap     bool
	ToneMapMissing string
	// Failure is why this decode carries no picture, nil while one is opening.
	// Read beside Live, which separates a decode still connecting from one nothing arrives on.
	Failure *screensharev1.Text
}

// AudioLevel is how loud one decode is at one instant, measured before its volume element,
// so a muted stream still reports what it is carrying.
type AudioLevel struct {
	Stream StreamRef
	// PeakDB is the loudest sample of the interval and RMSDB its power average,
	// in decibels relative to full scale: at most 0, and negative infinity for silence.
	// No floor is applied, a floor being a drawing decision and this a measurement.
	PeakDB float64
	RMSDB  float64
}

// AudioLevels carries one instant's levels across.
// A decode with no audio branch has no entry at all,
// a different fact from a silent one and drawn differently.
func AudioLevels(levels []AudioLevel) *screensharev1.AudioLevels {
	out := make([]*screensharev1.AudioLevel, 0, len(levels))
	for _, l := range levels {
		assert.Assert(l.Stream.StreamName != "" && l.Stream.Transport != "",
			"a level belongs to a decode identified by a stream and a transport",
			l.Stream.StreamName, l.Stream.Transport)
		out = append(out, &screensharev1.AudioLevel{
			Stream: StreamRefMessage(l.Stream),
			PeakDb: l.PeakDB,
			RmsDb:  l.RMSDB,
		})
	}

	assert.Assert(len(out) == len(levels), "a message per metered decode", len(out), len(levels))
	return &screensharev1.AudioLevels{Levels: out}
}

// ReceiveState carries the running decodes across.
// A nil or empty slice crosses as an empty list,
// which is what "nothing is being decoded" looks like on the wire.
func ReceiveState(streams []ReceiveStream) *screensharev1.ReceiveState {
	out := make([]*screensharev1.ReceiveStream, 0, len(streams))
	for _, r := range streams {
		assert.Assert(r.Stream.StreamName != "" && r.Stream.Transport != "",
			"a decode is identified by a stream and the transport it is received over",
			r.Stream.StreamName, r.Stream.Transport)
		out = append(out, &screensharev1.ReceiveStream{
			Stream:         StreamRefMessage(r.Stream),
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
			Failure:        r.Failure,
		})
	}

	assert.Assert(len(out) == len(streams), "a message per running decode", len(out), len(streams))
	return &screensharev1.ReceiveState{Streams: out}
}

// ReceiveStatValue is one counter an element keeps, under the element's own name for it.
// Nothing here is labelled: a key is the element's word, a label is a reader's
// (api/proto/screenshare/v1/text.proto).
type ReceiveStatValue struct {
	Key   string
	Value float64
}

// ReceiveStatGroup is one element's counters.
// Factory names the kind of element that is counting, Element tells two of a kind apart.
type ReceiveStatGroup struct {
	Factory string
	Element string
	Values  []ReceiveStatValue
}

// ReceiveStreamStats is one sample of one running decode.
//
// A sample, where ReceiveStream is a state, which is why one decode has two shapes:
// what a decode is settles when the pipeline negotiates and is announced when it moves,
// and what it is doing has to be read off the running pipeline on a clock.
//
// A figure with no measurement yet is a pointer, "not measured yet" being a state a sample reports:
// a rate is a delta between two samples and the first sample of a run has none behind it,
// and a latency bound or a position exists only once the pipeline can answer for it.
// Absent is not zero, and a zero here would say a decode is receiving nothing.
type ReceiveStreamStats struct {
	Stream StreamRef

	// The encoded stream, off the video decoder's input pad.
	Codec         string
	Profile       string
	Level         string
	VideoBytes    uint64
	VideoFrames   uint64
	Keyframes     uint64
	SinceKeyframe *float64
	VideoMbps     *float64
	VideoFPS      *float64

	// The decoded picture, off the decoder's output pad.
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

	// What decoded it, and what the sink took.
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
	// DiscardedFPS is what the decoder shed to hold the stream on the clock,
	// absent on the first sample of a run as every rate here is.
	DiscardedFPS *float64

	// Timing, off the pipeline's clock and its latency query.
	Live       bool
	LatencyMin *float64
	LatencyMax *float64
	Position   *float64
	Uptime     float64

	// Audio: zero and empty until the branch exists, which reads differently from a quiet stream.
	AudioCodec    string
	AudioDecoder  string
	AudioFormat   string
	AudioRate     int
	AudioChannels int
	AudioBytes    uint64
	AudioKbps     *float64

	Groups []ReceiveStatGroup

	// What the path costs a frame, stage by stage.
	Delay DelayBudget
}

// DelayBudget is what each stage of the path between a screen and a window costs a frame,
// in milliseconds.
//
// A stage nothing measured is absent, every field being a pointer for the reason the figures above
// are: an unmeasured stage and a stage that cost nothing are the two readings this may never mix.
// Which stages a machine can measure follows from where it sits
// (api/proto/screenshare/v1/events.proto, DelayBudget).
type DelayBudget struct {
	Publish *float64
	// Path is the two legs and the relay between them as one measurement, off the clock the publisher
	// wrote into the coded picture.
	Path    *float64
	Receive *float64
	// ReceivePeak is the worst Receive has been for a single frame since the decode started,
	// the one field here that is not a figure over the interval between two samples.
	ReceivePeak *float64
	Present     *float64
	Total       *float64
}

// ReceiveStats carries one sample of every running decode across.
// A nil or empty slice crosses as an empty list,
// which is what a tick with nothing decoding looks like.
func ReceiveStats(streams []ReceiveStreamStats) *screensharev1.ReceiveStats {
	out := make([]*screensharev1.ReceiveStreamStats, 0, len(streams))
	for _, s := range streams {
		assert.Assert(s.Stream.StreamName != "" && s.Stream.Transport != "",
			"a sample belongs to a decode identified by a stream and the transport it is received over",
			s.Stream.StreamName, s.Stream.Transport)

		msg := &screensharev1.ReceiveStreamStats{
			Stream: StreamRefMessage(s.Stream),

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

			Delay: &screensharev1.DelayBudget{
				PublishMs:     s.Delay.Publish,
				PathMs:        s.Delay.Path,
				ReceiveMs:     s.Delay.Receive,
				ReceivePeakMs: s.Delay.ReceivePeak,
				PresentMs:     s.Delay.Present,
				TotalMs:       s.Delay.Total,
			},
		}

		msg.SinceKeyframeSec = s.SinceKeyframe
		msg.VideoMbps = s.VideoMbps
		msg.VideoFps = s.VideoFPS
		msg.RenderFps = s.RenderFPS
		msg.DiscardedFps = s.DiscardedFPS
		msg.LatencyMinMs = s.LatencyMin
		msg.LatencyMaxMs = s.LatencyMax
		msg.PositionSec = s.Position
		msg.AudioKbps = s.AudioKbps

		out = append(out, msg)
	}

	assert.Assert(len(out) == len(streams), "a message per sampled decode", len(out), len(streams))
	return &screensharev1.ReceiveStats{Streams: out}
}

// receiveStatGroups carries the leg's own counters across,
// in the order the pipeline holds the elements.
// A group with no values never reaches here:
// the receiver leaves out an element that answers none of the keys it is read for.
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

// PreviewedMonitor is one monitor the backend is reading into a picture the frame channel can hand
// over.
//
// A much shorter row than ReceiveStream, missing what a decode has and a screen capture does not:
// nothing encoded these frames, so there is no decoder to name,
// and nothing carried them, so there is no leg.
type PreviewedMonitor struct {
	// Monitor is the index the output is enumerated under, the whole identity.
	Monitor int
	// Live is whether a frame has left the pipeline.
	// Until it has, the capture element is still opening the screen.
	Live bool
}

// MonitorPreviewState carries the running monitor previews across.
// A nil or empty slice crosses as an empty list,
// which is what "no screen is being previewed" looks like on the wire.
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

// StreamRefMessage carries one viewer's identity across.
// One function: that identity travels on the state, on the exit event and on the two calls
// that open and close a viewer, and a pair spelled out at each of them is how a name and
// a stream_name end up naming one thing.
func StreamRefMessage(ref StreamRef) *screensharev1.StreamRef {
	return &screensharev1.StreamRef{StreamName: ref.StreamName, Transport: ref.Transport}
}

// StreamRefOf reads a viewer's identity back off the contract.
// A message that arrived without one reads as the zero ref,
// which the control service refuses with INVALID_ARGUMENT rather than acting on.
func StreamRefOf(m *screensharev1.StreamRef) StreamRef {
	return StreamRef{StreamName: m.GetStreamName(), Transport: m.GetTransport()}
}

// TestStreamSlot is one slot of the synthetic set:
// what it publishes, whether a child is filling it and what the last one left behind.
type TestStreamSlot struct {
	// Slot is the position in the set, counting from zero, and Name the relay path it publishes to.
	Slot int
	Name string
	// Running is whether a child is filling the slot, which a slot waiting out a relaunch is not.
	Running bool
	// Attempt is the one the slot is on, counting from one.
	Attempt int
	// Cause is this app's statement about why the slot carries no publisher, nil while one runs.
	// Message is the child's own last words.
	Cause   *screensharev1.Text
	Message string
	// LogPath is that child's whole run log, opened through OpenLog.
	LogPath string
}

// TestStreamState carries the synthetic set across:
// how many publishers are alive, and a row per slot the set holds.
// A nil or empty slice crosses as an empty list,
// which is what an unlaunched set looks like on the wire.
func TestStreamState(running int, slots ...TestStreamSlot) *screensharev1.TestStreamState {
	assert.Assert(running >= 0, "a count of live publishers is not negative", running)

	out := make([]*screensharev1.TestStreamSlot, 0, len(slots))
	for _, s := range slots {
		assert.Assert(s.Slot >= 0, "a synthetic publisher holds a slot in the set", s.Slot)
		assert.Assert(s.Name != "", "a synthetic publisher is carried under a name", s.Slot)
		out = append(out, &screensharev1.TestStreamSlot{
			Slot:    int32(s.Slot),
			Name:    s.Name,
			Running: s.Running,
			Attempt: int32(s.Attempt),
			Cause:   s.Cause,
			Message: s.Message,
			LogPath: s.LogPath,
		})
	}

	assert.Assert(len(out) == len(slots), "a message per slot of the set", len(out), len(slots))
	return &screensharev1.TestStreamState{RunningCount: int32(running), Slots: out}
}

// Member is one member of the group this machine is in.
type Member struct {
	// MemberID derives from a secret only that member holds,
	// which makes it an identity nobody else can state.
	MemberID string
	// DisplayName is what that member calls itself,
	// held by the first claim in the group and never an identity.
	DisplayName string
	// Publishing is whether the relay is carrying a stream from this member,
	// and Self marks this machine's own row.
	Publishing bool
	Self       bool
}

// MembersSnapshot is who this machine shares a group with, at one instant,
// and the domain side of screenshare.v1.MembersState.
//
// The fields travel together, a reader needing them all to draw one thing:
// an empty list under Joined is a group whose members have not been read,
// and the same list without it is a machine in no group at all.
type MembersSnapshot struct {
	Members []Member
	// Refusal is why the group service did not take this machine's presence, nil where it did.
	Refusal *screensharev1.Text
	// Joined is whether this machine is in the group, which quitting keeps and leaving clears.
	Joined bool
	// PublishingUnread is whether a connection list the group service reads Publishing off would not
	// answer, so every member's Publishing is false for want of an answer rather than a stream.
	PublishingUnread bool
}

// MembersState carries the group's membership across.
// A nil or empty slice crosses as an empty list,
// which is what a machine in no group looks like on the wire.
func MembersState(m MembersSnapshot) *screensharev1.MembersState {
	out := make([]*screensharev1.Member, 0, len(m.Members))
	for _, member := range m.Members {
		assert.Assert(member.MemberID != "", "a member of a group is identified by a member id", member.DisplayName)
		out = append(out, &screensharev1.Member{
			MemberId:    member.MemberID,
			DisplayName: member.DisplayName,
			Publishing:  member.Publishing,
			Self:        member.Self,
		})
	}

	assert.Assert(len(out) == len(m.Members), "a message per member of the group", len(out), len(m.Members))
	return &screensharev1.MembersState{
		Members:          out,
		Refusal:          m.Refusal,
		Joined:           m.Joined,
		PublishingUnread: m.PublishingUnread,
	}
}

// EncodeRate carries the probe's bracket across:
// the machine reached LowFps and did not reach HighFps.
func EncodeRate(r encoderate.Rate) *screensharev1.EncodeRate {
	assert.Assert(r.LowFps <= r.HighFps, "the harder content codes no faster than the easier", r.LowFps, r.HighFps)

	return &screensharev1.EncodeRate{
		LowFps:      r.LowFps,
		HighFps:     r.HighFps,
		LowBounded:  r.LowBounded,
		HighBounded: r.HighBounded,
	}
}

// ExitInfo carries how a child process ended across.
// An empty message is a clean exit, so nothing here refuses one.
//
// cause is this app's statement about the ending,
// message the child's own words, which stay raw (api/proto/screenshare/v1/text.proto).
// A nil cause is an ending nothing here names, and the message stands alone.
func ExitInfo(message, logPath string, cause *screensharev1.Text) *screensharev1.ExitInfo {
	return &screensharev1.ExitInfo{Message: message, LogPath: logPath, Cause: cause}
}
