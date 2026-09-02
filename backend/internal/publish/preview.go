package publish

import (
	"fmt"
	"net"
	"strconv"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
)

// The local preview leg: how a publish child hands a copy of the stream it is already encoding
// to a decoder in this process, without the relay ever seeing it (docs/viewer-architecture.md,
// "What the broadcast preview draws").
//
// The encoder is an external child.
// Both engines run one, a gst-launch-1.0 pipeline or an ffmpeg command, keeping a dying pipeline
// from taking the backend with it (supervise.go), so there is no in-process appsink to attach
// to the encoder's output.
// What both engines do have is a way to write the encoded stream twice, so the child writes it
// to the relay and to a loopback port, and this process decodes what arrives there.
//
// Both halves of that wire format live here, for the reason gststats.go holds both halves
// of the progress format: the payloader the child is given and the caps the receiving pipeline
// is built with have to agree on a payload type and an encoding name, and two files stating
// that agreement are two places for it to stop being true.

// previewHost is the address the child sends the copy to and this process receives it on.
// Loopback alone: what travels here is the user's screen, and its only intended peer is the process
// that spawned the child.
const previewHost = "127.0.0.1"

// previewPayloadType is the RTP payload type both halves pin.
//
// RTP has no static type for any format here, so each is a dynamic type, and which number a dynamic
// type takes is normally negotiated out of band: in an SDP, or in the RTSP or WHEP exchange.
// This leg has no such exchange and needs none, one process writing both of its ends, so the number
// is stated once and read by both.
// 96 is the first of the dynamic range.
const previewPayloadType = 96

// previewClockRate is the RTP timestamp rate every video payload format here runs at.
// 90 kHz for all of them, hence a constant rather than a carriage column.
const previewClockRate = 90000

// previewCarriage is one bitstream format's local preview leg: how the GStreamer child payloads it,
// how the ffmpeg child does, and what the receiving pipeline is told to expect.
//
// A format belongs here when the three facts transport.Carriage is written against hold together:
// RTP has a payload format for it, both publish engines implement the payloader, and a receiving
// pipeline has a depayloader that reads it back.
// Every format the app publishes satisfies them, which is the same reason RTSP carries the whole
// codec table: this leg is RTP over UDP with the session exchange left out, so it inherits exactly
// RTP's reach.
//
// A format with no row is a format with no local preview.
// Nothing else about the publish changes for it: the child is launched without the second sink and
// the state reports no preview, what the broadcast screen says rather than drawing a picture
// that would never arrive.
type previewCarriage struct {
	// payloader is the GStreamer payloader element and its properties, one argument per token the way
	// every element list in this package is written.
	payloader []string
	// encoding is the RTP encoding name the receiving caps state, and what the depayloader
	// is autoplugged from.
	// The half of the agreement the child never spells out: both payloaders write the payload format
	// this names and neither announces which one it wrote.
	encoding string
	// draft says the RTP payload format is an IETF draft, which ffmpeg's RTP muxer refuses to write
	// without compliance loosened.
	// The same fact transport.draftRtpFormats carries for the RTSP publish leg, restated here
	// as a property of the payload format this leg also writes.
	draft bool
}

// previewCarriages is the local preview leg per bitstream format.
// The keys are capabilities.Codec.Format values, because the payload format follows the bitstream,
// so two encoders emitting one format share a row.
//
// config-interval=1 on the H.26x payloaders repeats the parameter sets in band once a second.
// The receiving side is a udpsrc with no session exchange, so nothing carries an SPS out of band,
// and a decoder joining between two of them would sit on a stream it cannot start.
// The rest carry their sequence headers in the bitstream already.
var previewCarriages = map[string]previewCarriage{
	"h264": {payloader: []string{"rtph264pay", "config-interval=1"}, encoding: "H264"},
	"hevc": {payloader: []string{"rtph265pay", "config-interval=1"}, encoding: "H265"},
	"av1":  {payloader: []string{"rtpav1pay"}, encoding: "AV1", draft: true},
	"vp9":  {payloader: []string{"rtpvp9pay"}, encoding: "VP9", draft: true},
	"vp8":  {payloader: []string{"rtpvp8pay"}, encoding: "VP8"},
}

// PreviewLeg is where a publish child copies its already-encoded video to for the local preview.
// The zero value is a run with no preview, what Command renders and what a format with no carriage
// row gets.
type PreviewLeg struct {
	// Port is the loopback UDP port the child sends to.
	// The backend allocates it per run (AllocatePreviewPort) rather than this package picking
	// a number, so two launches in a row cannot land on one socket.
	Port int
}

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
// The kernel picks it and this reads it off the socket, the same arrangement the progress meter's
// listener has (gststats.go): a number chosen here could collide with the run before it, and a run
// whose preview landed on another run's port would decode a picture it is not publishing.
//
// The socket closes before the port is handed out, and the window that opens is stated rather than
// hidden.
// Nothing here can hold it: the receiving pipeline's udpsrc binds the port itself, and two binds
// on one datagram socket deliver to whichever of them the operating system feels like.
// Losing that race costs a receiver that fails to bind and says so,
// so the publish comes up while the preview beside it does not.
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

// PreviewSource returns the launch-line fragment a receiving pipeline decodes the local preview
// from, up to the encoded stream.
// The same shape transport.GstSource yields for a relay decode, so internal/receive is handed one
// kind of thing whichever side the frames came from.
//
// The caps are stated rather than discovered because a udpsrc has nothing to discover them from:
// RTP carries no format announcement, and the exchange that normally supplies one is what this leg
// leaves out.
// They are the other half of previewCarriages, and a depayloader is autoplugged from them
// by the decodebin the chain already ends the source in.
//
// The jitter buffer holds one frame rather than the tens of milliseconds a network leg is given.
// Reordering and loss are what a jitter buffer absorbs and neither happens on loopback, so what
// is left for it to do is hold the RTP timestamps against the pipeline clock.
// Sized for a network it does not cross, it would spend the preview's whole latency budget
// on jitter that cannot occur.
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

// previewJitterMs is the reorder window the preview's jitter buffer holds, in milliseconds.
// PreviewSource says why it is this small.
const previewJitterMs = 20

// gstPreviewTap returns the branch a GStreamer publish pipeline copies its encoded video to,
// empty for a format with no local preview leg.
//
// The queue leaks downstream and the sink neither synchronizes to the clock nor prerolls, the same
// shape the meter's branch has and for the same reason: a branch able to backpressure the encode
// path is a preview able to stall the stream it previews.
// What a slow process costs here is preview frames, what a leaky queue drops.
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

// ffmpegPreviewTap returns the second output an ffmpeg publish command copies its already-encoded
// video to, and false for a format with no local preview leg.
//
// select=v keeps it to the video alone.
// The RTP muxer writes one stream and the preview draws pictures, so an audio track on this leg
// would be a track nothing reads, and the mux would refuse it before it got the chance.
//
// onfail=ignore keeps the preview from ending the stream.
// Every other slave of the tee aborts the whole command when it cannot be opened, right
// for the relay leg and wrong here.
// A publish that fails because this process could not hold a loopback port loses the stream, and
// loses it for a picture nobody outside the window would have seen.
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
