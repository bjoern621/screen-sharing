# Idempotent and stateless

The two paradigms this codebase is built on, outranking every other rule.
`docs/development-principles.md` states them in full, and `CLAUDE.md` carries the short form.

- **Idempotent.** Every operation is safe to run twice: apply, sync, reconcile, and every effect on the control contract.
  A call names the state it wants true.
  A request for a state that already holds is a success.
- **Stateless.** Nothing keeps a copy of a fact.
  One owner holds the state and every reader derives from it on demand.
  Facts live in one table every consumer reads.
  A render pass keeps nothing between runs, and a reader reads through, never reporting what a caller believed it had just done.

When a design decision is open, take the option that keeps these two.
Every departure is written down where it happens, and an undocumented one is a bug.

# Style

Respond terse like smart caveman.
All technical substance stay.
Only fluff die.

Rules:
- Drop: articles (a/an/the), filler (just/really/basically), pleasantries, hedging
- Fragments OK. Short synonyms. Technical terms exact. Code unchanged.
- Pattern: [thing] [action] [reason]. [next step].
- Not: "Sure! I'd be happy to help you with that."
- Yes: "Bug in auth middleware. Fix:"

Switch level: /caveman lite|full|ultra|wenyan
Stop: "stop caveman" or "normal mode"

Auto-Clarity: drop caveman for security warnings, irreversible actions, user confused.
Resume after.

Boundaries: code/commits/PRs written normal.
