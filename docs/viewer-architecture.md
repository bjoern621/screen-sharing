# Viewer architecture

Watching a stream is pulling it from the relay and decoding it for display.
Three ways to watch, each with its own decoder.

| Way | Decode | Needs |
|---|---|---|
| single-stream native player | a player window per stream | ffmpeg and nothing else |
| the shell's tile grid | a receiving pipeline in the backend, its decoded frames reaching the window over the frame channel | the frame channel |
| the relay's own player page | the page fetches the stream itself, this side decoding and serving nothing | a browser |

Two more surfaces consume the frame channel without being a way to watch, neither picture reaching the relay: the broadcast screen's preview and the wizard's screen picker.

The tile grid runs where the frame channel's handle type is built, which is Windows and Linux.
On macOS a tile says so rather than falling back to a copy through system memory, and the native player covers that machine.

Which codecs exist and which protocol carries which format are `domain-model.md`.
The publishing side is `capture-architecture.md`, and what each stage costs is `delay-measurement.md`.

## Two legs, two protocols

A stream crosses the relay in two independent legs, and each names its own protocol, the relay re-serving every ingested stream on all its listeners.
A stream published over SRT is watched over RTSP by one viewer and over WHEP by another, at once.

The watch leg has three receivers: a player opening a URL, a receiving pipeline, and the browser reading the relay's page.
They reach different protocol sets, so the roster offers each receiver its own list.

The tile is the only receiver with a stored leg.
A player and a page are opened per press and neither persists, so a stored leg would be a value nothing reads.

Each protocol's knobs are per leg for the same reason.
The two SRT latency windows are separate fields, each leg its own link with its own retransmit window, and the glass-to-glass delay their sum.
The jitter buffer is the tile receiver's alone.

Every one of those fields is edited in the viewer and none in the publish wizard, and which screen draws a group is placement, the shell's.
The viewer's panel persists and starts nothing, where a wizard draft reaches the backend only on a start.

A running pipeline keeps what it was built with, so a leg or chain changed here reaches the next decode rather than the one on screen.

SRT negotiates one delay per direction and takes the larger of the two sides' values, so a link is never faster than the peer's setting.
The relay exposes no latency option and runs on its library's 120 ms default.
That is the floor of both hops against it: 400 ms is honoured, 60 ms comes back as 120.

## Where the bytes go

All three paths are one network hop from relay to decoder, and differ in what happens after the decoder.

```
native player

  relay ══ SRT or RTSPS ══════════════▶ player window
       └──── network ────┘ └ receiver machine ┘

shell tile

  relay ══ SRT, RTSP, WHEP or HLS ════▶ receive pipeline ──GPU handle──▶ window
       └──── network ────┘ └────────── receiver machine ──────────────┘

browser page

  relay ══ WHEP, HLS or MoQ ══════════▶ browser tab
       └──── network ────┘ └ receiver machine ┘
```

The native player needs no app process, so it survives a shell crash and works where frame import is unsolved.
The tile path puts the decode in the backend and the window in the shell, and the handle between them never leaves the machine.
Nothing is re-encoded on any path.

The browser address carries the relay token as its userinfo.
The relay's HTTP servers read a credential off a header and out of no query.
A page handed to the desktop sets no header,
so the one form a browser can carry is the password it builds itself.
The token is then in the address bar and in history, the cost of a page this side does not serve.

## What each path decodes

A native path decodes every stream this app publishes, so the watch leg limits it rather than the decoder.
A receiving pipeline autoplugs by rank, so a hardware element takes the stream wherever it advertises the profile and a software element takes the rest.
Nothing prefers software, and the publisher's pixel format decides which can take the stream.
No viewer here refuses a chroma or a bit depth.

What each decoder takes describes the machines watching rather than this one.
A stream is published once and watched on whatever the watchers have,
so nothing is probed and nothing restricts a choice.

Every fixed-function decoder covers 8-bit 4:2:0, and adds 10-bit wherever the format's own 10-bit profile is in silicon.
H.264 is the exception, its High 10 implemented by too few generations to declare, so a 10-bit H.264 stream costs a core.
Full chroma and RGB reach silicon in HEVC alone, through the Range Extensions profiles some vendors carry and others do not.
4:2:2 divides one step narrower: software decodes it in both H.26x formats, and one vendor's HEVC decoder is the only hardware element that does.
H.264 has no hardware 4:4:4 anywhere, which is also why lossless H.264 is a software decode everywhere, the mode existing only in that profile.

Audio follows the path.
A player plays the second track the mux carries.
A receiving pipeline grows a branch when an audio pad appears,
and that branch ends in a sink of its own rather than travelling to the shell.
The backend runs on the shell's machine,
so a second channel would carry samples across a process boundary to reach the same output device.

Which pad the branch grows from depends on how the leg carries its tracks.
A transport handing over one muxed stream leaves the separating to the decoder.
RTSP carries each track as its own stream, so the source hands out a pad per track.
The fragment pins one to the picture,
a decoder taking any caps otherwise taking whichever track the relay announced first.
The other track is decoded beside the picture, added to the running pipeline.
Without both halves an RTSP tile drops the audio at the source and plays silently while the relay reports two tracks.

Loudness is a property of the decode and not of a window: one pipeline holds one audio branch, so a per-window volume would be several controls over one element.
The level is measured **before** the volume, a stream muted and metered after appearing silent, leaving a reader unable to see it had started making noise again.

Levels leave on a subscription of their own rather than as an event kind.
The event stream carries whole states when something changed, where a level changes continuously, so folding it in would push the receive state at metering rate.
One tick is fifteen a second and carries the whole set, a decode with no audio track having no entry and a silent one reading negative infinity.

## The HLS tile's two detours

Every other watch leg writes its source from settings.
HLS asks the relay first, because the master playlist names each rendition under a session the relay mints per reader, and that address is answered 401 without it.

**What is opened is the video rendition.**
A stream carrying sound is announced as an audio rendition beside the video one, and the demuxer stalls before the first frame of a master that names one, where a player plays the same stream.
Opening the video rendition steps around it, at the price of the leg: an HLS tile is silent whatever the stream carries.

**A playlist whose muxer has just started carries gap placeholders in place of segments.**
The standard has a client skip them; the demuxer downloads one, is answered 401, and fails the pipeline.
So the resolve reads through until a real segment is in the playlist.
The wait is bounded, and a relay still serving gaps at the end of it is a refusal naming the playlist.

## The viewability verdict

A tile decodes whatever the machine decodes, so the leg is the only gate.
The verdict asks whether a receiving pipeline is served the codec's format over the tile's watch leg.
It reads the watch entry rather than the publish one:
a stream published over RTMP is one the same protocol will not hand back at anything but H.264.
A format with no listener on the selected leg reports not viewable, names the leg, and names the protocols that would carry it.

## The decode host

Every receiving pipeline plays in a child process, one for all of them, and the backend keeps the policy while holding no pipeline.

A GPU reset is why.
The kernel marks every context on the reset ring lost, the innocent ones as well, and the driver aborts a context that did not ask for robustness, which none of these do.
In the backend that would cost the control socket, the group membership and the publish supervision along with the picture.
In the child it costs the decodes, and the backend reports each one ending the way it reports a pipeline that stopped by itself.

One child rather than one per stream, for the same reason: a ring reset takes the innocent contexts too, so a child per stream would abort together and buy nothing.

**Pixels do not cross that boundary.**
A pool announces a socket the consumer dials for its handles, so the shell reads them from the child directly.
What crosses is the control calls one way and the frame events the other.

**The host owns which decodes exist.**
A decode that ended stays in the host's set carrying its reason until the backend stops it.
An entry dropped where it ended would take the reason with it, and the reason is what the tile shows.

The child ends when its control connection does, taking every pipeline down first, and the backend waits for that: a host killed mid-teardown leaves a decoder on the device.

## Render chains

Which elements sit between source and sink is a render chain, the decoder being the same on every row.
The rows differ in where the frames are converted and what that says about their colour.

| Chain | Converts in | Colour |
|---|---|---|
| `gl` | GL memory, RGBA and sRGB | stated |
| `cpu` | system memory, RGBA and sRGB | stated |
| `d3d11` | Direct3D 11 memory | the driver's |
| `d3d12` | Direct3D 12 memory, then a download | the driver's |
| `raw` | nothing | unstated |

**The default is per platform, and the frame channel is why.**
Only a chain leaving its frames in the memory this platform's handle names can produce one.
On Windows that is Direct3D 11, a shared texture being exported from a Direct3D resource and from nothing else, and that platform's OpenGL producing textures the shell's device cannot open.
It is also why that row ends in no download: pulling every frame into system memory for the exporter to push back onto the same GPU is the copy this table exists to avoid.
Everywhere else the default is GL.

That leaves the Windows default stating no exact colour, its converter free to pass the work to a video processor configured through an API the caps do not describe.
A reader who wants the colour stated picks the CPU chain and pays the download.

The GL chain earns its place by measurement.
Rendered through it and through the CPU chain, flat dark, flat bright and gradient content are bit-identical, and a saturated colour-bar frame differs by at most one code value per channel.
Dark content agreeing is the evidence that matters,
washed-out shadows being the failure the pinned sRGB caps prevent.
Without them the sink also takes YUV, and an unknown transfer function is read as BT.709, lifting every shadow.
What it saves is the download, which at 1440p144 in 4:4:4 is gigabytes a second per tile.

The chain is one value for every tile rather than one per stream, a chain falling back because a driver cannot run it, which is a property of the machine.

### Tone mapping

A stream carrying one of the BT.2100 curves carries more range than a standard display shows, and no chain converts a transfer function.
Rolling that range down is a rung of its own, built between the decoder and the chain where the frames still carry the range they were coded in.

| Rung | Where it runs |
|---|---|
| the driver's own tone-mapping filter | Linux alone, that being the one driver interface in reach stating such a filter |
| a shader | every platform, depending on no driver feature |

Whether the driver has the filter is a different question from whether the element registers, and it is the question the probe exists for.
The element registers wherever a driver loads at all, while the property appears only where the driver reports the capability, and one common open-source driver reports it on no generation.
A rung chosen by a registry lookup then builds a line the parser rejects, failing the decode instead of falling back.
Probing by parse is the same operation the pipeline performs, which stops the two from disagreeing.

The shader inverts the PQ curve, puts reference white at display white,
rolls what is above a knee into what is left,
converts the wide primaries to BT.709 and encodes sRGB.
A conversion ahead of it is not optional: the shader samples one RGBA texture where a decoder hands over planar YUV, and that conversion applies matrix and range and no transfer function, leaving the curve intact.
A download after it is what lets one rung serve every chain, passing GPU memory straight through where what follows accepts it.

The rung rolls PQ down and leaves HLG alone.
PQ is absolute, so an untouched PQ picture is wrong by the ratio between the display's peak and the format's ten thousand nits.
HLG is display-referred and its lower range tracks a standard gamma curve, so an HLG stream drawn as it arrives is approximately right.

The software converter substitutes for neither, measured rather than assumed: it normalizes PQ against the format's ten thousand nits rather than the display's hundred, and a mid-grey frame comes out at a fifth of the code value it went in at.
A darker picture is not a tone map.

The choice is per tile, asked for when the decode is opened because it is part of what the decode is built from, and stored nowhere.
A preference kept per stream path would outlive the stream it was made about.

A machine with no rung builds the decode without one and reports that it did, and the receive state carries the transfer the stream turned out to have beside it.

## What a tile reports

A decode answers two different questions and the contract asks them separately.

What a decode **is** settles when the pipeline negotiates and is announced whenever it moves: chain, decoder, memory at each end, transfer characteristic, whether the range is rolled down.

What a decode **is doing** never settles, so it is read off the running pipeline on a clock and pushed as its own event: what is arriving and at what rate, what came out of the decoder, what the sink took and threw away for being late, and the counters the transport's own elements keep.

**The rates are computed here rather than by each shell.**
A rate is a delta divided by an interval, and an interval each reader chose would make one decode read differently in two windows.
The interval is the difference between two readings of the pipeline's own uptime rather than the ticker's period, so a tick the scheduler held back divides its delta by the time that passed.
A rate is absent on the first sample of a run and on the first after a rebuild, a zero there saying a stream is arriving at nothing.

**The counters cross as identifiers and figures**, the element's own field name reaching a shell and what it is called on screen being the shell's.
A shell with no word for a key shows the key, which is what gets the word written.

The one figure the backend cannot supply is what the window did with the frames.
A compositor too slow to take one is invisible from this side: the backend sees a slot that has not come back, and the shell is the only place that becomes a count.

### The pointer a tile draws

A publisher whose cursor mode sends the position instead of drawing it publishes frames with no mouse in them, so the decode is what can answer for it.
The position reaches the shell on a subscription naming the stream, followed on the token the frame channel is opened on, so a tile that stopped drawing stops asking.
The read is slower than the capture's own leg, the position changing once a frame however often it is asked for.

The marker is placed on the picture rather than on the card, the host letterboxing to the stream's shape, so a fraction of the card would drift into the bars.

## The frame channel

Frames never cross the control API.
The frame channel is a second service on the same socket, carrying handle metadata and release-backs, and the pixels stay in shared GPU memory the handle names.

| Platform | Handle | Leg |
|---|---|---|
| Windows | a shared texture with a keyed mutex, which the compositor imports | built |
| Linux | a descriptor per slot, exported from the render chain's own textures | built |
| macOS | the window server's surface, with no first-class import handle type | unbuilt |

On Windows the globally shared handle is used rather than the process-scoped one, which would have to be duplicated into the shell's process and so would need this backend to open it.
The two are halves of one application on one box, so the less privileged form is the one used.

### The Linux leg

The pool is textures and the descriptors are what leaves.
Each slot is named once and exported, yielding the descriptor, stride and offset the contract carries per slot, and the format and modifier it carries per pool.

**A descriptor cannot travel in a message.**
It indexes one process's table, so the pool names a socket and the consumer reads one descriptor per slot over it, in index order.
The socket answers the same set for as long as the pool lives, so reading it is repeatable rather than a handshake that happened once.

**A modifier that names nothing is passed on as such.**
The driver exports these textures saying it picked the layout rather than that the layout is linear, so the pool carries that value and the import states no modifier for it, which is how the two sides agree on a layout neither can spell.

**Both halves have to be on the same graphics binding.**
The older one exports nothing and imports nothing here, so each side asks for the newer one explicitly.
A window that ends up on the older one draws a tile saying the frames cannot be opened, the refusal every unbuilt leg makes.

**The copy is waited for rather than fenced.**
The contract carries no fence for a descriptor handle, so the export finishes the copy on the device before the frame is announced.
That is what makes a ready frame mean the pixels are there.

**The shell draws rather than hands over.**
The compositor imports a shared texture and an opaque descriptor and not this handle type, so the tile's surface imports the descriptor itself and draws it.
Still a composition visual rather than a native child window, so what is drawn over a tile stays over it.

### The buffer-ownership protocol

**The backend owns the memory and lends it.**
Each subscription gets a pool of its own, three buffers on the device the decoder is already using, announced once and imported slot by slot.
Two tiles on one stream are two pools and two copies rather than one buffer with two owners, which stops a slow tile from holding a slot the other is waiting for.

**Each frame is a loan.**
A decoded frame is copied on the GPU into a free slot and the slot is the consumer's until a release comes back on the same call, which the shell sends only after the compositor has taken the texture.
A release on a second call could outlive the subscription it belongs to, so the channel is bidirectional.

**A consumer that is slow costs frames.**
With every slot on loan there is nowhere to put the next decoded frame, so it is dropped and counted.
Nothing blocks: every step either succeeds now or is a dropped frame.

**A pool is re-announced when the pipeline renegotiates**, a slot being allocated at one size and a picture of another size not fitting.
Each pool carries a generation and releases carry it back, so one naming a pool that is gone is discarded rather than freeing a slot of the pool that replaced it.

**Either side dying is the same teardown.**
The call ending frees the pool, so a shell that crashed costs this process nothing.
The pipeline ending sends an end on the call the consumer is blocked reading, a window waiting for frames learning nothing from an event it is not reading.

**The render size travels on this channel rather than the control one**, a count of pixels a consumer will draw at being a fact about frames.
The pipeline takes the largest of its consumers' asks, rendering at the largest meaning the smallest tile scales down at draw time.

It is quantised and debounced, both about this channel's cost.
A shell whose grid rearranges moves every tile's exact size, so it rounds each ask onto a ladder and sends it only once the size has held still.
What is paid is a tile between two rungs drawing frames slightly larger than it needs, a resample the GPU was doing anyway.

## What the broadcast preview draws

The broadcast screen's preview draws the stream this machine is publishing, by one of two routes, and the card's toggle is which, or off.

The **local** route is a copy that never leaves the machine.
The **end-to-end** route reads this machine's own stream back off the relay like any other tile, so it crosses the uplink, the relay and the way back.
They differ by where the picture is taken and by nothing else, so what one shows and the other cannot is everything downstream of the encoder.

**The two costs are opposite, which is why it is a choice.**
The local route costs one decode here, spends no bandwidth and takes no reader slot, so the viewer count and worst-viewer round trip describe viewers rather than this machine watching itself.
The end-to-end route is a relay client: it occupies a reader slot, is counted among those figures, and pays a viewer's downstream bandwidth.
So the card opens on the local route and the other is asked for by name, each stating its own cost on screen.
The third segment is off, which is why it stands on the same toggle rather than under a control of its own.

**The constraint that shapes it is where the encoder runs.**
Publishing is an external child, which keeps a pipeline that dies from taking the backend with it.
So there is no in-process pipeline to hang a sink on, and nothing this process can do to the encoded stream before the child has sent it somewhere.
What both engines can do is send it twice, off one encoder, the second copy going to a loopback port this side reads.

The local carriage is RTP over loopback, for the reason RTSP carries the whole codec table: a payload format for every format this app publishes, and a depayloader that reads it back.
MPEG-TS would have needed no caps and would have carried the two H.26x formats alone.

The payloader the child is given and the caps the receiving pipeline is built with have to agree on a payload type and an encoding name, and no session protocol here negotiates one, so one process writes both ends.

**The port is allocated per run and reported**, so a reader can see it and nothing assumes it.
It stays out of the rendered command, which is what the rebuild decision compares.

**The publish opens it and the publish closes it.**
The port has to be in the child's arguments, so the decision belongs to the launch, and there is nothing for a shell to call.
Both halves are idempotent, every path that ends the child ends the preview, and a preview that fails to come up leaves the stream running.

**The local picture gives up the delivery half, and the card says so.**
It shows what is being sent and nothing about what anybody receives, so a congested uplink, a relay dropping packets and a viewer on a bad link all leave it looking perfect.

**One decode serves every window drawing it**, keyed by the stream and the leg, so a tile in the grid on the same pair is the same pipeline.
The preview reads the grid's answer through before closing anything, and asks again for a decode it saw running and does not find, making a pipeline another window closed a blink rather than a card that stays dark.

**The card opens drawing and follows no window.**
A publisher's window stands behind the thing being shared for most of a session, so a card that stopped whenever nobody was looking would be dark at the moment a reader came back to check on it.

## What the screen picker draws

The wizard's source step offers a picture of every monitor, so a screen is chosen by looking at it.
It is the third consumer of the frame channel and the only one that decodes nothing.

It is the same rectangle the stream would carry, both being built from one head, a preview cropped differently being a picture that lies about what is shared.

**A preview is asked for, unlike the publish's.**
A publish preview exists because a publish does, where a screen is read because somebody wants to look at it, which no other state implies.
The ask is idempotent in both directions and the running set is announced to every shell.

**Previews outlive the window that asked for one, exactly as decodes do**, which is what makes the set worth announcing: a shell that restarted reads it and closes what nothing is drawing.
The shell's own rule is narrower, opening them while the reader stands on the source step with the window in front.

**The pacing and the size are the preview's own.**
Five frames a second is what tells one screen from another.
The size is a bound the scaler fixates inside, and the reduction happens at the source rather than in the render chain: a preview that did not reduce its own frames would upload whole desktops for a picture drawn at a fraction of one.

**Where a session cannot read one output apart from another there is no picture and the catalog says so**, so the wizard offers the plain list instead of opening captures that would all be refused.

## The native player

The player opens a URL built from the transport's watch form, the leg passed by name rather than read off the publish setting, keeping a viewer free to receive over a protocol the stream was not published with.
A viewer is identified by stream name and transport together, one stream being open over several at once.
A transport whose playback is an exchange rather than an address is reachable by a receiving pipeline and by no player here.

The default player is pinned to the X11 backend, whose window a compositor renders reliably where the Wayland one may not.
The alternative renders 4:4:4 and a native Wayland window.

## The relay's page in a browser

The address of the relay's own page is handed to the desktop, which opens it the way it opens a log file.
No viewer program to find and no pipeline to supervise: the relay serves a page per listener, and the page runs the exchange, fetches the playlist or subscribes itself.

MoQ is the one leg with a demand on the watcher's own network, its page arriving over TCP and its media over HTTP/3 on the same port, so a network passing the first and dropping the second loads a player that never plays.

**It is the viewer with no dependency of its own.**
A player needs ffmpeg or mpv and a tile needs the frame channel.
A page needs a browser, and the same address opened by hand is what a watcher without this app uses.

**Nothing about it is a state, and the interface says so.**
A tab belongs to the browser, so this process can neither read whether it is still open nor close it: no stop, no member on the viewer state, and the menu rows carry no tick where every other row on that surface does.
A second press opens a second tab, the departure from idempotency the effect is written down as.

A leg the stream's format does not cross is refused with the format named.
What a browser then does with a format the relay does carry is its own affair, so the carriage states the relay's set and no narrower one.

## The synthetic set

Three synthetic publishers run for as long as the backend does, so the viewer roster carries streams whether or not this machine is capturing anything.
Each encodes a test pattern into the relay, named after the slot it holds, and the relay re-serves each on every listener as it does a real one.

What each draws is a row of a table stating the whole surface rather than the pattern alone: the pixel layout and the colour it is drawn in.
One row is HDR, drawn in PQ at ten bits, so the viewer's HDR path is reachable on a machine whose own screens are all standard range.
That row is the one the browser page cannot decode, which is why the rest of the set stays 4:2:0.

One row sounds, drawing its track from a test source and coding it with the elements a real publish uses.
Pink noise at a fifth of full scale reaches a meter at about -30 dBFS, the meter wanting a signal that is there continuously where a tick a second leaves it reading silence between ticks.
The other rows stay silent, so the volume a tile carries, the meter beside it and two streams playing at once all have something to be compared against.

Measured through a running relay: the HDR row is received carrying PQ at ten bits, so a tile draws it as HDR and offers the tone-map choice rather than merely looking bright, and the sounding row is received with its track beside the picture.

They are always on because the screens that watch are built against them.
A relay carrying nothing puts the roster in its empty state rather than the one under construction.

The slot is the stream's identity rather than the process's, so a publisher that dies is relaunched into the slot it held and returns as the row the roster already shows.
The wait walks 2, 4, 8, 15 and 30 seconds and then holds, with no attempt budget unlike the publish leg's: what is waited out is usually the relay, which this process starts before and outlives, and giving up would leave the roster empty for the rest of the run over an outage that ended a minute in.
The exit reaches the session log once per outage rather than once per attempt.

The count is settable for one run and zero turns the set off, three encoders not being free and a machine measuring its own encode having reason to want them gone.
