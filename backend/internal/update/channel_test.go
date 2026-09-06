package update

import (
	"os"
	"path/filepath"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// The table decides what a reader is offered before anything reaches the network,
// so every channel the recipes stamp is asserted here rather than on a machine that has one.
func TestWhatEachChannelMayDo(t *testing.T) {
	cases := []struct {
		channel       Channel
		version       string
		check         string
		unchecked     screensharev1.TextCode
		uninstallable screensharev1.TextCode
		method        Method
	}{
		{channel: WindowsSetup, version: "0.5.0", method: MethodRun},
		{channel: WindowsZip, version: "0.5.0", method: MethodSwap},
		{channel: Portable, version: "0.5.0", method: MethodSwap},

		// A package manager's copy: the release is worth naming and the files are not the app's.
		{
			channel:       Nix,
			version:       "0.5.0",
			uninstallable: screensharev1.TextCode_TEXT_CODE_UPDATE_CHANNEL_MANAGED,
		},
		{
			channel:       Pacman,
			version:       "0.5.0",
			uninstallable: screensharev1.TextCode_TEXT_CODE_UPDATE_CHANNEL_MANAGED,
		},

		// The environment switches the whole thing off, whatever the channel could do.
		{
			channel:   Portable,
			version:   "0.5.0",
			check:     "0",
			unchecked: screensharev1.TextCode_TEXT_CODE_UPDATE_CHECK_OFF,
			method:    MethodSwap,
		},
		// Any other value leaves it on, absence included.
		{channel: Portable, version: "0.5.0", check: "1", method: MethodSwap},

		// A build nobody stamped has nothing to compare a tag against.
		{
			channel:       Unstamped,
			version:       "dev",
			unchecked:     screensharev1.TextCode_TEXT_CODE_UPDATE_BUILD_UNSTAMPED,
			uninstallable: screensharev1.TextCode_TEXT_CODE_UPDATE_BUILD_UNSTAMPED,
		},
		// A stamped channel carrying an unstamped version is a recipe that dropped the flag.
		{
			channel:   Portable,
			version:   "dev",
			unchecked: screensharev1.TextCode_TEXT_CODE_UPDATE_BUILD_UNSTAMPED,
			method:    MethodSwap,
		},
	}

	for _, tc := range cases {
		offer := Decide(tc.channel, tc.version, tc.check)

		if got := offer.Unchecked.GetCode(); got != tc.unchecked {
			t.Errorf("%s asks under %v, want %v", tc.channel, got, tc.unchecked)
		}
		if got := offer.Uninstallable.GetCode(); got != tc.uninstallable {
			t.Errorf("%s installs under %v, want %v", tc.channel, got, tc.uninstallable)
		}
		if offer.Method != tc.method {
			t.Errorf("%s installs by %v, want %v", tc.channel, offer.Method, tc.method)
		}
		if (offer.Asset == "") != (tc.method == MethodNone) {
			t.Errorf("%s names asset %q under method %v", tc.channel, offer.Asset, offer.Method)
		}
	}
}

// The Windows installer and the Windows archive carry the same binaries,
// so the uninstaller beside them is what separates a copy the app may replace
// from one somebody extracted where they wanted it.
func TestWhichWindowsCopyThisIs(t *testing.T) {
	extracted := t.TempDir()
	if got := Resolve(Windows, extracted, ""); got != WindowsZip {
		t.Errorf("a directory with no uninstaller reads as %s, want %s", got, WindowsZip)
	}

	installed := t.TempDir()
	if err := os.WriteFile(filepath.Join(installed, UninstallerName), nil, 0o600); err != nil {
		t.Fatalf("writing the uninstaller: %v", err)
	}
	if got := Resolve(Windows, installed, ""); got != WindowsSetup {
		t.Errorf("a directory with an uninstaller reads as %s, want %s", got, WindowsSetup)
	}

	// Every other stamp answers itself, whatever is on disk beside it.
	if got := Resolve(Nix, installed, ""); got != Nix {
		t.Errorf("%s resolved to %s", Nix, got)
	}

	// The Flatpak bundle carries the tarball build's stamp, and its sandbox is what corrects it.
	if got := Resolve(Portable, extracted, "de.bjoernblessin.MirrorMe"); got != Flatpak {
		t.Errorf("a copy inside a sandbox resolved to %s, want %s", got, Flatpak)
	}
}
