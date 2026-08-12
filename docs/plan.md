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

**Infrastructure track.** The reverse proxy and certificates, then the key, token and index service, then the group model in the app.
None of it exists today.

## Effort and tune

Each encoder declares its own ladder, in its own identifiers: x264 counts in names, SVT-AV1 in numbers to 13, NVENC from p1 to p7.
A scale normalized across codecs was rejected because a number carried across a codec change would land on a different real setting than the one that was held.

Two fields rather than one.
Effort is how hard the encoder works; tune is what it works towards, and a live encode drops the lookahead and the reordering a quality encode keeps whatever effort it spends.

A step off the ladder is reset to the one the codec's row declares for the mode, never mapped by position, and the field is named in the repaired list so the change is readable.
A ladder that pins a mode greys the control there and names the step in force: NVENC pins its preset in CBR, because a low-latency preset is what lets it hold a constant rate, and that is the encoder's fact rather than the mode's.

The shell names every rung.
A codec whose rung it has no name for renders the identifier, which is honest, visible, and still a defect.

**What is left.** One atomic change: both builders read the two fields, the NVENC-only preset limit goes (it checks the raw field, so it refuses the empty value that means "the codec's declared step"), the availability gate moves from the encoder family to the ladder, and the fixtures that build a draft from `settings.Defaults()` and change the codec stop carrying another encoder's step.
Splitting it fails: an ffmpeg-only swap breaks the cross-engine SVT-AV1 comparison, and flipping availability first offers a knob nothing forwards.
Then the tune control, the rung copy, and the QSV and AMF ladders, which have to be read off those encoders rather than declared from memory.

## Audio

Sources mix into one track.
Two tracks were rejected on carriage: RTMP carries one audio track, the relay re-serves every ingest on all its listeners, and a two-track stream is unplayable on the narrowest leg while the form says it published.

Sources are enumerated rather than listed.
Kinds stay a declared table (`desktop`, `mic`, `application`), and the device or application inside a kind is enumerated, cached for the process lifetime and read back separately from the probe.
Per-application capture is PipeWire-native on Linux, and needs platform code on Windows (WASAPI process loopback) and macOS (ScreenCaptureKit or CoreAudio taps).

The setting is a repeated list of `{source, device, gain, mute}`, addressed by indexed keys (`publish.audio_sources[2].gain`), so every existing control kind keeps working and a rule can grey one item's field.
It is the largest contract change in the plan, larger than auth, and doing it later would mean migrating saved presets as well as redoing the form contract.

An application is identified by its binary and then its name, and a selection the enumeration no longer reports stays on the list with a note, the way a monitor index no enumeration reported does.
The enumeration follows PipeWire node add and remove events: the application just launched is the one worth selecting, and that is the case a cache gets wrong every time.

Gain is live.
That is what the control socket below exists for.

## Liveness

Each field carries a `live` flag from the backend, and `ApplyToStream` applies the live subset and restarts only where a non-live field changed.
A fixed list in the shell was rejected for the usual reason: a field that becomes live later would need a shell edit.

The GStreamer engine gets a first-party binary that builds the pipeline and reads a control socket, replacing `gst-launch-1.0` while keeping the crash isolation the supervisor exists for.
The ffmpeg engine is declared non-live, expressed as a rule on the engine axis, which makes engine liveness the first real customer of the axis registry.
A wrapper per engine was considered and dropped as a second child implementation for one knob.

Every socket message carries the whole desired live state and the child converges to it.
A crash-restart is then indistinguishable from an apply, and a dropped message cannot leave the pipeline on a value nobody chose.

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

## Groups, auth and encryption

Not end to end.
MediaMTX terminates every protocol and re-muxes for every listener, so it sees plaintext by construction, and a relay that did not would take HLS, WebRTC, the browser viewer and every relay statistic with it.
The relay operator and the key service can both watch a private stream, and the interface says so rather than implying otherwise.

**A group is a path prefix.** The path is `<group-id>/<name>`, where the group id derives from a random group key.
MediaMTX's per-path permissions then do the enforcement, and "which streams may I see" is a string match rather than a query the relay API cannot answer.

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

## The pointer channel

The `metadata` cursor mode is declared and refused, because nothing carries a pointer position to a viewer and no viewer draws one.
Shipping it is deleting that one rule once the channel exists.

The position rides its own stream on the control gRPC at its own rate, carrying the frame timestamp it belongs to.
Binding it to the frame rate would throw away the reason to draw the pointer client-side: it costs no frame, so a 240 Hz pointer over a 30 fps stream is the whole win, and the timestamp lets a viewer hold it back if leading the picture looks wrong.

## Assumptions to verify

These are assumptions the design rests on, not established facts.

- MediaMTX validates a JWT at connection time only, and does not drop a live session when the token expires.
  The token-lifetime decision rests on it.
- `ddagrab` exposes `draw_mouse`, and kmsgrab genuinely cannot include the cursor plane.
- Wayland compositors report a usable transfer characteristic through the portal's PipeWire caps.
  Without it, HDR is Windows-only in practice.
- `flake.nix` pins MediaMTX and `docker-compose.yml` runs `latest`.
  Auth and encryption keys are exactly where that skew would bite.
- `buf` is in neither the dev shell nor on PATH, so `task api` does not run.
  Regeneration currently goes through `protoc` with a `protoc-gen-go` built from the module cache.
