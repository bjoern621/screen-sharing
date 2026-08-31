package framestamp

import (
	"bytes"
	"testing"
	"time"
)

func h264(format string) Carriage {
	return Carriage{Media: MediaH264, Format: format, Alignment: "au", LengthSize: 4}
}

// A stamp read back off the unit that carries it is what was written, to the nanosecond and
// to the millisecond.
func TestUnitReadsBack(t *testing.T) {
	want := Stamp{
		At:            time.Unix(1_700_000_000, 123_456_789),
		PublishMs:     4_294_967_295,
		PublishFrames: 123_456,
		LinkMs:        65_535,
	}

	for _, c := range []Carriage{
		h264("byte-stream"),
		h264("avc"),
		{Media: MediaH265, Format: "byte-stream", Alignment: "au"},
		{Media: MediaH265, Format: "hvc1", Alignment: "au", LengthSize: 4},
	} {
		unit, ok := Unit(c, want)
		if !ok {
			t.Fatalf("%s/%s carries no stamp", c.Media, c.Format)
		}
		got, found := Read(unit)
		if !found {
			t.Fatalf("%s/%s: no stamp read back out of %d bytes", c.Media, c.Format, len(unit))
		}
		if !got.At.Equal(want.At) {
			t.Errorf("%s/%s: read %v, want %v", c.Media, c.Format, got.At, want.At)
		}
		if got.PublishMs != want.PublishMs || got.PublishFrames != want.PublishFrames {
			t.Errorf("%s/%s: read %d ms over %d frames, want %d over %d",
				c.Media, c.Format, got.PublishMs, got.PublishFrames, want.PublishMs, want.PublishFrames)
		}
		if got.LinkMs != want.LinkMs {
			t.Errorf("%s/%s: read a window of %d ms, want %d", c.Media, c.Format, got.LinkMs, want.LinkMs)
		}
	}
}

// The publishing side's reading rides with the picture it describes,
// so it reaches a viewer of somebody else's stream.
// Carried as a sum and a count, so the viewer divides over its own interval rather than taking
// a rate somebody else averaged.
func TestUnitCarriesThePublishingSidesReading(t *testing.T) {
	unit, ok := Unit(h264("byte-stream"), Stamp{At: time.Now(), PublishMs: 800, PublishFrames: 100, LinkMs: 300})
	if !ok {
		t.Fatal("h264 carries no stamp")
	}

	got, found := Read(unit)
	if !found {
		t.Fatal("no stamp read back")
	}
	if got.PublishMs != 800 || got.PublishFrames != 100 {
		t.Errorf("read %d ms over %d frames, want 800 over 100", got.PublishMs, got.PublishFrames)
	}
	if got.LinkMs != 300 {
		t.Errorf("read a window of %d ms, want 300", got.LinkMs)
	}
}

// A publish measuring none of its own stages stamps the clock and nothing else,
// read back as nothing measured rather than as a stage that cost nothing.
func TestUnitWithNothingMeasuredOnThePublishingSide(t *testing.T) {
	unit, ok := Unit(h264("byte-stream"), Stamp{At: time.Now()})
	if !ok {
		t.Fatal("h264 carries no stamp")
	}

	got, _ := Read(unit)
	if got.PublishFrames != 0 || got.PublishMs != 0 || got.LinkMs != 0 {
		t.Errorf("read %d ms over %d frames and a window of %d, want all absent",
			got.PublishMs, got.PublishFrames, got.LinkMs)
	}
}

// A stamp is found in front of the rest of an access unit, where an inserted one sits.
func TestReadFindsStampAheadOfAFrame(t *testing.T) {
	at := time.Unix(1_700_000_001, 0)
	unit, ok := Unit(h264("byte-stream"), Stamp{At: at})
	if !ok {
		t.Fatal("h264 carries no stamp")
	}

	frame := append(append([]byte{}, unit...), 0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84, 0x00)
	got, found := Read(frame)
	if !found {
		t.Fatal("no stamp read out of a stamped access unit")
	}
	if !got.At.Equal(at) {
		t.Errorf("read %v, want %v", got.At, at)
	}
}

// Nothing is read out of a frame nobody stamped, however long it is.
func TestReadOfAnUnstampedFrame(t *testing.T) {
	frame := bytes.Repeat([]byte{0x00, 0x00, 0x01, 0x65, 0xAB}, 200)
	if _, found := Read(frame); found {
		t.Error("a stamp was read out of an unstamped frame")
	}
}

// A marker with less than a whole stamp behind it is not a stamp: reading one would run off the end
// of the frame and report whatever followed as a measurement.
func TestReadOfATruncatedStamp(t *testing.T) {
	unit, ok := Unit(h264("byte-stream"), Stamp{At: time.Now()})
	if !ok {
		t.Fatal("h264 carries no stamp")
	}

	// Every length that cuts into the reading itself, which ends one byte short of the unit:
	// the unit closes with a trailing bit no reader needs.
	for short := len(unit) - 2; short > len(startCode)+len(marker); short-- {
		if _, found := Read(unit[:short]); found {
			t.Fatalf("a stamp was read out of the first %d of %d bytes", short, len(unit))
		}
	}
}

// The unit holds no zero byte past its start code,
// so a decoder's emulation-prevention rule never rewrites the bytes a reader matches on.
func TestUnitHoldsNoZeroByte(t *testing.T) {
	// The instant whose nanosecond field is all zero bits, and nothing measured beside it: the values
	// that would carry zero bytes if the encoding did not spread them.
	unit, ok := Unit(h264("byte-stream"), Stamp{At: time.Unix(1_700_000_000, 0)})
	if !ok {
		t.Fatal("h264 carries no stamp")
	}

	body := unit[len(startCode):]
	if i := bytes.IndexByte(body, 0x00); i >= 0 {
		t.Errorf("byte %d of the unit is zero, which emulation prevention would escape", i)
	}
}

// A codec with no user-data unit carries no stamp, and says so rather than producing bytes
// its bitstream cannot hold.
func TestCodecWithNoCarriage(t *testing.T) {
	for _, media := range []string{"video/x-vp9", "video/x-vp8", "video/x-raw"} {
		if _, ok := Unit(Carriage{Media: media, Format: "byte-stream", Alignment: "au"}, Stamp{At: time.Now()}); ok {
			t.Errorf("%s reported a stamp unit", media)
		}
	}
}

// A stream whose buffers are not whole access units carries no stamp: a unit prepended
// to a fragment lands wherever that fragment sits, not necessarily ahead of a picture.
func TestAlignmentBelowAnAccessUnit(t *testing.T) {
	c := h264("byte-stream")
	c.Alignment = "nal"
	if _, ok := Unit(c, Stamp{At: time.Now()}); ok {
		t.Error("a NAL-aligned stream reported a stamp unit")
	}
}

// A unit goes behind the parameter sets that open an access unit and in front of the picture,
// where the codecs put a prefix message,
// and where a parser looking for the start of a stream keeps it.
func TestOffsetIsBehindTheParameterSets(t *testing.T) {
	sps := []byte{0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0x00, 0x1E}
	pps := []byte{0x00, 0x00, 0x00, 0x01, 0x68, 0xCE, 0x38, 0x80}
	idr := []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84, 0x00}

	frame := append(append(append([]byte{}, sps...), pps...), idr...)
	if got, want := Offset(h264("byte-stream"), frame), len(sps)+len(pps); got != want {
		t.Errorf("offset %d, want %d, which is where the picture starts", got, want)
	}
}

// A frame opening with the picture takes the unit in front of everything, there being no parameter
// set to sit behind.
func TestOffsetOfAPictureAlone(t *testing.T) {
	frame := []byte{0x00, 0x00, 0x00, 0x01, 0x41, 0x9A, 0x00, 0x10}
	if got := Offset(h264("byte-stream"), frame); got != 0 {
		t.Errorf("offset %d, want 0", got)
	}
}

// The same reading on a length-prefixed frame, whose units state their size instead of being
// separated by a start code.
func TestOffsetOfALengthPrefixedFrame(t *testing.T) {
	sps := []byte{0x00, 0x00, 0x00, 0x04, 0x67, 0x42, 0x00, 0x1E}
	idr := []byte{0x00, 0x00, 0x00, 0x04, 0x65, 0x88, 0x84, 0x00}

	frame := append(append([]byte{}, sps...), idr...)
	if got, want := Offset(h264("avc"), frame), len(sps); got != want {
		t.Errorf("offset %d, want %d", got, want)
	}
}

// H.265 states its unit type in a header of its own shape, so the same reading needs the codec's
// own row rather than H.264's.
func TestOffsetOfAnH265Frame(t *testing.T) {
	vps := []byte{0x00, 0x00, 0x00, 0x01, 0x40, 0x01, 0x0C, 0x01}
	sps := []byte{0x00, 0x00, 0x00, 0x01, 0x42, 0x01, 0x01, 0x01}
	idr := []byte{0x00, 0x00, 0x00, 0x01, 0x26, 0x01, 0xAF, 0x00}

	frame := append(append(append([]byte{}, vps...), sps...), idr...)
	c := Carriage{Media: MediaH265, Format: "byte-stream", Alignment: "au"}
	if got, want := Offset(c, frame), len(vps)+len(sps); got != want {
		t.Errorf("offset %d, want %d", got, want)
	}
}

// A frame nothing can be read out of takes the unit in front, a guessed offset landing inside
// a unit and breaking the frame.
func TestOffsetOfAnUnreadableFrame(t *testing.T) {
	for _, frame := range [][]byte{nil, {0x00}, {0xAB, 0xCD, 0xEF}} {
		if got := Offset(h264("byte-stream"), frame); got != 0 {
			t.Errorf("offset %d into %d unreadable bytes, want 0", got, len(frame))
		}
	}
}

// The length-prefixed framing states the size of the unit that follows it, in the size of prefix
// the caps declare.
func TestLengthPrefixedFraming(t *testing.T) {
	for _, size := range []int{1, 2, 4} {
		c := h264("avc")
		c.LengthSize = size
		unit, ok := Unit(c, Stamp{At: time.Now()})
		if !ok {
			t.Fatalf("length size %d carries no stamp", size)
		}
		var stated int
		for _, b := range unit[:size] {
			stated = stated<<8 | int(b)
		}
		if stated != len(unit)-size {
			t.Errorf("length size %d: prefix states %d bytes, %d follow", size, stated, len(unit)-size)
		}
	}
}
