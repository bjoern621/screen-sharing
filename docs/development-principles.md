# Development principles

Four rules govern every package in both modules.
They are ordered by what they protect: state that cannot drift, work that can be repeated, files that hold one idea, and contracts that fail loudly.

Everything else in `docs/` describes what the app does.
This page describes how its code is allowed to be shaped.

## State is written explicitly and read continuously

One piece of state has one owner, and the owner is a plain struct.
A write is a named method on that owner, and nothing outside it assigns the field.
A reader never keeps a copy: it reads through on demand, or it re-reads the owner on every change notification.

The failure this removes is two copies of one fact drifting apart.
A view that caches `stream.Transport` at construction and never refreshes it keeps naming the old transport after a leg change, and nothing in the type system says so.

Rules:

- A field on a view that mirrors a model field is a defect, unless it is a widget handle or a cache the render function refills on every pass.
- A model exposes accessors, not its slice. `Session.Stream(i)` is read through; a view that stores the result must refresh it from the same accessor when the model says it changed.
- State that must survive a restart is written through a store, not reconstructed from widgets.
- An observer that ignores a change kind is silently stale. Handle every kind or assert on the ones that cannot occur.

## One render function per component

Every view has exactly one function that maps model state to widget state.
The house names are `apply`, `draw` and `sync`.

Event handlers change the model and call the render function.
They do not reach into widgets themselves.
A handler that sets a label directly means the render function alone can no longer restore a correct view, and the component has two definitions of what it looks like.

The render function sets everything the component can show on every pass, including the branch that turns something off.
A property set only in the "on" branch is a property that sticks after the state that justified it is gone.

## Idempotency

Every apply, sync and reconcile is safe to run twice.
Running it a second time with unchanged input produces no visible change, no new widgets, no new signal handlers and no restarted work.

Naming carries the contract:

- `SetX`, `Apply`, `Sync`, `Draw`, `Rebuild`: idempotent. Calling twice equals calling once.
- `ToggleX`, `Move`, `Step`, `Advance`: not idempotent, and the verb says so.

A reconcile takes desired state and converges to it.
It does not take a diff, unless the diff is genuinely the input the process receives.
Where a subtree is small, clear-then-fill is preferred over an incremental patch: it is idempotent by construction and cannot leak a handler.

Where an operation is expensive, the guard belongs inside it.
`Attach(p)` with the player already attached returns without renegotiating, rather than asking every caller to check first.

## Components

One file, one responsibility.
A file over roughly 150 lines is a prompt to look for the seam, not a hard error.

A UI component is a directory or a small file set with the same shape each time:

| File | Holds |
| --- | --- |
| `<name>.go` | the struct, its constructor and its lifecycle |
| `render.go` | the single render function and what it derives |
| `input.go` | controllers, gestures and signal wiring |
| a data file | the declarative table the component reads |

Construction, rendering, input wiring and formatting are four responsibilities.
A file that does all four is the shape this rule exists to break up.

Static facts belong in a table, not in a `switch` spread through the logic.
`ui/stats/rows.go` and `ui/sidebar` status faces are the pattern: one table of facts, and every consumer reads it instead of restating the rule.
`docs/domain-model.md` covers the same principle for the codec and transport tables.

## Contracts

`bjoernblessin.de/go-utils/util/assert` is always on and panics.
It states internal contracts, which are bugs in this code ("Entwicklungsfehler").
An error value states an environment failure, which is a condition the app must survive ("Umgebungsfehler").
A malformed roster push from another process is an error; an index a widget computed wrong is an assert.

`logger.Errorf` ends the process through `log.Fatalf`, and `logger.Panicf` panics.
Both are hard stops, and neither reports a failure the app continues past: the code after such a call is unreachable, including the code that would have carried the reason to the user.
An Umgebungsfehler takes `logger.Warnf` and whatever the surrounding code already does to surface it.
`Errorf` also fixes the exit status at 1, so a process that owes its caller a different one writes to `os.Stderr` and calls `os.Exit` itself.

**Preconditions** go at the top of the function, before any work.
A function asserts every parameter it cannot safely take garbage on: an index, a pointer or interface it will dereference, a callback it will call, a slice it will index, an enum value it will dispatch on, and any relation between two parameters.

```go
assert.Assert(fromPos >= 0 && toPos >= 0, "both streams hold a slot in the display order", fromPos, toPos)
```

**Postconditions** go before the return, and state what the caller is now entitled to assume.
They matter most where a function's result feeds an assert somewhere else: the producing side should fail, not the consumer three layers away.

**Invariants** are asserted where they are relied on and after every mutation that could break them.
Asserting a permutation only on the path that builds it leaves the paths that reshuffle it unchecked.

**Exhaustive dispatch** ends in `assert.Never`.
Every `switch` over a closed internal enum has a `default: assert.Never("unexpected <thing>", int(v))`, and every map lookup over one asserts its `ok`.
Adding an enum value then fails at the sites that forgot it, instead of falling through in silence.

### Message style

An assertion message is a present-tense sentence stating the invariant that holds, not the failure that occurred.

- Lowercase, no trailing period.
- Subject is the domain role: `a spotlit stream`, `the display order`, `a coalescer`.
- Verb is simple present and active: `is`, `holds`, `covers`, `needs`, `yields`, `belongs to`.
- The offending values ride in the trailing varargs, never in the sentence.

```go
assert.Assert(s.spot == noSpot || s.at(s.spot).state.Watched(), "a spotlit stream is watched", s.spot)
assert.Assert(len(v.rows) == v.sess.Len(), "a row per stream", len(v.rows), v.sess.Len())
assert.IsNotNil(dispatch, "a coalescer needs a UI loop to defer to")
```

Not `"bad index"`, not `"i must be >= 0"`, not `"expected non-nil"`.
The sentence describes the world when the code is correct, so reading a panic tells the reader which truth stopped being true.

`assert.Never` is the one inversion: it names what turned up instead, as in `assert.Never("unexpected watch state", int(s))`.
It is never called without a message.
