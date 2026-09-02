# Tooltips

Every settings field and every option a control offers can carry a tooltip.
A tooltip teaches the encoding model the form is configuring, so it carries what the label cannot.

**Every word of a tooltip is the shell's.**
The backend sends a field, its options and, where something is inert, a code naming the fact that made it so.
The sentence a reader sees is written where the column width, the tone and the language are visible (`ipc-api.md`).
The component that renders one belongs to the shell (`avalonia/README.md`).

## Where the text lives

One name and one sentence per identifier the backend can send, in `avalonia/ScreenShare.App/Copy`:

| File | Holds |
| --- | --- |
| the field table | a control's heading and help |
| the vocabulary table | what each identifier is called |
| the description table | the paragraph behind a choice |
| the statement table | the sentence each code the backend can send is rendered as |
| the counter table | what each figure on a tile's stats panel is called, and what a reading of it is evidence of |

An identifier with no name renders as the raw identifier, and a statement with no sentence renders as its code.
Both show on screen, which is what gets the missing text written.

A tip whose text depends on the settings is still the shell's sentence, built from what the form resolved.
The quantizer scale is the case that needs it.
The scale follows the codec and, where the two engines set different properties, the capture backend's engine.
So the tip places its quality landmarks on the scale the running combination counts on.
Which scale that is arrives on the field.

## A tooltip that names a transport names its leg

Publish (publisher to relay) or watch (relay to viewer), never the bare protocol name.
The two legs are chosen independently, so one name would read as both (`viewer-architecture.md`, "Two legs, two protocols").
A fact that holds on one leg only belongs to that field's own text.
A leg-neutral fact about the protocol itself (what SRT's retransmission is, what HLS segments) is said once and serves every field that offers the protocol.

## Availability notes

An unavailable control or option states why on the screen, beside what it does.
The description comes first, then the reason it is inert.

A whole control the settings ignore carries the field's own statement.
A single greyed option carries its entry's.

A live control can carry a note the same way, without greying the field: the value still reaches the encoder and does something the base text does not describe (`field-availability.md`).

A sentence standing on the screen takes no tooltip repeating it.
Nothing opens over a greyed entry, and a disabled control takes no pointer events anyway, so the tip would open on every entry except the ones with something to say.

A reason is one of the codes the availability pass produces from the tables, so the shell cannot explain a greying the encoder does not have.

## Adding a tooltip

- A field or option: add the sentence to `Copy` under the identifier the backend sends. Nothing renders it specially, and no markup is written.
- A reason: the backend adds a code, and the shell adds the sentence for it. A code with no sentence is visible on screen, which is the point at which somebody writes one.
- A figure on a live screen: name it as the glossary names it, and say what a reading of it is evidence of. The unit is printed beside it already.
