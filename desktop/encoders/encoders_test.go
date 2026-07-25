package encoders

import (
	"testing"

	"bjoernblessin.de/screenshare/capabilities"
	"bjoernblessin.de/screenshare/publish"
)

// Every publish engine has to answer the probe, or the UI reports a codec available on
// an engine nothing tested and the publish fails at launch.
func TestEveryEngineIsProbed(t *testing.T) {
	for _, engine := range publish.Engines() {
		if _, ok := engineProbes[engine]; !ok {
			t.Errorf("publish engine %s has no encoder probe", engine)
		}
	}
}

// A probed codec has to exist in the capability table, on both engines: a name that
// matches no row would be reported unusable forever, greying a codec nothing offers.
func TestProbedCodecsAreInTheTable(t *testing.T) {
	for engine, probe := range engineProbes {
		for _, codec := range probe.codecs() {
			c, ok := capabilities.Get(codec)
			if !ok {
				t.Errorf("%s probes %s, which is not in the capability table", engine, codec)
				continue
			}
			if !c.Implemented {
				t.Errorf("%s probes %s, which the argument builders do not map", engine, codec)
			}
		}
	}
}

// The GStreamer half probes an element per codec, so every codec it lists has to
// resolve to one.
func TestGstProbedCodecsResolveToAnElement(t *testing.T) {
	for _, codec := range gstProbed() {
		if _, ok := publish.GstEncoderElement(codec); !ok {
			t.Errorf("%s is probed on the GStreamer engine but maps to no element", codec)
		}
	}
}
