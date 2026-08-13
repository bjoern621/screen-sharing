# Working in this repo

`docs/` is the source of truth for architecture, the domain model, terminology and style.
Read the relevant page before guessing.

`docs/development-principles.md` governs the shape of every change: explicit state writes and continuous reads, one render function per component, idempotent apply paths, and preconditions, postconditions and invariants asserted with `bjoernblessin.de/go-utils/util/assert`.
It outranks brevity and convenience. Read it before writing code, not after.

# Idempotent and stateless

These are the two paradigms this codebase is built on, they outrank every other rule, and they are not negotiable. `docs/development-principles.md` states them in full. In short:

- **Idempotent.** Every operation is safe to run twice. Apply, sync, reconcile and every effect on the control contract: a second run with unchanged input changes nothing, creates nothing, restarts nothing. A call names the state it wants true, not the transition. A request for a state that already holds is a success, never an error - `StartReceive` on an open decode and `StopReceive` on a closed one both succeed. Anything that cannot be repeated makes a lost answer unrecoverable.
- **Stateless.** Nothing keeps a copy of a fact. One owner holds the state and every reader derives what it shows from that owner on demand. A render pass writes the whole component from the model and keeps nothing between runs. Static facts live in one table every consumer reads, never a `switch` restated per site. A reader reads through and reports what a read would answer, never what a caller believed it had just done.

When a design decision is open, take the option that keeps these two. Do not trade either for brevity, for a saved round trip or for a shorter diff. Every departure is written down where it happens, with its reason; an undocumented departure is a bug.

`avalonia/README.md` adds the layer rules for the shell; `docs/ipc-api.md` states what the shell may decide, which is nothing.
`docs/domain-model.md` covers the codec and transport tables every consumer derives from.

# Every function states its contract

Preconditions and postconditions are asserted in every function, with `bjoernblessin.de/go-utils/util/assert` in Go and `Contracts/Assert.cs` in C#.
Both are always on, in a release build as well, and neither is caught: the Go one panics and the C# one throws `ContractViolationException`.
A broken contract therefore ends the process at the frame that broke it, with the stack that led there.
That is the whole point of asserting rather than returning: the program stops where the bug is, before a wrong value travels far enough to look plausible somewhere else.
`docs/development-principles.md` ("Contracts") states the placement, the exhaustive-dispatch rule and the message style in full.

**A precondition goes at the top of the function, before any work.**
Asserted is every parameter the function cannot safely take garbage on: an index, a pointer or reference it dereferences, a callback it calls, a slice it indexes, an enum it dispatches on, and any relation between two parameters.

**A postcondition goes before the return**, and states what the caller is now entitled to assume.
The producing side is the one that should fail, not a consumer three layers away.

**An Entwicklungsfehler and an Umgebungsfehler are different failures and get different machinery.**
An Entwicklungsfehler, a Programmierfehler, is a broken contract inside this codebase: an index a widget computed wrong, a nil the constructor guarantees is set, an enum value no switch arm covers.
It is asserted, and the crash is the correct outcome.
An Umgebungsfehler is a condition the app has to survive: an unreachable relay, no GStreamer on the machine, a malformed roster push from another process, a log file that is not there.
It is carried as an error value or an exception to whatever surfaces it, and it never asserts.
`logger.Errorf` is a hard stop through `log.Fatalf`, so an Umgebungsfehler takes `logger.Warnf` instead.

The test is who can fix it.
A failure a user or an operator can fix is an Umgebungsfehler.
A failure only an edit to this repo can fix is an Entwicklungsfehler.
Getting it wrong costs in both directions: asserting an Umgebungsfehler kills the app over a network hiccup, and returning an error for an Entwicklungsfehler hands the caller a value this code cannot honour.

# Comments

A comment states the constraint the code cannot show, and nothing else.
Terse wins over complete. Fragments are fine, and a comment that restates the code is deleted rather than shortened.
The same applies to doc comments (`///`, `/** */`) and to `//` inside a function.

**Wrap at a sentence end, never mid-sentence.**
A source line holds one sentence, however short that leaves the line.
A sentence too long for the file's width wraps at a clause boundary, and a continuation line never starts a new sentence.
A diff then shows the sentence that changed rather than a reflowed block, and a sentence that ran away shows up as a line that ran away.

**A touched comment is rewritten in the same change.**
Editing the code under a comment means re-reading that comment and bringing it to this style, whether or not the edit made it false.
A stale comment that survived an edit is a defect on the same terms as a cached field nobody refreshed.

# Never drive the GUI

Never move the mouse, click, type into windows, or otherwise control the desktop/app UI. No `computer` tool, no automation scripts (AutoHotkey, PowerShell SendKeys, xdotool, nircmd), no browser-pane clicking to operate the app.

When manual UI interaction is needed, hand the user a checklist instead:

1. Numbered steps, one action per step. Name the exact control: window, tab, button label, field name.
2. Keep each step one short line. No prose paragraphs.
3. End with a **Screenshot:** line naming exactly what to capture: which window/region, which state it must be in, and what must be visible in frame.
4. Say what you will look for in the screenshot, so the user knows whether the capture is usable.

Example:

1. Open app, Settings tab.
2. Set Encoder to `NVENC`.
3. Click Start Stream.
4. Wait for status badge to stop saying `connecting`.

**Screenshot:** whole app window, Settings tab visible, status badge and encoder dropdown both in frame.
Looking for: badge text and selected encoder value.

Applies to running the app, reproducing bugs, and verifying fixes. Code, tests, CLI commands and file edits stay normal. That restriction is UI only.

# Caveman

Respond terse like smart caveman. All technical substance stay. Only fluff die.

Rules:

- Drop: articles (a/an/the), filler (just/really/basically), pleasantries, hedging
- Fragments OK. Short synonyms. Technical terms exact. Code unchanged.
- Pattern: [thing] [action] [reason]. [next step].
- Not: "Sure! I'd be happy to help you with that."
- Yes: "Bug in auth middleware. Fix:"

Switch level: /caveman lite|full|ultra|wenyan
Stop: "stop caveman" or "normal mode"

Auto-Clarity: drop caveman for security warnings, irreversible actions, user confused. Resume after.

Boundaries: code/commits/PRs written normal.
