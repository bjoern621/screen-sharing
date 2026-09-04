# tools

Things to point at a running relay or a running backend.

Nothing here is built or shipped,
and no task in `Taskfile.yml` calls any of it but `task dev:stop`.

## What is here

| Tool | Does |
| --- | --- |
| `bruno/` | the group service, membership and relay HTTP APIs as a [Bruno](https://usebruno.com) collection, with its own README |
| `publish.ps1` | capture a screen and publish it over SRT with ffmpeg alone, no app in the way |
| `watch.ps1` | open a stream in `ffplay`, low-latency flags set |
| `whoislive.ps1` | who is publishing on a relay, their tracks and their readers, over the MediaMTX API |
| `diagnose.ps1` | run while a publish and a watch are up and the picture is broken |
| `dev-stop.ps1` | release the control pipe a previous `task dev` left held on Windows |

The scripts want `pwsh` and ffmpeg on `PATH`,
and the capture backends `publish.ps1` offers are Windows'.

## Splitting a broken picture

`diagnose.ps1` opens a second, headless reader on the same stream and counts decode errors over ten seconds.
Errors there put the fault on the publisher or the relay,
and a clean read puts it on the player window or its environment.
Relay statistics are dumped beside that verdict.

The app measures itself while it runs, which is the first place to look:
`docs/delay-measurement.md`.
