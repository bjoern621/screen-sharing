package form

import (
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// The field keys, qualified by the settings group they belong to: the group's own field
// name on Settings, a dot, then the field name inside it, both as settings.proto spells
// them.
//
// A key is the identity a shell binds its widget by and the identity a Gap names, so
// the two meet with no mapping in between. That is what makes the spelling worth
// stating once here rather than typing per table: the wire name is snake_case, the Go
// struct's field name is not always the same word, and a form that invented its own
// third spelling would give a shell a control no gap can ever point at.
//
// The qualification is also what makes a key an address. settingsField resolves one
// against the message by walking those two names, so a repair writes through the key
// rather than through a per-field setter table, and the same two names are what the
// shell's own draft writer walks (avalonia/ScreenShare.App/Backend/SettingsDraft.cs).
const (
	KeyRelayHost  = "relay.host"
	KeySrtPort    = "relay.srt_port"
	KeyAPIPort    = "relay.api_port"
	KeyRtspPort   = "relay.rtsp_port"
	KeyWebrtcPort = "relay.webrtc_port"
	KeyRtmpPort   = "relay.rtmp_port"
	KeyHlsPort    = "relay.hls_port"

	KeyName = "publish.name"

	KeyTransport  = "publish.publish_transport"
	KeyCodec      = "publish.codec"
	KeyMode       = "publish.mode"
	KeyChroma     = "publish.chroma"
	KeyColorRange = "publish.color_range"
	KeyFps        = "publish.fps"
	KeyCq         = "publish.cq"
	KeyBitrateM   = "publish.bitrate_mbps"
	KeyMaxrateM   = "publish.maxrate_mbps"
	KeyVbvMs      = "publish.vbv_ms"
	KeyGop        = "publish.gop"
	KeyBframes    = "publish.bframes"
	KeyEffort     = "publish.effort"
	KeyTune       = "publish.tune"

	KeyCapture       = "publish.capture"
	KeyAudio         = "publish.audio"
	KeyAudioCodec    = "publish.audio_codec"
	KeyDrmMap        = "publish.drm_map"
	KeyMonitor       = "publish.monitor"
	KeyCaptureMemory = "publish.capture_memory"
	KeyCursor        = "publish.cursor"

	KeySrtPublishLatencyMs = "publish.srt_publish_latency_ms"
	KeyRtspPublishProtocol = "publish.rtsp_publish_protocol"

	KeyUplinkMbps = "publish.uplink_mbps"

	// KeyOutputResolution is one compound "WIDTHxHEIGHT" control rather than a width
	// and a height, because the user picks one thing and a form has no way to say that
	// two fields are only ever legal in pairs. The legal values are a list this
	// package generates, so the only strings that ever arrive are ones it wrote.
	KeyOutputResolution = "publish.output_resolution"

	// The two watch legs are two fields because the two receivers reach different
	// protocol sets, so one field would let each store a leg the other cannot run.
	KeyPlayerWatchTransport = "viewer.player_watch_transport"
	KeyTileWatchTransport   = "viewer.tile_watch_transport"

	KeyRtspWatchProtocol  = "viewer.rtsp_watch_protocol"
	KeySrtWatchLatencyMs  = "viewer.srt_watch_latency_ms"
	KeyRtspWatchLatencyMs = "viewer.rtsp_watch_latency_ms"

	KeyRenderChain = "viewer.render_chain"
)

// The group keys, in no order: the order is the groups table's.
//
// They are the headings a shell draws, and they are not the settings groups a key is
// qualified by: a screen is grouped by what the user is deciding, and a message by what
// the value belongs to. "watch" holds fields from two of the three, and "relay" holds
// the relay group plus the stream name that names a path on it.
const (
	GroupStream    = "stream"
	GroupSource    = "source"
	GroupQuality   = "quality"
	GroupAudio     = "audio"
	GroupTransport = "transport"
	GroupWatch     = "watch"
	GroupRelay     = "relay"
)

// keySeparator divides a key's group from its field.
const keySeparator = "."

// settingsField resolves a qualified key against a settings message: the group named
// before the dot, then the field named after it.
//
// It returns the group's own message rather than the outer one, because that is what a
// write goes through. A group message that is not set yet is created on the way, so a
// draft that arrived with a group absent is repaired into rather than panicked on: an
// absent group is another process's message, which makes it an environment condition
// (docs/development-principles.md).
//
// The two names are looked up rather than switched on, which is what keeps this from
// being a third list of fields to hold in step with settings.proto and the table above.
func settingsField(m *screensharev1.Settings, key string) (protoreflect.Message, protoreflect.FieldDescriptor, bool) {
	group, field, ok := strings.Cut(key, keySeparator)
	if !ok {
		return nil, nil, false
	}

	root := m.ProtoReflect()
	groupField := root.Descriptor().Fields().ByName(protoreflect.Name(group))
	if groupField == nil || groupField.Message() == nil {
		return nil, nil, false
	}

	inner := root.Mutable(groupField).Message()
	descriptor := inner.Descriptor().Fields().ByName(protoreflect.Name(field))
	if descriptor == nil {
		return nil, nil, false
	}
	return inner, descriptor, true
}
