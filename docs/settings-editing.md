# Editing a setting

The draft belongs to the shell.
Nothing a reader moves reaches the backend as a setting until they say so.
What crosses while they edit is one read that stores nothing.

## Who holds what

| Fact | Owner | Lifetime |
|---|---|---|
| Draft, the settings being edited | Shell, one per window | Until the window closes |
| Form: which controls exist, which entries are greyed, the ends of a range, what a configuration costs | Backend, answered per draft (`ResolveForm`) | Replaced by the next answer |
| Stored settings | Backend, on disk | Until a write replaces them |

The window opens on the stored settings, read once, and edits that copy.

## What crosses, and when

| Reader does | Call | Cadence |
|---|---|---|
| Picks an entry, flips a switch, steps a slider by key | `ResolveForm` | Per change, one in flight at a time |
| Sweeps a slider's thumb | `ResolveForm` | On letting go |
| Types in a text box | `ResolveForm` | On leaving the box |
| Presses a group's save | `SaveSettings` | Per press |
| Presses publish | `StartPublish` | Per press |
| Edits a group the form marks `applied` | `SaveSettings` | Per change, one write in flight at a time |

`ResolveForm` is a read: no file is written, no pipeline is touched, and asking twice for one draft answers twice the same (`ipc-api.md`).

## The form is an answer about one draft

A codec decides which pixel formats are offered, a transport decides which codecs it carries, and a rate-control mode decides which knobs mean anything.
So the form for one draft says nothing about the next: a changed draft is a different question, and the answer to it is the backend's alone (`field-availability.md`).

A resolve therefore sits on the path of every keystroke, which fixes what it may cost.
The encoder probe is taken once and read from memory, the monitor enumeration answers from a bounded snapshot, and every static table is built once rather than per lookup.

## One resolve at a time

A write while an answer is out does not start a second call.
The answer lands, and where the draft has moved since, one more resolve asks about what the reader is holding now.
A slider dragged across its range therefore costs a round trip per answer rather than one per pointer move.
The reader waits the same round trip either way.

The same shape holds for the write half: one `SaveSettings` is out at a time, and the newest draft is the one that lands.

An answer describing a draft the reader has already left is still drawn, being the newest answer there is.
The draft does not take it back, since adopting it would drag a control back to where the pointer was a round trip ago.

## What the reader sees while an answer is out

A control shows what it holds, and the figure beside it is printed from that.
So a number follows the thumb, and it is right in the window between a move and the form that confirms it.

A control takes a value off the form only where that form answers for the draft as it stands.
Outside that window the two disagree, the reader having moved since the question went out.
A repair therefore lands on the answer that carries it, which is what moves a thumb the backend walked to a legal value.

The greyings, the ranges and the entries around the control are the last answer's throughout.
A sweep holds them for the length of the gesture, and the release brings them up to date.

## A sweep is one value, held

A thumb under the reader's pointer writes a value per pointer move.
Each of them is a configuration passed through on the way to the one they stop on.
The draft takes all of them, and the backend is asked about the last.

The gesture is the view's to know.
The widget reports taking the thumb and letting it go, and reports letting go for a pointer taken away by something else.
A draft is therefore never left holding a question the reader has stopped asking.

A key press is not a sweep, being a settled value the moment it happens.

## Staged and applied

A group is staged unless the form marks it `applied`.

Staged is the default: the draft carries the change and a commit is what stores it.
Applied is for settings the backend reads on a schedule of its own, the relay's address among them.
The form says which, and the shell reads the mark (`ipc-api.md`).
