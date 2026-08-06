# Desktop client

The graphical front for the screen-sharing project: a [Wails](https://wails.io) app that wraps the publish and watch flows (settings UI, live bandwidth meter, stream discovery) around the same MediaMTX relay and ffmpeg pipeline the PowerShell scripts in the repo root drive.
The relay and transport background is documented in the repository root README.

## Layout

The Go module `bjoernblessin.de/screenshare` is the Wails backend.

Package `main` at the module root holds only what `go:embed` pins there, since an embed reads no path above its own directory: the frontend bundle in `main.go` and the tray icon in `tray_icon_windows.go` / `tray_icon_other.go`.
It calls `app.New` with those icon bytes and binds the result.

Package `internal/app` is that backend. It binds one `App` struct to the frontend, its methods grouped by domain across `settings.go`, `system.go`, `publish.go` and `watch.go`, with the struct and process lifecycle in `app.go`.
`startup` and `shutdown` stay unexported and reach `wails.Run` through `app.Hooks`, because Wails binds every exported method on the struct it is given and neither belongs in the frontend's API.

Domain logic lives in leaf packages under `internal/`, one concern each, imported as `bjoernblessin.de/screenshare/internal/<package>`:

| Package | Responsibility |
|---------|----------------|
| `ffmpeg` | Build publish/watch argument lists, spawn and supervise ffmpeg/ffplay children, parse their stats. |
| `relay` | Query the MediaMTX API for the live-stream snapshot. |
| `settings` | Load and persist the stream configuration. |
| `display` | Enumerate monitors (platform-specific files). |
| `platform` | Detect OS and display server. |
| `netspeed` | Measure real uplink throughput. |
| `transport` | Registry of stream protocols, serializing both the publish leg and the watch leg. |

`frontend/` holds the React + Vite UI. `frontend/wailsjs/` is generated bindings; do not edit it by hand.

## Develop and build

Both tasks run from the repository root via the Taskfile:

```bash
task dev     # wails dev with live reload
task build   # binary into desktop/build/bin
```

The `webkit2_41` build tag targets webkitgtk 4.1 (libsoup3), matching the Nix dev shell.
Project settings live in `wails.json`.
