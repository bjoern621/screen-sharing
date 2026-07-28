# Native grid

The native stream grid: an Adwaita window with a retractable stream sidebar and one video tile per watched stream, each decoding through its own GStreamer pipeline into a `gtk4paintablesink` paintable.
It is a separate GTK4 binary because the app's webview process is GTK3, and the two toolkits cannot share a process.
Styling maps the web design language (`docs/design-language.md`); the icons are the web app's Tabler set, vendored as SVGs.

## Running

The app spawns the built binary from its "Native grid" button (`desktop/app_nativegrid.go`), passes the stream list as one JSON argument, and pushes the full list again as one JSON line on stdin whenever it changes.
The list may be empty; the sidebar shows a placeholder until streams appear.
The window writes on stdout only to ask for a watch leg, so everything it logs goes to stderr.
`task nativegrid` builds it into `desktop/build/bin`, where the app looks for it.

```
nix develop .#nativegrid --command go run ./nativegrid
```

Without `-config`, built-in `videotestsrc` streams drive a standalone demo run, including one broken source to exercise the failure path and one with a muxed sine tone to exercise the audio path.
The demo patterns are H.264 rather than raw, because a raw stream exercises none of the figures the stats overlay reads off the encoded side.
`-player` picks the decode backend; `LOG_LEVEL` (`NONE`, `WARN`, `INFO`, `DEBUG`, `TRACE`) sets how much the run reports.

The `nativegrid` shell exists because the app's own shell carries neither gtk4 nor libadwaita.
It also carries GStreamer core for go-gst's cgo build and exports the plugin path, so the demo decodes from this shell directly.

## Measuring the render path

`bench` runs N pipelines off one relay stream into N `GtkPicture`s and prints, once a second, the frames each tile drew, what its sink rendered and dropped, and the widget's frame-clock rate.
The frame-clock line is the ceiling any render rate is read against: an occluded or throttled window ticks slower than the monitor, and a measurement taken under one is not a measurement of the chain.

```
nix develop .#nativegrid --command go run ./bench -chain shipped -streams 4
```

`-chain shipped` plays the line `gstreamer.Describe` renders, so it cannot drift from what a tile runs; the other chains are alternatives to compare it against, each carrying the shipped queue so a comparison is a comparison of their conversions.
`-sync`, `-qos` and `-lateness` expose the base-sink knobs, since a render rate under a syncing sink is not the same measurement as one without.

## Layers

One model, two views of it, and a decode seam under both.

```
                  main
                    │  builds the model, opens the window
        ┌───────────┴────────────┐
        │  internal/session      │  what is watched, in what order, what is spotlit
        └───┬────────────────┬───┘
   Observer │                │ player.Factory
   ┌────────┴─────────┐  ┌───┴──────────────────┐
   │ internal/ui      │  │ internal/player      │
   │ sidebar · grid   │  │ gstreamer backend    │
   │ tile · dnd       │  └──────────────────────┘
   └──────────────────┘
```

- `internal/session` decides and remembers: the watch set, the display order, the spotlight, and the player behind each watched stream.
  It holds no widget. A view subscribes as an `Observer`, is told what changed, and reads the model back, so the tile area and the sidebar cannot drift apart.
  Player callbacks arrive on pipeline threads and are hopped to the UI loop, where a generation counter drops the reports of a player an unwatch or a retry has already replaced.
- `internal/player` is the decode seam: `Player` exposes the sink's `GdkPaintable`, the volume controls, and the figures the stats overlay reads, and `Events` reports the first frame, the audio branch coming up, and a fatal error.
  Backends register themselves under a name, so another decoder is one package and one registration; `internal/player/gstreamer` is the one that ships.
- `internal/ui` draws: `sidebar` and `grid` are the two views, `tile` is one stream's tile, `stats` its overlay, `dnd` the reordering both views share, `theme` the stylesheet, icons and colors, `widgets` the controls they have in common.
- `internal/roster` is the process contract with the app, `internal/layout` the arrangement kept across runs, `internal/idle` the deferral both views use to keep a relayout out of a drag callback.

## Remembered state

`internal/layout` keeps the watch set, the display order and the spotlit stream in `nativegrid.json`, in the same config directory as the app's `settings.json`.
Streams are keyed by name, so the window reopens on the streams it was watching, in their order, and a stream that only shows up in a later roster push still lands in its remembered slot and gets watched when it does.
The order also remembers streams the current roster does not carry, which keeps their place for the run they come back in.
The `Store` seam separates what is remembered from where it is kept, so a test drives the model against an in-process store.

## The decode seam

- `internal/roster` parses the roster JSON, both the `-config` argument and each stdin push: one entry per live stream, the name of the watch leg it arrives over, and that transport's gst-launch source fragment.
  The producing half is `watch.BuildGridConfig` (`desktop/watch/grid.go`); the fragment comes from the transport registry (`transport.GstWatcher`), so this binary holds no transport knowledge.
  The transport name is a label for the stats overlay, nothing this side acts on, and it is always the relay-to-viewer leg: how the stream was published is not visible here.
- An entry also carries the legs that stream could move to and the knobs of the one it is on, which the sidebar's watch-leg popover renders one control per, without knowing what any of them mean.
  Moving a stream is a `roster.Request` on stdout, the whole leg for that one stream; the app decides what it means and answers by pushing the roster it produced.
  Nothing changes here until that push arrives, and a watched stream whose source fragment moved with it restarts on the new one.
- The GStreamer backend completes that fragment with `decodebin ! videoconvert ! RGBA/sRGB ! queue ! gtk4paintablesink`, so it plays everything a native ffplay/mpv window plays, HEVC 4:4:4 and RGB included.
  `decodebin` autoplugs by rank: a hardware decoder takes the stream where its sink caps advertise the profile, and a software one (gst-libav for H.264 and HEVC, libvpx for VP9) takes the rest, which is what covers the 4:4:4 and high-bit-depth combinations no hardware element lists.
  When decodebin exposes an audio pad, the pipeline grows an audio branch (`queue ! audioconvert ! audioresample ! volume ! autoaudiosink`) while it plays; a video-only stream carries no idle audio elements.
  The `RGBA/sRGB` capsfilter is not optional: without it GTK color-manages the raw YUV itself and washes out dark screen content.
  The queue between conversion and sink decouples the decode thread from the render thread and does not leak.
  A leaking queue in front of a sink that syncs on the clock sits at its bound for most of every frame period, because the sink holds each buffer until its presentation time, so every arrival drops a frame that was about to be shown and the tile renders at a fraction of the rate the source sends.
  Frames that really are too late are dropped once, by the sink, which is the element that knows what late means.
- `Stats` reads the running pipeline rather than remembering it: caps off the decoder's input, off the decoded frames entering `videoconvert` and off the sink, the sink's own rendered/dropped counters, a latency and a position query, and byte counters a pad probe on each decoder's input fills.
  It reports counters, not rates, so the poll interval stays the overlay's business.
  Elements the launch line does not name are found through the pipeline's `deep-element-added` and a walk of the elements parse-launch already built, which is how the decoders inside decodebin and the transport's own source turn up.

## Media controls

Hovering a tile fades in the web tile's control bar: mute with a hover-out volume slider (only when the stream carries audio), the stats overlay, spotlight, disconnect.
Spotlight swaps the grid for the web grid's layout: the spotlit tile fills the page and the other watched streams shrink to a centered film strip below it.
Pop-out stays web-only because the grid already is its own window; hide-video stays web-only because it needs the roster's audio-only strip.

Tiles reorder by drag and drop with a live preview: the other tiles re-slot while the pointer moves, and the sidebar rows follow the same order.
One drag controller serves both views, so a drag started on a row moves the tiles and the other way round, including for a stream nobody watches yet.

A sidebar row carries the watch-leg button beside its check: the transports the app offers for that stream and the knobs of the one it is on.
It applies on Apply rather than per keystroke, because a change reconnects that stream, and closing the popover any other way puts the values that hold back into the controls.
A stream the app offers nothing for shows no button.

## Stats overlay

The overlay is a table of rows (`internal/ui/stats`), which both the card's widgets and every refresh walk, so a row is described once: its key, whether it disappears while its figure is missing, how it reads the poll, and the tooltip that says what the figure means.
It is blocked by where a figure comes from: the `stream` it plays (the watch leg it receives over, source fragment, uptime, running time, latency window), the `video` on the wire (picture size and rate, codec description with profile and level, measured bitrate, keyframe spacing, pixel format with its subsampling and bit depth, colorimetry, pixel aspect and scan mode), what this side does with it under `decode` (the decoder decodebin picked and whether it decodes on the GPU, the format the sink takes, measured fps, rendered and dropped frames), and `audio` when the stream carries it.
Codec names come from `pbutils`, subsampling and bit depth from GStreamer's raw-format table, so neither is a table in this binary.

A block per transport element follows, keyed by the element's pipeline name, from the `stats` structure elements like `srtsrc` and `rtpjitterbuffer` keep: packet, loss and retransmission counters, and an SRT link's rate and round-trip time.
`statSources` in the GStreamer backend names the fields to show and what each counts, which reaches the card as `Tip` on the labelled row; a field an element does not report is skipped, so a key that table has wrong costs its own row and nothing else.

Rows that only some streams have disappear while their figure is missing; the rest hold their place and show a placeholder, so the card does not jump around while a pipeline negotiates.
The card scrolls inside the tile, because the whole set is taller than a tile in a dense grid.
