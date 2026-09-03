# Build directory

Where a build lands, plus the Windows pieces a build needs that are not source.

`Taskfile.yml` drives the build: `go build` for the backend, `dotnet publish` for the shell.
`docs/packaging.md` covers what the app needs at run time and how each channel provides it.

## Layout

| Path | Holds |
| --- | --- |
| `bin/` | build output, rebuilt from scratch |
| `dist/` | release archives and the Windows installer, written by the packaging scripts |
| `windows/redist/` | ffmpeg and ffplay, fetched once per machine and kept |
| `appicon.png` | the source image the platform icons derive from |
| `icons/` | what `task icons` draws from it: the hicolor sizes, and `mirrorme.ico` for the Windows installer |

`bin/` and `dist/` are outputs and carry nothing worth keeping.
`windows/redist/` is fetched rather than built.
`scripts/get-ffmpeg.ps1` fills it.
`task build:windows` copies the pair into `bin/` beside the backend, where the app's own lookup finds them (`backend/internal/ffmpeg`, `FindExe`).

## Windows packaging

`scripts/bundle-windows.sh` puts the GStreamer runtime and its command-line tools into `bin/` first.
`scripts/package-windows.ps1` then produces the release zip: checks every program the app spawns is present, publishes the shell into the same directory as the backend, archives the result.
`scripts/installer-windows.ps1` compiles the installer over the directory that staged, so both downloads carry one set of files.

One directory holds all of it: what the Windows loader searches first for a DLL is also where the app's own lookups start.
