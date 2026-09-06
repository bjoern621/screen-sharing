package form

import (
	"strconv"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// The field keys: the settings group's own field name on Settings, a dot,
// then the field name inside it, both as settings.proto spells them.
// "publish.encoder", "viewer.render_chain", "relay.host".
//
// A key is what a shell binds its widget by and what a Gap names,
// so the two meet with no mapping in between.
// The wire name is snake_case and the Go struct's field name is not always the same word,
// so a form inventing a third spelling would hand a shell a control no gap can ever point at.
//
// The qualification is what makes a key an address.
// settingsField resolves one against the message by walking those two names,
// so a repair writes through the key rather than through a per-field setter table,
// and the shell's own draft writer walks the same two
// (avalonia/ScreenShare.App/Backend/SettingsDraft.cs).
const (
	KeyRelayHost   = "relay.host"
	KeyRelayTls    = "relay.tls"
	KeyDiscordMode = "relay.discord_mode"
	KeyGroupKey    = "relay.group_key"
	KeyDisplayName = "relay.display_name"
	KeySrtPort     = "relay.srt_port"
	KeyRtspPort    = "relay.rtsp_port"
	KeyWebrtcPort  = "relay.webrtc_port"
	KeyRtmpPort    = "relay.rtmp_port"
	KeyHlsPort     = "relay.hls_port"
	KeyMoqPort     = "relay.moq_port"

	KeyTransport = "publish.publish_transport"
	// The encode, as the two controls it is: which bitstream, and what produces it.
	// The pair addresses one row of the capability table, and that row is stored nowhere
	// (settings.Publish.Codec).
	KeyFormat     = "publish.format"
	KeyEncoder    = "publish.encoder"
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

	KeyCapture = "publish.capture"
	// The controls of one entry of the audio source list, each a template rather than a key.
	// The list holds as many entries as the user made, so a resolve draws these once per entry
	// with the index filled in and a shell binds "publish.audio_sources[2].gain" (indexedKey).
	KeyAudioSource       = "publish.audio_sources[].source"
	KeyAudioSourceDevice = "publish.audio_sources[].device"
	KeyAudioSourceGain   = "publish.audio_sources[].gain"
	KeyAudioSourceMute   = "publish.audio_sources[].mute"
	KeyAudioCodec        = "publish.audio_codec"
	KeyDrmMap            = "publish.drm_map"
	KeyMonitor           = "publish.monitor"
	KeyCaptureMemory     = "publish.capture_memory"
	KeyCursor            = "publish.cursor"

	KeySrtPublishLatencyMs = "publish.srt_publish_latency_ms"
	KeyRtspPublishProtocol = "publish.rtsp_publish_protocol"

	KeyUplinkMbps = "publish.uplink_mbps"

	// KeyOutputResolution is one compound control, "1920x1080", rather than a width and a height:
	// the user picks one thing and a form has no way to say two fields are legal only in pairs.
	// This package generates the legal values, so the only strings that ever arrive are ones it wrote.
	KeyOutputResolution = "publish.output_resolution"

	// The tile's leg alone.
	// A player and a browser page are opened per press on a leg the roster names,
	// so neither has a setting on this form.
	KeyTileWatchTransport = "viewer.tile_watch_transport"

	KeyRtspWatchProtocol  = "viewer.rtsp_watch_protocol"
	KeySrtWatchLatencyMs  = "viewer.srt_watch_latency_ms"
	KeyRtspWatchLatencyMs = "viewer.rtsp_watch_latency_ms"

	KeyRenderChain = "viewer.render_chain"

	// What the app does for itself, which is about no stream.
	KeySendCrashReports    = "app.send_crash_reports"
	KeyCheckUpdatesOnStart = "app.check_updates_on_start"
)

// The group keys.
// Their order is the groups table's and not this block's.
//
// They are the headings a shell draws, and they are not the settings groups a key is qualified by.
// A screen is grouped by what the user is deciding and a message by what the value belongs to,
// so neither list is derived from the other and one group may hold keys from several messages.
const (
	GroupSource    = "source"
	GroupQuality   = "quality"
	GroupAudio     = "audio"
	GroupTransport = "transport"
	GroupWatch     = "watch"
	GroupRelay     = "relay"
	GroupApp       = "app"
)

// keySeparator stands between a key's group and its field.
const keySeparator = "."

// settingsGroupPublish qualifies every key of the publish settings,
// which is the group a preset speaks for.
const settingsGroupPublish = "publish"

// The brackets around a repeated field's index, and the template both of them with nothing between:
// a row of the field table carries "publish.audio_sources[].gain"
// and a shell binds "publish.audio_sources[2].gain".
//
// The template is what makes the two one identifier.
// The row states the control once and the resolve draws it per entry,
// so every table keyed by that control (its availability, its options,
// its copy on a surface) is keyed by the template rather than gaining an entry per index nobody
// can enumerate in advance.
const (
	keyIndexOpen  = "["
	keyIndexClose = "]"
	keyIndexEmpty = keyIndexOpen + keyIndexClose
)

// indexedKey is one entry's key, from the row's template and the entry's place in the list.
func indexedKey(template string, i int) string {
	return strings.Replace(template, keyIndexEmpty, keyIndexOpen+strconv.Itoa(i)+keyIndexClose, 1)
}

// keyTemplate is the template a key was drawn from,
// and the key itself where it names no entry of a list.
// Every table keyed by a control is looked up with it, so a statement about one entry's control
// is written once for the control.
func keyTemplate(key string) string {
	open := strings.Index(key, keyIndexOpen)
	close := strings.Index(key, keyIndexClose)
	if open < 0 || close < open {
		return key
	}
	return key[:open+1] + key[close:]
}

// keyIndex is which entry of a list a key names. false where it names no entry.
func keyIndex(key string) (int, bool) {
	open := strings.Index(key, keyIndexOpen)
	close := strings.Index(key, keyIndexClose)
	if open < 0 || close <= open+1 {
		return 0, false
	}
	i, err := strconv.Atoi(key[open+1 : close])
	if err != nil || i < 0 {
		return 0, false
	}
	return i, true
}

// settingsField walks a qualified key into a settings message: the group named before the dot,
// then the field named after it.
//
// It answers with the group's own message rather than the outer one,
// a write going through that one.
// A group message not set yet is created on the way, so a draft that arrived with a group absent
// is repaired into rather than panicked on: an absent group is another process's message,
// which makes it an Umgebungsfehler (docs/development-principles.md).
//
// The two names are looked up rather than switched on, which keeps this from being a third list
// of fields to hold in step with settings.proto and the table above.
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
	if list, rest, ok := strings.Cut(field, keyIndexOpen); ok {
		return listField(inner, list, rest)
	}
	descriptor := inner.Descriptor().Fields().ByName(protoreflect.Name(field))
	if descriptor == nil {
		return nil, nil, false
	}
	return inner, descriptor, true
}

// listField resolves the second half of an indexed key: the entry of a repeated field,
// and the field inside that entry.
//
// rest is what followed the opening bracket: "2].gain".
// An entry the list does not hold yet is appended on the way,
// up to the one past its end and no further.
// That entry is the row a form draws for a reader to grow the list by,
// so a write through its key adds it, and a write past it would leave a hole nothing chose.
func listField(group protoreflect.Message, name, rest string) (protoreflect.Message, protoreflect.FieldDescriptor, bool) {
	index, after, ok := strings.Cut(rest, keyIndexClose)
	if !ok {
		return nil, nil, false
	}
	field, ok := strings.CutPrefix(after, keySeparator)
	if !ok {
		return nil, nil, false
	}
	i, err := strconv.Atoi(index)
	if err != nil || i < 0 {
		return nil, nil, false
	}

	listField := group.Descriptor().Fields().ByName(protoreflect.Name(name))
	if listField == nil || !listField.IsList() || listField.Message() == nil {
		return nil, nil, false
	}
	list := group.Mutable(listField).List()
	if i > list.Len() {
		return nil, nil, false
	}
	if i == list.Len() {
		list.Append(list.NewElement())
	}

	entry := list.Get(i).Message()
	descriptor := entry.Descriptor().Fields().ByName(protoreflect.Name(field))
	if descriptor == nil {
		return nil, nil, false
	}
	return entry, descriptor, true
}

// keyRepeats reports whether a key is a template, so the control it names is drawn per entry
// of a list rather than once.
func keyRepeats(key string) bool {
	return strings.Contains(key, keyIndexEmpty)
}
