# Frontend coding style

The rules the React/TypeScript frontend follows. The guiding principle is **one
file, one responsibility, and UI kept separate from logic**. A component renders;
it does not fetch, compute, or hold cross-cutting state. Logic lives in hooks and
plain modules that a component consumes through a small, typed contract.

## Layers

Every file belongs to exactly one layer. The layer decides what the file may
import and what it may do.

| Layer            | Location            | May contain                                              | Must NOT                                        |
| ---------------- | ------------------- | -------------------------------------------------------- | ----------------------------------------------- |
| **types**        | `src/types/`        | shared type/interface definitions                        | runtime code                                    |
| **util**         | `src/util/`         | pure functions and static data, framework-agnostic       | import React; hold state; touch the DOM         |
| **services**     | `src/services/`     | stateful, framework-agnostic classes (managers, writers) | import React                                    |
| **hooks**        | `src/hooks/`        | React-bound state and effects; wraps util/services/bindings | render JSX                                    |
| **components**   | `src/components/`   | presentational JSX                                        | fetch, poll, or own cross-cutting state         |
| **composition**  | `src/App.tsx`       | wire hooks to components                                  | contain logic of its own                        |

Data flows one way: `util` (pure) → `hooks` (stateful) → `components` (render) →
`App` (compose). A component receives everything it needs as props; it never
reaches back into a hook or a binding.

### types/

One place for shared shapes (`Stream`, `Stats`, `Option`, `Deps`, …). Re-export
backend-generated types here (e.g. `export type Stream = settings.Stream`) so the
rest of the app imports domain names, not generated paths.

### util/

Pure logic and static metadata. No React, no side effects, trivially testable.
Examples in this repo:

- `deps.ts` — `evaluateDeps`, `normalize` (constraint logic, mirrors the Go encoder)
- `browser.ts` — `browserCheck` (a verdict from settings)
- `options.ts`, `presets.ts` — option metadata and presets, plus small helpers
  like `labelFor` and `monitorOptions`
- `format.ts` — display formatters (`mbps`, `dropPercent`)

If a function does not need React, it goes here — not in a component and not in a
hook.

### services/

Reserved for stateful, framework-agnostic classes (connection managers, file
writers, stat samplers). Instantiated and driven by a hook or context, never by a
component directly. (None yet in this repo; the layer exists for when logic
outgrows a pure function.)

### hooks/

All React-bound logic: `useState`/`useEffect`/`useCallback`, event
subscriptions, backend-binding calls, polling. A hook owns one slice of concern
and returns a plain object of state and callbacks.

- `useStreamSettings` — the settings object and everything derived from it
- `usePublish` — publish lifecycle + insight figures
- `useLive` — relay polling + watch state
- `useUplinkMeasure` — the speed-test action
- `useMonitors` — one-shot monitor count

Backend bindings (`wailsjs/…` App methods) and runtime events are imported
**only** by hooks and types — never by a component. The one exception is
stateless runtime helpers (e.g. "open a URL"): those are confined to a thin util
wrapper (`util/openExternal.ts`) that components import instead of the binding.

### components/

Pure presentation. A component takes props and renders. It may hold trivial,
self-contained UI state (a "did this text overflow" flag, an open/closed toggle),
but never application state, data fetching, or business rules.

## Component conventions

- **Folder per component**, PascalCase: `components/OptionRow/OptionRow.tsx`.
- **Default export**, function named after the file.
- **Props interface** named `<Component>Props`, declared above the component.
- **JSDoc** one-liner on every exported component saying what it is.
- Reusable primitives over duplication. Fields are built from shared parts:
  `FieldShell` (label + tooltip) is wrapped by `SelectField`, `NumberField`,
  `TextField`, `UplinkField`. A new field type composes these — it does not
  re-implement the label/tooltip markup.

```tsx
interface NumberFieldProps {
    label: string;
    labelTip: string;
    value: number;
    disabledReason?: string;
    onChange: (value: number) => void;
}

/** Integer input wrapped in the shared field shell. */
export default function NumberField({ ... }: NumberFieldProps) { ... }
```

## Hook conventions

- Named `useX`, camelCase, one file each.
- JSDoc says what state the hook owns and any non-obvious behavior.
- Returns a named object (`{ publishing, error, toggle }`), not a positional
  tuple, once it exposes more than one value.
- Cross-hook data is passed as an argument (`usePublish(settings.s)`), keeping a
  single source of truth in the composition root rather than duplicating state.

## DRY

- Extract the moment a second call site appears — shared markup into a component,
  shared logic into a util function.
- Constants and metadata are declared once (`options.ts`, `presets.ts`) and
  imported; never inline the same option list twice.
- Magic numbers become named constants (`ROLLING_WINDOW_MS`, `POLL_INTERVAL_MS`).

## Types

- `strict` TypeScript. No `any` in app code; when a backend/DOM type is missing,
  narrow explicitly at the boundary.
- Prefer `interface` for object shapes, `type` for aliases and unions.
- Import domain types from `types/`, not from generated binding paths.

## Formatting

Enforced by `.prettierrc` (matches the wider PeerDrop convention):

- 4-space indent, no tabs
- double quotes, semicolons
- `arrowParens: "avoid"` → `v => v.x`, not `(v) => v.x`
- `printWidth: 80`, `trailingComma: "es5"`, LF line endings

## Import order

1. third-party (`react`, icon packs, UI primitives under `@/`)
2. backend bindings (`../../wailsjs/…`) — hooks/types only (plus the `util/openExternal` runtime wrapper)
3. local by layer: `types` → `util` → `hooks` → sibling components

## The smell test

Before adding code to a file, ask which layer it belongs to:

- fetching, polling, subscribing, or `useState` → a **hook**
- a pure calculation or a constant → **util**
- JSX that renders props → a **component**
- deciding which components exist and passing them data → **App**

If a component starts fetching, or a util starts importing React, the
responsibility is in the wrong file — move it.
