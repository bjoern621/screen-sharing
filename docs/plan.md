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

Built: every ladder, both controls, both builders, on both engines.
The reasoning moved to `domain-model.md`, "The two ladders".

The last two are the QSV and AMF ones, declared from the scales the two vendors define rather than from a reading taken off silicon: oneVPL's target usage runs 1 for quality to 7 for speed, and AMD's quality preset has the three steps every VCN generation implements.
Each engine spells them its own way and the ladder is the scale itself, which is what keeps a stream's look off the capture backend that produced it: the GStreamer qsv elements take the number on `target-usage`, ffmpeg names the same seven points on `-preset`, and all three AMF encoders take the step verbatim.
The higher AMF preset newer generations add is deliberately absent, because a step the older hardware refuses is a publish that dies at launch.

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

**Built: per-application capture.** An application playing sound is a PipeWire node, so recording one is taking that node's output: the enumeration reports the output streams beside the sinks and the sources, and the GStreamer branch opens one with `pipewiresrc target-object=` where the other kinds take a `pulsesrc device=`.
It is Linux's alone and the GStreamer engine's alone, and the two refusals are separate because they send a reader to different places: Windows needs WASAPI process loopback and macOS a ScreenCaptureKit or CoreAudio tap, and ffmpeg's pulse input takes a device where PulseAudio cannot record one program's stream at all.
An application is named by its own name and identified by its node, and a selection the enumeration no longer reports stays on the list with a note.

**What is left.** The enumeration is taken once and cached for the process lifetime, which is right for the devices a machine has and wrong for the applications it is running: the one just launched is the one worth selecting, and that is the case a cache gets wrong every time.
Following PipeWire's own add and remove events is what replaces it.

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

**The other engine says what it cannot do.** ffmpeg tags every encode BT.709 and reports no caps - a running ffmpeg tells its caller what it is encoding and never what it read - so an HDR desktop captured through one of its backends would go out as a standard-range stream carrying HDR samples.
Nothing there can detect it, which is why it is a note and not a refusal: a refusal needs a fact, and the fact is what that engine cannot establish.
The note is a rule on the engine axis and it lands on the 10-bit format alone, which is the only one an HDR surface rides in, so it appears where somebody is reaching for HDR and nowhere else, naming the engine that does carry it.


**Built: the viewer half.** The render chain gains a rung between the decoder and the chain, where the frames still carry the range they were coded in, and the choice rides on `StartReceive` because it is part of what the decode is built from (`receive/tonemap.go`).
It is per tile and stored nowhere, so two tiles can watch one stream through two answers and neither outlives the decode it was made about.

One rung is declared and it is `vapostproc hdr-tone-mapping=true`, which is the only element in reach that states a luminance rolloff.
The rest of the paragraph this replaces was a guess, and measuring it corrected two things.
`videoconvert gamma-mode=remap` does convert the transfer function, on every platform and with no rung at all - but it normalizes PQ against the format's ten thousand nits rather than the display's hundred, and a mid-grey PQ frame through it comes out at a fifth of the code value it went in at.
A darker picture is not a tone map, so it is not offered as one.
`d3d11convert` states gamma and primaries conversion, the same two the software converter states, and neither is a rolloff either, so Windows declares no rung and the tile says so rather than offering a conversion the element does not promise.

A machine with no rung builds the decode without one and reports that it did, which is the chain's own fallback, and the comparison a repeated call makes is against what a request builds rather than against the request: held the other way round, a viewer on a machine that cannot convert would tear the same decode down on every pass.
The tile draws the transfer the decode negotiated beside what is being done about it, in both states, because an HDR stream drawn as it arrives is not obviously wrong - it is a picture with the wrong brightness, which reads as a bad stream rather than as a setting.

**What is left.** The publish's own preview is not offered the choice: its decode is opened by the publish rather than by `StartReceive`, so there is no call to carry an answer and no field on the preview to report one.
A capture that is HDR previews as it arrives.

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

**Built: the deployment and the app's half.** `deploy/` carries the relay configured for groups - `authJWTJWKS` pointed at the service, the SRT passphrase in `pathDefaults`, every other listener on loopback - the reverse proxy that terminates TLS and renews the certificate for all of them, and the compose file that runs the three together.
They are second files rather than edits to the ones at the root, because both deployments are real: that one is a relay on a trusted network where anybody may publish, and turning it into this one in place would refuse every existing publisher on the next pull.

The app publishes under its group: the key is a relay setting, every transport builds its path through `Relay.Path`, and the SRT passphrase rides both legs.
What makes a group required is the relay refusing an unauthenticated publish rather than the app inventing a prefix, so a machine with no key still publishes under the bare name - which is what a relay with no auth serves and what every LAN stream does.

**What is left.** Getting a key without leaving the app: creating one, pasting one and rotating one are three calls to a service the app does not speak to yet, so today the key arrives however its group distributes it and is pasted into the field.

## The pointer channel

**Built: the channel.** The position leaves the publish child on its own line, crosses the control gRPC on a stream of its own, and reaches this machine's screens, where the broadcast preview draws it.
The `metadata` mode is offered on the X11 capture backend and carries a note saying how far the position travels.


The position rides its own stream on the control gRPC at its own rate, carrying the frame timestamp it belongs to.
Binding it to the frame rate would throw away the reason to draw the pointer client-side: it costs no frame, so a 240 Hz pointer over a 30 fps stream is the whole win, and the timestamp lets a viewer hold it back if leading the picture looks wrong.

Where the position comes from is the first-party binary's, and which source it has is the display server's answer.
X11 tells any client that asks, so on that session the child polls and there is nothing to subscribe to; the reader holds one connection open and answers whenever the child wants a position.
A Wayland client cannot ask at all, so what answers there is the cursor metadata PipeWire carries beside each frame, which only a process holding the stream can read.

**What is left.** Two legs, and they are separate.
The portal backend keeps its refusal, because reading SPA cursor metadata means taking the buffers off the stream through libpipewire rather than through `pipewiresrc`.
And nothing carries the position over the relay, so a viewer on another machine still sees no pointer - which is the note the offered mode carries rather than a silence.

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
