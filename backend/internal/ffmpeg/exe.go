package ffmpeg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"bjoernblessin.de/go-utils/util/assert"
)

// FindExe locates a media executable (ffmpeg, ffplay, mpv, gst-launch-1.0).
// A copy shipped beside the app binary wins over one on PATH, so a bundled build is self-contained.
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
		// The message names the program rather than the project shipping it:
		// this resolves the GStreamer launcher as well as the ffmpeg pair,
		// and a missing GStreamer binary reported under an instruction to install ffmpeg sends the reader
		// after the wrong package.
		return "", fmt.Errorf("%s not found: put it on PATH or place %s next to the app", name, name)
	}
	return path, nil
}

// EnvKmsgrabFFmpeg names the executable kmsgrab capture runs,
// overriding the backend's default resolution.
// kmsgrab reads the raw KMS scanout, which the kernel gates behind CAP_SYS_ADMIN,
// so it needs a privileged ffmpeg the other backends must not share.
// A packaging layer points this at the capability wrapper (nix/mirrorme.nix names
// security.wrappers' ffmpeg-kmsgrab).
const EnvKmsgrabFFmpeg = "MIRRORME_FFMPEG_KMSGRAB"

// kmsgrabWrapper is the privileged build's conventional name on PATH.
const kmsgrabWrapper = "ffmpeg-kmsgrab"

// FindCaptureExe locates the ffmpeg build to run for a capture backend.
//
// kmsgrab is the one backend needing a build of its own: without CAP_SYS_ADMIN (see
// EnvKmsgrabFFmpeg) the plain ffmpeg FindExe resolves cannot open the input.
// Its order is the EnvKmsgrabFFmpeg override, then a wrapper named ffmpeg-kmsgrab on PATH,
// then the plain ffmpeg, which fails on the capability and is no worse than not looking.
// Every other backend runs the plain ffmpeg,
// which keeps the privileged binary off the unprivileged capture backends.
func FindCaptureExe(capture string) (string, error) {
	// captureArgs built the command against this same table and refuses a backend absent from it,
	// so an unmapped one here is a run about to spawn
	// a binary for a capture this builder cannot drive.
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

const logDirMode = 0o755

// LogDir returns the directory holding the per-run ffmpeg logs, creating it if it is not there.
// It sits beside the settings file, under the user config directory.
func LogDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user config directory: %w", err)
	}

	dir := filepath.Join(base, "mirrorme", "logs")
	err = os.MkdirAll(dir, logDirMode)
	if err != nil {
		return "", fmt.Errorf("cannot create log directory %s: %w", dir, err)
	}

	assert.Assert(dir != "", "a resolved log directory is a path", base)
	return dir, nil
}
