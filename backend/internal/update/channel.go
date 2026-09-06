package update

import (
	"os"
	"path/filepath"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/release"
	"bjoernblessin.de/screenshare/internal/text"
)

// EnvCheck switches the release check off for a whole install.
// "0" is off and every other value, absence included, leaves it on.
// The one way a packager states that its own tooling delivers updates here.
const EnvCheck = "MIRRORME_UPDATE_CHECK"

// Channel names where a copy of this app came from.
//
// Stamped at link time by the recipe that built it, -X main.channel=...,
// and empty on every build outside the release pipeline
// (docs/packaging.md, "The build stamp").
//
// What it decides is whether the app replaces its own files.
// A copy a package manager put on disk is that manager's to move,
// and an app writing into it leaves the two disagreeing about what is installed.
type Channel string

const (
	// Unstamped is a build nobody released.
	Unstamped Channel = ""
	// Windows is the Windows build, shipped both as an installer and as a plain archive.
	// Never an answer on its own: Resolve settles which of the two a copy is.
	Windows Channel = "windows"
	// WindowsSetup is a copy the installer put down, WindowsZip one somebody extracted.
	WindowsSetup Channel = "windows-setup"
	WindowsZip   Channel = "windows-zip"
	// Portable is the Linux tarball, which belongs to whoever extracted it.
	Portable Channel = "portable"

	Pacman  Channel = "pacman"
	Dnf     Channel = "dnf"
	Flatpak Channel = "flatpak"
	Nix     Channel = "nix"
)

// Method is how a staged release is put in place.
type Method int

const (
	// MethodNone is a channel this app installs nothing into.
	MethodNone Method = iota
	// MethodRun executes what was downloaded, an installer replacing the tree itself.
	MethodRun
	// MethodSwap replaces the directory holding the app with the archive's own.
	MethodSwap
)

// row is one channel's whole answer.
//
// A channel either names an asset and a method, or names why it installs nothing.
// Both together is a row that contradicts itself, which the table's own check catches.
type row struct {
	// asset is the release file this channel installs,
	// with %s taking the version as the tag spells it without its leading "v".
	asset  string
	method Method
	// held is the statement behind a channel that installs nothing.
	held screensharev1.TextCode
}

// channels is every channel a build can be stamped with.
//
// Exhaustive, so a recipe stamping a name nothing here declares fails at the first check
// rather than quietly reading as a copy somebody else maintains.
var channels = map[Channel]row{
	Unstamped:    {held: screensharev1.TextCode_TEXT_CODE_UPDATE_BUILD_UNSTAMPED},
	WindowsSetup: {asset: "mirrorme-%s-windows-x86_64-setup.exe", method: MethodRun},
	WindowsZip:   {asset: "mirrorme-%s-windows-x86_64.zip", method: MethodSwap},
	Portable:     {asset: "mirrorme-%s-linux-x86_64-portable.tar.gz", method: MethodSwap},

	Pacman:  {held: screensharev1.TextCode_TEXT_CODE_UPDATE_CHANNEL_MANAGED},
	Dnf:     {held: screensharev1.TextCode_TEXT_CODE_UPDATE_CHANNEL_MANAGED},
	Flatpak: {held: screensharev1.TextCode_TEXT_CODE_UPDATE_CHANNEL_MANAGED},
	Nix:     {held: screensharev1.TextCode_TEXT_CODE_UPDATE_CHANNEL_MANAGED},
}

// UninstallerName is the file Inno Setup writes beside an installed copy,
// and the only mark separating that copy from an archive of the same files
// (packaging/windows/mirrorme.iss).
const UninstallerName = "unins000.exe"

// EnvFlatpak is set inside a Flatpak sandbox and nowhere else.
const EnvFlatpak = "FLATPAK_ID"

// Resolve settles which channel a copy is, from the stamp and two facts about where it is running.
//
// Two stamps do not answer themselves.
// The Flatpak bundle is assembled out of the portable tarball, so it carries that build's stamp,
// and its sandbox is read-only; flatpakID being set is what separates the two.
// The Windows installer and the Windows archive carry identical binaries,
// so which of the two is on disk is a question about the disk.
//
// dir holds the running app and flatpakID is the value of EnvFlatpak.
// Both are parameters, so the rule settles with no environment and no filesystem in a test.
func Resolve(stamped Channel, dir, flatpakID string) Channel {
	if flatpakID != "" {
		return Flatpak
	}
	if stamped != Windows {
		return stamped
	}

	if _, err := os.Stat(filepath.Join(dir, UninstallerName)); err == nil {
		return WindowsSetup
	}
	return WindowsZip
}

// Offer is what one install may do about the release published beside it.
//
// Two statements for two questions, each nil where the install may go ahead.
// Unchecked stops the release service being asked at all.
// Uninstallable leaves the answer worth stating and the files alone,
// so a reader is told a newer build exists and where to get it.
type Offer struct {
	Unchecked     *screensharev1.Text
	Uninstallable *screensharev1.Text
	// Asset is the release file to fetch, with %s still taking the version.
	// Empty exactly where Uninstallable stands.
	Asset  string
	Method Method
	// Channel is what the stamp resolved to, which a statement about a missing download names.
	Channel Channel
}

// Decide reads the table for one install.
//
// version is the running build's stamp and check the value of EnvCheck,
// both taken as parameters so the rule is settled without an environment or a linker.
func Decide(channel Channel, version, check string) Offer {
	r, declared := channels[channel]
	assert.Assert(declared, "a build is stamped with a channel the table declares", string(channel))
	assert.Assert((r.asset == "") == (r.method == MethodNone),
		"a channel names an asset exactly where it installs one", string(channel))
	assert.Assert((r.held == screensharev1.TextCode_TEXT_CODE_UNSPECIFIED) == (r.method != MethodNone),
		"a channel that installs nothing says why", string(channel))

	var offer Offer
	offer.Channel = channel
	switch {
	case check == "0":
		offer.Unchecked = text.Of(screensharev1.TextCode_TEXT_CODE_UPDATE_CHECK_OFF)
	case !release.Comparable(version):
		// Nothing to compare a tag against, so the answer would be unknown whatever came back.
		offer.Unchecked = text.Of(screensharev1.TextCode_TEXT_CODE_UPDATE_BUILD_UNSTAMPED)
	}

	if r.method == MethodNone {
		offer.Uninstallable = text.Of(r.held,
			text.ID(screensharev1.TextArgName_TEXT_ARG_NAME_CHANNEL, string(channel)))
		return offer
	}

	offer.Asset, offer.Method = r.asset, r.method
	return offer
}
