package webviewer

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// ivfStream builds an IVF byte stream with the given time base and frames, so
// the reader can be exercised without ffmpeg.
func ivfStream(tbDen, tbNum uint32, frames [][]byte, ts []uint64) []byte {
	var b bytes.Buffer
	hdr := make([]byte, 32)
	copy(hdr[0:4], "DKIF")
	binary.LittleEndian.PutUint16(hdr[6:8], 32)
	copy(hdr[8:12], "VP90")
	binary.LittleEndian.PutUint32(hdr[16:20], tbDen)
	binary.LittleEndian.PutUint32(hdr[20:24], tbNum)
	b.Write(hdr)
	for i, f := range frames {
		fh := make([]byte, 12)
		binary.LittleEndian.PutUint32(fh[0:4], uint32(len(f)))
		binary.LittleEndian.PutUint64(fh[4:12], ts[i])
		b.Write(fh)
		b.Write(f)
	}
	return b.Bytes()
}

func TestReadIVFFramesAndPTS(t *testing.T) {
	// Time base 1/1000000 makes the timestamp equal the microsecond PTS.
	key := []byte{0xA0, 1, 2, 3} // profile 1 key frame
	inter := []byte{0xA4, 4, 5}  // profile 1 inter frame
	stream := ivfStream(1_000_000, 1, [][]byte{key, inter}, []uint64{0, 33333})

	type got struct {
		pts      uint64
		keyframe bool
		size     int
	}
	var frames []got
	err := readIVF(bytes.NewReader(stream), func(payload []byte, ptsUs uint64, keyframe bool) error {
		frames = append(frames, got{ptsUs, keyframe, len(payload)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	if frames[0] != (got{0, true, 4}) {
		t.Errorf("frame 0 = %+v, want {0 true 4}", frames[0])
	}
	if frames[1] != (got{33333, false, 3}) {
		t.Errorf("frame 1 = %+v, want {33333 false 3}", frames[1])
	}
}

func TestReadIVFRejectsBadSignature(t *testing.T) {
	bad := make([]byte, 32)
	copy(bad[0:4], "XXXX")
	if err := readIVF(bytes.NewReader(bad), func([]byte, uint64, bool) error { return nil }); err == nil {
		t.Fatal("expected an error for a non-IVF stream")
	}
}

func TestVP9Keyframe(t *testing.T) {
	tests := []struct {
		name  string
		b     byte
		isKey bool
	}{
		{"profile 0 key", 0x80, true},
		{"profile 0 inter", 0x84, false},
		{"profile 1 key", 0xA0, true},
		{"show-existing is not key", 0x88, false},
		{"bad frame marker", 0x00, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := vp9IsKeyframe([]byte{tc.b}); got != tc.isKey {
				t.Errorf("vp9IsKeyframe(%#x) = %v, want %v", tc.b, got, tc.isKey)
			}
		})
	}
}
