package publish

import (
	"fmt"
	"net"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
)

// The local preview leg: how a publish child hands a copy of the stream it is already
// encoding to a decoder in this process, without the relay ever seeing it
// (docs/viewer-architecture.md, "What the broadcast preview draws").
//
// It exists because the encoder is an external child. Both engines run one - a
// gst-launch-1.0 pipeline or an ffmpeg command - which is what keeps a pipeline that
// dies from taking the backend with it (supervise.go), and there is therefore no
// in-process appsink to attach to the encoder's output. What both engines do have is a
// way to write the encoded stream twice, so the child writes it to the relay and to a
// loopback port, and this process decodes what arrives there.
//
// This file holds both halves of that wire format, for the reason gststats.go holds
// both halves of the progress format: the payloader the child is given and the caps the
// receiving pipeline is built with have to agree on a payload type and an encoding
// name, and two files stating that agreement are two places for it to stop being true.

// previewHost is the address the child sends the copy to and this process receives it
// on. Loopback alone, and not a constant either side may assume beyond that: what
// travels here is the user's screen, and the only peer it is meant for is the process
// that spawned the child.
const previewHost = "127.0.0.1"

// previewPayloadType is the RTP payload type both halves pin.
//
// RTP has no static type for any format here, so every one of them is a dynamic type,
// and which number a dynamic type takes is normally negotiated out of band - in an SDP,
// or in the RTSP or WHEP exchange. This leg has no such exchange and needs none: one
// process writes both ends of it, so the number is stated once and read by both. 96 is
// the first of the dynamic range.
const previewPayloadType = 96

// previewClockRate is the RTP timestamp rate every video payload format here runs at.
// It is 90 kHz for all of them, which is what makes it a constant rather than a column.
const previewClockRate = 90000

// previewCarriage is one bitstream format's local preview leg: how the GStreamer child
// payloads it, how the ffmpeg child does, and what the receiving pipeline is told to
// expect.
//
// A format belongs here when three things hold together, the same three transport.
// Carriage is written against: RTP has a payload format for it, both publish engines
// implement the payloader, and a receiving pipeline has a depayloader that reads it
// back. All five are true here, which is the same reason RTSP is the transport that
// carries the whole codec table - this leg is RTP over UDP with the session exchange
// left out, so it inherits exactly RTP's reach.
//
// A format with no row is a format with no local preview. Nothing else about the
// publish changes for it: the child is launched without the second sink and the state
// reports no preview, which is what the broadcast screen says rather than drawing a
// picture that would never arrive.
type previewCarriage struct {
	// payloader is the GStreamer payloader element and its properties, one argument per
	// token the way every element list in this package is written.
	payloader []string
	// encoding is the RTP encoding name the receiving caps state. It is what the
	// depayloader is autoplugged from, and it is the half of the agreement the child
	// never spells out - both payloaders write the payload format this names and neither
	// announces which one it wrote.
	encoding string
	// draft says the RTP payload format is still an IETF draft, which ffmpeg's RTP muxer
	// refuses to write without compliance loosened. It is the same fact
	// transport.draftRtpFormats carries for the RTSP publish leg, restated here because
	// it is a property of the payload format and this leg writes the same one.
	draft bool
}

// previewCarriages is the local preview leg per bitstream format. The keys are
// capabilities.Codec.Format values, because the payload format follows the bitstream
// and never the encoder that produced it.
//
// config-interval=1 on the two H.26x payloaders repeats the parameter sets in band once
// a second. The receiving side is a udpsrc with no session exchange behind it, so there
// is nowhere for an SPS to travel out of band, and a decoder that joined between two of
// them would otherwise sit on a stream it cannot start. The other three carry their
// sequence headers in the bitstream already.
var previewCarriages = map[string]previewCarriage{
	"h264": {payloader: []string{"rtph264pay", "config-interval=1"}, encoding: "H264"},
	"hevc": {payloader: []string{"rtph265pay", "config-interval=1"}, encoding: "H265"},
	"av1":  {payloader: []string{"rtpav1pay"}, encoding: "AV1", draft: true},
	"vp9":  {payloader: []string{"rtpvp9pay"}, encoding: "VP9", draft: true},
	"vp8":  {payloader: []string{"rtpvp8pay"}, encoding: "VP8"},
}

// PreviewLeg is where a publish child copies its already-encoded video to for the local
// preview. The zero value is a run with no preview, which is what Command renders and
// what a format with no carriage row gets.
type PreviewLeg struct {
	// Port is the loopback UDP port the child sends to. It is the backend's to allocate
	// per run (AllocatePreviewPort) rather than a number this package picks, so two
	// launches in a row cannot land on one socket.
	Port int
}

// Wanted reports whether this run carries a preview leg at all.
func (p PreviewLeg) Wanted() bool { return p.Port > 0 }

// PreviewCarried reports whether the codec's bitstream format has a local preview leg,
// and names the format it asked about so a caller can say which one has none.
func PreviewCarried(codec string) (format string, carried bool) {
	c, ok := capabilities.Get(codec)
	if !ok {
		return "", false
	}
	_, carried = previewCarriages[c.Format]
	return c.Format, carried
}

// AllocatePreviewPort returns a loopback UDP port nothing else holds.
//
// The kernel picks it and this reads it off the socket, which is the same arrangement
// the progress meter's listener has (gststats.go): a number chosen here could collide
// with the run before it, and a run whose preview landed on another run's port would
// decode a picture that is not the one it is publishing.
//
// The socket is closed before the port is handed out, and the window that opens is
// stated rather than hidden. Nothing here can hold it: the receiving pipeline's udpsrc
// binds the port itself and two binds on one datagram socket deliver to whichever of
// them the operating system feels like. What the window costs if it is ever lost is a
// receiver that fails to bind and says so, which is a preview that does not come up
// beside a publish that does - never a stream that fails.
func AllocatePreviewPort() (int, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(previewHost), Port: 0})
	if err != nil {
		return 0, fmt.Errorf("cannot allocate a loopback port for the local preview: %w", err)
	}
	defer conn.Close()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	assert.Assert(ok, "a udp socket has a udp address", conn.LocalAddr().String())
	assert.Assert(addr.Port > 0, "the kernel answers an allocation with a port", addr.Port)
	return addr.Port, nil
}

// PreviewSource returns the launch-line fragment a receiving pipeline decodes the local
// preview from, up to the encoded stream - the same shape transport.GstSource yields for
// a relay decode, so internal/receive is handed one kind of thing whichever side the
// frames came from.
//
// The caps are stated rather than discovered because a udpsrc has nothing to discover
// them from: RTP carries no format announcement, and the exchange that normally supplies
// one is what this leg leaves out. They are the other half of previewCarriages, and a
// depayloader is autoplugged from them by the decodebin the chain already ends the source
// in.
//
// The jitter buffer is one frame's worth rather than the tens of milliseconds a network
// leg is given. Reordering and loss are what a jitter buffer absorbs and neither happens
// on loopback, so what is left for it to do is hold the RTP timestamps against the
// pipeline clock; sized for a network it does not cross, it would spend the preview's
// whole latency budget on jitter that cannot occur.
func PreviewSource(codec string, port int) (string, error) {
	assert.Assert(port > 0, "a preview source names the port it receives on", codec, port)

	c, ok := capabilities.Get(codec)
	if !ok {
		return "", fmt.Errorf("unknown codec %q", codec)
	}
	carriage, ok := previewCarriages[c.Format]
	if !ok {
		return "", fmt.Errorf("%s produces %s, which has no local preview leg", codec, c.Format)
	}

	caps := fmt.Sprintf("application/x-rtp,media=video,clock-rate=%d,encoding-name=%s,payload=%d",
		previewClockRate, carriage.encoding, previewPayloadType)
	return fmt.Sprintf("udpsrc address=%s port=%d caps=%q ! rtpjitterbuffer latency=%d",
		previewHost, port, caps, previewJitterMs), nil
}

// previewJitterMs is the reorder window the preview's jitter buffer holds, in
// milliseconds. See PreviewSource for why it is this small.
const previewJitterMs = 20

// gstPreviewTap returns the branch a GStreamer publish pipeline copies its encoded video
// to, empty for a format with no local preview leg.
//
// The queue leaks downstream and the sink neither synchronizes to the clock nor prerolls,
// which is the same shape the meter's branch has and for the same reason: a branch that
// could backpressure the encode path would be a preview able to stall the stream it is
// previewing. What it costs when this process is slow is preview frames, which is what a
// leaky queue is.
func gstPreviewTap(codec string, preview PreviewLeg) ([]string, error) {
	assert.Assert(preview.Wanted(), "a preview branch is built for a run that wants one", codec)

	c, ok := capabilities.Get(codec)
	if !ok {
		return nil, fmt.Errorf("unknown codec %q", codec)
	}
	carriage, ok := previewCarriages[c.Format]
	if !ok {
		return nil, fmt.Errorf("%s produces %s, which has no local preview leg", codec, c.Format)
	}

	tap := []string{"queue", "max-size-buffers=8", "leaky=downstream", "!"}
	tap = append(tap, carriage.payloader...)
	return append(tap,
		"pt="+strconv.Itoa(previewPayloadType),
		"!", "udpsink",
		"host="+previewHost,
		"port="+strconv.Itoa(preview.Port),
		"sync=false", "async=false",
	), nil
}

// ffmpegPreviewTap returns the second output an ffmpeg publish command copies its
// already-encoded video to, and false for a format with no local preview leg.
//
// select=v is what makes it the video alone. The RTP muxer writes one stream, and the
// preview draws pictures: an audio track on this leg would be a second track nothing
// reads, and the mux would refuse it before it got the chance.
//
// onfail=ignore is what keeps the preview from ever being able to end the stream. Every
// other slave of the tee aborts the whole command when it cannot be opened, which is
// right for the relay leg and would be exactly wrong here - a publish that failed because
// this process could not hold a loopback port is a stream lost to a picture nobody outside
// the window would have seen.
func ffmpegPreviewTap(codec string, preview PreviewLeg) (ffmpeg.Tap, bool) {
	assert.Assert(preview.Wanted(), "a preview output is built for a run that wants one", codec)

	c, ok := capabilities.Get(codec)
	if !ok {
		return ffmpeg.Tap{}, false
	}
	carriage, ok := previewCarriages[c.Format]
	if !ok {
		return ffmpeg.Tap{}, false
	}

	options := []string{
		"select=v",
		"f=rtp",
		"onfail=ignore",
		"payload_type=" + strconv.Itoa(previewPayloadType),
	}
	if carriage.draft {
		options = append(options, "strict=experimental")
	}
	return ffmpeg.Tap{
		Options: options,
		URL:     fmt.Sprintf("rtp://%s:%d", previewHost, preview.Port),
	}, true
}
