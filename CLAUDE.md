# Working in this repo

`docs/` is the source of truth for architecture, the domain model, terminology and style.
Read the relevant page before guessing.

`docs/development-principles.md` governs the shape of every change: explicit state writes and continuous reads, one render function per component, idempotent apply paths, and preconditions, postconditions and invariants asserted with `bjoernblessin.de/go-utils/util/assert`.
It outranks brevity and convenience. Read it before writing code, not after.

`avalonia/README.md` adds the layer rules for the shell; `docs/ipc-api.md` states what the shell may decide, which is nothing.
`docs/domain-model.md` covers the codec and transport tables every consumer derives from.

# Never drive the GUI

Never move the mouse, click, type into windows, or otherwise control the desktop/app UI. No `computer` tool, no automation scripts (AutoHotkey, PowerShell SendKeys, xdotool, nircmd), no browser-pane clicking to operate the app.

When manual UI interaction is needed, hand the user a checklist instead:

1. Numbered steps, one action per step. Name the exact control: window, tab, button label, field name.
2. Keep each step one short line. No prose paragraphs.
3. End with a **Screenshot:** line naming exactly what to capture — which window/region, which state it must be in, and what must be visible in frame.
4. Say what you will look for in the screenshot, so the user knows whether the capture is usable.

Example:

1. Open app, Settings tab.
2. Set Encoder to `NVENC`.
3. Click Start Stream.
4. Wait for status badge to stop saying `connecting`.

**Screenshot:** whole app window, Settings tab visible, status badge and encoder dropdown both in frame.
Looking for: badge text and selected encoder value.

Applies to running the app, reproducing bugs, and verifying fixes. Code, tests, CLI commands and file edits stay normal — that restriction is UI only.

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
