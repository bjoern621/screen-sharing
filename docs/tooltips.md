# Tooltips

Every settings field and every option a control offers can carry an explanatory tooltip.
They are the form's teaching layer: they explain the encoding model as it is configured, rather than labelling a control the user is already looking at.

**Every word of them is the shell's.**
The backend sends a field, its options and, where something is inert, a code naming the fact that made it so; the sentence a reader sees is written where the column width, the tone and the language are visible (`ipc-api.md`).
So this page is about what a tooltip must say, not about the component that renders one - that belongs to the shell, and `avalonia/README.md` states its layering.

## Where the text lives

The shell keeps one name and one sentence per identifier the backend can send, in `avalonia/ScreenShare.App/Copy`: `Fields.cs` for a control's heading and help, `Vocabulary.cs` for what each identifier is called, `Descriptions.cs` for the paragraph behind a choice, `Statements.cs` for the sentence each code the backend can send is rendered as, and `Counters.cs` for what each figure on a tile's stats panel is called and what a reading of it is evidence of.

An identifier with no name renders as the raw identifier, and a statement with no sentence renders as its code.
Both are visible rather than swallowed, because that is what gets them written.

A tip whose text depends on the settings is still the shell's sentence, built from what the form resolved rather than from a rule the shell evaluates.
The quantizer scale is the case that needs it: the scale follows the codec and, where the two engines set different properties, the capture backend's engine, so the tip places its quality landmarks on the scale the running combination actually counts on - and which scale that is arrives on the field.

## A tooltip that names a transport names its leg

Publish (publisher to relay) or watch (relay to viewer), never the bare protocol name, because the two legs are chosen independently and one name would read as both (`viewer-architecture.md`, "Two legs, two protocols").
A fact that holds on one leg only belongs to that field's own text.
A leg-neutral fact about the protocol itself - what SRT's retransmission is, what HLS segments - is said once and serves every field that offers the protocol.

## Availability notes are additive

When a control or an option is unavailable, its reason is appended to the normal tip rather than replacing it.
The tooltip states what the thing does, then a blank line, then why it is inert, so the description is never lost.

A whole control the current settings ignore carries the field's own statement; a single greyed option carries its entry's.
A field with no help text of its own shows the status note alone rather than an empty line above it.

A live control can carry a note the same way, appended without greying the field: the value still reaches the encoder and does something the base text does not describe (`field-availability.md`).

The reasons are not written per field.
They are the codes the availability pass produces from the tables (`form/availability.go`), so a tooltip cannot explain a greying the encoder does not have.

## Adding a tooltip

- A new field or option: add the sentence to `Copy` under the identifier the backend sends. Nothing renders it specially, and no markup is written.
- A new reason: the backend adds a code, and the shell adds the sentence for it. A code with no sentence is visible on screen, which is the point at which somebody writes one.
- A figure on a live screen: name it as the glossary names it, and say what a reader is meant to do with it rather than restating the unit already printed beside it.
