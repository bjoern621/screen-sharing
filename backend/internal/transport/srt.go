package transport

import (
	"bjoernblessin.de/go-utils/util/assert"

	"errors"
	"fmt"
	"net/url"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// SRT streams through the relay's SRT listener, MPEG-TS inside a retransmit window.
// Every constant here is measured, and changing one takes a measurement:
//   - ffmpeg's srt protocol counts latency in MICROSECONDS, not milliseconds.
//   - sndbuf, rcvbuf and ffs are large so a lossless keyframe burst survives while a display-paced
//     player drains slowly.
//   - pkt_size 1316 is 7 MPEG-TS packets per SRT datagram.
//
// A latency set here is what this end asks for, not what the link runs at.
// The handshake negotiates one delay per direction and the larger of the two ends' values wins, so
// the relay's own figure is a floor under every hop.
// MediaMTX exposes no SRT latency option and runs on its library's 120 ms default, so a window
// below 120 ms comes back as 120 and only one above it changes anything.
type SRT struct{}

func init() {
	Register(SRT{})
}

// srtBufBytes sizes the SRT window and the socket buffers, 150 MB.
const srtBufBytes = 150_000_000

func (SRT) Name() string { return "srt" }

// srtCarriage is what MediaMTX registers an MPEG-TS stream type for: H.264 and H.265, with Opus and
// AAC beside them.
// AV1, VP9 and VP8 have no mapping there, so a stream in one of them reaches no SRT publish and
// comes out of no SRT read, whatever leg the relay ingested it over.
//
// One value covers all four engine legs, MPEG-TS being where the two engines meet: ffmpeg's mpegts
// muxer and mpegtsmux write the same stream types, and libavformat and tsdemux read them back.
// Naming it once makes that agreement a statement rather than four lists that happen to match.
var srtCarriage = Carriage{
	Video: []string{"h264", "hevc"},
	Audio: []string{"opus", "aac"},
}

var srtFormats = Formats{
	Publish: map[string]Carriage{
		capabilities.EngineFfmpeg: srtCarriage,
		capabilities.EngineGst:    srtCarriage,
	},
	Watch: map[string]Carriage{
		capabilities.EngineFfmpeg: srtCarriage,
		capabilities.EngineGst:    srtCarriage,
	},
}

func (SRT) Formats() Formats { return srtFormats }

// ListenerURL is the relay's SRT listener, "srt://relay:8890".
// No TLS on this leg at all: it is UDP, and what protects it is the relay-wide passphrase
// (deploy/mediamtx-groups.yml).
func (SRT) ListenerURL(s settings.Settings) string {
	return fmt.Sprintf("srt://%s:%d", s.Relay.Host, s.Relay.SrtPort)
}

func (SRT) PublishArgs(s settings.Settings) []string {
	// ffmpeg's srt protocol takes every knob as a URL query: latency in MICROSECONDS, and the buffers
	// under ffmpeg's own names (pkt_size, sndbuf, ffs).
	url := SRT{}.ListenerURL(s) + fmt.Sprintf(
		"?streamid=%s&pkt_size=1316&latency=%d&sndbuf=%d&ffs=%d",
		srtStreamID(s, "publish", s.Relay.Path(s.Publish.Name)),
		s.Publish.SrtPublishLatencyMs*1000, srtBufBytes, srtBufBytes) + srtPassphraseQuery(s)

	return []string{"-f", "mpegts", url}
}

// GstSink configures srtsink the way libsrt takes it, which is not the way ffmpeg's srt protocol
// does: the URI is a bare srt://host:port, and streamid and latency are properties, latency in
// MILLISECONDS rather than microseconds.
// alignment=7 packs 7 * 188-byte TS packets per buffer, matching the SRT payload size.
func (SRT) GstSink(s settings.Settings) []string {
	return append([]string{
		"mpegtsmux", "name=" + GstMuxName, "alignment=7",
		"!", "srtsink",
		"uri=" + SRT{}.ListenerURL(s),
		"mode=caller",
		"streamid=" + srtStreamID(s, "publish", s.Relay.Path(s.Publish.Name)),
		fmt.Sprintf("latency=%d", s.Publish.SrtPublishLatencyMs),
		"wait-for-connection=false",
	}, srtPassphraseProperty(s)...)
}

func (SRT) WatchURL(s settings.Settings, streamName string) string {
	assert.Assert(streamName != "", "a watch URL names the stream it opens")

	return SRT{}.ListenerURL(s) + fmt.Sprintf(
		"?streamid=%s&latency=%d&rcvbuf=%d&ffs=%d",
		srtStreamID(s, "read", streamName),
		s.Viewer.SrtWatchLatencyMs*1000, srtBufBytes, srtBufBytes) + srtPassphraseQuery(s)
}

// GstSource splits as GstSink does: srtsrc takes streamid and latency (milliseconds) as properties
// on a bare srt:// URI.
// The buffer options WatchURL carries are ffmpeg protocol knobs with no srtsrc equivalent.
func (SRT) GstSource(s settings.Settings, streamName string) []string {
	assert.Assert(streamName != "", "a receive source names the stream it decodes")

	return append([]string{
		"srtsrc",
		"uri=" + SRT{}.ListenerURL(s),
		"mode=caller",
		"streamid=" + srtStreamID(s, "read", streamName),
		fmt.Sprintf("latency=%d", s.Viewer.SrtWatchLatencyMs),
	}, srtPassphraseProperty(s)...)
}

// srtWatchKnobs are the knobs a viewer changes per stream, the settings fields GstSource and
// WatchURL read.
var srtWatchKnobs = []watchKnob{
	intKnob("srtWatchLatencyMs", "SRT latency (ms)",
		"Retransmit window of the watch leg (relay to viewer), where internet loss usually lives. "+
			"It is display delay: a lossy remote link wants more, a LAN less. "+
			"SRT negotiates the larger of the two ends' windows, and MediaMTX asks for 120 ms, so anything below that is raised to it.",
		minWatchLatencyMs,
		func(s *settings.Settings) *int { return &s.Viewer.SrtWatchLatencyMs }),
}

func (SRT) WatchOptions(s settings.Settings) []WatchOption { return knobOptions(srtWatchKnobs, s) }

// ValidatePublishSettings refuses a stream id that would not survive the wire, and a stream that
// would leave this machine unencrypted.
//
// The id carries the path and the token, and SRT truncates at srtStreamIDBytes rather than
// refusing, so a cut token reaches the relay as a signature error naming nothing a user can act on.
// A settings problem with a settings fix, caught here and named as one.
//
// The passphrase is what encrypts SRT, there being no TLS on it: it is UDP, and the reverse proxy
// that wraps every HTTP leg of an encrypted relay never sees this one.
// So a relay reached over TLS and an empty passphrase is a stream that crosses the internet in the
// clear, which is refused here rather than sent.
func (SRT) ValidatePublishSettings(s settings.Settings) error {
	id := srtStreamID(s, "publish", s.Relay.Path(s.Publish.Name))
	if !srtStreamIDFits(id) {
		return fmt.Errorf("the SRT stream id is %d bytes and the protocol carries %d: shorten the stream name by %d characters",
			len(id), srtStreamIDBytes, len(id)-srtStreamIDBytes)
	}
	if s.Relay.Tls() && s.Relay.SrtPassphrase == "" {
		return errors.New("SRT is UDP and carries no TLS, so the relay's SRT passphrase is what encrypts it, and this relay has none set")
	}
	return nil
}

func (t SRT) SetWatchOption(s *settings.Settings, key, value string) error {
	return knobSet(t.Name(), srtWatchKnobs, s, key, value)
}

// srtPassphraseQuery is the passphrase as ffmpeg's srt protocol takes it, empty where the relay is
// keyed with none.
//
// Both legs carry it because the relay keys both, pathDefaults holding a publish value and a read
// value that an operator sets alike.
// A passphrase on one side only is a stream that connects and never plays.
func srtPassphraseQuery(s settings.Settings) string {
	if s.Relay.SrtPassphrase == "" {
		return ""
	}
	return "&passphrase=" + url.QueryEscape(s.Relay.SrtPassphrase)
}

// srtPassphraseProperty is the same value as srtsink and srtsrc take it, a property rather than a
// URI query, which is the split the latency and the stream id already have between the two engines.
func srtPassphraseProperty(s settings.Settings) []string {
	if s.Relay.SrtPassphrase == "" {
		return nil
	}
	return []string{"passphrase=" + s.Relay.SrtPassphrase}
}
