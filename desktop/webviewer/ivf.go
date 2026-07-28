package webviewer

import (
	"encoding/binary"
	"fmt"
	"io"

	"bjoernblessin.de/go-utils/util/assert"
)

// readIVF reads an IVF elementary stream (ffmpeg's "-f ivf" output) and calls
// onFrame for each VP9 frame with its payload, presentation timestamp in
// microseconds, and keyframe flag. It returns nil at end of stream and an error
// on a malformed header or a truncated frame. onFrame returning an error stops
// the read (used to end when the WebSocket client goes away).
//
// IVF layout: a 32-byte file header, then per frame a 12-byte header (4-byte
// little-endian size, 8-byte little-endian timestamp in time-base units)
// followed by the frame bytes. The file header carries the time base as
// denominator at offset 16 and numerator at offset 20, matching ffmpeg's ivf
// muxer, so a timestamp converts to seconds as ts*num/den.
func readIVF(r io.Reader, onFrame func(payload []byte, ptsUs uint64, keyframe bool) error) error {
	assert.IsNotNil(r, "an IVF read has a stream to read from")
	assert.IsNotNil(onFrame, "an IVF read has a sink for every frame")

	var fileHdr [32]byte
	if _, err := io.ReadFull(r, fileHdr[:]); err != nil {
		return fmt.Errorf("read IVF file header: %w", err)
	}
	if string(fileHdr[0:4]) != "DKIF" {
		return fmt.Errorf("not an IVF stream (bad signature)")
	}
	tbDen := binary.LittleEndian.Uint32(fileHdr[16:20])
	tbNum := binary.LittleEndian.Uint32(fileHdr[20:24])
	if tbDen == 0 {
		tbDen = 1
	}

	var frameHdr [12]byte
	for {
		if _, err := io.ReadFull(r, frameHdr[:]); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		size := binary.LittleEndian.Uint32(frameHdr[0:4])
		ts := binary.LittleEndian.Uint64(frameHdr[4:12])
		payload := make([]byte, size)
		if _, err := io.ReadFull(r, payload); err != nil {
			return fmt.Errorf("read IVF frame: %w", err)
		}
		ptsUs := ts * uint64(tbNum) * 1_000_000 / uint64(tbDen)
		if err := onFrame(payload, ptsUs, vp9IsKeyframe(payload)); err != nil {
			return err
		}
	}
}

// vp9IsKeyframe reports whether a VP9 frame is a key frame, from the
// uncompressed header's first byte. The bit layout after the 2-bit frame marker
// (0b10) is: profile low bit, profile high bit, (a reserved bit only for profile
// 3), show_existing_frame, then frame_type (0 = key). A shown existing frame is
// not a key frame.
func vp9IsKeyframe(frame []byte) bool {
	if len(frame) < 1 {
		return false
	}
	b := frame[0]
	if b>>6 != 0x2 {
		return false
	}
	profile := (b>>4)&1<<1 | (b>>5)&1
	var showExisting, frameType byte
	if profile == 3 {
		showExisting = (b >> 2) & 1
		frameType = (b >> 1) & 1
	} else {
		showExisting = (b >> 3) & 1
		frameType = (b >> 2) & 1
	}
	return showExisting == 0 && frameType == 0
}
