# Windows redistributable binaries

The Windows build ships ffmpeg and ffplay next to the app executable so a fresh
machine needs nothing on PATH.
`ffmpeg.FindExe` prefers a copy sitting beside the app binary, and the
`windows/amd64` entry in `postBuildHooks` (see `wails.json`) copies these files
into `build/bin` after each Windows build.

Place the two executables here before building for Windows:

- `ffmpeg.exe`
- `ffplay.exe`

Use a static build (for example the `gpl` variant from
https://www.gyan.dev/ffmpeg/builds/ or https://github.com/BtbN/FFmpeg-Builds)
so the copied `.exe` files carry no external DLL dependencies.

The binaries are gitignored: they are large and licensed separately from this
repository, so they are not committed. The build hook fails loudly if they are
missing, which is the intended signal that a bundling input is absent.
