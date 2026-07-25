# Declarative domain model

The codec, pixel format and rate-control mode are not free-form strings scattered through the code.
Each is a table of facts, and every rule the app enforces is derived from those tables rather than restated.
One definition governs the encoder, the settings form, the bitrate estimate and the two grid-viewability verdicts.

## The problem it removes

A codec carries many rules: whether it runs on NVENC, which pixel formats it can encode, which transport can carry it, how efficient it is, which viewer can decode it.
Written imperatively, those rules spread across files and drift.
The failure mode is two encodings of one constraint: the settings form greys out an option while the normalizer still lets the value through, or one copy is updated and the other is missed.

The disable rules and the repair rules in the frontend were previously two hand-kept copies of the same codec/chroma/transport constraints, and the NVENC test was written once in Go and again in TypeScript.

## Where each fact lives

Constraints the encoder and the UI must agree on live in Go and are the single source:

- `capabilities/capabilities.go`: per codec, the encoder family and its NVENC flag, the pixel formats it may encode, the transports that can publish it, the rate-control modes its encoder has no form of, and the scale its constant-quality knob counts on.

The transport column is the publish leg, publisher to relay.
Which protocol a viewer receives over is a separate choice per viewer and is not in any table here (`viewer-architecture.md`, "Two legs, two protocols").

The encoder reads this table directly.
`ffmpeg/args.go` branches on `capabilities.IsNvenc` and `IsVaapi`, and `capabilities.Validate` rejects a codec/chroma/transport/mode/quantizer combination the table forbids.
Both publish engines call that validator, naming themselves, so neither path accepts what the other rejects and a mode gap that belongs to one engine binds only there.
The same table reaches the frontend through the `App.Capabilities` binding, so a combination the encoder would reject is the same combination the UI greys out.

Which media engine runs a capture backend is a fact of the publish layer, and `App.CaptureEngines` carries it to the frontend.
It is a settings input because the two engines express the same five rate-control modes through different properties, so a knob one forwards the other may drop.

Presentation and heuristics are UI-only and live in the frontend:

- `frontend/src/util/domain.ts`: per codec, chroma and mode, the label, tooltip, reference link, coding efficiency, raw bits-per-pixel, what a non-4:2:0 chroma asks of a decoder, and which controls each mode uses.
- `frontend/src/util/domain.ts` `ENGINE_RULES`: per engine and control, where a builder departs from the mode table, either dropping a knob the mode uses or forwarding one it marks unused.
  Each rule mirrors a branch of `encoderArgs` or `gstEncoder` and carries the sentence the form shows for it.

## What derives from the tables

Each consumer reads the tables instead of restating a rule:

- `deps.ts` `evaluateDeps`: greys out an option when the tables make it illegal for the current settings, and greys a rate-control field unless the mode uses it, the codec's encoder has it and the capture's engine forwards it.
- `deps.ts` `normalize`: repairs an illegal combination by walking the same tables to the first legal value.
- `estimate.ts`: the pre-publish bitrate prediction, from coding efficiency and chroma weight.
- `webgrid.ts`: the web-grid viewability verdict, from the codec's format, the chroma's 4:2:0 flag and the `WEB_GRID_DECODE` paths.
- `nativegrid.ts`: the native-grid viewability verdict, from the publish transports the capability table gives the codec, which double as the answer for the grid's RTSP watch leg.
- `options.ts`: the dropdown lists, built from the meta tables so a control cannot offer a value the tables do not define.

Because `evaluateDeps` and `normalize` read one source, a greyed option and its fallback always agree.

## Adding a codec, chroma or mode

Add the row to the table.
The dropdowns, constraints, estimate and verdict follow with no further edits.
The `Codec`, `Chroma` and `Mode` union types force a new value into every meta table, so an incomplete addition fails to compile instead of falling through a runtime default.
A codec whose constraints also reach the encoder is added to the Go `capabilities` table, and the frontend receives it over the wire.

## What stays imperative

The model governs domain rules, not effects.
Process supervision, event subscriptions, relay polling and the one-time encoder probe are imperative by nature and stay in the hooks and the Go process layer.
A table describes what is true; it does not run a child process or subscribe to a stream.
