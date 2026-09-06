package discordrpc

import (
	"encoding/binary"
	"fmt"
	"io"

	"bjoernblessin.de/go-utils/util/assert"
)

// A frame is a little-endian opcode, a little-endian payload length, and that many bytes of JSON.
//
// The opcodes the desktop client speaks.
// Handshake opens the connection and frame carries every command and answer after it.
// Close arrives with a reason where the client hangs up,
// and the two heartbeat opcodes are answered by the client rather than asked for here.
const (
	opHandshake uint32 = 0
	opFrame     uint32 = 1
	opClose     uint32 = 2
	opPing      uint32 = 3
	opPong      uint32 = 4
)

// payloadLimit bounds a frame read off the socket.
// The client's answers are small JSON,
// so a longer length is a stream out of step rather than a payload worth allocating for.
const payloadLimit = 64 * 1024

// writeFrame sends one whole frame.
func writeFrame(w io.Writer, op uint32, payload []byte) error {
	assert.IsNotNil(w, "a frame is written to a connection")
	assert.Assert(len(payload) <= payloadLimit, "a frame written here fits the bound reads take", len(payload))

	frame := make([]byte, 8+len(payload))
	binary.LittleEndian.PutUint32(frame[0:4], op)
	binary.LittleEndian.PutUint32(frame[4:8], uint32(len(payload)))
	copy(frame[8:], payload)

	if _, err := w.Write(frame); err != nil {
		return fmt.Errorf("cannot write to the Discord client: %w", err)
	}
	return nil
}

// readFrame reads one whole frame.
func readFrame(r io.Reader) (uint32, []byte, error) {
	assert.IsNotNil(r, "a frame is read off a connection")

	var header [8]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, fmt.Errorf("cannot read from the Discord client: %w", err)
	}

	op := binary.LittleEndian.Uint32(header[0:4])
	length := binary.LittleEndian.Uint32(header[4:8])
	if length > payloadLimit {
		return 0, nil, fmt.Errorf("the Discord client announced a frame of %d bytes, past the %d this reads", length, payloadLimit)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, fmt.Errorf("cannot read from the Discord client: %w", err)
	}
	return op, payload, nil
}
