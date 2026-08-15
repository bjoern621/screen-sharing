# Build directory

Where a build lands, plus the Windows pieces a build needs that are not source.

`Taskfile.yml` drives the build: `go build` for the backend, `dotnet publish` for the shell.
`docs/packaging.md` covers what the app needs at run time and how each channel provides it.

## Layout

| Path | Holds |
| --- | --- |
| `bin/` | build output, rebuilt from scratch |
| `dist/` | release archives, written by `scripts/package-windows.ps1` and `scripts/package-linux.sh` |
| `windows/redist/` | ffmpeg and ffplay, fetched once per machine and kept |
| `appicon.png` | the source image the platform icons derive from |

`bin/` and `dist/` are outputs and carry nothing worth keeping.
`windows/redist/` is fetched rather than built, which is why it lives here instead of being downloaded on every build.
`scripts/get-ffmpeg.ps1` fills it.
`task build:windows` copies the pair into `bin/` beside the backend, where the app's own lookup finds them (`backend/internal/ffmpeg`, `FindExe`).

## Windows packaging

`scripts/bundle-windows.sh` puts the GStreamer runtime and its command-line tools into `bin/` first.
`scripts/package-windows.ps1` then produces the release zip: checks every program the app spawns is present, publishes the shell into the same directory as the backend, archives the result.

One directory holds all of it: what the Windows loader searches first for a DLL is also where the app's own lookups start.

## Leftovers

`darwin/`, `windows/installer/`, `windows/wails.exe.manifest`, `windows/info.json` and `windows/icon.ico` come from an earlier Wails build.
No recipe in the tree reads them.
