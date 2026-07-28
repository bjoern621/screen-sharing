# Native grid

The native stream grid: an Adwaita window with a retractable stream sidebar and one video tile per watched stream, each decoding through its own GStreamer pipeline into a `gtk4paintablesink` paintable.
It is a separate GTK4 binary because the app's webview process is GTK3, and the two toolkits cannot share a process.
Styling maps the web design language (`docs/design-language.md`); the icons are the web app's Tabler set, vendored as SVGs.

## Running

The app spawns the built binary from its "Native grid" button (`desktop/app_nativegrid.go`), passes the stream list as one JSON argument, and pushes the full list again as one JSON line on stdin whenever it changes.
The list may be empty; the sidebar shows a placeholder until streams appear.
The app state the sidebar's foot draws travels in the same line.
The window writes on stdout only to ask the app for something and to report what it watches, so everything it logs goes to stderr.
`task nativegrid` builds it into `desktop/build/bin`, where the app looks for it.

```
nix develop .#nativegrid --command go run ./nativegrid
```

Without `-config`, built-in `videotestsrc` streams drive a standalone demo run, including one broken source to exercise the failure path and one with a muxed sine tone to exercise the audio path.
The demo patterns are H.264 rather than raw, because a raw stream exercises none of the figures the stats overlay reads off the encoded side.
`-player` picks the decode backend; `LOG_LEVEL` (`NONE`, `WARN`, `INFO`, `DEBUG`, `TRACE`) sets how much the run reports.

The `nativegrid` shell exists because the app's own shell carries neither gtk4 nor libadwaita.
It also carries GStreamer core for go-gst's cgo build and exports the plugin path, so the demo decodes from this shell directly.

On Windows the same dependencies come from MSYS2 instead, and `task nativegrid` builds from its MINGW64 shell.
The binary is built for the GUI subsystem there, so the app spawning it opens no console window beside the grid.
See `docs/packaging.md`, "Windows", for the packages and for the runtime the bundle has to carry.

## Measuring the render path

`bench` runs N pipelines off one relay stream into N `GtkPicture`s and prints, once a second, the frames each tile drew, what its sink rendered and dropped, and the widget's frame-clock rate.
The frame-clock line is the ceiling any render rate is read against: an occluded or throttled window ticks slower than the monitor, and a measurement taken under one is not a measurement of the chain.

```
nix develop .#nativegrid --command go run ./bench -chain shipped -streams 4
```

`-chain shipped` plays the line `gstreamer.Describe` renders, so it cannot drift from what a tile runs; the other chains are alternatives to compare it against, each carrying the shipped queue so a comparison is a comparison of their conversions.
`-sync`, `-qos` and `-lateness` expose the base-sink knobs, since a render rate under a syncing sink is not the same measurement as one without.
`-fit` bounds the scaler to the tile's size where the chain carries one, so running a chain with it on and off is the measurement of what rendering at tile size costs and saves.

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

The window's own shape is in the same file: its size, whether it was maximized, and whether the sidebar was shown.
Fullscreen is not, because it is a mode rather than a shape, and a grid that reopens covering the app that spawned it is a window nobody asked for.
The file has two owners, the model and the window, so a write replaces the keys of the record being written and carries every other key over untouched, including one a later version put there.

## The decode seam

- `internal/roster` parses the roster JSON, both the `-config` argument and each stdin push: one entry per live stream, the name of the watch leg it arrives over, and that transport's gst-launch source fragment.
  The producing half is `watch.BuildGridConfig` (`desktop/watch/grid.go`); the fragment comes from the transport registry (`transport.GstWatcher`), so this binary holds no transport knowledge.
  The transport name is a label for the stats overlay, nothing this side acts on, and it is always the relay-to-viewer leg: how the stream was published is not visible here.
- An entry also carries the legs that stream could move to and the knobs of every one of them, which the sidebar's watch-leg popover renders one control per, without knowing what any of them mean.
  Holding all of them is what lets the popover swap its controls the instant another leg is picked, instead of waiting for the app to say what that leg offers.
  Moving is a `roster.Request` on stdout, the whole leg: the transport and the values of the knobs shown with it.
  The app decides what it means and answers by pushing the roster it produced.
  Nothing changes here until that push arrives, and a watched stream whose source fragment moved with it restarts on the new one.
- Stdout carries a second kind of line, told apart from the first by a `type` field: the names of the streams with a tile open, stated whenever that set changes.
  It is one-way, and the app has no answer to it: what the window watches is the window's, and the report only lets the app say what is on screen.
- The GStreamer backend completes that fragment with `decodebin ! videoscale ! capsfilter ! videoconvert ! RGBA/sRGB ! queue ! gtk4paintablesink`, so it plays everything a native ffplay/mpv window plays, HEVC 4:4:4 and RGB included.
  `decodebin` autoplugs by rank: a hardware decoder takes the stream where its sink caps advertise the profile, and a software one (gst-libav for H.264 and HEVC, libvpx for VP9, dav1d for AV1) takes the rest.
  The hardware decoders rank above the software ones, so which of them takes a stream follows the pixel format the publisher chose rather than a preference here.
  `capabilities.Decoders` in the app states that per element: every hardware decoder covers 4:2:0 at both bit depths, HEVC's 4:4:4 and RGB profiles are NVDEC's and Intel's alone, and H.264 4:4:4 is nobody's, which leaves those combinations on the software path.
  When decodebin exposes an audio pad, the pipeline grows an audio branch (`queue ! audioconvert ! audioresample ! volume ! autoaudiosink`) while it plays; a video-only stream carries no idle audio elements.
  The `RGBA/sRGB` capsfilter is not optional: without it GTK color-manages the raw YUV itself and washes out dark screen content.
  The scaler ahead of the conversion is what keeps a tile from converting more pixels than it draws: a thumbnail in the film strip would otherwise convert every frame of a 4K stream to RGBA to draw it at thumbnail size.
  It scales in the format the decoder produced, well under the four bytes a pixel the conversion works in, so the cheaper operation runs on the larger picture.
  The tile bounds it through the `capsfilter` behind it, as a range rather than a fixed size, so the scaler corrects the pixel aspect instead of adding borders and a tile larger than its stream negotiates the stream's own size.
  The size comes from the widget, over the `player.RenderSizer` seam, and a backend that cannot resize its output does not implement it and renders as it always did.
  The sink's `reconfigure-on-window-resize` is not that mechanism: its `window-width` and `window-height` reach upstream only through the allocation query and never constrain caps, so a chain relying on it scales nothing.
  The queue between conversion and sink decouples the decode thread from the render thread and does not leak.
  A leaking queue in front of a sink that syncs on the clock sits at its bound for most of every frame period, because the sink holds each buffer until its presentation time, so every arrival drops a frame that was about to be shown and the tile renders at a fraction of the rate the source sends.
  Frames that really are too late are dropped once, by the sink, which is the element that knows what late means.
- `Stats` reads the running pipeline rather than remembering it: caps off the decoder's input, off the decoded frames entering the scaler and off the sink, the sink's own rendered/dropped counters, a latency and a position query, and byte counters a pad probe on each decoder's input fills.
  The decoded caps are read ahead of the scaler on purpose: behind it they would report the tile's size as the picture on the wire.
  It reports counters, not rates, so the poll interval stays the overlay's business.
  Elements the launch line does not name are found through the pipeline's `deep-element-added` and a walk of the elements parse-launch already built, which is how the decoders inside decodebin and the transport's own source turn up.
- Taking a pipeline down runs under a main context of the stopping thread's own (`withOwnMainContext`).
  gtk4paintablesink hands the paintable's share of that state change to the default main context, and GLib runs such a hand-off inline on any thread that can acquire that context instead of queueing it.
  The paintable belongs to the UI loop, and the sink's Rust half aborts the process over one touched from another thread.
  A window closing is exactly that case: it stops every pipeline at once, on threads of their own, after the loop that held the context has returned.

## Losing a stream

A receive pipeline that ends takes its stream into a reconnect rather than straight into a failure: the tile keeps the last frame it drew, and the model reopens the pipeline on a backoff that ends.
The budget is spent per outage and refilled by a frame arriving, so a stream that flaps comes back on its own and one nobody publishes any more lands in the failure state instead of retrying forever.
A stream the roster drops and lists again restarts as well, because whatever took it away killed its pipeline, and the tile would otherwise hold a failure message for a stream that is back on air.
The tile shows what the element reported, in its own wording.

A stream that stops sending without ending is the other way to lose one, and nothing in the pipeline reports it: the elements stay healthy and the sink keeps its last picture.
The model reads the frame counter of every live player instead, and marks the stream once the count stops moving, which the tile and its sidebar row both say.

## Media controls

Hovering a tile fades in the web tile's control bar: mute with a hover-out volume slider (only when the stream carries audio), the stats overlay, spotlight, disconnect.
Spotlight swaps the grid for the web grid's layout: the spotlit tile fills the page and the other watched streams shrink to a centered film strip below it.
A double click on a tile spotlights it too.
Pop-out stays web-only because the grid already is its own window; hide-video stays web-only because it needs the roster's audio-only strip.

Outside the spotlight the tile area picks the column count that leaves a 16:9 tile the most area in the window it has, so three tiles are one row on a wide window and one column on a tall one, and an incomplete last row sits centered under the full ones.

Hiding the sidebar hands the tile area the width the sidebar held and the margin the grid keeps under a frame, so a tile meets the window on the sides its aspect fills.
The space between tiles is not that margin and stays.
The header does not move with it: it is the frame the tiles sit under whatever the sidebar is in, so the sidebar toggle and the window buttons are always where they were.
What shape the window itself is in is separate again: maximize sits beside close in the header, and F11 takes the screen.

Tiles reorder by drag and drop with a live preview: the other tiles re-slot while the pointer moves, and the sidebar rows follow the same order.
One drag controller serves both views, so a drag started on a row moves the tiles and the other way round, including for a stream nobody watches yet.

A sidebar row carries the watch-leg button beside its check: the transports the app offers for that stream and the knobs of whichever one the dropdown shows.
Picking another transport swaps the knobs at once, since the entry declares a set per leg, so the controls that Apply reads always belong to the leg it names.
It applies on Apply rather than per keystroke, because a change reconnects the tiles, and closing the popover any other way puts the values that hold back into the controls.
The leg is the app's setting rather than this stream's: the row decides which transports may be offered, and what Apply changes is the window's own leg, which the app saves.
A stream the app offers nothing for shows no button.

## The app bar

Under the rows sit the two controls that act on the app rather than on a stream: its window to the front on the settings form, and this machine's own publish on or off on the settings the app holds.
Both are commands on stdout, and the app answers each with a push, so the button draws the state that holds rather than the one it asked for.
The publish commands name the state they want instead of toggling, which is what keeps a button drawn from an overtaken push from flipping the state the other way; a refused command comes back with its reason, under the button that sent it.
A run with no app behind it receives no app state and draws no bar.

## Keyboard

Escape peels one thing per press, fullscreen before the spotlight, so a window in both takes two.
F11 gives the window the screen, Ctrl+B shows or hides the sidebar, and the digits 1 to 9 spotlight that tile of the grid, the same digit again dropping the spotlight.
The bindings are one table, and the sidebar toggle's tooltip is composed from the row that drives it, so a binding is described where it is declared.
They act in the bubble phase, so a digit typed into a watch-leg field stays that field's.

## Stats overlay

The overlay is a table of rows (`internal/ui/stats`), which both the card's widgets and every refresh walk, so a row is described once: its key, whether it disappears while its figure is missing, how it reads the poll, and the tooltip that says what the figure means.
It is blocked by where a figure comes from: the `stream` it plays (the watch leg it receives over, source fragment, uptime, running time, latency window), the `video` on the wire (picture size and rate, codec description with profile and level, measured bitrate, keyframe spacing, pixel format with its subsampling and bit depth, colorimetry, pixel aspect and scan mode), what this side does with it under `decode` (the decoder decodebin picked and whether it decodes on the GPU, the format the sink takes and its size while that differs from the decoded one, measured fps, rendered and dropped frames), and `audio` when the stream carries it.
Codec names come from `pbutils`, subsampling and bit depth from GStreamer's raw-format table, so neither is a table in this binary.

A block per transport element follows, keyed by the element's pipeline name, from the `stats` structure elements like `srtsrc` and `rtpjitterbuffer` keep: packet, loss and retransmission counters, and an SRT link's rate and round-trip time.
`statSources` in the GStreamer backend names the fields to show and what each counts, which reaches the card as `Tip` on the labelled row; a field an element does not report is skipped, so a key that table has wrong costs its own row and nothing else.

Rows that only some streams have disappear while their figure is missing; the rest hold their place and show a placeholder, so the card does not jump around while a pipeline negotiates.
The card scrolls inside the tile, because the whole set is taller than a tile in a dense grid.
