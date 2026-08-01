package stats

// row is one key/value line of the card. A row whose figure only some streams have
// (a codec profile, an audio track) disappears while the value is missing; the rest
// keep their place and show a placeholder, so the card does not jump around while a
// pipeline negotiates.
type row struct {
	key   string
	hides bool
	// tip is what the figure means, shown while the pointer rests on the row. The
	// card prints keys and numbers, so the reading is only explained here.
	tip   string
	value func(v View) string
	// tipOf explains the reading the row currently shows, on a row whose value is a
	// verdict rather than a figure: what "no download" means depends on which of the
	// four paths produced it. It falls back to tip, so a poll that has no verdict yet
	// still explains the row.
	tipOf func(v View) string
}

// tipAt is the explanation the row shows for one poll.
func (r row) tipAt(v View) string {
	if r.tipOf == nil {
		return r.tip
	}
	if tip := r.tipOf(v); tip != "" {
		return tip
	}
	return r.tip
}

// block is one titled group of rows. visible reports whether the whole block
// shows, and nil is a block that always does. tip explains the group, on its
// heading.
type block struct {
	title   string
	tip     string
	rows    []row
	visible func(v View) bool
}

// blocks is the overlay, top to bottom: the stream it plays, the video on the
// wire, what this side does with it, and the audio a stream may carry. The
// transport's own counters follow, one block per element, from what the player
// reports. Every transport figure here is the watch leg, relay to viewer: this
// process only ever sees the leg it subscribes on.
var blocks = []block{
	{title: "stream", tip: "What this tile subscribes to, and how long it has been playing.", rows: []row{
		{
			key:   "transport",
			hides: true,
			tip:   "Protocol this viewer receives the stream over, the watch leg (relay to viewer). The publisher picks the protocol of its own leg into the relay separately, so the two can differ.",
			value: func(v View) string { return v.Stream.Transport },
		},
		{
			key:   "source",
			hides: true,
			tip:   "Source element and relay address the pipeline plays from.",
			value: func(v View) string { return v.Stream.Source },
		},
		{
			key:   "uptime",
			tip:   "Wall time since the pipeline started, counting whether or not frames arrive.",
			value: func(v View) string { return shortDuration(v.Stats.Uptime) },
		},
		{
			key:   "position",
			hides: true,
			tip:   "Running time the pipeline has played. A stall freezes it while uptime keeps counting.",
			value: func(v View) string { return shortDuration(v.Stats.Position) },
		},
		{
			key:   "latency",
			hides: true,
			tip:   "Buffer the pipeline holds against jitter, minimum / maximum. Live marks a source that produces in real time and cannot be sped up to catch up.",
			value: func(v View) string { return latencyText(v.Stats) },
		},
	}},
	{title: "video", tip: "The encoded video as it arrives, and the raw frames the decoder makes of it.", rows: []row{
		{
			key:   "resolution",
			tip:   "Picture size in pixels, off the decoded frames.",
			value: func(v View) string { return sizeText(v.Stats.Width, v.Stats.Height) },
		},
		{
			key:   "framerate",
			hides: true,
			tip:   "Frame rate the caps declare, as the fraction they carry: 30000/1001 is not 29.97. What arrives is the fps row under decode.",
			value: func(v View) string { return rateText(v.Stats.FPSNum, v.Stats.FPSDen) },
		},
		{
			key:   "codec",
			tip:   "Compression the video is encoded with, as the decoder describes it.",
			value: func(v View) string { return v.Stats.Codec },
		},
		{
			key:   "profile",
			hides: true,
			tip:   "Codec profile and level: which coding tools the stream uses, and the resolution and bitrate ceiling a decoder has to manage.",
			value: func(v View) string {
				return join(v.Stats.Profile, levelText(v.Stats.Level))
			},
		},
		{
			key:   "bitrate",
			hides: true,
			tip:   "Measured rate of the encoded video across the poll interval, and the bytes received since the tile opened.",
			value: func(v View) string {
				return bitrateText(v.VideoRate, v.Stats.VideoBytes)
			},
		},
		{
			key:   "keyframes",
			hides: true,
			tip:   "Keyframes received and the age of the last one, the GOP length as it arrives rather than as the encoder was asked for. Decoding can only start on a keyframe.",
			value: func(v View) string { return keyframeText(v.Stats) },
		},
		{
			key: "format",
			tip: "Raw pixel format the decoder outputs: memory layout, chroma subsampling, bits per component.",
			value: func(v View) string {
				return join(v.Stats.Format, v.Stats.Subsampling, depthText(v.Stats.Depth))
			},
		},
		{
			key:   "color",
			hides: true,
			tip:   "Colorimetry the caps carry (primaries, transfer function, matrix, range) and where chroma samples sit relative to luma.",
			value: func(v View) string {
				return join(v.Stats.Colorimetry, siteText(v.Stats.ChromaSite))
			},
		},
		{
			key:   "geometry",
			hides: true,
			tip:   "Pixel aspect ratio and scan mode. A PAR of 1/1 is square pixels, anything else stretches the picture on display.",
			value: func(v View) string {
				return join(aspectText(v.Stats.PixelAspect), v.Stats.Interlace)
			},
		},
	}},
	{title: "decode", tip: "What this side does with the frames it receives.", rows: []row{
		{
			key:   "decoder",
			tip:   "Decoder element the pipeline picked, and whether it decodes on the GPU. It says where the decoding ran and nothing about where the frames went afterwards: a hardware decoder can download its own output. The memory and path rows are what answer that.",
			value: func(v View) string { return decoderText(v.Stats) },
		},
		{
			key:   "chain",
			tip:   "Render chain the pipeline was built from, and whether it states the colour it produces. A chain that states it converts to a colour the caps describe; one that does not leaves the interpretation to the driver or to GTK.",
			value: func(v View) string { return chainText(v.Stats) },
		},
		{
			key:   "path",
			hides: true,
			tip:   "What happened to the frames between the decoder and the sink, read off the memory each end negotiated.",
			value: func(v View) string { return pathText(v.Stats) },
			tipOf: func(v View) string { return pathTip(v.Stats) },
		},
		{
			key:   "memory",
			hides: true,
			tip:   "Memory features the caps carry on the decoder's output and on the sink's input, verbatim and in that order. They are the evidence the path row's verdict is read from.",
			value: func(v View) string { return memoryText(v.Stats) },
		},
		{
			key:   "renderer",
			hides: true,
			tip:   "GSK renderer drawing the window. It is the last link in the path: a GL texture handed to a renderer that does not draw GL textures is downloaded again, after everything the pipeline did to keep it on the GPU.",
			value: func(v View) string { return v.Renderer },
		},
		{
			key:   "render",
			hides: true,
			tip:   "Pixel format and colorimetry the sink takes. It differs from format where the pipeline converts before display.",
			value: func(v View) string { return v.Stats.Render },
		},
		{
			key:   "fps",
			tip:   "Frames per second reaching the screen, measured across the poll interval. Short of framerate means this side is not keeping up.",
			value: func(v View) string { return v.FPS },
		},
		{
			key:   "frames",
			tip:   "Frames the sink showed, and frames it dropped for arriving too late to show, as a count and as a share of what it was handed. Until the sink reports, the paintable's own count of painted frames stands in.",
			value: func(v View) string { return framesText(v.Stats) },
		},
		{
			key:   "encoded",
			hides: true,
			tip:   "Encoded frames handed to the decoder. It leads the rendered count by the frames in flight, and far ahead of it when the queue leaks.",
			value: func(v View) string { return codedText(v.Stats) },
		},
	}},
	{
		title:   "audio",
		tip:     "The audio branch, on a stream that carries sound.",
		visible: func(v View) bool { return v.Stats.AudioCodec != "" || v.Stats.AudioFormat != "" },
		rows: []row{
			{
				key:   "codec",
				hides: true,
				tip:   "Compression the audio is encoded with.",
				value: func(v View) string { return v.Stats.AudioCodec },
			},
			{
				key:   "decoder",
				hides: true,
				tip:   "Audio decoder element the pipeline picked.",
				value: func(v View) string { return v.Stats.AudioDecoder },
			},
			{
				key:   "format",
				hides: true,
				tip:   "Raw sample format, sample rate and channel count the decoder outputs.",
				value: func(v View) string { return audioFormatText(v.Stats) },
			},
			{
				key:   "bitrate",
				hides: true,
				tip:   "Measured rate of the encoded audio across the poll interval, and the bytes received since the tile opened.",
				value: func(v View) string {
					return bitrateText(v.AudioRate, v.Stats.AudioBytes)
				},
			},
		},
	},
}
