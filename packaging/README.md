# packaging

Every recipe that turns this checkout into something a person installs.

One directory per channel, each holding what that channel's tooling reads.
`docs/packaging.md` states what the app needs at run time and how each channel provides it, and is the page to read before changing any recipe here.

## Layout

| Path | Holds |
| --- | --- |
| `arch/` | `PKGBUILD`, the AUR recipe |
| `fedora/` | `mirrorme.spec`, built with `rpmbuild` |
| `flatpak/` | the Flatpak manifest, which assembles what `linux/package.sh` staged |
| `linux/` | the desktop entry, and `package.sh` for the portable tarball |
| `windows/` | the Inno Setup script, and the scripts that stage, bundle and compile |
| `nix/` | the derivations `flake.nix` exposes: the app, the group service, three container images, the NixOS module |
| `icons/` | `appicon.png` and the hicolor sizes drawn from it |

## Building

Every task writes into `build/dist`.

```bash
task package:linux              # tarball for a distribution with no package here
task package:flatpak            # bundle, over what package:linux staged
task package:windows            # zip
task package:windows:installer  # setup.exe, over the directory the zip staged
```

Nix builds from the flake and reads nothing under `build/`:

```bash
nix build .#mirrorme       # the app
nix build .#groupd         # the service beside the relay
nix build .#relay-image    # and .#proxy-image, .#groupd-image
```

Arch and Fedora build from a `git archive` of a tag, which is what the release workflow hands them (`.github/workflows/release.yml`).

## Version

No file in the tree carries the number.
A build reads `VERSION` from the environment, and a run without it stamps `dev`, the mark of a binary nobody released.
The tag is the one place the number lives, and `.github/workflows/version.yml` reads it off there.

## Icons

`icons/appicon.png` is the master.
`task icons` redraws the hicolor sizes beside it, and the multi-size `.ico` inside the shell's project, where the Nix build can reach it.
All of them are committed, so no recipe carries an ImageMagick build dependency.
