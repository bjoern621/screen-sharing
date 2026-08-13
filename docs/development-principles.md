# Development principles

Everything else in `docs/` describes what the app does.
This page describes how its code is allowed to be shaped.

## The two paradigms

**Idempotent** and **stateless** outrank every other rule on this page, and the rules below are how they are held to.
Where a design decision is open, the one that keeps these two is the one taken, and neither is traded for brevity, for a round trip, or for a shorter diff.

**Idempotent** means an operation is safe to run twice.
Applying, syncing, reconciling and asking the backend for something are all repeatable: the second run with unchanged input changes nothing, creates nothing and restarts nothing.
A call names the state it wants to be true, not the transition it wants performed, and that is what makes the second call mean nothing rather than mean it again.
What this buys is a caller that never has to know what has already happened.
A step that cannot be repeated forces every caller to track whether it ran, and a caller whose answer went missing then has nothing left to do but wait.

**Stateless** means nothing keeps a copy of a fact.
One owner holds each piece of state and everything else derives what it needs from that owner at the moment it needs it: a render function writes the whole component from the model, a consumer reads the one table rather than restating the rule at its own site, an effect answers empty and the state arrives read back off the thing that owns it.
What this buys is one definition of each fact.
A second copy - cached at construction, restated per site, or remembered from what a caller believed it had just done - drifts from the first, and the two then disagree without either being wrong.

The two hold each other up.
An operation that names a state is idempotent by construction, because a state that already holds needs nothing done to it, and an operation that is idempotent can be called from a render pass, which is what lets that pass keep nothing of its own between runs.

The rules below are ordered by what they protect: state that cannot drift, work that can be repeated, facts that are stated once, files that hold one idea, and contracts that fail loudly.

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

### Effects across a process boundary

An effect on the control contract names the state it wants, and a request for a state that already holds succeeds.
`StartReceive` on a decode that is already open is not a second decode and is not an error; `StopReceive` on one that is already closed is not a failure.
`StartPublish` naming the pipeline that is already publishing is the same case, backoff and all.
Each of them is the state the caller asked for, and it is true.

The state is read before the request is validated, and the order is not an economy.
A precondition moves under a state that already holds - the relay reports a format the running decode's leg stopped carrying, the settings the viewer was opened on changed - and a validation placed first would refuse a repeat on behalf of a state it was never asked to establish.

The reason is the answer, not the effect.
A shell that sent a call and did not hear back cannot tell "not done" from "done, answer lost", and the only move that resolves it is asking again.
A method that refuses a repeat takes that move away and leaves the caller waiting on an answer that is not coming, which is a control that never comes back rather than one that failed.

What a repeat is not is a second, different request.
`StartPublish` naming a *different* pipeline while one is publishing is still refused, because that would put two encoders on one relay path; `ApplyToStream` names a transition on purpose and a second one is a second restart.

The third departure is the handful of effects that end in a program this process does not own: `OpenLog`, `OpenLogsFolder` and `OpenInBrowser` hand a path or an address to the desktop.
A second call opens a second window, because there is no state to read back that would let it decide the first one is still there - the browser owns the tab and the file manager owns its window, and neither reports.
An effect of this kind states no state and is offered as an action rather than as something with a tick beside it, which is what keeps the departure visible in the interface instead of only in the code.

Those are the departures, they are written down where they happen, and the sentence a timed-out call shows is worded against them.

Every call over the socket is bounded.
An unbounded call turns a lost answer into a permanent wait, and on a local socket "no answer" means the other side died, wedged or lost the connection - all facts worth showing rather than waiting through.
The bound belongs on the channel, in one place, and not at each call site: a rule applied per call site holds only where somebody remembered it.

A failure that says the backend went quiet says what is and is not known about the attempt, and says that anything naming a state is safe to ask for again.
That sentence is only truthful because those effects are idempotent, which is the paradigm paying for itself - and it says "naming a state" rather than "every call" because of the departures above.

## Stateless

Three shapes carry the paradigm, and each is stated in full elsewhere on this page.
They are collected here because they are one idea wearing three costumes.

**A pass keeps nothing between runs.**
A render function writes every property the component can show, including the off branch, so the pass by itself defines the view and is free to run at any time; a reconcile takes desired state and converges to it.
Neither works from a diff it was handed, unless the diff is genuinely what the process received - a diff is a fact somebody had to keep between two moments, and the pass that needs one has state in it.

**A fact lives in one table.**
Static knowledge - which transports carry which formats, which chain a platform renders through, what a row shows - is a table every consumer reads, never a `switch` restated at each site.
`docs/domain-model.md` covers the codec and transport tables; the render chains in `internal/receive/chains.go` are the same shape.

**A reader reads through.**
Nothing here reports what a caller believed it had just done.
An effect answers empty and the state arrives on the event stream, read back off the thing that owns it: the receive state is assembled from the running pipelines, the viewer roster from the processes, the relay snapshot from the poll.
A value cached at construction and never refreshed is the defect this removes.

The one departure is written down where it happens.
The shell's tile grid is shell state and not the backend's, because the contract describes no grid, and `internal/app` has nothing to read one back from.
A departure that is not written down is a bug.

## Components

One file, one responsibility.
A file over roughly 150 lines is a prompt to look for the seam, not a hard error.

A UI component is a directory or a small file set with the same shape each time:

| File | Holds |
| --- | --- |
| `<name>.go` | the struct, its constructor and its lifecycle |
| `render.go` | the single render function and what it derives |
| `input.go` | controllers, gestures and signal wiring |
| a data file | the table of facts the component reads |

Construction, rendering, input wiring and formatting are four responsibilities.
A file that does all four is the shape this rule exists to break up.

Static facts belong in a table, not in a `switch` spread through the logic.
`platform.AudioSources` is the pattern: one row per capture source, read by the form, the repair and both publish engines instead of each restating what a machine can capture.
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
