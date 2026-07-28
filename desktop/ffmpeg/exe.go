package ffmpeg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"bjoernblessin.de/go-utils/util/assert"
)

// FindExe locates a media executable (ffmpeg, ffplay, mpv). A copy shipped
// next to the app binary wins over one on PATH, so a bundled build is
// self-contained.
func FindExe(name string) (string, error) {
	assert.Assert(name != "", "an executable lookup names the program to find")

	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	if self, err := os.Executable(); err == nil {
		bundled := filepath.Join(filepath.Dir(self), name)
		if info, statErr := os.Stat(bundled); statErr == nil && !info.IsDir() {
			return bundled, nil
		}
	}

	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found: install ffmpeg or place %s next to the app", name, name)
	}
	return path, nil
}

// EnvKmsgrabFFmpeg names the executable kmsgrab capture runs, overriding the
// backend's default resolution. kmsgrab reads the raw KMS scanout, which the
// kernel gates behind CAP_SYS_ADMIN, so it needs a privileged ffmpeg that the
// other backends must not share. A packaging layer sets this to the capability
// wrapper (nix/screen-share.nix points it at security.wrappers' ffmpeg-kmsgrab).
const EnvKmsgrabFFmpeg = "SCREENSHARE_FFMPEG_KMSGRAB"

// kmsgrabWrapper is the privileged build's conventional name on PATH.
const kmsgrabWrapper = "ffmpeg-kmsgrab"

// FindCaptureExe locates the ffmpeg build to run for a given capture backend.
//
// Only kmsgrab needs a different binary from the rest: its CAP_SYS_ADMIN
// requirement (see EnvKmsgrabFFmpeg) means the plain ffmpeg from FindExe cannot
// open the input. Its resolution order is the EnvKmsgrabFFmpeg override, then a
// wrapper named ffmpeg-kmsgrab on PATH, then the plain ffmpeg as a last resort
// (which fails on the capability, no worse than before). Every other backend
// uses the plain ffmpeg directly, keeping the privileged binary off the
// unprivileged capture backends.
func FindCaptureExe(capture string) (string, error) {
	// The caller has already built the command through captureArgs, which refuses
	// a backend absent from the same table, so an unmapped one here means the run
	// would spawn a binary for a capture this builder cannot drive.
	_, mapped := captureBackends[capture]
	assert.Assert(mapped, "a publish run names a capture backend this builder maps", capture)

	if capture == "kmsgrab" {
		if override := os.Getenv(EnvKmsgrabFFmpeg); override != "" {
			return override, nil
		}
		if wrapper, err := exec.LookPath(kmsgrabWrapper); err == nil {
			return wrapper, nil
		}
	}
	return FindExe("ffmpeg")
}

// logDirMode is the permission the per-run log directory is created with.
const logDirMode = 0o755

// LogDir returns the directory that holds per-run ffmpeg logs, creating it if
// needed. It sits beside the settings file under the user config directory.
func LogDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user config directory: %w", err)
	}

	dir := filepath.Join(base, "screenshare", "logs")
	err = os.MkdirAll(dir, logDirMode)
	if err != nil {
		return "", fmt.Errorf("cannot create log directory %s: %w", dir, err)
	}
	return dir, nil
}
