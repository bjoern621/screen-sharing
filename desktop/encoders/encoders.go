// Package encoders reports which hardware video encoders this machine can
// actually run, so the UI can grey out codecs that would only fail at launch.
//
// The NVENC codecs need an NVIDIA GPU, its driver, and an ffmpeg build with
// nvenc compiled in. None of those can be inferred from the OS alone, and the
// encoder appearing in "ffmpeg -encoders" only proves the build has it, not that
// a card is present. The reliable test is to run a one-frame encode and see if it
// exits cleanly, which is what Detect does.
package encoders

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"bjoernblessin.de/screenshare/ffmpeg"
)

// probed lists the hardware encoders worth testing. Software libx264 is always
// present in the build and never fails to initialize, so it is not probed.
var probed = []string{"hevc_nvenc", "h264_nvenc", "av1_nvenc"}

// probeTimeout bounds a single test encode. A working encoder returns in well
// under a second; the timeout only guards against a hung ffmpeg.
const probeTimeout = 10 * time.Second

// Availability maps each probed codec to whether a test encode succeeded on this
// machine. Codecs absent from probed (e.g. libx264) do not appear here and the
// UI treats them as always available.
type Availability struct {
	Usable map[string]bool `json:"usable"`
}

// Detect probes every hardware codec concurrently and returns which ones this
// machine can run. If ffmpeg cannot be located, every probed codec is reported
// unusable, leaving software libx264 as the only choice.
func Detect(ctx context.Context) Availability {
	usable := make(map[string]bool, len(probed))

	exe, err := ffmpeg.FindExe("ffmpeg")
	if err != nil {
		for _, codec := range probed {
			usable[codec] = false
		}
		return Availability{Usable: usable}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, codec := range probed {
		wg.Add(1)
		go func(codec string) {
			defer wg.Done()
			ok := probe(ctx, exe, codec)
			mu.Lock()
			usable[codec] = ok
			mu.Unlock()
		}(codec)
	}
	wg.Wait()

	return Availability{Usable: usable}
}

// probe encodes a single 64x64 frame with codec and reports whether ffmpeg
// exited cleanly. Success confirms the whole chain: the build has the encoder,
// the driver is loaded, and a GPU accepted the session.
func probe(ctx context.Context, exe, codec string) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe,
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "nullsrc=s=64x64",
		"-frames:v", "1", "-c:v", codec, "-f", "null", "-",
	)
	return cmd.Run() == nil
}
