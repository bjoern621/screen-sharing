# Glossary

Abbreviations and initialisms this project uses, in code, configuration, documentation or the user interface.
A term earns a row by appearing somewhere in the repository, not by being common in the field.
Which combinations are offered is the capability table's answer (`backend/internal/capabilities`), not this page's.

Expansion first, meaning second.
An abbreviation carrying two senses in this repository gets a row per sense.
`video-stack.md` holds the other half of the subject: what each of these things sits between, what it constrains, and what breaks when it disagrees with its neighbour.

The first section is different: it fixes the words for the app's own moving parts, and those names are normative rather than descriptive.

## Domain language

One name per concept, in tooltips, labels, log messages, comments, commit messages and docs.
A concept with two names reads as two concepts, and a user who met "GStreamer pipeline" in one tooltip cannot tell it is the thing another tooltip calls the "portal backend".
The **Not** column lists the synonyms this repository refuses for the same thing.

| Term | Means | Not |
| --- | --- | --- |
| Capture backend | How frames leave the desktop, named as its own framework names the source (`x11grab`, `portal`, `d3d11screencapturesrc`, and the rest of `publish.Captures`). The user's first choice: it fixes the publish engine. | capture API, capture path, capture method, grabber, screen source |
| Publish engine | Media framework running capture, encode and publish in one process: ffmpeg or GStreamer. Follows from the capture backend, never picked directly. | pipeline, publish path, portal path, media backend, capture engine |
| ffmpeg | The ffmpeg publish engine, and the executable. | FFmpeg, FFMPEG |
| GStreamer | The GStreamer publish engine, driven as a `gst-launch-1.0` pipeline description. | gstreamer (in prose), Gstreamer, gst |
| Element | One node in a GStreamer pipeline, e.g. `x264enc`. The unit a GStreamer capability gap names. | plugin (a plugin ships elements), filter, component |
| Encoder family | Silicon or library a group of encoders runs on: software, NVENC, VAAPI, QSV, AMF, V4L2 M2M, Rockchip MPP, Vulkan Video. A column of the capability table (`capabilities.Codec.Family`), read to name an entry rather than picked. | encoder backend, encoder type, hardware backend |
| Encoder | What produces a bitstream, at the grain the "Encoded by" dropdown offers one (`publish.encoder`): the family wherever that family is one encoder, and the library where several share a family (`nvenc`, `x264`, `svt-av1`). `capabilities.Codec.Encoder`. | codec (a codec is the format), encoder engine |
| Video codec | Coding format an encoder produces: H.264, HEVC, AV1, VP9, VP8. What the "Video format" dropdown offers (`publish.format`), and a column of the capability table (`capabilities.Codec.Format`). | codec family, video format (in the UI) |
| Codec row | One encode: the format and the encoder together, named as the ffmpeg encoder (`libx264`, `hevc_nvenc`). Addressed by the pair rather than stored, and what a command line, a log line and a probe verdict spell. `capabilities.Row`. | codec (ambiguous), encoder name |
| Audio codec | Coding format the desktop audio track is encoded in: Opus, AAC. Separate from the audio source, which says where the track comes from. | audio format, sound codec |
| Pixel format | Color model, subsampling and bit depth handed to the encoder: `gbrp`, `yuv444p`, `yuv422p`, `yuv420p`, `p010le`. | chroma (alone), color format |
| Rate-control mode | How the encoder spends bits over time: CBR, VBR, ABR, CRF, lossless. | bitrate mode, rate mode, quality mode |
| Capability gap | One thing a codec cannot do, on one publish engine or on both, carrying the reason the UI shows. `capabilities.Gap`. | limitation, restriction, exclusion |
| Relay | The MediaMTX server every publisher pushes to and every viewer pulls from. | server, host, MediaMTX (when the role is meant) |
| Publish leg | Publisher to relay. The only leg an encoder is built for, and one of the two the settings form configures: the `watch` group carries the other. | hop 1, publish hop, upstream |
| Watch leg | Relay to viewer. Chosen per receiver, independent of the publish leg: `tile_watch_transport` for a tile, and a leg named per press for a player window or a browser page. | hop 2, viewer hop, downstream, playback path |
| Shell | The Avalonia window, and the only one. Decides nothing, draws what the backend describes. | frontend, client, UI process |
| Backend | Headless Go process owning capture, encode, publish, decode and the domain model. | server, daemon, core (when the role is meant) |
| Receive pipeline | In-process GStreamer pipeline pulling one stream off the relay and decoding it for a tile. `backend/internal/receive`. | player pipeline, decode chain, grid pipeline |
| Render chain | The elements between a receive pipeline's decoder and its sink, and what they say about the colour they produce: `gl`, `cpu`, `d3d11`, `d3d12`, `raw`. | render path, conversion path, video sink chain |
| Frame channel | Second gRPC service between backend and shell: pool announcements naming the GPU memory, frame-ready notices, and the releases that hand a slot back. Carries no pixels and no control. | video channel, frame API, stream channel |

## Video codecs

| Term | Expansion | Meaning |
| --- | --- | --- |
| AVC | Advanced Video Coding | ITU-T H.264, the format every publish transport here can carry. |
| HEVC | High Efficiency Video Coding | ITU-T H.265, roughly half the bitrate of AVC at equal quality. |
| AV1 | AOMedia Video 1 | Royalty-free codec, published over RTSP and RTMP alone: MPEG-TS has no mapping for it, and a WebRTC leg negotiates it and then yields no picture. |
| VP8 | (no expansion) | Royalty-free codec with one profile, 8-bit 4:2:0 only. |
| VP9 | (no expansion) | Royalty-free codec whose profiles 0-3 cover 4:2:0, 4:4:4 and high bit depth. |
| RExt | Range Extensions | HEVC extension adding 4:2:2, 4:4:4 and bit depths above 10, the only VAAPI path to 4:4:4. |

## Encoder families

| Term | Expansion | Meaning |
| --- | --- | --- |
| NVENC | NVIDIA Encoder | Fixed-function encoder block on NVIDIA GPUs, the only hardware family here that reaches 4:4:4 and direct RGB. |
| NVDEC | NVIDIA Decoder | The matching fixed-function decoder block. |
| VAAPI | Video Acceleration API | Linux hardware encode and decode API covering both Intel and AMD GPUs. |
| QSV | Quick Sync Video | Intel's own encoder path, reached through oneVPL. |
| VPL | Video Processing Library | Intel's oneVPL API, successor to Media SDK and the library behind QSV. |
| AMF | Advanced Media Framework | AMD's encoder API, an alternative to VAAPI on AMD hardware. |
| V4L2 | Video4Linux2 | Linux kernel media API, exposing SoC encoders through its M2M device class. |
| M2M | Memory to Memory | V4L2 device class where the kernel transforms buffers, the form hardware codecs take. |
| MPP | Media Process Platform | Rockchip's media API for RK35xx and similar SoC encoders. |
| SVT | Scalable Video Technology | Intel's open-source encoder family, here SVT-AV1. |
| ASIC | Application-Specific Integrated Circuit | Fixed-function silicon, what "hardware encoder" means as opposed to a CPU encoder. |

## Rate control and bitstream structure

| Term | Expansion | Meaning |
| --- | --- | --- |
| QP | Quantization Parameter | Per-block quantizer step, the main quality-to-bitrate control. |
| CQ | Constant Quality | Quality-targeted rate control, whose scale differs per encoder and per publish engine (51 for the H.26x encoders, 63 for libvpx and software AV1, 127 or 255 for encoders exposing a raw quantizer index). |
| CQP | Constant QP | Rate control holding the quantizer fixed and letting bitrate vary. |
| CRF | Constant Rate Factor | Quality target that varies QP by frame type and motion. |
| CBR | Constant Bitrate | Rate control holding output bitrate fixed. |
| VBR | Variable Bitrate | Rate control letting bitrate follow content complexity, up to a ceiling. |
| ABR | Average Bitrate | Rate control hitting an average over the stream. |
| VBV | Video Buffering Verifier | Bitstream buffer model whose size is exposed as the rate buffer setting, bounding short-term bitrate. |
| GOP | Group of Pictures | Repeating frame-type pattern between keyframes, set as the keyframe interval. |
| IDR | Instantaneous Decoder Refresh | Keyframe clearing all reference buffers, so a viewer joining mid-stream can start decoding. |
| SPS | Sequence Parameter Set | Sequence-wide bitstream header carrying resolution, profile and chroma format. |
| PPS | Picture Parameter Set | Picture-level bitstream header shared by slices. |
| B-frame | Bidirectionally predicted frame | Frame predicted from both earlier and later frames, which reorders decode against display and adds latency. |

## Chroma, pixel formats and color

| Term | Expansion | Meaning |
| --- | --- | --- |
| 4:4:4 | Chroma sampling ratio | Full chroma resolution, required for text to stay free of color fringing. |
| 4:2:2 | Chroma sampling ratio | Chroma halved horizontally and kept vertically, the step between the other two, coded here by x264 and x265 alone. |
| 4:2:0 | Chroma sampling ratio | Chroma halved in both directions, the default for delivery codecs. |
| YUV | (historical analog term) | Used as a synonym for Y′CbCr, the model separating luma from two color-difference channels. |
| Y′CbCr | Luma prime, Chroma blue, Chroma red | The gamma-encoded color model video codecs actually code. |
| yuv420p | Pixel format | 8-bit 4:2:0 in three planes. |
| yuv444p | Pixel format | 8-bit 4:4:4 in three planes. |
| yuv422p | Pixel format | 8-bit 4:2:2 in three planes. |
| p010le | Pixel format | 10-bit 4:2:0 stored in little-endian 16-bit samples, luma plane plus interleaved chroma. |
| gbrp | Pixel format | Planar RGB in green, blue, red plane order, coded through a codec's identity matrix so RGB stays RGB. |
| NV12 | Pixel format | 8-bit 4:2:0 with a luma plane and one interleaved chroma plane. |
| I420 | Pixel format | 8-bit 4:2:0 in three planes, the GStreamer name for yuv420p. |
| RGBA | Pixel format | Packed 8-bit red, green, blue and alpha, the format a render chain pins on the frames it hands the sink. |
| BT.709 | ITU-R Recommendation BT.709 | High definition color primaries, matrix and transfer function. |
| sRGB | standard Red Green Blue | The color space desktop content is authored in, whose transfer differs from BT.709. |
| EOTF | Electro-Optical Transfer Function | Mapping from code value to displayed light, the assumption that washes out sRGB content when a sink guesses BT.709. |
| HDR | High Dynamic Range | Extended brightness range, the reason a source would need more than 8 bits per component. |
| Full range | (no expansion) | Luma spanning the whole 0-255 code range at 8-bit, standard for computer graphics. |
| Limited range | (no expansion) | Luma spanning 16-235 at 8-bit, standard for broadcast video. |
| Chroma site | Chroma siting | Where a subsampled chroma sample sits relative to its luma samples. |

## Transports and containers

| Term | Expansion | Meaning |
| --- | --- | --- |
| RTP | Real-time Transport Protocol | Packet format carrying media with a sequence number, timestamp and SSRC, used by both RTSP and WebRTC. |
| SRTP | Secure RTP | RTP with encryption and authentication, what a WHIP session ships after signaling. |
| RTSP | Real Time Streaming Protocol | Text control protocol negotiating SDP, after which media flows over RTP. |
| SRT | Secure Reliable Transport | UDP protocol with retransmission and a configurable latency window, carrying MPEG-TS here. |
| WebRTC | Web Real-Time Communication | Browser real-time media stack, reached here through WHIP and WHEP. |
| WHIP | WebRTC-HTTP Ingestion Protocol | RFC 9725, an SDP offer posted over HTTP to start a WebRTC publish session. |
| WHEP | WebRTC-HTTP Egress Protocol | The playback counterpart of WHIP. |
| MoQ | Media over QUIC | Publish-subscribe streaming over QUIC. A watch leg here and a browser's alone: the relay re-serves every ingested stream on it, no engine this app drives publishes it, and no player or receive pipeline opens it. |
| QUIC | (no expansion, not an acronym) | UDP transport with per-stream reliability and its own TLS, under HTTP/3 and MoQ. |
| WebTransport | (no expansion) | Browser API opening a bidirectional QUIC session over HTTP/3, how a page subscribes to MoQ. Accepts a certificate no CA vouches for by pinning its SHA-256, which bounds that certificate in exchange: no RSA key, and no validity beyond 14 days. |
| WebCodecs | (no expansion) | Browser API handing an encoded frame to a decoder directly, without a media element. What the MoQ page decodes with. |
| SDP | Session Description Protocol | Text format describing media streams, codecs and transport parameters. |
| ICE | Interactive Connectivity Establishment | Candidate gathering and connectivity checking that finds a working path between peers. |
| STUN | Session Traversal Utilities for NAT | Protocol a client learns its mapped public address:port from, and the binding checks ICE probes candidate paths with. |
| TURN | Traversal Using Relays around NAT | Relay a peer falls back to when no direct path survives, at the cost of carrying all media through it. |
| SSRC | Synchronization Source | 32-bit RTP identifier of one media source. |
| MPEG-TS | MPEG Transport Stream | Container of fixed 188-byte packets built for lossy links, the SRT payload here. |
| TS | Transport Stream | Short form of MPEG-TS, also the packet unit that `alignment=7` groups seven of per SRT datagram. |
| IVF | Indeo Video Format | Minimal container carrying a bitstream with one length-and-timestamp header per frame and nothing about colour, which is why the colorimetry tests decode through it. |
| WebM | (no expansion) | Matroska subset restricted to royalty-free codecs, the container name browsers associate with VP8 and VP9. |
| HLS | HTTP Live Streaming | Segment-and-playlist protocol the relay serves and does not ingest, so it is a watch leg and never a publish one. |
| RTMP | Real-Time Messaging Protocol | TCP protocol carrying FLV, the one broadcast tools speak. |
| E-RTMP | Enhanced RTMP | Extension adding codec tags past FLV's H.264, which is what carries H.265, AV1 and VP9 over RTMP here. |
| FLV | Flash Video | The container RTMP carries, whose original tag set is H.264 and AAC. |
| HTTP | Hypertext Transfer Protocol | Carries WHIP and WHEP signaling and the relay's HLS segments. |
| TCP | Transmission Control Protocol | Reliable ordered transport, the RTSP default here because interleaving needs no port beyond the one the session connected on. |
| UDP | User Datagram Protocol | Unreliable datagram transport, the basis of SRT and WebRTC media. |
| NAT | Network Address Translation | Router address rewriting. Return traffic reaches a private host only through a mapping one of its own outbound packets created, which is why RTSP's separate UDP port pair has to be punched open and WebRTC uses ICE. |
| Hole punching | (no expansion) | Sending outbound from the port that has to receive, so the NAT creates the mapping the far end's packets return through. |
| PTS | Presentation Timestamp | When a frame is displayed, carried by the container or the pipeline that framed it. |

## Capture backends

| Term | Expansion | Meaning |
| --- | --- | --- |
| Portal | xdg-desktop-portal | Sandboxed permission layer whose ScreenCast interface hands out a PipeWire capture node. |
| PipeWire | (no expansion) | Linux media routing daemon, the transport a portal capture's frames arrive on. |
| KMS | Kernel Mode Setting | Linux kernel display mode control, the surface `kmsgrab` reads scanout buffers from. |
| DRM | Direct Rendering Manager | The Linux kernel graphics subsystem containing KMS. |
| X11 | X Window System version 11 | Legacy Linux display protocol, captured with `x11grab` on ffmpeg and `ximagesrc` on GStreamer. |
| SHM | Shared Memory extension | X11 extension both X capture backends read screen contents through without a server round trip. |
| XWayland | (no expansion) | X11 server running on a Wayland compositor, which SDL is pinned to for the ffplay viewer window. |
| EGL | (no expansion) | The GL context and surface API a dmabuf is imported through on Linux, via `eglCreateImageKHR`. |
| DDA | Desktop Duplication API | Windows DXGI screen capture interface, reached through `ddagrab` on ffmpeg and `d3d11screencapturesrc` on GStreamer. |
| DXGI | DirectX Graphics Infrastructure | The Windows graphics layer DDA belongs to. |
| D3D11 | Direct3D 11 | The Windows graphics API `d3d11screencapturesrc` reads its textures through. |
| GDI | Graphics Device Interface | Legacy Windows drawing and capture API, reached through `gdigrab`. |
| AVF | AVFoundation | Apple's media framework, the macOS screen source of both engines: `avfoundation` on ffmpeg, `avfvideosrc` on GStreamer. |

A frame that stays on the GPU from capture to encoder crosses the bus not at all.
One that does not crosses it twice.

| Term | Expansion | Meaning |
| --- | --- | --- |
| DMA-BUF | Direct Memory Access Buffer | Linux kernel mechanism for sharing a GPU buffer between processes and devices by file descriptor. What a Wayland compositor exports capture frames as. |
| PRIME | (no expansion) | The DRM buffer-sharing framework DMA-BUF file descriptors are imported and exported through. |
| Frame memory | (no expansion) | Where a run's captured frames reach the encoder: the same device memory the capture produced, or a copy in system RAM. Four values, since a device path can be one whose conversion states the colour the form shows (`gpu`) or one where no such filter exists and the encoder converts by a colour of its own (`gpu-encoder-color`). The "Frame memory" dropdown. |
| VPP | Video Post-Processing | A driver's fixed-function scale, format and colour conversion block, reached as `vapostproc`, `scale_vaapi` and `vpp_qsv`. |

Two rates describe one publish, and they part company on a damage-driven backend.

| Term | Expansion | Meaning |
| --- | --- | --- |
| Capture rate | (no expansion) | How often the screen produced a new picture. A static screen and a capture path too slow to keep up both show up here. |
| Encoded rate | (no expansion) | How often the encoder emitted a frame. A backend that repeats the newest frame to hold a constant rate reports its target here whatever the screen does. |
| Damage | (no expansion) | A compositor's notification that a region of the screen changed, and the only thing that makes a portal capture emit a frame. |
| Duplicated | (no expansion) | Frames the encoder repeated to hold the output rate, the count that rises when capture or encode falls behind. |
| Dropped | (no expansion) | Frames discarded before the encoder for arriving faster than the output rate, a different event from a duplicate. |

## Playback

| Term | Expansion | Meaning |
| --- | --- | --- |
| SDL | Simple DirectMedia Layer | The output layer ffplay renders through. |
| appsink | (no expansion) | The GStreamer element a receive pipeline ends in, handing each decoded frame to Go instead of drawing it. |
| NT handle | (no expansion) | The Windows kernel handle a D3D11 texture is shared across processes by, paired with a keyed mutex. |
| Keyed mutex | (no expansion) | The D3D11 synchronization object two processes hand a shared texture back and forth with. |
| IOSurface | (no expansion) | The macOS shareable framebuffer VideoToolbox decodes into, and the frame channel's weakest import leg. |
| Fence | (no expansion) | Synchronization primitive saying when a producer's writes are complete. None crosses the frame channel: Windows pairs its handle with a keyed mutex, and the Linux export returns only once the copy has finished on the device. |

## Audio

| Term | Expansion | Meaning |
| --- | --- | --- |
| Opus | (no expansion) | Low-latency audio codec, the only one a WebRTC leg negotiates, and the default for the desktop track. |
| AAC | Advanced Audio Coding | The audio codec FLV has always carried, so it is the one an RTMP leg takes. |

## General

| Term | Expansion | Meaning |
| --- | --- | --- |
| Mux | Multiplex | Combining elementary streams into one container. |
| Demux | Demultiplex | Splitting a container back into elementary streams. |
| Remux | (no expansion) | Rewrapping into a different container without re-encoding. |
| FPS | Frames Per Second | Frame rate. |
| API | Application Programming Interface | A programmatic interface, used here mostly for the capture and encode APIs above. |
| GPU | Graphics Processing Unit | The device hosting the hardware encoder blocks. |
| CPU | Central Processing Unit | What the software encoders run on. |
| LAN | Local Area Network | The network a relay on the same site is reached over. |
| VPS | Virtual Private Server | A rented host, the usual home for a relay outside the LAN. |
