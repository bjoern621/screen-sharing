# Planned work, and the decisions behind it

This page records design decisions taken for work that is not built yet, and the order it is built in.
It is the one page here that is not time-agnostic, and it is meant to shrink: as each piece lands, its reasoning moves into the topic page it belongs to and the section here goes away.

What is already built is not repeated.
The rule system is `domain-model.md`, "One evaluator"; the pointer is `capture-architecture.md`, "The pointer".

## Two tracks

The work splits into a contract track and an infrastructure track.
They meet only where group fields enter the settings model, so they can run in either order or at once.

**Contract track.** Effort and tune, then the audio list, then HDR.
Each is a settings field or a set of them, and each is expressed as rules rather than as a table with a consumer of its own.
Effort and tune is built, so the audio list is the head of this track, and it is the one to take first for the reason its own section gives: it changes the shape of a settings field, and every preset saved before it lands is one to migrate afterwards.

**Infrastructure track.** The reverse proxy and certificates, then the key, token and index service, then the group model in the app.
None of it exists today.

## Effort and tune

Built: both ladders, both controls, both builders, on both engines.
The reasoning moved to `domain-model.md`, "The two ladders".

**What is left.** The QSV and AMF ladders, which have to be read off those encoders rather than declared from memory.
Both builders still spend a constant on oneVPL's target-usage scale and AMF's quality preset, and both controls grey for those families meanwhile.
VAAPI needs no ladder: neither engine's VAAPI path has such a knob at all.

The NVENC steps on the GStreamer engine are forwarded but not yet run.
The nicks come off `GstNvEncoderPreset` and `GstNvEncoderTune` in the shipped plugin, which is where a launch would read them too, and no machine here has the hardware to launch one.

## Audio

**Built.** The setting is a repeated list of `{source, device, gain, mute}`, addressed by indexed keys (`publish.audio_sources[2].gain`), so every existing control kind edits an entry and a statement lands on one entry rather than on the whole control.
The reasoning moved to `domain-model.md`, "The second-track capture sources".

The list grows through the settings and not through an effect: the form draws one row past the end, picking a kind on it is the write that adds the entry, and setting a kind back to none is what takes one off.
Both are ordinary writes through ordinary controls, so a shell decides nothing about the list's shape.

Two tracks were rejected on carriage, and the sources mix into one.
Kinds stay a declared table (`desktop`, `mic`, `application`), and what is inside a kind is enumerated (`internal/audiodev`), cached for the process lifetime and read back separately from the probe.
Gain and mute are one live field beside the bitrate: they reach the mixer that is already running, where an entry added or taken off is a different graph and a relaunch.

**What is left.** Per-application capture, which is the third kind: it is declared and greyed everywhere, because it is PipeWire-native on Linux and needs platform code on Windows (WASAPI process loopback) and macOS (ScreenCaptureKit or CoreAudio taps).
An application is identified by its binary and then its name, and a selection the enumeration no longer reports stays on the list with a note, the way a monitor index no enumeration reported does - that half is built, and what is missing is anything to enumerate.

The enumeration should follow PipeWire node add and remove events rather than being taken once: the application just launched is the one worth selecting, and that is the case a cache gets wrong every time.
What is built reads `pactl` on the first resolve and answers from memory afterwards, which is right for the devices a machine has and wrong for the applications it is running.

## HDR

HDR is a property of the captured surface, not a value the user picks.
What decides it is the transfer characteristic in the capture's caps, and caps carrying none are SDR, never "probably HDR": guessing upward publishes a PQ tag over an SDR desktop.
A monitor capability column is a later refinement for the picker's benefit; on Linux there is no way to ask a monitor today through xrandr, the wlr protocols or the portal.

An HDR capture cannot ride in 8-bit, so a rule refuses the 8-bit chromas while the capture reports HDR, naming both ends.
Tone-mapping down silently is a fallback, and publishing 8-bit with the tag dropped sends wrong colour with nothing saying so.

Mastering display metadata passes through where the capture reports it and is absent where it does not.

Viewers tone-map by choice, per tile, in memory.
A tile that is watching an HDR stream without tone mapping says so, which is the part that tells the reader the toggle exists.
Persisting the choice per stream path was rejected: a stream that stops being HDR would carry a stale preference nobody can find.

**Built: the publish half.** The child reports what the capture negotiated, and a run whose surface turns out to be HDR while the settings ask for an 8-bit format is stopped with both ends named (`publish/gsthdr.go`).
A wide-gamut SDR desktop is not HDR, so the verdict reads the transfer characteristic and never the primaries, and caps carrying no colorimetry at all are SDR.

The surface's own colour reaches the encoder with it.
The encoder input states one structure per colour the publish accepts - standard range, and the two BT.2100 curves where the pixel format carries ten bits - and the child narrows them to the one whose transfer the capture is producing, before anything negotiates (`gstrun/surface.go`).
A value list was measured and rejected: videoconvert fixates one to its first entry whatever the frames carry, so a list would have converted every HDR surface into the standard-range row and called it negotiation.
Mastering display metadata rides through because nothing names it: the encoder input pins the memory, the format, the colorimetry and the size, and every other field the capture stated survives the intersection.

**What is left.** The ffmpeg engine tags every encode BT.709 (`ffmpeg.colourFilter`) and reports no caps, so a Windows capture through it publishes an HDR desktop as if it were standard range with nothing saying so.
Either it gains the same report, or HDR is declared the GStreamer engine's as a rule on the engine axis and an HDR capture is refused there.

The viewer half waits on an element that tone-maps.
`videoconvertscale` converts primaries and nothing else, and the elements that roll PQ down to SDR are the device ones - `vapostproc` on VA, `d3d11convert` on Windows - so the render chain gains a rung per platform rather than one route, and a machine with neither keeps the choice greyed with what is missing.
That is the same shape the chain ladder already has for every other conversion (`viewer-architecture.md`).

## Groups, auth and encryption

Not end to end.
MediaMTX terminates every protocol and re-muxes for every listener, so it sees plaintext by construction, and a relay that did not would take HLS, WebRTC, the browser viewer and every relay statistic with it.
The relay operator and the key service can both watch a private stream, and the interface says so rather than implying otherwise.

**A group is a path prefix.** The path is `<group-id>/<name>`, where the group id derives from a random group key.
MediaMTX's per-path permissions then do the enforcement, and "which streams may I see" is a string match rather than a query the relay API cannot answer.

**Built: the derivation** (`internal/group`), which is the piece both sides run.
The client computes the prefix it publishes under and the service computes the prefix it grants a token for, so two implementations of one hash would be a member issued a token for a path nobody is publishing to.
The id is a keyed digest under its own label rather than a hash of the key, because the id is public - it is in every URL a member pastes - and must say nothing about the secret behind it; a key with a second use derives that one under a second label, so what one use publishes cannot be replayed as another's input.
A stream with no key is refused rather than published under its bare name, which is the "publishing always requires a group" rule where it can actually be enforced.

Nothing reads it yet, and that is deliberate: wiring the prefix into the transports before the service exists would be an app that cannot publish, since there would be no way to obtain a key.

**Possession of the group key is membership.** There are no accounts.
The API creates a group and returns the secret, the client distributes it, and a Discord bot later distributes keys to whoever is in a voice channel.
Deriving the key from a channel identifier was rejected: a channel id is a public snowflake, so anyone could enumerate channels and compute prefixes.
Discord is a transport for the key, never its source, which is what keeps the security story unchanged when a second integration arrives.

Creation is open and rate limited.
**Rotation ships from the start**, because possession as membership means a member who leaves still holds the key, and without rotation the model is advisory.

**One group at a time**, on the mental model of a voice channel.
Switching groups moves the stream's path, so it stops the publish; switching while live is out of scope, and the failure that must not happen is a user moving channels and broadcasting to the old one.

**Public means watchable and discoverable.** Publishing always requires a group.
The index takes credentials and returns that group's streams, or public streams without them, and it enforces the split rather than leaving a shell to filter.
A group listing hides public streams.

**Relay auth is JWT** through `authJWTJWKS`, so the relay makes no call per connection.
Tokens are short and validated at connect, and a live connection survives expiry; revocation lands at the next connection, which is what rotation is for.

**Every leg is encrypted.** A reverse proxy fronts everything, including the API, with the relay's own listeners on loopback, and ACME lives in the proxy because MediaMTX has no ACME of its own.
SRT is the one exception: it is UDP with no TLS, and it takes a relay-wide passphrase through `pathDefaults`, one user-set value written into both the publish and read keys.

Encryption is a flag plus a second port only where the relay has a second listener.
RTSP and RTMP have their own TLS listeners; HLS and WebRTC flip the same one.
The asymmetry is a fact about the relay, and a settings shape that hid it would ask for a port nothing binds.

A self-signed relay is trusted through a per-app CA file both engines are pointed at.
Neither engine does fingerprint pinning, so an "accept this fingerprint" step would collapse into disabling verification, which accepts an attacker's certificate too.

The key, token and index service lives in this repository under `cmd/`, because the path-prefix derivation has to be identical on both sides and two repositories means two copies of it.

**Built: the service** (`cmd/groupd`, `internal/groupsvc`, `internal/token`).
It holds a signing key and nothing else: a group is created by drawing a key, a key is traded for a short relay token granting that key's prefix, and the index answers a caller's group or the public streams by reading the relay's own path list.
There is no membership store because there is nothing to store - possession of the key is membership, and the prefix is the key's own digest - which is also what makes rotation drawing a second key and using it.
The token is RS256 against `crypto/rsa` rather than a JWT library: one algorithm, one claim set and one key, where a library would carry the other twenty algorithms including the ones whose presence is the vulnerability.

**What is left.** The relay's own configuration - `authJWTJWKS` pointed at this service, the per-path permissions, the SRT passphrase in `pathDefaults` - and the reverse proxy with ACME in front of all three.
Then the group model in the app: the key as a setting, the prefix in front of every transport's path, and a shell to create, paste and rotate one.
Nothing wires the prefix into the transports yet, because an app that required a group before there was a way to obtain one is an app that cannot publish.

## The pointer channel

The `metadata` cursor mode is declared and refused, because nothing carries a pointer position to a viewer and no viewer draws one.
Shipping it is deleting that one rule once the channel exists.

The position rides its own stream on the control gRPC at its own rate, carrying the frame timestamp it belongs to.
Binding it to the frame rate would throw away the reason to draw the pointer client-side: it costs no frame, so a 240 Hz pointer over a 30 fps stream is the whole win, and the timestamp lets a viewer hold it back if leading the picture looks wrong.

Where the position comes from is the first-party binary's, not a platform poll.
A Wayland client cannot ask where the pointer is outside its own surfaces, so polling has no answer to give there; what does is the cursor metadata PipeWire carries beside each frame, which is what the `metadata` mode asks the portal for and what only a process holding the stream can read.

## Assumptions to verify

These are assumptions the design rests on, not established facts.

- MediaMTX validates a JWT at connection time only, and does not drop a live session when the token expires.
  The token-lifetime decision rests on it.
  Verifying it takes a running relay and a token that expires during a publish.
- `ddagrab` exposes `draw_mouse`.
  It is a D3D11 filter, so a Linux ffmpeg does not carry it and the reading takes a Windows build.
- Wayland compositors report a usable transfer characteristic through the portal's PipeWire caps.
  Without it, HDR is Windows-only in practice.
  Reading it means completing a portal capture, which asks the desktop for consent, so it is a check somebody runs rather than a test.

Settled, and kept here until the work they belong to lands:

- kmsgrab cannot include the cursor plane.
  Its demuxer takes a device, a CRTC, one plane, a format and a rate, and no cursor option of any kind; the pointer is a plane of its own and the capture takes one.
- The MediaMTX skew is real.
  `flake.nix` pins v1.20.0, because `mediamtx.yml` turns on the MoQ server and a relay that predates `moqQUICAddress` refuses the whole config; `docker-compose.yml` runs `bluenviron/mediamtx:latest`.
  Auth and encryption keys are exactly where that skew bites, so the compose relay is the one to pin when they land.
- `buf` is in neither the dev shell nor on PATH, so `task api` does not run.
  Regeneration goes through `protoc` with a `protoc-gen-go` built from the module cache, which is what this repository's generated Go was last written by.
