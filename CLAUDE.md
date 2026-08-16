# Working in this repo

`docs/` is the source of truth for architecture, the domain model, terminology and style.
Read the relevant page before guessing.
"Docs are short" below is the shape every page under it takes.

`docs/development-principles.md` governs the shape of every change: explicit state writes and continuous reads, one render function per component, idempotent apply paths, and preconditions, postconditions and invariants asserted with `bjoernblessin.de/go-utils/util/assert`.
It outranks brevity and convenience. Read it before writing code, not after.

# Idempotent and stateless

These are the two paradigms this codebase is built on, they outrank every other rule, and they are not negotiable. `docs/development-principles.md` states them in full. In short:

- **Idempotent.** Every operation is safe to run twice. Apply, sync, reconcile and every effect on the control contract: a second run with unchanged input changes nothing, creates nothing, restarts nothing. A call names the state it wants true, not the transition. A request for a state that already holds is a success, never an error - `StartReceive` on an open decode and `StopReceive` on a closed one both succeed. Anything that cannot be repeated makes a lost answer unrecoverable.
- **Stateless.** Nothing keeps a copy of a fact. One owner holds the state and every reader derives what it shows from that owner on demand. A render pass writes the whole component from the model and keeps nothing between runs. Static facts live in one table every consumer reads, never a `switch` restated per site. A reader reads through and reports what a read would answer, never what a caller believed it had just done.

When a design decision is open, take the option that keeps these two. Do not trade either for brevity, for a saved round trip or for a shorter diff. Every departure is written down where it happens, with its reason; an undocumented departure is a bug.

`avalonia/README.md` adds the layer rules for the shell; `docs/ipc-api.md` states what the shell may decide, which is nothing.
`docs/domain-model.md` covers the codec and transport tables every consumer derives from.

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

A comment states the constraint the code cannot show, and nothing else.
A comment that restates the code is deleted rather than shortened.
This governs `//`, `#`, `///`, `/** */`, XML doc comments and docstrings.
Commit messages are prose and are not governed here, and Markdown pages take "Docs are short" instead.

**Whether a comment is needed at all is decided first.**
A name that already says it takes no comment: `getPlayerGuid`, `isEmpty`, `maxRetries`.
Written down is only what the name cannot carry: a unit, a range, an invariant, a reason for an odd choice, an obligation on the caller.
The language rule below fixes the shape of a comment that exists, it does not call for one, so a doc comment with nothing but the name in it is not written.
- Bad: `// GetPlayerGuid returns the player GUID.`
- Good: no comment.
- Good: `// Zero until the roster push lands.`

**A comment is written clipped, not in prose.**
Articles, copulas and self-reference go: "a", "an", "the", "is the", "which is", "this function", "this method".
A noun phrase is a whole comment, and a fragment needs no trailing period.
- Bad: `// This function returns the negotiated codec for the given transport, or nil if the transport does not carry video.`
- Good: `// Negotiated codec. nil if transport carries no video.`

**Shortest form that keeps every fact wins.**
A comment that shrinks and still answers the "why" got better; one that shrinks by dropping a constraint got worse.
If a word can go without a fact going with it, the comment is not finished.

**A format, a value or a range is shown by example, not described by a rule.**
A sample value fixes the shape in fewer characters than a sentence about it, and it cannot contradict itself.
- Bad: `// The key is the transport name in lowercase, a slash, then the codec name in uppercase.`
- Good: `// Key: "rtsp/H264".`
- Bad: `// Accepts a duration in milliseconds between one and sixty thousand.`
- Good: `// ms, 1..60000.`

**The language's own comment convention comes first.**
Clipping happens inside that shape and never replaces it.
Where the convention demands a word the rules above would cut, the convention wins.
- Go: a doc comment starts with the identifier it documents. `// StartReceive opens decode for stream. Already open is success.`
- C#: XML doc comments, `<summary>`, `<param>`, `<returns>`, one clipped line each.
- TypeScript and JavaScript: JSDoc, with `@param` and `@returns` carrying the facts instead of a prose paragraph.
- Python: a docstring in the style the file already uses.
- Rust: `///` on the item, `//!` on the module.

**Wrap at a sentence end, never mid-sentence.**
A source line holds one sentence, however short that leaves the line.
A sentence too long for the file's width breaks after a comma or before a conjunction ("and", "or", "but", "so"), never mid-clause, and a continuation line never starts a new sentence.
A diff then shows the sentence that changed rather than a reflowed block, and a sentence that ran away shows up as a line that ran away.

**A touched comment is rewritten in the same change.**
Editing the code under a comment means re-reading that comment and bringing it to this style, whether or not the edit made it false.
A stale comment that survived an edit is a defect on the same terms as a cached field nobody refreshed.

**A change sweeps every comment it puts a reader in front of, not only the ones it touched.**
Each one is asked whether it can be shorter, whether it can be sharper, and whether it can go.
A fact stated in two places is kept in one, at the site that owns it: two copies drift, and the reader then trusts whichever is wrong.
Volume is itself the defect being cut, a file carrying more explanation than code being one nobody edits without reading it twice.

**Every comment gets a second pass.**
The first draft states the thought, the review pass makes it worth reading.
Re-read each comment before moving on and rewrite it, every time, not only when it looks bad.
"It already reads fine" is not an outcome of the pass, so find the cut.

Ask, in order:
- Does this state a constraint the code cannot show? If it restates the code, delete it.
- Is this fact already stated at a site that owns it better? Then delete it here.
- Which words carry no fact? Cut them.
- Does the reader need every sentence, or only the "why"? Keep the "why".
- Can it be shorter without losing a fact? Then it is not finished.

Cutting words is free. Cutting facts is not.

# Docs are short

`docs/auth-flow.md` is the shape every Markdown page in this repository takes.
A page is read once, by somebody meeting the thing for the first time, and it is written for that reading.

**Lead with the fact that orients.**
Two or three lines before the first heading, saying what the thing is and what the reader has to hold on to.
Not a preview of the sections below.

**A diagram replaces a paragraph.**
Mermaid in a fenced block: `sequenceDiagram` for an exchange between parts, `flowchart` for the path something takes.
Anything enumerable is a table.
Prose is what neither of those could carry.

**One sentence per line, and the sentence is short.**
A line that runs to three clauses is two sentences that have not been separated yet.

**A page is written clipped, not in prose.**
The register is "Comments" above, applied to a whole page: articles, copulas and self-reference go, and a fragment is a whole line.
"The seam is the `publish.Publisher` interface" is "Seam: `publish.Publisher`".
"which is what keeps a stream's look off the capture backend" is "so a stream's look stays off the capture backend".
What never goes is the fact under the words, so a line that lost a constraint on the way to being shorter is reverted rather than kept.

**One telling, not two.**
A reason stated when the rule is introduced is not restated when the rule is used.
A sentence that begins "That is why", "which is the reason" or "the same fact that" is pointing at something the reader has already read, and it goes.

**A section is one screen.**
Longer than that is either two sections or a page holding something another page owns.

**Point at the code rather than restating it.**
Name a file, a symbol or a config key and let it answer "what is set".
The page answers "why it is shaped that way".

**Every fact stays.**
Volume is the defect, never detail.
What goes is the second telling, the transition sentence, the recap, the adjective, the "Overview" that previews the page.
A page that got shorter by dropping a constraint got worse.

**A page states no fact that rots.**
No counts, no dates, no "currently", no status snapshot, no version the repository does not itself pin.
State the invariant that produces the fact instead.

**Every page gets a second pass.**
Re-read it and find the cut, every time, not only when it reads badly.
"It already reads fine" is not an outcome of the pass.

# Every word states what is, never what changed

What a thing replaced, what it used to be, what is planned for it and what building it was like are **changelog voice**.
A commit message and a PR description are where that belongs, being the two things whose subject is a change.
Everywhere else it is cut, and everywhere else is all of it: every string the app shows, every comment, every page under `docs/`, every `logger` line, assert message, test name and failure message, and every identifier.

**Changelog voice is text about a thing's timeline or its construction, standing where the reader came for the thing itself.**
Three faces.

- *Past.* "and never was", "used to", "as before", "since the rewrite", "moved here from", "new".
  History the reader does not hold and cannot use.
- *Future.* "not yet", "still to come", "coming", "planned", "for now".
  A promise the sentence cannot keep, leaving the reader waiting instead of using what is there.
- *Workshop.* "by design", "we decided", "after some experimentation", "it turned out that".
  The making of the thing rather than the thing.

**A clause arguing with somebody is the tell.**
"and never was" rebuts a complaint nobody made.
Writing here has no opponent: it reports a state and names a consequence.

**Naming an absence teaches a capability and then takes it away.**
"There is no live-safe change here" hands a reader who never imagined reconfiguring a running encoder the idea, and denies it in the same breath.
Name what does exist instead.

Per surface:

- *Anything the app shows* is strictest and takes no exception, wherever the string lives: `Copy/`, a feature table such as `Features/Setup/Model/CommitCopy.cs`, a literal in an `.axaml` view, a refusal the backend puts on the contract.
  The reader met this app a minute ago and will never open this repository, so its history and its making are about somebody else, and so are its internals: a pipeline element, a process split, a cause they cannot act on.
  An error string is the one exception in one direction, an identifier a bug report needs staying because getting it to a maintainer is the reader's next action.
  - Bad: "Applying restarts the stream. The encoder is torn down and launched again on these settings - there is no live-safe change here and never was - so viewers on X lose the picture."
  - Good: "Applying restarts the stream on these settings. Viewers on X lose the picture for a moment and reconnect."
  - Bad: "The quality track is no longer editable while sharing." / Good: "Locked while sharing. Stop the stream to change it."
  - Bad: "Nothing carries the pointer over the relay yet." / Good: "The pointer is drawn into the picture on the way out."

- *Comments* are written from the file, never from the diff.
  Git holds what changed, and holds it accurately.
  A comment narrating an edit is stale under the next edit, which is the defect "Comments" above already calls a stale comment.
  - Bad: `// Now returns nil instead of an error.` / Good: `// nil when the transport carries no video.`
  - Bad: `// Changed to a table after the switch got unmanageable.` / Good: `// One row per codec, so a new codec is a row.`

- *Pages under `docs/`* describe the system, not the road to it, which is "A page states no fact that rots" in "Docs are short" applied to the road as well as to the clock.

- *Identifiers* date themselves and then lie.
  `newParser`, `parserV2`, `legacyPath`, `oldHandler`: true for one release, confusing after the next, and a third arrival has nowhere to go.

- *Logs, asserts and test failures* report the reading, not the regression.
  - Bad: `t.Fatal("this broke when the ladder table landed")` / Good: `t.Fatalf("%s declares no ladder step", codec)`.

**A state whose name holds a past is not history.**
"Disconnected", "Reconnecting", "Frozen for 12 s", "stale", "a client no longer listening" are readings of the running system and they stay.
Banned is the *product's* history, not the *session's*.
So is code whose subject genuinely is change: `settings/migrate.go` names an old key and a new one because that is its present job.

**An absence is explained only where a reader would restore it**, which is the one place "was there before" earns its line.

The test, clause by clause:
- Would a reader who met this a minute ago, and will never see its history, act differently because of the clause? No: cut it.
- Does the clause only parse for somebody who knows a previous version, a rejected design, or how the work went? Cut it.
- Strike every word answering "when did this change" and "what was building it like". What is left is the sentence.

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
It is a debugging instrument, not a closing ritual.

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
