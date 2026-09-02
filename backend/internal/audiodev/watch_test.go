package audiodev

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"bjoernblessin.de/screenshare/internal/platform"
)

// playExe plays a file into the daemon, which registers it as an output stream of its own.
// The one way to make an application appear inside the application kind without being one.
const playExe = "pw-play"

// silentWav writes a WAV carrying seconds of silence and returns its path.
// Silence rather than a tone: the machine running this suite may have somebody sitting at it.
func silentWav(t *testing.T, seconds int) string {
	t.Helper()

	const rate, channels, bits = 48000, 2, 16
	samples := rate * seconds
	data := samples * channels * bits / 8

	var w []byte
	put := func(v ...any) {
		for _, each := range v {
			switch n := each.(type) {
			case string:
				w = append(w, n...)
			case uint32:
				w = binary.LittleEndian.AppendUint32(w, n)
			case uint16:
				w = binary.LittleEndian.AppendUint16(w, n)
			}
		}
	}
	put("RIFF", uint32(36+data), "WAVEfmt ", uint32(16), uint16(1), uint16(channels),
		uint32(rate), uint32(rate*channels*bits/8), uint16(channels*bits/8), uint16(bits),
		"data", uint32(data))
	w = append(w, make([]byte, data)...)

	path := filepath.Join(t.TempDir(), "silence.wav")
	if err := os.WriteFile(path, w, 0o600); err != nil {
		t.Fatalf("writing the silence to play: %v", err)
	}
	return path
}

// hasApplication reports whether an application stream of this name is on offer.
func hasApplication(devices []platform.AudioDevice, name string) bool {
	for _, d := range devices {
		if d.Kind == platform.AudioSourceApplication && d.Name == name {
			return true
		}
	}
	return false
}

// The application just launched is the one worth selecting, and an answer taken once has it wrong
// every time: a read after it started still describes the machine as it was at the first read.
//
// The daemon's own events are what close that, so this holds a read against a stream
// that appeared after one.
func TestAnApplicationThatStartsLaterIsOnOffer(t *testing.T) {
	ctx := context.Background()
	if len(Devices(ctx)) == 0 {
		t.Skip("no PipeWire daemon on this machine, so nothing appears inside any kind")
	}
	if _, err := exec.LookPath(playExe); err != nil {
		t.Skipf("no %s to raise a stream with", playExe)
	}

	play := exec.Command(playExe, silentWav(t, 10))
	if err := play.Start(); err != nil {
		t.Skipf("starting %s: %v", playExe, err)
	}
	defer func() {
		_ = play.Process.Kill()
		_ = play.Wait()
	}()

	// The daemon registers the stream a moment after the process starts,
	// so the read is repeated to the point the answer is allowed to take.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if hasApplication(Devices(ctx), playExe) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("%s has been playing for 5s and is inside no kind, so the answer is the first one taken",
		playExe)
}

// A stream nothing is playing any more is one a publish cannot open,
// so it comes off the offer the way it went on.
func TestAnApplicationThatStoppedComesOff(t *testing.T) {
	ctx := context.Background()
	if len(Devices(ctx)) == 0 {
		t.Skip("no PipeWire daemon on this machine, so nothing appears inside any kind")
	}
	if _, err := exec.LookPath(playExe); err != nil {
		t.Skipf("no %s to raise a stream with", playExe)
	}

	play := exec.Command(playExe, silentWav(t, 10))
	if err := play.Start(); err != nil {
		t.Skipf("starting %s: %v", playExe, err)
	}

	appeared := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if hasApplication(Devices(ctx), playExe) {
			appeared = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !appeared {
		t.Skip("the daemon never registered the stream, which the test above is the one about")
	}

	_ = play.Process.Kill()
	_ = play.Wait()

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !hasApplication(Devices(ctx), playExe) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("%s has stopped and is still on offer, so a control names a stream nothing can open",
		playExe)
}
