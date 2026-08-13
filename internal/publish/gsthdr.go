package publish

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"bjoernblessin.de/screenshare/internal/colour"
	"bjoernblessin.de/screenshare/internal/gstrun"
	"bjoernblessin.de/screenshare/internal/pointer"
	"bjoernblessin.de/screenshare/internal/settings"
)

// gstReadChild splits the publish child's standard output between the two readers of it: the meter,
// which counts frames off the progress lines, and onCaps, which is handed what the capture
// negotiated.
//
// One stream rather than a second pipe or socket, because the child is already spawned with one and
// the two kinds of line cannot be confused: the caps carry a prefix nothing else writes,
// and the meter skips every line its own pattern does not match.
//
// A nil meter is a run nobody asked for progress from, which still reports its caps:
// what the capture turned out to be is not instrumentation.
func gstReadChild(r io.Reader, meter *gstMeter, onCaps func(caps string), onPointer func(p pointer.Position)) {
	pr, pw := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if meter == nil {
			io.Copy(io.Discard, pr)
			return
		}
		meter.parse(pr)
	}()

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if caps, ok := strings.CutPrefix(line, gstrun.CapsPrefix); ok {
			onCaps(caps)
			continue
		}
		if p, ok := gstrun.ParsePointer(line); ok {
			if onPointer != nil {
				onPointer(p)
			}
			continue
		}
		if _, err := pw.Write([]byte(line + "\n")); err != nil {
			break
		}
	}
	pw.Close()
	<-done
}

// Whether a capture is HDR is a property of the surface and never a value the user picks.
//
// What decides it is the transfer characteristic the capture negotiated, which is why this reads
// the caps the pipeline reports rather than a settings field: a monitor cannot be asked on Linux
// today - not through xrandr, not through the wlr protocols, not through the portal - and a field
// would be a claim the app has no way to check.
//
// Which transfer characteristics are HDR is internal/colour's, because a viewer reads the same
// verdict off the stream this publish sends.

// tenBitChroma is the one pixel format this app encodes that carries more than eight bits per
// component.
// Every other format the codec table declares is 8-bit, so an HDR surface has exactly one way
// through.
const tenBitChroma = "p010le"

// captureTransfer is the transfer characteristic in a caps string, and the empty string where the
// caps name none.
//
// Reading which part of the colorimetry is the transfer is the child's, because the child narrows
// the encoder input by the same answer: two spellings of it would let the parent call a surface HDR
// while the pipeline coded it as something else.
func captureTransfer(caps string) string {
	value, ok := capsField(caps, "colorimetry")
	if !ok {
		return ""
	}
	return colour.TransferOfColorimetry(value)
}

// capsField is one field's value out of a caps string, and false where the caps carry no such
// field.
// GStreamer prints them as `name=(type)value`, comma separated.
func capsField(caps, name string) (string, bool) {
	for _, field := range strings.Split(caps, ",") {
		field = strings.TrimSpace(field)
		rest, ok := strings.CutPrefix(field, name+"=")
		if !ok {
			continue
		}
		// The type in parentheses is GStreamer's; what a reader of this wants is the value.
		if i := strings.Index(rest, ")"); strings.HasPrefix(rest, "(") && i > 0 {
			rest = rest[i+1:]
		}
		return strings.Trim(rest, `"`), true
	}
	return "", false
}

// hdrCapture reports whether caps describe an HDR surface.
func hdrCapture(caps string) bool {
	return colour.IsHDR(captureTransfer(caps))
}

// hdrRefusal is why this publish must not continue, and nil where the capture and the pixel format
// agree.
//
// An HDR capture cannot ride in eight bits.
// The two ways to continue anyway are both silent failures: tone-mapping down without being asked
// is a fallback that changes the picture nobody said to change, and coding the surface into an
// 8-bit format with the tag dropped sends wrong colour with nothing saying so.
// So the publish stops, and the message names both ends - what the capture turned out to carry,
// and what the settings asked to code it as - because either one is a way out and the user picks
// which.
func hdrRefusal(s settings.Settings, caps string) error {
	if !hdrCapture(caps) || s.Publish.Chroma == tenBitChroma {
		return nil
	}
	return fmt.Errorf(
		"the capture is HDR (transfer %s) and %s carries eight bits per component: encode it as %s, or capture a standard-range surface",
		captureTransfer(caps), s.Publish.Chroma, tenBitChroma)
}
