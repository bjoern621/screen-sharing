# Tooltips

Every settings field and every option a control offers can carry an explanatory tooltip.
They are the form's teaching layer: they explain the encoding model as it is configured, rather than labelling a control the user is already looking at.

**Every word of them is the shell's.**
The backend sends a field, its options and, where something is inert, a code naming the fact that made it so.
The sentence a reader sees is written where the column width, the tone and the language are visible (`ipc-api.md`).
So this page covers what a tooltip must say, not the component that renders one.
That belongs to the shell, whose layering `avalonia/README.md` states.

## Where the text lives

One name and one sentence per identifier the backend can send, in `avalonia/ScreenShare.App/Copy`:

| File | Holds |
| --- | --- |
| `Fields.cs` | a control's heading and help |
| `Vocabulary.cs` | what each identifier is called |
| `Descriptions.cs` | the paragraph behind a choice |
| `Statements.cs` | the sentence each code the backend can send is rendered as |
| `Counters.cs` | what each figure on a tile's stats panel is called, and what a reading of it is evidence of |

An identifier with no name renders as the raw identifier, and a statement with no sentence renders as its code.
Both are visible rather than swallowed, because that is what gets them written.

A tip whose text depends on the settings is still the shell's sentence, built from what the form resolved rather than from a rule the shell evaluates.
The quantizer scale is the case that needs it.
The scale follows the codec and, where the two engines set different properties, the capture backend's engine.
So the tip places its quality landmarks on the scale the running combination counts on.
Which scale that is arrives on the field.

## A tooltip that names a transport names its leg

Publish (publisher to relay) or watch (relay to viewer), never the bare protocol name.
The two legs are chosen independently, so one name would read as both (`viewer-architecture.md`, "Two legs, two protocols").
A fact that holds on one leg only belongs to that field's own text.
A leg-neutral fact about the protocol itself (what SRT's retransmission is, what HLS segments) is said once and serves every field that offers the protocol.

## An availability note is a line

An unavailable control or option states why on the screen, beside what it does rather than instead of it.
The description first, then the reason it is inert, so nothing that was there is lost.

A whole control the settings ignore carries the field's own statement.
A single greyed option carries its entry's.

A live control can carry a note the same way, without greying the field: the value still reaches the encoder and does something the base text does not describe (`field-availability.md`).

A sentence standing on the screen takes no tooltip repeating it.
Nothing opens over a greyed entry, and a disabled control takes no pointer events anyway, so the tip would open on every entry except the ones with something to say.

The reasons are not written per field.
They are the codes the availability pass produces from the tables (`form/availability.go`), so the shell cannot explain a greying the encoder does not have.

## Adding a tooltip

- A new field or option: add the sentence to `Copy` under the identifier the backend sends. Nothing renders it specially, and no markup is written.
- A new reason: the backend adds a code, and the shell adds the sentence for it. A code with no sentence is visible on screen, which is the point at which somebody writes one.
- A figure on a live screen: name it as the glossary names it, and say what a reader is meant to do with it rather than restating the unit already printed beside it.
