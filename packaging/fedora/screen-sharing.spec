# ffmpeg and GStreamer are declared dependencies rather than bundled copies,
# what docs/packaging.md picks for every channel with a package manager behind it,
# and what rpm expects.
#
# This spec builds with network access,
# both toolchains fetching their dependencies during the build:
# `go build` resolves modules and `dotnet publish` restores NuGet packages.
# That rules it out of Fedora's own build system, which builds offline against bundled sources,
# and suits a local `rpmbuild` or a CI job, where the released package comes from.
#
# Two things Fedora's repositories do not carry, and neither is substituted silently:
#   x264enc lives in RPM Fusion's gstreamer1-plugins-ugly, not in the -free package.
#   gst-plugins-rs is not packaged at all, so whipclientsink and whepsrc are absent
#   and the WebRTC legs (WHIP publish, WHEP watch) fail at pipeline start on a stock install.
# Both are Recommends where a package exists and a note in docs/install.md where none does.

%global appname     screen-sharing
%global appdir      %{_libdir}/%{appname}

# No debuginfo package.
# Half of what this installs is a .NET publish output carrying no DWARF to extract,
# and rpm's extraction over the rest finds no sources to go with it.
# The Arch PKGBUILD disables its split package for the same reason.
%global debug_package %{nil}

# Fedora's default takes the build timestamp from the newest changelog entry,
# and this spec has no changelog to take one from (the note at the end of the file states why).
%global source_date_epoch_from_changelog 0

Name:           screen-sharing
Version:        0.6.0
Release:        1%{?dist}
Summary:        Self-hosted, high-quality group screen sharing

# The project's own code.
# What this package installs beyond it is compiled-in Go and .NET libraries
# under MIT, BSD-3-Clause and Apache-2.0;
# ffmpeg and GStreamer are Requires rather than bundled copies,
# so their terms are theirs to declare.
License:        Apache-2.0
URL:            https://github.com/bjoern621/screen-sharing
Source0:        %{url}/archive/refs/tags/v%{version}.tar.gz#/%{name}-%{version}.tar.gz

ExclusiveArch:  x86_64

BuildRequires:  gcc
BuildRequires:  golang >= 1.25
BuildRequires:  dotnet-sdk-10.0
BuildRequires:  pkgconfig(gstreamer-1.0)
BuildRequires:  pkgconfig(gstreamer-gl-1.0)
BuildRequires:  pkgconfig(gstreamer-video-1.0)
BuildRequires:  pkgconfig(gstreamer-pbutils-1.0)
BuildRequires:  pkgconfig(egl)
BuildRequires:  desktop-file-utils

# Capture, encode, publish and the single-stream viewer.
# The paths rather than a package name,
# both ffmpeg-free and RPM Fusion's ffmpeg providing them and either serving.
Requires:       /usr/bin/ffmpeg
Requires:       /usr/bin/ffplay
# The shell is a framework-dependent build, so the runtime is the machine's.
Requires:       dotnet-runtime-10.0
# The GStreamer publish engine spawns gst-launch-1.0,
# and the tile grid decodes through the libraries the backend links.
# Both read the same plugin set.
Requires:       gstreamer1
Requires:       gstreamer1-plugins-base
Requires:       gstreamer1-plugins-good
Requires:       gstreamer1-plugins-bad-free
Requires:       gstreamer1-rtsp-server
Requires:       gstreamer1-plugin-libav
Requires:       pipewire-gstreamer
Requires:       libnice
# libEGL and libGL, which the shell's render path resolves by soname rather than links,
# and which the frame channel imports decoded frames through.
# rpm's dependency generator reads what a binary links and cannot see either,
# so both are named here.
Requires:       libglvnd-egl
Requires:       libglvnd-glx
# The monitor enumerators the backend spawns, one per session type
# (backend/internal/display, listX11 and listWlrRandr).
# Without one the enumeration answers a single placeholder carrying no geometry,
# so the setup screen offers one nameless screen and every monitor past the first is refused.
# By file name, as ffmpeg is above: what matters is the program being on PATH.
Requires:       /usr/bin/xrandr

# RPM Fusion, for the encoders Fedora's own packages leave out.
Recommends:     gstreamer1-plugins-ugly
Recommends:     gstreamer1-plugins-bad-freeworld
# Recommended rather than required, being the wlroots half of the pair above,
# which a session running X11 or GNOME never calls.
# A hard requirement would also make the package uninstallable wherever it is not carried.
Recommends:     /usr/bin/wlr-randr

%description
Group screen sharing for a self-hosted MediaMTX relay, with no accounts and no remote
control. Everyone publishes and watches at once, in full color at 4:4:4 and full range.

The package installs two programs: a headless backend that owns capture, encode,
publish and decode, and the window in front of it. Opening the app starts both.

%prep
%autosetup

%build
# backend/internal/receive links GStreamer through cgo.
# Go turns cgo off by itself when it finds no C compiler,
# which surfaces as "build constraints exclude all Go files"
# in packages that look unrelated to the compiler;
# asking for it outright names the missing compiler instead.
export CGO_ENABLED=1
export GOFLAGS="-buildmode=pie -trimpath -mod=readonly -modcacherw"
export DOTNET_CLI_TELEMETRY_OPTOUT=1 DOTNET_NOLOGO=1

go build -C backend -ldflags "-X main.version=%{version}" \
  -o ../dist/screenshare-backend ./cmd/backend

# Framework-dependent: the runtime is a dependency of this package,
# so the build carries no second copy of it.
# Both binaries land in one directory, the shell starting the backend from beside itself.
dotnet publish avalonia/ScreenShare.App/ScreenShare.App.csproj \
  --configuration Release \
  --runtime linux-x64 \
  --self-contained false \
  --output dist

%install
install -dm 755 %{buildroot}%{appdir}
cp -r dist/. %{buildroot}%{appdir}/
chmod 755 %{buildroot}%{appdir}/screenshare-backend \
          %{buildroot}%{appdir}/screenshare-avalonia

# A launcher rather than a symlink into %{_libdir}:
# the shell resolves the backend against its own directory,
# and a symlinked entry point would answer %{_bindir}.
install -dm 755 %{buildroot}%{_bindir}
cat > %{buildroot}%{_bindir}/screenshare-avalonia <<EOF
#!/bin/sh
exec %{appdir}/screenshare-avalonia "\$@"
EOF
chmod 755 %{buildroot}%{_bindir}/screenshare-avalonia

install -Dm 644 packaging/linux/screen-sharing.desktop \
  %{buildroot}%{_datadir}/applications/%{appname}.desktop
# hicolor's index declares 48 through 512,
# and a size it omits is a directory no lookup walks.
# `task icons` draws these from build/appicon.png and they are committed,
# so no channel needs ImageMagick at build time.
for size in 48 64 128 256 512; do
  install -Dm 644 build/icons/${size}.png \
    %{buildroot}%{_datadir}/icons/hicolor/${size}x${size}/apps/%{appname}.png
done

%check
desktop-file-validate %{buildroot}%{_datadir}/applications/%{appname}.desktop

%files
%license LICENSE
%doc README.md docs/install.md THIRD-PARTY-NOTICES.md
%{_bindir}/screenshare-avalonia
%{appdir}/
%{_datadir}/applications/%{appname}.desktop
%{_datadir}/icons/hicolor/*/apps/%{appname}.png

# No %%changelog section.
# The one rpm would carry is the git history,
# and rpmautospec, which generates it from that history,
# is part of Fedora's build system rather than something a local rpmbuild has.
