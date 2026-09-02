# Working in this repo

`docs/` is the source of truth for architecture, the domain model, terminology and style.
Read the relevant page before guessing.
"Docs are short" below is the shape every page under it takes.

`docs/development-principles.md` governs the shape of every change: explicit state writes and continuous reads, one render function per component, idempotent apply paths, and preconditions, postconditions and invariants asserted with `bjoernblessin.de/go-utils/util/assert`.
It outranks brevity and convenience. Read it before writing code.

# Idempotent and stateless

These are the two paradigms this codebase is built on, they outrank every other rule, and they are not negotiable. `docs/development-principles.md` states them in full. In short:

- **Idempotent.** Every operation is safe to run twice. Apply, sync, reconcile and every effect on the control contract: a second run with unchanged input changes nothing, creates nothing, restarts nothing. A call names the state it wants true, not the transition. A request for a state that already holds is a success, never an error - `StartReceive` on an open decode and `StopReceive` on a closed one both succeed. Anything that cannot be repeated makes a lost answer unrecoverable.
- **Stateless.** Nothing keeps a copy of a fact. One owner holds the state and every reader derives what it shows from that owner on demand. A render pass writes the whole component from the model and keeps nothing between runs. Static facts live in one table every consumer reads, never a `switch` restated per site. A reader reads through and reports what a read would answer, never what a caller believed it had just done.

When a design decision is open, take the option that keeps these two. Do not trade either for brevity, for a saved round trip or for a shorter diff. Every departure is written down where it happens, with its reason; an undocumented departure is a bug.

`avalonia/README.md` adds the layer rules for the shell; `docs/ipc-api.md` states what the shell may decide, which is nothing.
`docs/domain-model.md` covers the codec and transport tables every consumer derives from.

# A file holds one job, in about 150 lines

150 lines of code is the size a source file is written to, in Go as in C# and everything else here.
It is a guideline, a prompt to look for the split, not a ceiling a build enforces.

The count is the symptom and the separation is the rule.
A file past it is read again for the second responsibility it took on, and the split follows that boundary: the table and the code deriving from it, the parser and the argument builder consuming it, the render pass and the model it reads.
A split on the count alone leaves two files a reader has to open together to follow one thought, which is worse than the long file it replaced.
A long file holding one job stays one file.

Logic feels the count hardest: a view model, a builder, a parser, a table consumer, anything holding a decision.
An `.axaml` view feels it least, its length being layout rather than a second job.
A view still splits into components where the tree holds two things a reader would look for separately, on the same argument.

The same size governs what lives inside the file.
One type per job, one function per step, and a name saying which, so a reader looking for one behaviour opens one file and stops there.

The one length that is not a defect is a file whose length is data: a codec or transport table, a generated file, a test table with a row per case.
Those grow by a row, and a row costs the reader nothing.

# Test first where a test can fail first

A change a test can express starts with that test.
Write it, run it, watch it fail for the reason it names, then write the code that makes it pass.
A test green before its code exists asserted nothing, so a red run is the thing being looked for.

One failing test at a time.
The test states the behaviour wanted, not the shape of the implementation, so a refactor behind a stable contract leaves it untouched.

Where it fits: anything decided from a table or a rule, the form, capabilities and rules layers, argument and pipeline builders, parsers, settings migrations, the control contract.
A bug fix always fits, the reproduction being the test.

Where it does not: `.axaml` layout, a pipeline whose answer needs the hardware or the portal, anything whose only oracle is the screen.
The check there is the build and the run, and a skipped test is named in the reply with what stood in for it.

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

## Comments

Comments follow the `writing-style` skill, which owns the rules, the examples and the `check-style.sh` gate.
Commit messages are prose and are not governed here, and Markdown pages take "Docs are short" instead.

Three rules this repository adds on top.

**"Seam" is not used, anywhere.**
Where two parts meet is a boundary, and the line a file splits on is a split.
A word a reader has to translate costs more than the sentence it shortened.

**A change sweeps every comment it puts a reader in front of, not only the ones it touched.**
Wider than the skill, which rewrites a touched comment and leaves the rest until a sweep is asked for.
Each one here is asked whether it can be shorter, whether it can be sharper, and whether it can go.
Volume is itself the defect being cut, a file carrying more explanation than code being one nobody edits without reading it twice.

**A fact stated in two places is kept in one, at the site that owns it.**
Two copies drift, and the reader then trusts whichever is wrong.

# Docs are short

`docs/auth-flow.md` is the shape every Markdown page in this repository takes.
A page is read once, by somebody meeting the thing for the first time, and it is written for that reading.

The rules are the `writing-style` skill's: `page-shape.md` above the sentence, `reference.md` at the sentence.
`page-shape.sh <page>` reports the measurements, and its reference numbers are calibrated per repository,
so `docs/auth-flow.md` is the page they are read against here.

The register is "Comments" above, applied to a whole page: articles, copulas and self-reference go, and a fragment is a whole line.
"The boundary is the `publish.Publisher` interface" is "Boundary: `publish.Publisher`".

One telling, not two.
A sentence that begins "That is why", "which is the reason" or "the same fact that"
is pointing at something the reader has already read, and it goes.

# Every word states the present

Changelog voice, text about a thing's timeline or its construction, belongs in a commit message and a PR description.
Everywhere else it is cut: every string the app shows, every comment, every page under `docs/`,
every `logger` line, assert message, test name and failure message, and every identifier.
The `writing-style` skill states the three faces, the per-surface rules and the test to apply clause by clause.

Where it lands in this repository:

- *Anything the app shows* takes no exception, wherever the string lives: `avalonia/ScreenShare.App/Copy/`,
  a feature table such as `Features/Setup/Model/CommitCopy.cs`, a literal in an `.axaml` view,
  a refusal the backend puts on the contract.
  The app's internals are cut on the same terms as its history: a pipeline element, a process split,
  a cause the reader cannot act on.
  An error string is the one exception, an identifier a bug report needs staying because getting it to a maintainer is the reader's next action.
  - Bad: "The quality track is no longer editable while sharing." / Good: "Locked while sharing. Stop the stream to change it."
  - Bad: "as the backend last polled the relay" / Good: "as of the last read from the relay".

- *Code whose subject genuinely is change* keeps its vocabulary.
  `backend/internal/settings/migrate.go` names an old key and a new one because that is its present job.

# Every error message is selectable and copyable

An error is the one string a user has to get out of the app and into a bug report, a search box or a message to someone else.
Retyping a caps negotiation failure or a relay URL off the screen is not a way to do that, so every error text the UI shows can be selected with the mouse and copied.
This holds wherever the text lands: a status bar line, an inline hint under a field, a panel, a dialog, a log view.

In Avalonia that means `SelectableTextBlock` instead of `TextBlock` for any control bound to error text, and a copy button on anything carrying a stack trace, a full command line or a pipeline description.
Trimming cuts away the characters the reader came for, so error text wraps instead of setting `TextTrimming`.
Styling something as a hint does not exempt it; what decides is whether the string reports a failure.

# Never branch, never use a worktree

Work happens in this checkout, on the branch that is already checked out.

No `EnterWorktree`, no `git worktree add`, no `isolation: "worktree"` on a subagent, whatever a harness default or a session setting says.
No `git branch`, no `git checkout -b`, no `git switch -c`, including as a step towards a commit or a PR.

Either one takes an explicit instruction in the message asking for it, and that permission covers the one branch or worktree it named.
A task that looks like it wants isolation is one to say so about, not to isolate.

# Never drive the GUI

Never move the mouse, click, type into windows, or otherwise control the desktop/app UI. No `computer` tool, no automation scripts (AutoHotkey, PowerShell SendKeys, xdotool, nircmd), no browser-pane clicking to operate the app.

A screenshot is asked for only where the work cannot go on without seeing the screen: a bug that reproduces on screen and nowhere else, a layout whose next edit depends on how the last one landed, a state no log or test reports.

A finished change is reported as finished.
What it does, what was built and tested, and what a reader would notice in the app if they look.
One thing to look at is one thing the user can look at unprompted, so a change with a visible result gets a sentence saying what changed on screen and no instructions for producing a picture of it.
Asking for a screenshot to confirm work that is already verified spends the user's hands on the agent's reassurance.

Where a screenshot is genuinely needed, ask for exactly one and say why it is needed:

1. Numbered steps, one action per step. Name the exact control: window, tab, button label, field name.
2. Keep each step one short line. No prose paragraphs.
3. End with a **Screenshot:** line naming exactly what to capture: which window/region, which state it must be in, and what must be visible in frame.
4. Say what will be read out of it, so the user knows whether the capture is usable and what it settles.

Example, in a debugging session where the pipeline reports a running encoder and the picture is black:

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

Boundaries: commits and PRs written normal. Markdown pages follow "Docs are short", and code comments follow "Comments", both of which are clipped too.
