# Style sweep status

Working file for the pass that brings every comment, doc page and user-facing string under the `bjoern` skills.
Disposable: delete it once the tree is committed.

The `writing-style` and `interface-text` skills state the rules, which surface takes which, and the gate each one runs.
This page records only what the sweep found and what was decided about it.

## The docs rewrite

`writing-style` carries two rules the sweep works to, uncommitted in the skills checkout.

- Architecture and programming are separate subjects, so a page describing the system names no source file, function, type or package path.
  This reverses the skill's former **Pointer** row, which endorsed `internal/authsvc` at the end of a section.
  `page-shape.sh` measured pointer share and rewarded it; it now counts implementation spans and the reference is zero.
- A decision is one line, the reasoning that reached it being the author's working.
  A rewrite landing near a tenth of the length is the ordinary result.

A page carrying more than one thesis is split by moving each section to the page whose subject owns it, rather than by adding pages.

Every page was read.
Total across `docs/`: 57834 words down to 52200, and every implementation span the rule bans is gone.

| Page | Words | Implementation spans | What happened |
|---|---|---|---|
| `capture-architecture.md` | 7319 to 1535 | 63 to 0 | rewritten, four sections moved out |
| `viewer-architecture.md` | 10442 to 5406 | 53 to 0 | rewritten, two tables moved to `domain-model.md` |
| `domain-model.md` | 4088 to 3931 | 45 to 0 | rewritten, and it took the codec and carriage tables |
| `video-stack.md` | 7198 to 6893 | 1 to 0 | the package-responsibility table cut, the delay section folded into a pointer |
| `delay-measurement.md` | 1074 to 1385 | 1 to 0 | took the publishing-side rate and shed figures |
| `plan.md` | 4308 to 4297 | 20 to 0 | spans stripped |
| `field-availability.md` | 3177 to 3142 | 10 to 0 | spans stripped, the argument section cut |
| `ipc-api.md` | 4181 to 4087 | contract | the argument section cut |
| `design-language.md` | 4019 to 4027 | third-party | token files named by role rather than by filename |
| `glossary.md` | 3657 to 3647 | 7 to 0 | code symbols cut from the term rows |
| `membership.md` | 1408 to 1393 | 12 to 0 | spans stripped |
| `packaging.md`, `install.md` | unchanged | config keys | Nix attributes and shipped binaries are contract |
| `development-principles.md` | unchanged | its subject | the page states that its subject is how the code may be shaped |
| `presets.md`, `network-architecture.md`, `tooltips.md`, `settings-editing.md`, `decode-timing.md`, `auth-flow.md` | near unchanged | to 0 | spans stripped |

Where a span survives it is contract vocabulary and stays: wire message and field names on `ipc-api.md`,
Nix attributes and shipped binary names on the packaging pages, a third-party package name on `design-language.md`.

Two pages stay long by genre rather than by padding.
`video-stack.md` is a field reference whose bulk is tables of standards, codecs and transfer functions.
`glossary.md` is a term list.
Both are the case `page-shape.md` names under "Where the numbers read differently".

The section cut and not relocated is `capture-architecture.md`'s "Adding a capture backend",
a code tour naming packages and types throughout.
It is in git, and it belongs on a page whose title says its subject is the code.

## Where the tree stands

`check-style.sh` over every file it governs, `CLAUDE.md` and `AGENTS.md` excluded as the rule files themselves:

| Check | Opened at | Stands at | Residual |
|---|---|---|---|
| MID-CLAUSE-WRAP | 99 | 0 | none |
| CHANGELOG-VOICE | 5 | 0 | none |
| PAIRED-NEGATION | 413 | 347 | read and kept, see below |
| VALUE-GLOSS | 27 | 26 | false positives |
| EM-DASH | 9 | 9 | ASCII art |
| INFLATED-VERB | 9 | 7 | the `harness` type |
| INTENSIFIER, INFLATED-WORD | 2 | 2 | false positives |
| HEDGE, PARTICIPIAL, VAGUE-ATTRIBUTION, CHAT-LEAKAGE, SELF-NARRATION, "seam" | 0 | 0 | none |

Every residual outside PAIRED-NEGATION was opened and read.

## How a finding is judged

The mechanical checks report candidates.
Five of them are read by eye before anything is edited, and most of what they raise here is left alone.

**PAIRED-NEGATION** is kept where the negated half answers an assumption the reader arrives with,
and where the sentence around it collapses without the contrast.
The repository leans on that shape for the two paradigms it is built on:
a read reports what a read would answer rather than what a caller believed it had done,
and an event carries a whole state rather than a delta.
Both name exactly what a reader would otherwise assume, so both stay.
The security guarantees in `group`, `token`, `membership` and `relay` read the same way,
and so does a correction of fact: `bframes, not b-frames`, `twelve kilobits a second, not twelve megabits`.
What is cut is the mirror that carries nothing:
`weighed and discarded, never read`, `A note and never a block`, `A reading and never a control`,
`A scrape is a read and never a write`.

**VALUE-GLOSS** fires on any comment opening with a number, so a proto field index (`// 4: reason as prose.`),
a chroma layout (`// 4:2:0 stores the luma plane whole`) and a port (`# 9997: the relay's API`) all raise it and all stay.
The twenty-sixth is `token.go`, where the mask blanks a quoted key and leaves `// No  : optional`.
It explains an absence a reader would restore, JWT's conventional `typ` header, and names the byte budget that rules it out.

**INFLATED-VERB** fires on `harness`, the name of a Go test type in `internal/decode`.

**INFLATED-WORD** fires on `underscores` in `portal/dbus.go`, where the word names the character.
**INTENSIFIER** fires on `absolutely` in `nix/relay-image.nix`, where it means the paths are absolute.

**EM-DASH** fires on the box-drawing horizontal inside the ASCII-art diagram in `docs/network-architecture.md`,
whose rows carry no corner character for the check to recognise them by.
That is the one place the character is allowed.

## Coverage

Every area below was read against both skills.

| Area | Files |
|---|---|
| `api/proto`, `api/buf.yaml` | 9 |
| `docs/` | 20 |
| `README.md` and the per-tree READMEs | 8 |
| `avalonia/ScreenShare.App` in full, including `Copy`, `Features`, `Controls`, `Design`, `Backend` | 214 |
| `avalonia/ScreenShare.App.Tests` | 43 |
| `backend/internal`, every package | 384 |
| `backend/cmd`, including `soak/scripts` | 22 |
| `scripts/`, `nix/`, `deploy/`, `Taskfile.yml`, `flake.nix` | 14 |

`docs/auth-flow.md` is untouched, being the reference page the others are shaped against.

## What this pass changed

Comments and docs:

- Mid-clause comment wraps across the tree: 99 down to 0.
  Every one now breaks at a sentence end, after a comma or colon, or before a conjunction.
- `backend/internal/publish`: 122 raw candidates down to 6, each of the 6 a judged keeper.
- `backend/internal/app`: 28 down to 18 keepers.
- `backend/internal/form`: eight label-plus-mirror openers rewritten, one `stands as` cut.
- `backend/internal/wire`, `encoders`, `metrics`, `membership`, `reach`: six mirrors cut.
- `api/buf.yaml`: rewrapped, and the breaking-change note rewritten off the commits it narrated.
- `backend/cmd/soak/scripts`: rewrapped.
- Four test failure messages rewritten to report the reading rather than the regression:
  `no longer reads as HDR` to `does not read as HDR`,
  `no longer declares the tune step` to `declares no tune step`,
  and the two like them in `gstpipeline_test.go` and `preview_test.go`.
  `soak/main.go` keeps `the backend process %d is no longer there`, a reading of the running system.

User-facing strings:

- `Maximize` in the title bar, which read `Maximise` beside an American-spelled `Minimize`
  (`Features/Shell/TitleBar/ViewModel/TitleBarViewModel.cs`).
  It is the only string in the shell that carried a second English variant.
- The viewer table's timestamp hint, which named the backend, a process split the reader cannot act on.
  Now `as of the last read from the relay`.
- `Worst viewer: round trip and loss`, the ampersand spelled out.

No user-facing sentence runs past 25 words, and no string carries a blame word, a semicolon,
an em-dash, an exclamation mark, `we`, `our`, or `the user`.

## Verification

- `go build ./...`, `go vet ./...`, `gofmt -l` clean across the backend.
- `go test` green on `publish`, `app`, `form`, `receive`, `wire`, `encoders`, `reach`, `metrics`, `membership`.
- `bash -n` clean on the three soak scripts.
- `dotnet build` clean, 0 warnings; 393 shell tests green.
