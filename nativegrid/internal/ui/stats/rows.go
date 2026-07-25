package stats

// row is one key/value line of the card. A row whose figure only some streams have
// (a codec profile, an audio track) disappears while the value is missing; the rest
// keep their place and show a placeholder, so the card does not jump around while a
// pipeline negotiates.
type row struct {
	key   string
	hides bool
	value func(v View) string
}

// block is one titled group of rows. visible reports whether the whole block
// shows, and nil is a block that always does.
type block struct {
	title   string
	rows    []row
	visible func(v View) bool
}

// blocks is the overlay, top to bottom: the stream it plays, the video on the
// wire, what this side does with it, and the audio a stream may carry. The
// transport's own counters follow, one block per element, from what the player
// reports.
var blocks = []block{
	{title: "stream", rows: []row{
		{key: "transport", hides: true, value: func(v View) string { return v.Stream.Transport }},
		{key: "source", hides: true, value: func(v View) string { return v.Stream.Source }},
		{key: "uptime", value: func(v View) string { return shortDuration(v.Stats.Uptime) }},
		{key: "position", hides: true, value: func(v View) string { return shortDuration(v.Stats.Position) }},
		{key: "latency", hides: true, value: func(v View) string { return latencyText(v.Stats) }},
	}},
	{title: "video", rows: []row{
		{key: "resolution", value: func(v View) string { return sizeText(v.Stats.Width, v.Stats.Height) }},
		{key: "framerate", hides: true, value: func(v View) string { return rateText(v.Stats.FPSNum, v.Stats.FPSDen) }},
		{key: "codec", value: func(v View) string { return v.Stats.Codec }},
		{key: "profile", hides: true, value: func(v View) string {
			return join(v.Stats.Profile, levelText(v.Stats.Level))
		}},
		{key: "bitrate", hides: true, value: func(v View) string {
			return bitrateText(v.VideoRate, v.Stats.VideoBytes)
		}},
		{key: "keyframes", hides: true, value: func(v View) string { return keyframeText(v.Stats) }},
		{key: "format", value: func(v View) string {
			return join(v.Stats.Format, v.Stats.Subsampling, depthText(v.Stats.Depth))
		}},
		{key: "color", hides: true, value: func(v View) string {
			return join(v.Stats.Colorimetry, siteText(v.Stats.ChromaSite))
		}},
		{key: "geometry", hides: true, value: func(v View) string {
			return join(aspectText(v.Stats.PixelAspect), v.Stats.Interlace)
		}},
	}},
	{title: "decode", rows: []row{
		{key: "decoder", value: func(v View) string { return decoderText(v.Stats) }},
		{key: "render", hides: true, value: func(v View) string { return v.Stats.Render }},
		{key: "fps", value: func(v View) string { return v.FPS }},
		{key: "frames", value: func(v View) string { return framesText(v.Stats) }},
		{key: "encoded", hides: true, value: func(v View) string { return codedText(v.Stats) }},
	}},
	{
		title:   "audio",
		visible: func(v View) bool { return v.Stats.AudioCodec != "" || v.Stats.AudioFormat != "" },
		rows: []row{
			{key: "codec", hides: true, value: func(v View) string { return v.Stats.AudioCodec }},
			{key: "decoder", hides: true, value: func(v View) string { return v.Stats.AudioDecoder }},
			{key: "format", hides: true, value: func(v View) string { return audioFormatText(v.Stats) }},
			{key: "bitrate", hides: true, value: func(v View) string {
				return bitrateText(v.AudioRate, v.Stats.AudioBytes)
			}},
		},
	},
}
