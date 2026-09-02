// Package gpu reports the video driver an encode runs through: which implementation,
// which adapter it drives, and which release it is.
//
// Answers the one question a driver-scoped capability gap asks:
// whether this machine runs a driver whose defect the codec table records
// (capabilities.DriverDefect).
// A defect that takes the graphics device down cannot be probed for, the probe being the crash,
// so the driver is identified and matched instead.
package gpu

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/capabilities"
)

// vainfoExe prints what libva's vaQueryVendorString answered,
// which is the one string every VA driver fills in.
const vainfoExe = "vainfo"

// readTimeout bounds the read: vainfo opens the display and initializes the driver,
// and a driver that never returns would otherwise hold the first form resolve.
const readTimeout = 10 * time.Second

// Info is the driver an encode runs through, as a gap matches on it.
//
// A field this machine does not answer stays empty, and an empty field matches no gap.
// A driver nothing identified has no recorded defects, so nothing is withheld.
type Info struct {
	// Driver names the implementation as the driver names itself: "radeonsi".
	Driver string `json:"driver"`
	// Model is the adapter the driver drives.
	// Example: "AMD Radeon 780M Graphics".
	Model string `json:"model"`
	// Version is the driver's release as one comparable figure: 26.1.6 reads 26001006.
	Version int `json:"version"`
	// Vendor is the whole string the driver introduced itself with, for a log line or a bug report.
	Vendor string `json:"vendor"`
}

// Version packs a release into the figure Info.Version carries and DriverDefect compares against.
// Three fields into one integer, so a gap states its fix as a number rather than as three.
func Version(major, minor, patch int) int {
	assert.Assert(major >= 0 && minor >= 0 && patch >= 0,
		"a driver release counts up from zero in every field", major, minor, patch)
	assert.Assert(minor < 1000 && patch < 1000,
		"a packed release keeps three digits per field", minor, patch)

	return major*1_000_000 + minor*1_000 + patch
}

var (
	once   sync.Once
	cached Info
)

// Detect reads the VA driver this machine encodes through, once per process.
// Held after the first read: the driver behind a running process does not change,
// and a form resolved on every keystroke cannot start a subprocess per pass.
func Detect() Info {
	once.Do(func() { cached = read() })
	return cached
}

// Device is what the codec table matches its DriverDefects rows against.
// The conversion sits here rather than at each caller,
// so the capability package depends on the rule vocabulary and on nothing else in the domain.
func Device() capabilities.Device {
	i := Detect()
	return capabilities.Device{Driver: i.Driver, Model: i.Model, Version: i.Version}
}

// read runs vainfo and parses what the driver said about itself.
// A machine with no VA driver and one with no vainfo answer alike, both Umgebungsfehler:
// the app publishes through a software encoder either way,
// and only the driver-scoped gaps go unread.
func read() Info {
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, vainfoExe, "--display", "drm").Output()
	if err != nil {
		// vainfo takes the drm display only where it was built with it,
		// and answers on the session's display otherwise.
		out, err = exec.CommandContext(ctx, vainfoExe).Output()
	}
	if err != nil {
		logger.Warnf("no VA driver identified on this machine, so driver-scoped codec gaps go unread: %v", err)
		return Info{}
	}
	return parse(string(out))
}

// vendorPrefix is what vainfo puts ahead of the vendor string on the line that carries it.
const vendorPrefix = "Driver version:"

// parse reads the vendor string out of vainfo's report.
// Split out from read so a test without a card can hold the formats the drivers write.
func parse(out string) Info {
	for line := range strings.SplitSeq(out, "\n") {
		_, vendor, found := strings.Cut(line, vendorPrefix)
		if !found {
			continue
		}
		return parseVendor(strings.TrimSpace(vendor))
	}
	return Info{}
}

// parseVendor reads one vendor string, e.g.
// "Mesa Gallium driver 26.1.6 for AMD Radeon 780M Graphics (radeonsi, phoenix, ACO, DRM 3.64, 7.1.5)".
//
// Three readings, each independent,
// so a string yielding one field and not another carries the one it yielded:
//   - Driver: first item of the trailing parenthesized list.
//   - Model: what stands between " for " and that list.
//   - Version: first dotted figure.
//
// A field the string does not carry that way stays empty rather than being guessed at,
// leaving the gaps naming it unmatched.
func parseVendor(vendor string) Info {
	info := Info{Vendor: vendor}
	rest := vendor

	if open := strings.LastIndex(vendor, "("); open >= 0 && strings.HasSuffix(vendor, ")") {
		list := vendor[open+1 : len(vendor)-1]
		driver, _, _ := strings.Cut(list, ",")
		info.Driver = strings.TrimSpace(driver)
		rest = strings.TrimSpace(vendor[:open])
	}
	if _, model, found := strings.Cut(rest, " for "); found {
		info.Model = strings.TrimSpace(model)
	}
	info.Version = parseRelease(rest)

	return info
}

// parseRelease is the first dotted figure in s, e.g. 26001006 out of "Mesa Gallium driver 26.1.6".
// Zero where no field reads as a number, which is a release no defect's fix bound compares against.
func parseRelease(s string) int {
	for field := range strings.FieldsSeq(s) {
		parts := strings.Split(field, ".")
		if len(parts) < 2 || len(parts) > 3 {
			continue
		}
		nums := make([]int, 0, 3)
		for _, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil || n < 0 || n >= 1000 {
				break
			}
			nums = append(nums, n)
		}
		if len(nums) != len(parts) {
			continue
		}
		nums = append(nums, 0)
		return Version(nums[0], nums[1], nums[2])
	}
	return 0
}
