# Viewer architecture

Watching a stream means pulling it from the relay and decoding it for display.

Two ways to watch exist.

- A single-stream native player (ffplay or mpv): one window per stream, opened from the shell's viewer roster, decoding through libavcodec.
- The shell's tile grid: a receiving GStreamer pipeline in the Go backend, whose decoded frames reach the Avalonia window over the frame channel.

The first works today and needs nothing installed beyond ffmpeg.
The second is being built, and the frame channel is the part that does not exist yet.

The capture and publish side is the mirror of this seam; see `capture-architecture.md`.

## What was removed, and why

Three viewers were deleted: the web grid inside the Wails window, the GTK4 `nativegrid` binary, and the standalone browser page the `webviewer` service served to the LAN.
The Wails app went with them, and the Avalonia shell is the only one left (`ipc-api.md`).

The reason is not that any of them worked badly.
Each held its own copy of the domain model - `webgrid.ts` decided what a tile could decode, `nativegrid.ts` decided the same question for the other grid, and `deps.ts` decided what the settings form offered - while `internal/form` decided all three in Go for the shell that arrived last.
One rule written three times in three languages is the drift `domain-model.md` exists to prevent, and deleting the copies was cheaper than keeping them in step for as long as the frame channel takes.

The decode knowledge was not deleted with them.
`nativegrid/internal/player/gstreamer` was lifted into `internal/receive` first, minus the GTK paintable: the receive pipelines, the render chains, the caps negotiation, the GPU memory features and the teardown order all survive the move, because none of them is about GTK.

Two Go packages went that had no such half.
`internal/webviewer` drove a browser `VideoDecoder` over a WebSocket, and `internal/moq` fetched and pinned a certificate so a webview could reach the relay's Media-over-QUIC listener.
Both existed only for a browser tile, so both left with it, and the `moq_port` setting left with them.
The relay still serves MoQ; nothing here reads it.
A native MoQ watcher would be a `transport` entry against a receive pipeline that subscribes, and it is not written, because every format MoQ was carrying to a webview reaches a receive pipeline over RTSP already.

## Two legs, two protocols

A stream crosses the relay in two independent legs: the publish leg from the publisher to the relay, and the watch leg from the relay to one viewer.
Each names its own protocol, because MediaMTX re-serves every ingested stream on all its listeners.
A stream published over SRT is watched over RTSP by one viewer and over WHEP by another, at the same time.

The two legs are not the same list of protocols, and a leg is not one list either.
HLS is served and never ingested, so it is a watch leg alone; RTMP takes four formats in and hands one back; WebRTC ingest is narrower than WHEP playback, and narrower on one publish engine than on the other.
Each transport declares a carriage per leg and per engine beside the code that serializes it (`transport.Formats`), and every rule that offers or refuses a protocol reads the entry for the leg and the engine it means.
The two watch engines are the two receivers here: ffplay and mpv open a URL through libavformat, and a receiving GStreamer pipeline is the tile grid's.

Three settings fields name a protocol, and each says which leg it is and, on the watch leg, which receiver:

| Field | Leg | Receiver |
| --- | --- | --- |
| `publish_transport` | publish | the publish engine |
| `player_watch_transport` | watch | ffplay or mpv, narrowed to the protocols a player opens by URL |
| `tile_watch_transport` | watch | the receive pipeline, which also reaches WHEP |

Two watch fields rather than one, because the two receivers reach different protocol sets and one field would let each store a leg the other cannot run.
A roster row and a tile can be watching the same stream over different protocols at the same time, which is the same fact from the user's side.

Each protocol's own knobs are per leg for the same reason.
The two SRT latency windows are separate fields because each leg is its own SRT link with its own retransmit window, and glass-to-glass delay is their sum.
RTSP names the RTP lower transport once per leg (`rtsp_publish_protocol`, `rtsp_watch_protocol`), because the two legs cross different networks and it is the network that decides whether a UDP port pair survives.
The jitter buffer is the tile receiver's alone: it sizes the reorder window of a receiving pipeline, and neither the publish leg nor an external player has one here.

An SRT latency window is a request, not a result.
SRT negotiates one delay per direction in the handshake and takes the larger of the two sides' values, so a link is never faster than the peer's own setting, whatever this side asks for.
MediaMTX exposes no SRT latency option and runs on its library's 120 ms default, which is therefore the floor of both hops against it: 400 ms is honoured, 60 ms comes back as 120.
The relay's `/v3/srtconns/list` reports both directions of both hops.

## Where the bytes go

Both paths are one network hop from the relay to the decoder.
They differ in what happens after the decoder, which is the whole of what the frame channel is for.
Ports are the defaults from `mediamtx.yml` and `settings.Defaults`.

Double rules cross the network; single rules stay inside one machine.

```
native player

  MediaMTX ══ SRT :8890 or RTSP :8554 ═══▶ ffplay or mpv window
  └─────────────── network ──────────────┘ └ receiver machine ┘

shell tile

  MediaMTX ══ SRT, RTSP or WHEP ═════════▶ receive pipeline ──GPU handle──▶ Avalonia window
  └─────────────── network ──────────────┘ └────────── receiver machine ──────────────────┘
```

The native player needs no app process at all, which is what makes it the viewer that survives a shell crash and the one that works on a platform whose frame import is unsolved.

The tile path puts the decode in the backend and the window in the shell, and the handle between them never leaves the machine.
Nothing is re-encoded on either path, and no frame crosses the control API.

## What each path decodes

A native path decodes every stream this app publishes, so what limits it is the watch leg rather than the decoder.
ffplay and mpv decode through libavcodec.
A receive pipeline's `decodebin` autoplugs by rank, so a hardware element takes the stream wherever its sink caps advertise the profile, and a software element takes the rest: `avdec_h264` and `avdec_h265` from gst-libav, `vp9dec` from libvpx, `dav1ddec` for AV1.
Nothing prefers software: the hardware decoders outrank the software ones, and the pixel format the publisher chose is what decides which of them can take the stream.

Which legs carry a format to a player and to a tile is "Which protocol carries which format" below, and it is the only gate either path has.
There is no longer a viewer here that refuses a chroma or a bit depth, which is the one thing the deletions bought the user rather than the code: a 4:4:4 HEVC stream that reached one grid and not the other now reaches both viewers.

`capabilities.Decoders` states what each decoder element takes, and the settings form reads it to say what a chroma costs the viewer.
It describes the viewers rather than this machine: a stream is published once and watched on whatever hardware the watchers have, so nothing in it is probed and nothing in it restricts a choice.
Every fixed-function decoder covers 4:2:0, with 10-bit where the format has a Main-10 equivalent.
Full chroma and RGB reach silicon in HEVC alone, through the Range Extensions profiles NVDEC and Intel's decoder carry and Mesa's VA drivers and DXVA do not.
4:2:2 divides the same way one step narrower: libavcodec decodes it in both H.26x formats, and Intel's HEVC decoder is the one hardware element that does, NVDEC implementing the 4:4:4 Range Extensions profiles without the 4:2:2 one.
H.264 has no hardware 4:4:4 anywhere, no vendor having implemented High 4:4:4 Predictive, which is also why lossless H.264 is a software decode on every machine: the mode exists only in that profile.
AV1 and VP9 are the same story in their own profiles, hardware decoding profile 0 and 2 and never the full-chroma ones.

Audio follows the path.
A player plays the second track the mux carries.
A receive pipeline grows a branch for it when `decodebin` exposes an audio pad, and that branch ends in a sink of its own rather than travelling to the shell: the backend runs on the machine the shell is on, so a second channel would carry the samples across a process boundary to reach the same output device.
The frame channel is therefore about frames alone, and the volume and mute a tile offers are effects on the receiver rather than anything the channel carries.

## Codec and chroma

`capabilities.Codecs` is the authoritative table; the rows below are the subset the argument builders map, in the shape the viewer side cares about.
The remaining families (V4L2 M2M, Rockchip MPP) are declared with `Implemented: false` and rejected before either builder runs.

Which protocol carries a row is not in it: that follows the bitstream format rather than the encoder, and the next section holds it.

| Codec | Chromas | GStreamer element |
|---|---|---|
| `h264_nvenc` | yuv444p, yuv420p, p010le | `nvh264enc` |
| `hevc_nvenc` | gbrp, yuv444p, yuv420p, p010le | `nvh265enc` |
| `av1_nvenc` | yuv420p, p010le | `nvav1enc` |
| `libx264` | yuv444p, yuv422p, yuv420p, p010le | `x264enc` |
| `libx265` | gbrp, yuv444p, yuv422p, yuv420p, p010le | `x265enc` |
| `libvpx-vp9` | gbrp, yuv444p, yuv420p, p010le | `vp9enc` |
| `libvpx` (VP8) | yuv420p | `vp8enc` |
| `libaom-av1` | gbrp, yuv444p, yuv420p, p010le | `av1enc` |
| `libsvtav1` | yuv420p, p010le | `svtav1enc` |
| `librav1e` | yuv444p, yuv420p, p010le | `rav1enc` |
| `h264_vaapi` | yuv420p | `vah264enc` |
| `hevc_vaapi` | yuv420p, p010le | `vah265enc` |
| `av1_vaapi` | yuv420p, p010le | `vaav1enc` |
| `vp9_vaapi` | yuv420p | `vavp9enc` |
| `vp8_vaapi` | yuv420p | `vavp8enc` |
| `h264_qsv` | yuv420p | `qsvh264enc` |
| `hevc_qsv` | yuv420p, p010le | `qsvh265enc` |
| `av1_qsv` | yuv420p, p010le | `qsvav1enc` |
| `vp9_qsv` | yuv420p | `qsvvp9enc` |
| `h264_amf` | yuv420p | none |
| `hevc_amf` | yuv420p, p010le | none |
| `av1_amf` | yuv420p, p010le | none |
| `h264_vulkan` | yuv420p | none |
| `hevc_vulkan` | yuv420p, p010le | none |
| `av1_vulkan` | yuv420p, p010le | none |

The chroma column is the union over the two publish engines.
A format one engine's encoder will not take carries a `Gap` on the row, so the viewer side sees every chroma a stream may arrive in; which of them a given capture backend can publish is the settings form's question.
4:2:2 is the two software H.26x rows' alone, coded through x264's High 4:2:2 and x265's Main 4:2:2 10: no hardware encoder here has an entrypoint for it, and the royalty-free formats have no 4:2:2 profile a fast encoder implements.

Whether a given GPU runs any of them is the driver's answer, not the table's: `encoders.Detect` probes each per publish engine, test-encoding on the ffmpeg engine and querying the plugin registry for the GStreamer element, and the settings form greys away what this machine refuses on the selected capture backend.

Two families have no GStreamer element at all, the only gaps in the table that take a whole family off an engine.
The `amfcodec` plugin builds its device layer on D3D11 and configures for Windows only; the vulkan plugin's encoder takes images on a Vulkan device, a memory no capture backend on that engine produces, and carries no HEVC or AV1 encoder to take them either.
The portal capture backend therefore publishes neither, and greys each family with its own reason.
On an AMD or Intel card the same silicon is reachable there through the VAAPI rows, which is what both alternatives drive.

## Which protocol carries which format

Each transport declares its carriage per leg and per engine (`transport.Formats`), and the reason a format is in or out belongs to the protocol and to the engine's muxer or source element, never to the encoder that produced the bitstream.
A cell reading none is an engine with no serialization for that leg, and the capability interface is absent with it.

| Transport | ffmpeg publish | GStreamer publish | Player watch | Tile watch |
|---|---|---|---|---|
| `srt` | h264, hevc | h264, hevc | h264, hevc | h264, hevc |
| `rtsp` | all five | all five | all five | all five |
| `rtmp` | h264, hevc, av1, vp9 | none | h264 | h264 |
| `webrtc` | h264 | h264, vp9, vp8 | none | h264, vp9, vp8 |
| `hls` | none | none | h264, hevc, av1, vp9 | none |

Why each row is shaped that way:

- **srt.** MPEG-TS registers a stream type for the two H.26x formats and for none of the others.
  One value covers all four cells, because ffmpeg's `mpegts` muxer and `mpegtsmux` write the same stream types and libavformat and `tsdemux` read them back.
- **rtsp.** RTP has a payload format for every video format and both audio codecs here, and both engines implement all of them, which is why RTSP is the fallback the other refusals point at.
- **rtmp.** The `flv` muxer writes the enhanced-RTMP tags the relay ingests, where `flvmux` writes the legacy ones alone, so there is no GStreamer publish form.
  Both watch cells sit on legacy-tag parsers, libavformat's FLV demuxer and `rtmp2src`.
- **webrtc.** ffmpeg's `whip` muxer writes one H.264 track and has no payloader for anything else, where `whipclientsink` payloads whatever `webrtcbin` negotiates and so reaches VP8 and VP9 over the same endpoint.
  Playback is the WHEP exchange rather than an address, so no player opens it and the receive pipeline's `whepsrc` is the only reader.
- **hls.** The relay segments and serves HLS and ingests nothing over it, and nothing on the GStreamer side reads the relay's playlist.

Audio is carried per protocol and not per engine: WebRTC carries Opus alone, RTMP AAC alone, and SRT, RTSP and HLS carry both.

Three rules fall out of the table:

- A codec no transport publishes cannot be published at all: `transport.ValidatePublish` rejects the combination before an encoder is built, and names the transports that would have carried it on the engine that is running.
- What a viewer may receive over is the watch entry for the engine it runs on, never the publish one.
  An SRT viewer opened on a VP9 stream would connect and receive nothing, so `WatchNamesFor` narrows the choice per stream and per engine instead.
- A publish leg the two engines carry differently is the capture backend's business as much as the transport's, since the backend fixes the engine.
  Publishing VP9 over WebRTC therefore means a GStreamer capture backend and no other.

The VP9 and AV1 formats also need `-strict experimental` on the ffmpeg publish leg, which `RTSP.PublishArgs` adds for them.
Both RTP payload formats are still IETF drafts, and the muxer refuses to write a draft payload without it.
The relay ingests them either way.

Two formats WebRTC negotiates are missing from its row all the same.
The relay refuses H.265 over WebRTC for any stream carrying B-frames, which is a property of the encode and unknowable for a stream this app did not produce.
An AV1 track negotiates over WHEP and then yields no picture, with an autoplugged depayloader or an explicit one, which takes AV1 off the publish cells too: a leg nothing can read back is not a leg.
Both stay out of the carriage rather than being offered and failing on the tile.

Which rate-control modes a row offers is not uniform, and a `Gap` naming the mode carries it.
Lossless is the mode that goes missing: only x264, x265 and NVENC H.264/HEVC code bit-exact, VP9 does so through ffmpeg but not through `vp9enc`, and no AV1, VP8, VAAPI, AMF or Vulkan encoder does at all.

Audio is one of two codecs at 128 kbit/s stereo, coded by `libopus` or `aac` on the ffmpeg engine and by `opusenc` or `avenc_aac` on the GStreamer one (`capabilities.AudioCodecs`).
Opus is the codec WebRTC negotiates and AAC the one FLV has always carried, so the publish leg decides which of the two a stream may use.

## The viewability verdict

One verdict remains where there were two, and it asks the question the two grids only ever answered differently by accident of decode reach.
A tile decodes whatever the machine's GStreamer decodes, so the leg is the only gate: the verdict asks the transport table whether a receiving pipeline is served the codec's format over `tile_watch_transport`.
It reads the GStreamer watch entry and never the publish one, since the two are separate sets: a stream published over RTMP is one the same protocol will not hand back at anything but H.264.
A format with no listener on the selected leg reports not-viewable, names the leg as the reason, and names the protocols that would carry it.

The verdict is computed in Go beside the tables it derives from, and reaches the shell as a `Form` statement like every other one (`ipc-api.md`).

## The receive package

`internal/receive` owns everything between the relay and a decoded frame: the source fragment for the chosen watch leg (`transport.GstWatcher`, the watch-side counterpart of `GstPublisher`), the decoder `decodebin` autoplugs, and the chain that converts what comes out of it.

Which elements sit between the source and the sink is a **render chain**, and the package carries a table of them rather than one line: the decoder is the same on every row and what differs is where the frames are converted and what that says about their colour.

| Chain | Between source and sink | Colour |
|---|---|---|
| `gl` | `glupload ! glcolorconvert ! glcolorscale`, RGBA/sRGB in `GLMemory` | stated |
| `cpu` | `videoscale ! capsfilter ! videoconvert`, RGBA/sRGB in system memory | stated |
| `d3d11`, `d3d12` | `d3d11upload ! d3d11convert`, then a download | the driver's |
| `raw` | nothing | unstated |

`gl` is the default, and it is the default because it was measured equal to `cpu` rather than because it is faster: rendered through both, flat dark, flat bright and gradient content are bit-identical, and a saturated colour-bar frame differs by at most one code value per channel.
Dark content agreeing is the evidence that matters, since washed-out shadows are the failure the pinned sRGB caps exist to prevent - without them the sink also takes YUV and an unknown transfer function is read as BT.709, lifting every shadow.
What `gl` saves is the download: the CPU chain pulls every decoded frame into system memory and converts and scales it there, which at 1440p144 in 4:4:4 is gigabytes a second per tile.
A machine that registers none of a chain's elements cannot run it, and `resolve` leaves it out, which is why the ladder exists at all.

The chain is a settings field like any other, offered by the form and greyed per element the machine does not register.
It is one value for every tile rather than one per stream: a chain falls back because a driver cannot run it, and that is a property of the machine.
`StartReceive` may carry an override later without a field to migrate.

What a running pipeline actually did is reported rather than assumed: the receive state names the chain, the memory the decoder handed its frames over in, and the memory the sink read them from, because a hardware decoder that downloaded its own frames costs the download the row promised to avoid.

## The frame channel

Frames do not cross the control API, and they will not.
`ipc-api.md` carries control and description; the frame channel is a second gRPC service on the same socket, and what travels on it is handle metadata, fences and release-backs.
The pixels stay in shared GPU memory that the handle names.

Each platform has its own handle type, and they are not equally ready.

- **Windows** - a D3D11 shared NT handle with a keyed mutex, which Avalonia's compositor imports today. This is the leg being built first.
- **Linux** - dmabuf, which `vah264dec` already produces. Avalonia lists dmabuf import as planned rather than shipped, so the near-term route is `OpenGlControlBase` plus `eglCreateImageKHR(EGL_LINUX_DMA_BUF_EXT)`, and format modifiers and a sync-file fence have to travel with the frame.
- **macOS** - IOSurface from VideoToolbox, with no first-class import handle type. The weakest leg and the last one scheduled; until it lands, macOS watches through the native player, which needs no frame channel at all.

The sink is `appsink` rather than a paintable: the chain above ends by exporting a handle instead of drawing into a widget, which is the one part of the harvested code that had to be rewritten.

The buffer-ownership protocol - pool ownership, per-frame fences, release-back messages, and what each side does when the other dies - is the actual design work, and it is not settled here.

What *is* settled is what the control API says about receiving.
`StartReceive` and `StopReceive` are effects, and receive state travels on the existing event stream, whole rather than as a delta, so a shell that asked and a shell that did not learn the same thing at the same time.
Nothing about a grid, a tile or a layout is on that contract: how a viewer arranges what it receives is the shell's job.

## The native player

`internal/watch` is the single-stream viewer, and it stays.
`watch.Select` picks the engine (ffplay by default, mpv via `SCREENSHARE_VIEWER`), and each builds its command line from the transport's `Watcher` URL.
The leg is passed in by name rather than read off the publish setting, which is what keeps a viewer free to receive over a protocol the stream was not published with.
A transport without a URL watch form - WebRTC, whose playback is the WHEP exchange rather than an address - is reachable by a receive pipeline and by no player here.

It is five files, it needs no frame channel, and it is the viewer that outlives a shell crash, which is why it is a permanent path rather than a stopgap.

A viewer is identified by stream name and transport together, not by name alone, because the relay re-serves each ingested stream on all its listeners and one stream can be open over several transports at once.

Two rendering differences are worth knowing when choosing between the two engines.
ffplay is pinned to the SDL X11/XWayland backend, whose window a compositor renders reliably where the SDL Wayland backend may not.
mpv renders 4:4:4 and a native Wayland window, which is what `SCREENSHARE_VIEWER=mpv` selects.

## Adding a watch path

A protocol is a `transport` entry declaring its watch carriage and implementing the capability the receiver reads: `Watcher` for a URL a player opens, `GstWatcher` for a source fragment a receive pipeline builds.
`transport.Register` holds the stated carriage and the serialization to each other, so an entry can neither offer a leg it has no code to build nor build one no caller may reach.

A render chain is a row in the receive package's table plus the element factories it needs, and `resolve` leaves it out on a machine that registers none of them.
The form offers what resolves and greys the rest with the element that is missing, so a chain becomes selectable by existing rather than by being listed a second time.
