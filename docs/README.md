# docs

Where each question about this project is answered.

These pages are the source of truth for architecture, the domain model, terminology and style.
Read the page before guessing, and put a new fact on the page that owns it.

## Running it

| Page | Answers |
| --- | --- |
| [install.md](install.md) | which download to take, what it carries, and how to run a relay |
| [packaging.md](packaging.md) | what the app needs at run time, and how each channel provides it |

## How it works

| Page | Answers |
| --- | --- |
| [network-architecture.md](network-architecture.md) | what travels between which machines, over which leg, encrypted with what |
| [membership.md](membership.md) | who is in a group, and what the relay does about anybody else |
| [auth-flow.md](auth-flow.md) | what a group key is, what a token grants, and what a leak costs |
| [discord-mode.md](discord-mode.md) | how a voice channel becomes a group, and who is cut when they leave it |
| [capture-architecture.md](capture-architecture.md) | how a screen becomes a stream on the relay |
| [viewer-architecture.md](viewer-architecture.md) | three ways to watch one stream, each with its own decoder |
| [decode-timing.md](decode-timing.md) | how a live decode stays on the clock |
| [ipc-api.md](ipc-api.md) | the contract between the backend and any shell in front of it |
| [video-stack.md](video-stack.md) | every layer between a compositor's buffer and a monitor, and what each one costs |

## What the app decides

| Page | Answers |
| --- | --- |
| [domain-model.md](domain-model.md) | the tables of facts every rule derives from |
| [presets.md](presets.md) | what a preset promises, and how the app reaches it on this machine |
| [field-availability.md](field-availability.md) | when a settings field is hidden and when it is greyed with a reason |
| [settings-editing.md](settings-editing.md) | who owns a draft, and when an edit reaches the backend |
| [delay-measurement.md](delay-measurement.md) | which stage of the delay is measured, and which is named without a figure |

## Writing and building

| Page | Answers |
| --- | --- |
| [development-principles.md](development-principles.md) | how the code may be shaped, which governs every change |
| [design-language.md](design-language.md) | one visual language, and the tokens that are its reference implementation |
| [tooltips.md](tooltips.md) | what a tooltip teaches, on which fields and options |
| [readme-audience.md](readme-audience.md) | who the root `README.md` is written for, and what a sentence in it may assume |
| [glossary.md](glossary.md) | every abbreviation this repository uses |
| [plan.md](plan.md) | work that is not built yet, and the decisions behind it |

## Elsewhere

`api/`, `backend/`, `avalonia/`, `packaging/`, `deploy/` and `tools/` each carry a README for their own layout.
