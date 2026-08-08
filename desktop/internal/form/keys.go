package form

// The field keys, which are the StreamSettings field names as settings.proto spells
// them.
//
// A key is the identity a shell binds its widget by and the identity a Gap names, so
// the two meet with no mapping in between. That is what makes the spelling worth
// stating once here rather than typing per table: the wire name is snake_case, the Go
// struct's JSON tag is not always the same word, and a form that invented its own
// third spelling would give a shell a control no gap can ever point at.
const (
	KeyName       = "name"
	KeyRelayHost  = "relay_host"
	KeyRelayPort  = "relay_port"
	KeyAPIPort    = "api_port"
	KeyRtspPort   = "rtsp_port"
	KeyWebrtcPort = "webrtc_port"
	KeyRtmpPort   = "rtmp_port"
	KeyHlsPort    = "hls_port"
	KeyMoqPort    = "moq_port"

	KeyTransport  = "transport"
	KeyCodec      = "codec"
	KeyMode       = "mode"
	KeyChroma     = "chroma"
	KeyColorRange = "color_range"
	KeyFps        = "fps"
	KeyCq         = "cq"
	KeyBitrateM   = "bitrate_mbps"
	KeyMaxrateM   = "maxrate_mbps"
	KeyVbvMs      = "vbv_ms"
	KeyGop        = "gop"
	KeyBframes    = "bframes"
	KeyEncPreset  = "enc_preset"

	KeyCapture       = "capture"
	KeyAudio         = "audio"
	KeyAudioCodec    = "audio_codec"
	KeyDrmMap        = "drm_map"
	KeyMonitor       = "monitor"
	KeyCaptureMemory = "capture_memory"

	KeySrtPublishLatencyMs = "srt_publish_latency_ms"
	KeySrtWatchLatencyMs   = "srt_watch_latency_ms"

	KeyRtspPublishProtocol = "rtsp_publish_protocol"
	KeyRtspWatchProtocol   = "rtsp_watch_protocol"

	KeyUplinkMbps = "uplink_mbps"

	KeyWatchTransport = "watch_transport"

	// KeyOutputResolution is one compound "WIDTHxHEIGHT" control rather than a width
	// and a height, because the user picks one thing and a form has no way to say that
	// two fields are only ever legal in pairs. The legal values are a list this
	// package generates, so the only strings that ever arrive are ones it wrote.
	KeyOutputResolution = "output_resolution"
)

// The group keys, in no order: the order is the groups table's.
const (
	GroupStream    = "stream"
	GroupSource    = "source"
	GroupQuality   = "quality"
	GroupAudio     = "audio"
	GroupTransport = "transport"
	GroupWatch     = "watch"
	GroupRelay     = "relay"
)
