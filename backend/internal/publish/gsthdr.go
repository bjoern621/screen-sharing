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

// gstReadChild splits the publish child's standard output between its readers: the meter counting
// frames off the progress lines and timing them off the delay lines, onCaps handed what the capture
// negotiated, and onPointer handed the cursor positions.
//
// One stream rather than a second pipe or socket, the child being spawned with this one already and
// the kinds of line telling themselves apart: the caps, the pointer and the delay carry prefixes
// nothing else writes, and the meter skips every line its own pattern does not match.
//
// A nil meter is a run nobody asked progress from, and a nil onPointer one that tracks no cursor.
// Either still reports its caps: what the capture negotiated is not instrumentation.
func gstReadChild(r io.Reader, meter *gstMeter, onCaps func(caps string), onPointer func(p pointer.Spot)) {
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
		// Recorded rather than sampled, as a capture progress line is: the delay report and the encoded
		// counter run on clocks of their own, and emitting on either would give two samples a second
		// with one half of each unchanged.
		if d, ok := gstrun.ParseDelay(line); ok {
			if meter != nil {
				meter.takeDelay(d)
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

// Whether a capture is HDR is the surface's property.
//
// The transfer characteristic the capture negotiated decides it, so the verdict is read off
// the caps the pipeline reports rather than off a settings field: no Linux interface answers what
// a monitor is driving, neither xrandr nor the wlr protocols nor the portal, so a field would be
// a claim nothing can check.
//
// Which transfer characteristics are HDR is internal/colour's, a viewer taking that verdict off
// the stream this publish sends.

// tenBitChroma is the one pixel format this app encodes at more than eight bits per component,
// every other format the codec table declares being 8-bit, so an HDR surface has one way through.
const tenBitChroma = "p010le"

// captureTransfer is the transfer characteristic a caps string carries, and empty where it names
// none.
//
// Which part of the colorimetry that is comes from internal/colour, the answer the child narrows
// the encoder input by: two readings of it would let the parent call a surface HDR while
// the pipeline coded it as something else.
func captureTransfer(caps string) string {
	value, ok := capsField(caps, "colorimetry")
	if !ok {
		return ""
	}
	return colour.TransferOfColorimetry(value)
}

// capsField is one field's value out of a caps string, false where the caps carry no such field.
// GStreamer prints them as `name=(type)value`, comma separated.
func capsField(caps, name string) (string, bool) {
	for _, field := range strings.Split(caps, ",") {
		field = strings.TrimSpace(field)
		rest, ok := strings.CutPrefix(field, name+"=")
		if !ok {
			continue
		}
		if i := strings.Index(rest, ")"); strings.HasPrefix(rest, "(") && i > 0 {
			rest = rest[i+1:]
		}
		return strings.Trim(rest, `"`), true
	}
	return "", false
}

func hdrCapture(caps string) bool {
	return colour.IsHDR(captureTransfer(caps))
}

// hdrRefusal is why the publish must not continue, and nil where the capture and the pixel format
// agree.
//
// An HDR capture cannot ride in eight bits, and continuing anyway is silent either way:
// tone-mapping down changes a picture nobody asked to change, and coding the surface as 8-bit
// with the tag dropped sends wrong colour with nothing saying so.
// The message names both ends, what the capture carries and what the settings asked to code it as,
// either one being a way out and the choice the user's.
func hdrRefusal(s settings.Settings, caps string) error {
	if !hdrCapture(caps) || s.Publish.Chroma == tenBitChroma {
		return nil
	}
	return fmt.Errorf(
		"the capture is HDR (transfer %s) and %s carries eight bits per component: encode it as %s, or capture a standard-range surface",
		captureTransfer(caps), s.Publish.Chroma, tenBitChroma)
}
