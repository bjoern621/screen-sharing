# Planned work, and the decisions behind it

Design decisions for work that is not built yet, and the order it is built in.
The one page here whose subject is change, and meant to shrink: as a piece lands, its reasoning moves into the topic page it belongs to and the section here goes.

Reasoning that already moved to a topic page is pointed at rather than repeated.
Rule system: `domain-model.md`, "One evaluator".
Pointer: `capture-architecture.md`, "The pointer".

## Two tracks

A contract track and an infrastructure track.
They meet only where group fields enter the settings model, so they run in either order or at once.

**Contract track.** Effort and tune, then the audio list, then HDR.
Each is a settings field or a set of them, expressed as rules rather than as a table with a consumer of its own.
The audio list comes before HDR because it changes the shape of a settings field, so presets saved against the other shape need a migration.

**Infrastructure track.** The reverse proxy and certificates, then the key, token and index service, then the group model in the app.
All three are built.
What the group section still lists as open is the index snapshot's missing columns.

## MoQ is assumed

A native MoQ leg is unscheduled and assumed, so a decision that would have to be unmade for it is made the other way now.
The relay serves the leg both ways and the app reaches it through the browser page alone, so what stands between is one MOQT client.

- **Carriage.** MoQ carries every format this app encodes, on both legs, so a rule narrowing to what SRT or WHEP carries stops holding when the leg lands.
- **Per-reader behaviour.** MoQ drops per subscriber where the re-serving legs treat every reader alike, so a design resting on one stream reaching every viewer rests on the legs built so far.
- **Shape.** A protocol is one file and no caller changes, so the client lands as that file beside a GStreamer plugin.

Nothing is built ahead of the client.

## Effort and tune

Built: every ladder a codec's encoder has, both controls, both builders, on both engines.
The reasoning moved to `domain-model.md`, "The two ladders".

**What is left.** Three ladders are declared off a vendor's own option table rather than off a launch, no machine here having the hardware:
NVENC's on the GStreamer engine, QSV's on both, and AMF's.
The NVENC nicks come off `GstNvEncoderPreset` and `GstNvEncoderTune` in the shipped plugin, where a launch would read them too.

Reading ffmpeg's VAAPI quality range off the driver would put that half of the ladder back.
The scale is the driver's there, measured 0..32 on Mesa's radeonsi against oneVPL's 1..7 on Intel's,
so the ladder's seven steps reach the `va` elements alone and the ffmpeg builder spends none.
The probe already opens a VA device per codec, which is where a range would be read.

## Audio

**Built.** A repeated list of `{source, device, gain, mute}`, addressed by indexed keys (`publish.audio_sources[2].gain`),
so every existing control kind edits an entry and a statement lands on one entry rather than on the whole control.
The reasoning moved to `domain-model.md`, "The second-track capture sources".

The list grows through the settings: the form draws one row past the end, picking a kind on it is the write that adds the entry, and setting a kind back to none takes one off.
Both are ordinary writes through ordinary controls, so a shell decides nothing about the list's shape.

Carriage rules out two tracks, so the sources mix into one.
Kinds stay a declared table (`desktop`, `application`).
What is inside a kind is read off the daemon's own events and answered from memory, separately from the probe.
Gain and mute are one live field beside the bitrate: they reach the mixer that is already running,
where an entry added or taken off is a different graph and a relaunch.

**Built: per-application capture.** An application playing sound is a PipeWire node, so recording one is taking that node's output.
The enumeration reports the output streams beside the sinks, and the GStreamer branch opens one with `pipewiresrc target-object=` where the desktop kind takes a `pulsesrc device=`.
Linux's alone and the GStreamer engine's alone.
The two refusals are separate because they send a reader to different places:
Windows needs WASAPI process loopback and macOS a ScreenCaptureKit or CoreAudio tap,
and ffmpeg's pulse input takes a device where PulseAudio cannot record one program's stream at all.
An application is named by its own name and identified by its node, and a selection the enumeration stops reporting stays on the list with a note.

**Built: Windows.** Desktop audio through `wasapi2src`, the GStreamer engine's alone, ffmpeg having no WASAPI input.
The element opens the default render device itself and takes no handle for it,
which is the difference from a sound server's named devices and why the element and its handle are one table read.
Per-application capture stays refused there: that API addresses a program by process id rather than by device, and nothing enumerates one.

**Built: the Linux enumeration follows the daemon.** One `pw-dump --monitor` runs as long as the app does, seeding from the whole state it opens with and folding in each batch of changes.
A read is a lock and a copy, which is what a form resolving on a keystroke can pay.
The application that just launched is the entry worth selecting, and an answer taken once has it wrong every time.

**What is left.** Two, in the order they cost a user something.

Nothing enumerates Windows devices, so both kinds offer their own default and nothing else.
The default is what a machine with no enumeration takes anyway, so the gap is a picker rather than a capability.
Closing it is two halves: a device monitor to list the render devices, and a handle on `wasapi2src`, which opens the default device and takes none.

macOS records nothing, and it is the one platform where the work is a component rather than a row.
Reading what a Mac plays takes a CoreAudio process tap or ScreenCaptureKit audio feeding an `appsrc`, neither engine having an element for either.

## HDR

HDR is a property of the captured surface, decided by the transfer characteristic in the capture's caps.
Caps carrying none are SDR, because guessing upward publishes a PQ tag over an SDR desktop.
A monitor capability column is a later refinement for the picker's benefit.
On Linux there is no way to ask a monitor at all, through xrandr, the wlr protocols or the portal.

An HDR capture cannot ride in 8-bit, so a rule refuses the 8-bit chromas while the capture reports HDR, naming both ends.
Tone-mapping down silently is a fallback, and publishing 8-bit with the tag dropped sends wrong colour with nothing saying so.

Mastering display metadata passes through where the capture reports it and is absent where it does not.

Viewers tone-map by choice, per tile, in memory.
A tile watching an HDR stream without tone mapping says so, which is what tells the reader the toggle exists.
Persisting the choice per stream path is ruled out: a stream that stops being HDR would carry a stale preference nobody can find.

## HDR on the publish side

**Built.** The child reports what the capture negotiated,
and a run whose surface turns out to be HDR while the settings ask for an 8-bit format is stopped with both ends named.
Wide gamut is a primaries fact and HDR is a transfer fact, so the verdict reads the transfer characteristic.

The surface's own colour reaches the encoder with it.
The encoder input states one structure per colour the publish accepts (standard range, and the two BT.2100 curves where the pixel format carries ten bits),
and the child narrows them to the one whose transfer the capture is producing, before anything negotiates.
A value list is ruled out by measurement: videoconvert fixates one to its first entry whatever the frames carry,
so a list would have converted every HDR surface into the standard-range row and called it negotiation.
Mastering display metadata rides through because nothing names it: the encoder input pins the memory, the format, the colorimetry and the size,
and every other field the capture stated survives the intersection.

**The other engine cannot read the surface.**
ffmpeg tags every encode BT.709 and reports no caps, so a running ffmpeg states what it is encoding and nothing about what it read.
An HDR desktop captured through one of its backends would go out as a standard-range stream carrying HDR samples.
Nothing there can detect it, so it is a note: a refusal needs a fact, and the fact is what that engine cannot establish.
The note is a rule on the engine axis, landing on the 10-bit format alone, the only one an HDR surface rides in.
So it appears where somebody is reaching for HDR and nowhere else, naming the engine that does carry it.

## HDR in the viewer

**Built.** The render chain gains a rung between the decoder and the chain, where the frames still carry the range they were coded in,
and the choice is asked for when the decode is opened, being part of what it is built from.
Stored nowhere, so two tiles can watch one stream through two answers and neither outlives the decode it was made about.

Two rungs are declared, and which one a machine builds is decided by parsing the fragment rather than by looking its factories up.
`vapostproc hdr-tone-mapping=true` is taken where the VA driver carries the tone-mapping filter,
and a `glshader` rung carrying its own PQ curve everywhere else, which is every platform and every driver that has no such filter.
The second rung's existence is what turned probing by parse from a nicety into a correctness question:
the element registers on a driver without the filter, the property does not, and the launch line the pair produced failed the decode rather than falling back.
The reasoning moved to `viewer-architecture.md`, "Tone mapping".

A machine with no rung builds the decode without one and reports that it did, the chain's own fallback.
A repeated call compares against what a request builds rather than against the request:
held the other way round, a viewer on a machine that cannot convert would tear the same decode down on every pass.
The tile draws the transfer the decode negotiated beside what is being done about it, in both states.
An HDR stream drawn as it arrives is a picture with the wrong brightness, which reads as a bad stream rather than as a setting.

**What is left.** The publish's own preview is not offered the choice:
its decode is opened by the publish rather than by `StartReceive`, so there is no call to carry an answer and no field on the preview to report one.
A capture that is HDR previews as it arrives.

## Groups

Encryption stops at the relay.
MediaMTX terminates every protocol and re-muxes for every listener, so it sees plaintext by construction.
A relay that did not would take HLS, WebRTC, the browser viewer and every relay statistic with it.
The relay operator and the group service can both watch a private stream, and the interface says so rather than implying otherwise.

**A group is a path prefix.** The path is `<group-id>/<name>`, the group id derived from a random group key.
MediaMTX's per-path permissions do the enforcement, so "which streams may I see" is a string match rather than a query the relay API cannot answer.

**Built: the derivation**, the piece both sides run.
The client computes the prefix it publishes under and the service computes the prefix it grants a token for,
so two implementations of one hash would be a member issued a token for a path nobody is publishing to.
The id is a keyed digest under its own label rather than a hash of the group key:
the id is public, in every URL a member pastes, and must say nothing about the secret behind it.
Every second use of the group key derives under a second label, the member id included, so what one use publishes cannot be replayed as another's input.
A stream with no group key is published under the public prefix rather than under its bare name,
which is where "every stream lives under a prefix somebody was granted" can be enforced:
the bare name is granted by no token, so a relay that authenticates refuses it.

## Joining a group

**Holding the group key is what lets somebody join.** There are no accounts.
The API creates a group and returns the group key, and the client distributes it.
Discord is a carrier for a group key rather than its source, so a second integration leaves the security story unchanged.
Deriving the group key from a channel identifier is ruled out: a channel id is a public snowflake, so anyone could enumerate channels and compute prefixes.

Creation is open and rate limited.
**Membership ships with removal**: a group key somebody holds cannot be taken off them,
so without something that closes what they hold the model is advisory.
What closes it is membership lapsing, drawing a new group key being neither necessary nor sufficient:
it leaves every live connection alone and moves every remaining member's streams to a new prefix.

**One group at a time**, on the mental model of a voice channel.
Switching groups moves the stream's path, so it stops the publish.
Switching while live is out of scope, and the failure that must not happen is a user moving channels and broadcasting to the old one.

## Public streams

**Public means watchable and discoverable.** Publishing always requires a token.
A publisher holding no group key trades for one granting the public prefix,
so the connection is authenticated and encrypted like every other and what "public" drops is who may watch.
The index takes credentials and returns that group's streams, or public streams without them, enforcing the split rather than leaving a shell to filter.
A group listing hides public streams.

An unreadable group key is not a publisher holding none.
A field nobody filled in is a stream nobody restricted, and a group key that came back damaged is a stream somebody meant to restrict,
so the second falls to the bare name the relay refuses rather than to the prefix everyone can read.

## Tokens and encryption

**Relay auth is JWT** through `authJWTJWKS`, so the relay makes no call per connection.
Tokens are short and validated at connect, and a live connection survives expiry.
So withholding one reaches only the next connection, which is why membership is a lease enforced by closing what a lapsed member holds.

**Every leg is encrypted.** A reverse proxy fronts everything, including the API, with the relay's own listeners on loopback,
and ACME lives in the proxy because MediaMTX has no ACME of its own.
SRT is the one exception: UDP with no TLS, taking a passphrase per path prefix.
A group's derives from its group key on both ends, the service writing it into the relay's path configuration and the app keying its legs with it, so nobody sets one.
The public prefix takes a well-known value spelled in the app and the relay configuration alike.

Encryption is a flag plus a second port only where the relay has a second listener.
RTSP and RTMP have their own TLS listeners.
HLS and WebRTC flip the same one.
The asymmetry is a fact about the relay, and a settings shape that hid it would ask for a port nothing binds.

A self-signed relay is trusted through a per-app CA file both engines are pointed at.
Neither engine does fingerprint pinning, so an "accept this fingerprint" step would collapse into disabling verification, which accepts an attacker's certificate too.

## The group service

The group key, token, index and membership service lives in this repository,
because the path-prefix derivation has to be identical on both sides and two repositories means two copies of it.

**Built: the service.**
It holds a signing key and the presence leases: a group is created by drawing a group key,
that key is traded for a short relay token granting its prefix,
and the index answers a caller's group or the public streams by reading the relay's own path list.
Nothing else is stored, holding the group key being what lets somebody join and the prefix being that key's own digest.
Rotation is therefore drawing a second group key and using it.
The leases are held in memory alone, so a restart forgets every one and every live app re-states its own within one refresh interval.
The token is ES256 against `crypto/ecdsa` rather than a JWT library:
one algorithm, one claim set and one key, where a library would carry the other twenty algorithms including the ones whose presence is the vulnerability.

SRT's stream-id budget picks the algorithm.
A token reaches the relay inside the SRT stream id, every SRT implementation caps that field at 512 bytes, and an RS256 signature is 342 characters on its own:
the transport carrying most of this app's streams could not carry its own credential.
An ES256 token measures 418 bytes with a group's prefix and a stream name beside it, which is what the stream-id budget bounds.
The name has no length rule of its own: an empty one and one carrying a path separator are refused, and nothing else.
What overflows is caught on the publish leg instead, where the two meet:
`transport.SRT.ValidatePublishSettings` assembles the stream id before a publish starts and refuses one past 512 bytes, naming how many characters the name has to lose.

How a token travels is the relay's answer per protocol, measured against MediaMTX 1.20.

| Leg | Carries the token as |
| --- | --- |
| SRT | the password field of the stream id, `publish:<path>:any:<token>` |
| RTSP | a `jwt` query parameter |
| HLS, WHIP, WHEP | a `jwt` query parameter, or a bearer header |

The relay's own API takes one too, and refuses every token this service issues:
a group's grant covers publishing and reading under one prefix and names no API action.

## Deployment, and the app's half

**Built: the deployment.** `deploy/` carries the relay configured for groups
(`authJWTJWKS` pointed at the service, the public prefix's SRT entry, every other listener on loopback)
and the reverse proxy that terminates TLS and renews the certificate for all of them.
The NixOS modules in the `nixos-config` repository read both files straight out of this one,
so what the relay carries stays the app's decision and which listeners a machine exposes stays the host's.
One relay configuration, and `task relay` runs it too:
a development relay and a deployment differ in the certificate and hook paths handed to MediaMTX through its own environment, and in nothing a token or a permission depends on.

The app publishes under its group: the group key is a relay setting, every transport builds its path from it,
and the SRT passphrase derives from the same key and rides both legs.
What makes a group required is the relay refusing an unauthenticated publish rather than the app inventing a prefix.
A machine holding no group key still publishes, under the public prefix anybody reaching the relay can watch,
and the bare name is what a machine pointed at no relay builds.

**Built: the app's credential.**
The group key is traded for a relay token, and the minted one is held until it is close to expiring.
It is attached to the settings snapshot every command is built from, and to nothing else.
Each leg carries it the way its protocol takes one, and the relay's own API is left unread where a group service answers:
the live-stream list comes from the index, which is what a member's token reaches.
One setting says a proxy fronts the HTTP legs, so those addresses become one name on 443 and the group service is reachable at the same one.

Every named relay has a service beside it to ask: the proxy's own name off a trusted network, `groupd`'s own port on one this network reaches directly.
A machine that has named no relay asks nothing, so every command is built without a token and every stream carries the bare name.

**Built: drawing a group key from the app.** `CreateGroup` on the control contract reaches the service's `POST /groups`, and the wizard offers it as a button beside the group key field.
What comes back is written through the path a pasted group key takes,
so setting a group is a settings change a reader can see, undo and hand on rather than something that happened to the machine.
The button is pressable wherever a relay is named, every one of them answering a group service, and states what it waits on where none is.

## Membership

**Built: removing a member** (`PUT /members`, `DELETE /members` and `POST /reconcile`, drawn in `membership.md`).
Rotation and expiry both leave a live connection alone,
so what a member who left keeps is exactly what they already hold, and closing it is the only thing that takes it away.

Membership is a presence lease a member's own app states and refreshes, on the loop that already polls the relay.
Enforcing it lists every connection under the group's prefix and closes the ones no live member holds, which takes only a member's own connections.
It is stated rather than stepped, so a second run over unchanged leases does nothing and it is safe on every statement of presence and on every read.

A member is told from another by the token's subject:
`POST /tokens` takes a member secret beside the group key, and the subject becomes that secret's keyed digest under the group key rather than the group's own id.
The relay lists a connection under that subject, so enforcement matches ids and neither the secret nor whatever else names a person reaches the relay.
The secret is drawn by that member's own app and issued by nobody, which is what makes identity unforgeable inside a group:
an app claiming another member's display name still derives its own id.

A close is not a revocation, so a member whose lease lapsed reconnects on the token they still hold until it expires.
The relay's read hook closes that too, reporting each starting read to `POST /reconcile` (`deploy/reconcile-on-read.sh`).

The leases are the one thing the service keeps, and it keeps the fact rather than a copy: nobody else knows who is in a group.
A group with no live member is not enforced, membership nobody stated being a different thing from a group nobody is in.

**Built: the app's side of it.**
This machine draws its own member secret the first time its settings name a group and keeps it in a file per group, owner-only beside `settings.json`,
so its identity there is nobody else's to state.
The relay poll draws that identity and states presence on every pass,
and a settings write that changes the group key releases the lease and drops the file.
The display name is an ordinary relay setting, claimed first-come in the group,
so a name another member holds comes back as a refusal a reader can act on rather than as a silent rename.

The relay states no reason when it closes a connection,
so the app reads its own membership against the close and says either that the relay closed it or that membership lapsed (`api/proto/screenshare/v1/text.proto`).

**Built: the figures beside a row.** The index answers each stream's ingest rate and its reader count, so a member behind the proxy reads the same two columns an operator reads off the relay's own API.

**What is left.** The reader roster stays at the service.
A count says how many are watching and naming them answers a question about other members that presence already answers under its own request.

## Adaptive bitrate

One bitstream reaches every viewer.
The relay re-muxes what it ingests, so what a publisher sends is what each viewer receives.
Three separate things go by this name and they are not equally reachable from here.

**The publish leg, off the link's own report.**
The scarce resource is the publisher's uplink, and the SRT leg already measures it: round-trip time, the delivery window and what the sender dropped arrive with every stats read.
The encoder's bitrate is a live field on the GStreamer engine, so a rate reaches a playing pipeline without relaunching it.
Both halves are built and the controller between them is not.
It reads one signal and writes one field, and the design decision in it is how slowly it may move, a rate chasing a noisy estimate being worse than one that stands.
This is the tier worth building: it costs no second encode, needs nothing of the relay, and every publisher benefits from it on the transport the app defaults to.

**A second rung, paid for in upload.**
A publisher encodes a lower rung to a sibling path under the same group prefix and a viewer picks between them.
The index lists a group's paths, so a rung is discoverable with no contract of its own.
It stays opt-in, being a second encode and a second upload spent so a viewer who cannot keep up has something.
Nothing switches on the viewer's behalf, which is what separates a rung from adaptation.

**Per-subscriber layers, which MoQ reaches.**
Dropping layers takes a server that decides per reader, and adopting a WebRTC SFU costs SRT, RTSP, RTMP and HLS, which is the carriage model itself.
A MOQT relay does that job off the metadata alone: a publisher gives each layer its own priority, a subscriber names its own in the subscribe, and a reader that falls behind loses the lowest-priority objects on its own connection while the others keep them.
The re-serving legs cannot have it, one byte stream reaching every reader they serve, so a slow one keeps up or is dropped whole.

**The temporal rung costs one encode.**
HEVC states a temporal id in every NAL header and AV1 in every OBU extension header, so the layer is read off the bitstream, and the encoder is asked for a hierarchical GOP and nothing further.
Dropping the top sub-layer halves that viewer's frame rate and leaves the picture decodable, an extraction both formats define.
H.264 states the id in an SVC prefix NAL and VP8 and VP9 in an RTP descriptor, so those three carry it outside the bitstream.

**The spatial rung is the same mechanism at the price of the second encode.**
It is a track under one catalog where the sibling-path rung above is a path of its own, so the relay drops it per subscriber and the viewer chooses nothing.

The order is the cost order: the publish-leg controller, then temporal layers, then spatial rungs.
Both MoQ tiers wait on a native leg.

## Assumptions to verify

Each one names the reading that would settle it.

- The relay's MoQ listener drops by priority when a reader falls behind.
  MOQT states the scheduling and leaves a relay free to forward everything and let the connection back up instead.
  Per-subscriber layers rest on it, and the reading takes a congested reader against a live relay.
- The encoders here produce a hierarchical GOP under the tunings a screen share uses.
  Those tunings drop B frames, so the shape needed is hierarchical P, and which vendors expose it is unread.
- `ddagrab` exposes `draw_mouse`.
  It is a D3D11 filter, so a Linux ffmpeg does not carry it and the reading takes a Windows build.
- Wayland compositors report a usable transfer characteristic through the portal's PipeWire caps.
  Without it, HDR is Windows-only in practice.
  Reading it means completing a portal capture, which asks the desktop for consent, so it is a check somebody runs rather than a test.
- `wasapi2src loopback=true` records what a Windows machine plays, and the same element without it records the default input.
  The Windows audio rows rest on it, and reading it takes a Windows session.
- The `vtenc_h264` and `vtenc_h265` elements take `realtime`, `allow-frame-reordering`, `max-keyframe-interval` and a `bitrate` in kbit,
  and the ffmpeg VideoToolbox encoders take `-realtime` beside an average `-b:v`.
  The VideoToolbox rows rest on both, and reading either takes a Mac.
  The rows declare an average bitrate alone, so a constant rate, a burst ceiling or a quality target is a mode to add once somebody can measure what the framework does with it.

Settled, and kept here until the work they belong to lands:

- MediaMTX validates a JWT at connection time and not again, so a connection outlives its own token.
  Measured against v1.20.0: an RTSP reader carried on for 75 s against a 20 s token and an SRT one for 45 s against a 15 s token, both uninterrupted,
  while a new connection on the expired token was refused with 401.
  An HLS reader is the same answer by another route,
  its entry playlist being authenticated per request and the media playlist and segments then served on the session it opened.
  The token lifetime therefore bounds opening connections and nothing else, which is why membership is enforced by closing them.
- Every leg the relay serves a reader over lists that reader with the subject of the token it connected with, and takes a kick.
  Measured against v1.20.0 across `rtspsessions`, `rtspssessions`, `srtconns`, `webrtcsessions`, `hlssessions`, `rtmpconns`, `rtmpsconns` and `moqsessions`.
  `rtspconns` and `rtspsconns` answer a list and have no kick, a connection being closed by kicking the session on it.
  The relay carries which is which, per protocol.

- kmsgrab cannot include the cursor plane.
  Its demuxer takes a device, a CRTC, one plane, a format and a rate, and no cursor option of any kind.
  The pointer is a plane of its own and the capture takes one.
- The MediaMTX version matters.
  `flake.nix` pins v1.20.0, because `deploy/mediamtx-groups.yml` turns on the MoQ server and a relay that predates `moqQUICAddress` refuses the whole config rather than ignoring the key.
  `scripts/relay.ps1` fetches that same version, and the NixOS relay module takes it from this flake's overlay for the same reason.
- `buf` is in neither the dev shell nor on PATH, so `task api` does not run.
  Regeneration goes through `protoc` with a `protoc-gen-go` built from the module cache, which is the path this repository's generated Go comes from.
