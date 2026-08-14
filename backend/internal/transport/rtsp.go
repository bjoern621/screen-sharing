package transport

import (
	"bjoernblessin.de/go-utils/util/assert"

	"fmt"
	"slices"
	"strings"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/settings"
)

// RTSP streams through the relay's RTSP listener, one RTP stream per track inside the session,
// which is why no MPEG-TS mux appears anywhere here.
//
// Each leg names the RTP lower transport it runs over: settings.Publish RtspPublishProtocol on the
// publish leg, settings.Viewer RtspWatchProtocol on the watch leg.
// Over TCP the tracks are interleaved on the connection the session already holds.
// Over UDP each takes a port pair, which drops the delay in-order delivery adds.
// Lost RTP is never retransmitted either way.
//
// TCP is the default on both legs because of that port pair.
// It is negotiated apart from the RTSP connection, and a client behind NAT reaches it only by
// sending from those ports first: sending creates the mapping, and the relay has to answer into it
// rather than at the port SETUP announced, which is the private one the NAT rewrote.
// The media itself does that sending on the publish leg, and probe packets do it on the watch leg
// before any RTP can come back.
// A network dropping outbound UDP ends both, silently: the control connection sets up over TCP as
// usual and no RTP follows.
type RTSP struct{}

func init() {
	Register(RTSP{})
}

func (RTSP) Name() string { return "rtsp" }

// rtspCarriage is what RTP has a payload format for, which is every format this app encodes and
// every audio codec it offers (docs/domain-model.md).
// The relay ingests and re-serves all of them, so RTSP carries the whole codec table, and is the
// leg the other refusals point at.
//
// All four engine legs share one value, RTP payloading being per format and implemented for every
// one on both engines: ffmpeg's rtp muxer and the rtp*pay elements write the same payload types,
// and their depayloaders read them back.
var rtspCarriage = Carriage{
	Video: []string{"h264", "hevc", "av1", "vp9", "vp8"},
	Audio: []string{"opus", "aac"},
}

func (RTSP) Formats() Formats {
	return Formats{
		Publish: map[string]Carriage{
			capabilities.EngineFfmpeg: rtspCarriage,
			capabilities.EngineGst:    rtspCarriage,
		},
		Watch: map[string]Carriage{
			capabilities.EngineFfmpeg: rtspCarriage,
			capabilities.EngineGst:    rtspCarriage,
		},
	}
}

// draftRtpFormats are the video formats whose RTP payload format is an IETF draft.
// ffmpeg's RTP muxer writes one only with compliance loosened, and otherwise ends the publish on
// "Could not write header" before a frame goes out.
// The relay ingests both regardless, so "-strict experimental" is the whole difference between an
// AV1 or VP9 publish and none.
// Keyed by format rather than by codec, since the payload format follows the bitstream and not the
// encoder that made it.
var draftRtpFormats = map[string]bool{"vp9": true, "av1": true}

// PublishArgs muxes to RTSP, which wraps the RTP muxer, so the draft-payload flag applies here too.
func (RTSP) PublishArgs(s settings.Settings) []string {
	args := []string{"-f", "rtsp", "-rtsp_transport", s.Publish.RtspPublishProtocol}
	if c, ok := capabilities.Get(s.Publish.Codec); ok && draftRtpFormats[c.Format] {
		args = append(args, "-strict", "experimental")
	}
	return append(args, rtspURL(s, s.Relay.Path(s.Publish.Name)))
}

// GstSink is one element: rtspclientsink is muxer and sink at once, payloading every attached
// parsed stream into its own RTP track of the same session.
// It therefore carries GstMuxName, which is where the pipeline's audio branch attaches.
func (RTSP) GstSink(s settings.Settings) []string {
	return []string{
		"rtspclientsink", "name=" + GstMuxName,
		"protocols=" + s.Publish.RtspPublishProtocol,
		"location=" + rtspURL(s, s.Relay.Path(s.Publish.Name)),
	}
}

func (RTSP) WatchURL(s settings.Settings, streamName string) string {
	assert.Assert(streamName != "", "a watch URL names the stream it opens")

	return rtspURL(s, streamName)
}

// GstSource carries both watch-leg knobs: latency sizes the rtpjitterbuffer in milliseconds, and
// protocols is the RTP lower transport rtspsrc offers the relay.
// Neither is left at the element's default, a 2000 ms buffer and a UDP-first negotiation.
//
// A track per RTP stream means a pad per track rather than one muxed pad, and the capsfilter is
// what decides which pad the pipeline's decoder is given.
// Without it that is whichever track the relay announced first: the decoder takes any caps and a
// launch line links the first pad that fits, so a session announcing audio first would decode the
// sound and leave the picture nowhere to go.
// The track left unlinked here is decoded beside the picture (internal/receive), so an audio track
// reaches the branch that plays it.
//
// The credential rides beside the address rather than in it, which is rtspsrc's doing and is stated
// with the pair it builds (credential.go).
func (RTSP) GstSource(s settings.Settings, streamName string) []string {
	assert.Assert(streamName != "", "a receive source names the stream it decodes")

	source := []string{
		"rtspsrc",
		"location=" + rtspAddress(s, streamName),
		"protocols=" + s.Viewer.RtspWatchProtocol,
		fmt.Sprintf("latency=%d", s.Viewer.RtspWatchLatencyMs),
	}
	if user, password, ok := rtspCredential(s); ok {
		source = append(source, "user-id="+user, "user-pw="+password)
	}
	// Last, the receiver linking the fragment's tail to its decoder.
	return append(source, "!", "application/x-rtp,media=video")
}

// RtspProtocols are the RTP lower transports this protocol offers, the values RtspPublishProtocol
// and RtspWatchProtocol both take.
// One list for both legs: which transports carry RTP is a fact of RTSP and not of a direction.
var RtspProtocols = []string{"tcp", "udp"}

// EncryptedRtspProtocol is the one lower transport an encrypted RTSP session carries media on.
//
// RTSPS encrypts the control connection and nothing else: RTP over UDP travels beside it in the
// clear, so a session negotiated that way sends the picture unencrypted however the control channel
// was set up.
// Interleaving puts the RTP inside the TLS connection, which is what makes the media encrypted.
const EncryptedRtspProtocol = "tcp"

// ValidatePublishSettings refuses a lower transport RTSP does not run over, and one that would put
// the media on the wire in the clear.
//
// ffmpeg's -rtsp_transport and rtspclientsink's protocols property take a fixed set of names, so a
// value outside it fails inside the publish process, where the reason reaches the user as another
// program's error text.
//
// The encrypted case is refused rather than corrected: silently interleaving a session the user
// asked to carry over UDP answers a question they did not ask, and the control that says "udp" would
// go on saying it while the stream did something else.
func (RTSP) ValidatePublishSettings(s settings.Settings) error {
	if !slices.Contains(RtspProtocols, s.Publish.RtspPublishProtocol) {
		return fmt.Errorf("rtsp publish protocol %q is not one of %s",
			s.Publish.RtspPublishProtocol, strings.Join(RtspProtocols, ", "))
	}
	if s.Relay.Tls() && s.Publish.RtspPublishProtocol != EncryptedRtspProtocol {
		return fmt.Errorf("an encrypted relay carries RTP inside the RTSP connection, so the lower transport is %s and not %q",
			EncryptedRtspProtocol, s.Publish.RtspPublishProtocol)
	}
	return nil
}

// rtspWatchKnobs are the knobs a viewer changes per stream, the settings fields GstSource reads.
// The publish leg's counterpart is a settings-form field, chosen once for the stream this machine
// sends rather than per stream it receives.
var rtspWatchKnobs = []watchKnob{
	intKnob("rtspWatchLatencyMs", "RTSP jitter buffer (ms)",
		"How long the receiver holds RTP packets before decoding, to reorder them and absorb network jitter. "+
			"It is display delay, so it belongs just above the link's jitter: 200 ms suits a LAN, a lossy remote link wants more.",
		minWatchLatencyMs,
		func(s *settings.Settings) *int { return &s.Viewer.RtspWatchLatencyMs }),
	choiceKnob("rtspWatchProtocol", "RTSP transport",
		"How RTP reaches the viewer inside the RTSP session. UDP takes a port pair per track and trades retransmission for delay, "+
			"but the media travels toward the viewer on it, so nothing arrives until the viewer's own probe packets have opened "+
			"the mapping through its NAT and the relay answers where they came from. "+
			"TCP interleaves both tracks on the RTSP connection, which needs no second port and is what a filtering network is likeliest to pass.",
		RtspProtocols,
		func(s *settings.Settings) *string { return &s.Viewer.RtspWatchProtocol }),
}

func (RTSP) WatchOptions(s settings.Settings) []WatchOption { return knobOptions(rtspWatchKnobs, s) }

func (t RTSP) SetWatchOption(s *settings.Settings, key, value string) error {
	return knobSet(t.Name(), rtspWatchKnobs, s, key, value)
}

// RtspsPort is where a relay's encrypted RTSP listener answers: MediaMTX's own default, and no
// setting, for the reason HTTPOrigin drops the direct port behind a proxy.
// A deployment that terminates TLS decides its own listeners, and the configured RtspPort names the
// cleartext one a LAN relay answers on.
const RtspsPort = 8322

// rtspURL addresses one path on the relay's RTSP listener and carries the credential,
// "rtsps://relay:8322/<path>?jwt=<token>" where the relay is encrypted and
// "rtsp://relay:8554/<path>?jwt=<token>" where it is not.
//
// The credential rides as a query and not as a userinfo password, which is where MediaMTX reads a
// JWT for RTSP.
// ffmpeg and rtspclientsink both keep it there for every request of the session, and rtspsrc does
// not, which is what rtspCredential covers.
func rtspURL(s settings.Settings, name string) string {
	return rtspAddress(s, name) + credentialQuery(s, "?")
}

// rtspAddress is that address with no credential on it, for the reader that takes one separately.
func rtspAddress(s settings.Settings, name string) string {
	scheme, port := "rtsp", s.Relay.RtspPort
	if s.Relay.Tls() {
		scheme, port = "rtsps", RtspsPort
	}
	return fmt.Sprintf("%s://%s:%d/%s", scheme, s.Relay.Host, port, name)
}
