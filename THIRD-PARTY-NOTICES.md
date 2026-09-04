# Third-party notices

The project's own source is under Apache-2.0 (`LICENSE`).
Some artifacts carry code the project did not write, and a few of those licenses require
their notice to travel with the binary, which is what this file is.
Every component keeps its own license; nothing here relicenses anything.

## What each artifact carries

| Artifact | Third-party code inside it |
|----------|----------------------------|
| Arch package, Fedora package, Nix package | Compiled-in Go and .NET libraries only. ffmpeg and GStreamer are declared dependencies, so the package manager ships them under their own terms. |
| Linux tarball | The same, plus the .NET runtime, which is published self-contained. |
| Windows zip | The same, plus ffmpeg, ffplay, the GStreamer runtime and its plugins, which are bundled because Windows has no dependency manager to declare them to (`docs/packaging.md`). |

## Compiled in

The backend links Go libraries pinned in `backend/go.mod`, under MIT, BSD-3-Clause and
Apache-2.0.
The GStreamer bindings (`go-gst`) are MIT; the library they bind is not compiled in and
is covered below.

The shell resolves NuGet packages pinned in
`avalonia/ScreenShare.App/ScreenShare.App.csproj`, under MIT, BSD-3-Clause and
Apache-2.0.
Avalonia, Skia, HarfBuzz, the Inter font package and the Tabler icon set are MIT; gRPC is
Apache-2.0; Protobuf is BSD-3-Clause.
The .NET runtime is MIT.

None of these require more than their copyright notice and license text, which their
packages carry and which a build reproduces into the output directory.

## Bundled in the Windows zip

This is the one artifact that redistributes copyleft binaries, so it is the one with
obligations beyond attribution.

| Component | License | Source |
|-----------|---------|--------|
| `ffmpeg.exe`, `ffplay.exe` | GPL, as configured by the build the release workflow fetches | https://github.com/BtbN/FFmpeg-Builds |
| GStreamer core, its plugin sets and glib | LGPL-2.1-or-later | https://gitlab.freedesktop.org/gstreamer/gstreamer |
| glib-networking's gnutls module, the TLS behind every `rtsps://` leg, and GnuTLS itself | LGPL-2.1-or-later | https://gitlab.gnome.org/GNOME/glib-networking and https://www.gnutls.org/ |
| Individual plugins that link a GPL library, among them `x264enc` in `gst-plugins-ugly`, `x265enc` in `gst-plugins-bad` and `gst-libav` | GPL, taken from the library each one links | The same, and https://www.videolan.org/developers/x264.html |
| libsrt | MPL-2.0 | https://github.com/Haivision/srt |
| libnice | LGPL-2.1-or-later or MPL-1.1 | https://gitlab.freedesktop.org/libnice/libnice |

The binaries come from MSYS2 and from the ffmpeg build the workflow downloads, unmodified.
Source for any of them is available from the projects above and from MSYS2's package
sources, and is offered on request for as long as the release is published.

The GPL-licensed programs are spawned as separate processes and the LGPL libraries are
loaded as separate DLLs, so the archive is an aggregate: the app's own code stays under
Apache-2.0 and each bundled component stays under its own license.
Replacing a bundled DLL or executable with another build of the same library is what the
LGPL reserves, and the layout permits it: everything sits in one directory the app
searches by name (`docs/packaging.md`).

The distinction that keeps this true is process and link boundaries, not intent.
Statically linking a GPL encoder into the backend, or building a plugin against its
headers as part of this repository, would make the result one work and would carry the
GPL to it.
