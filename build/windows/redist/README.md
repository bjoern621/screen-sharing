# Windows redistributable binaries

Where ffmpeg and ffplay are kept for the Windows build.
Fetched once per machine, not built, which is why they live here rather than in an output directory.

Place both before building for Windows:

- `ffmpeg.exe`
- `ffplay.exe`

`scripts/get-ffmpeg.ps1` fills this directory.
`task build:windows` copies the pair into `build/bin` beside the backend, where `ffmpeg.FindExe` looks first, so a fresh machine needs nothing on PATH.

Take a static build, the `gpl` variant from https://www.gyan.dev/ffmpeg/builds/ or https://github.com/BtbN/FFmpeg-Builds, so the copied executables carry no external DLL dependencies.

Gitignored: large, and licensed separately from this repository.
A missing file fails the copy step loudly, which is the signal that a bundling input is absent.
