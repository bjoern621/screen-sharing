# MirrorMe

[![Release](https://img.shields.io/github/v/release/bjoern621/screen-sharing)](https://github.com/bjoern621/screen-sharing/releases)
[![License](https://img.shields.io/github/license/bjoern621/screen-sharing)](LICENSE)

Screen sharing for a group, at the quality the machine can produce.
Everyone shares and everyone watches, at once.
Free and open source, with no accounts.

Built for the screens a video call blurs: code, a terminal, a spreadsheet, a game at 60 fps.
A call spends two to five megabits a second on a screen and softens everything on it.
MirrorMe spends forty and leaves the text readable, the thin lines whole and the dark scenes dark.

![Four members of a group, each stream a tile in the grid](docs/images/viewer-grid.png)

## What it does

- Everyone in a group shares and watches at the same time, each stream a tile in one window.
- Text stays sharp at its own size, so a terminal or a code editor reads the way it reads on the desk it came from.
- Resolution, frame rate and quality are settings, from a still page of text to a game at 60 fps.
- Three presets name a picture, and the app works out how to reach it here: Lossless, Gaming, Text and detail.
- What this machine cannot do is greyed out with the reason beside it, so nothing fails halfway through.
- The graphics card does the encoding wherever it can, leaving the processor to the work being shared.
- Desktop sound travels with the picture.
- The delay from screen to viewer is measured and shown while sharing.
- A stream is watched in the app, in a browser, or in a media player.
- A group is a key. Passing the key on is how someone joins, and dropping it is how they leave.

Windows and Linux.

## Install

Downloads are on the [releases page](https://github.com/bjoern621/screen-sharing/releases), one file per platform.
[`docs/install.md`](docs/install.md) covers what each one carries and what a first run asks for.

| Platform | Get it |
| --- | --- |
| Windows | `mirrorme-<version>-windows-x86_64-setup.exe`, or the `.zip` to run from anywhere |
| Arch Linux | `sudo pacman -U mirrorme-<version>-1-x86_64.pkg.tar.zst` |
| Fedora | `sudo dnf install ./mirrorme-*.rpm` |
| Flatpak | `flatpak install --user ./mirrorme-<version>-x86_64.flatpak` |
| Nix | `nix run github:bjoern621/screen-sharing` |
| Other Linux | `mirrorme-<version>-linux-x86_64-portable.tar.gz` |

One window, and no second program to install beside it.

## Share a screen

1. Open MirrorMe on the Setup tab.
2. Create a group, or paste in the key a member sent.
3. Pick a screen and a preset, or walk the steps and set the picture by hand.
4. Share.

![Picking a screen to share, with the cost of the picture on the right](docs/images/setup-capture.png)

Every stream in the group arrives on the Viewer tab, one tile each.
A stream lives in a group, so sharing waits until this computer has joined one under a name.

## Who sees it

Streams travel through a relay, one server the group's members publish to and read from.
Each member uploads one copy however many people watch, and no home connection has to be reachable from outside.
The relay can read what it carries.
A group that wants that ruled out runs one of its own: [`docs/install.md`](docs/install.md), "The relay".
The app comes pointed at the project's relay.

A group's key is what grants access to its streams, and anybody holding it can watch and share.

[`docs/`](docs/README.md) answers the rest: what travels where, what a group is, and how the picture is made.

## Developing

Go backend, Avalonia shell, a gRPC contract between them in [`api/`](api/).
`task` lists the development and packaging tasks, and `task all` runs relay, backend and shell together.
[`docs/development-principles.md`](docs/development-principles.md) governs every change, and [`docs/readme-audience.md`](docs/readme-audience.md) governs this page.
`api/`, `backend/`, `avalonia/`, `packaging/`, `deploy/` and `tools/` each carry a README for their own layout.

## License

Apache-2.0 ([`LICENSE`](LICENSE)).
The Windows downloads also carry ffmpeg and the GStreamer runtime, under their own GPL and LGPL terms.
[`THIRD-PARTY-NOTICES.md`](THIRD-PARTY-NOTICES.md) states what every artifact ships and where its source is.
