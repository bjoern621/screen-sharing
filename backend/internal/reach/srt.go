package reach

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/url"

	"bjoernblessin.de/go-utils/util/assert"
)

// SRT is UDP, where opening a socket reaches nothing and says nothing, so the check is the exchange
// itself: a caller's induction request, which a listener answers with a cookie before a stream id
// or a passphrase has been sent.
// Neither is needed to learn that it is there, and neither is offered.
const (
	// srtControlHandshake leads a handshake packet: the top bit says control, and the 15 type bits
	// and 16 subtype bits behind it are the zero that means handshake.
	srtControlHandshake = 0x80000000
	// srtInduction is the first of the two handshake phases.
	srtInduction = 1
	// A handshake packet: a 16-byte control header and the 48-byte handshake behind it.
	srtPacketBytes = 64
	// srtDatagram is the socket type SRT uses, the field carrying a type in the version-4 handshake
	// every caller opens with.
	srtDatagram = 2
	// What the caller offers for the connection it is not going to open. A listener answers the
	// induction whatever these say, and the conclusion that would negotiate them never follows.
	srtMTU             = 1500
	srtFlowWindow      = 8192
	srtInitialSequence = 1
	// The socket this probe calls itself, fixed because induction derives its cookie from the address
	// rather than holding a session for it, so two checks in a row cost the listener nothing.
	srtProbeSocket = 1
)

// probeSRT sends the induction request and answers what came back.
//
// A port with no listener behind it swallows the datagram, so the failure is the deadline rather
// than a refusal, and on a machine that answers ICMP it is the refusal.
func probeSRT(ctx context.Context, t target) (string, error) {
	u, err := url.Parse(t.url)
	assert.Assert(err == nil, "a leg's address parses", t.url)

	var dialer net.Dialer
	c, err := dialer.DialContext(ctx, "udp", u.Host)
	if err != nil {
		return "", err
	}
	defer c.Close()

	if deadline, ok := ctx.Deadline(); ok {
		if err := c.SetDeadline(deadline); err != nil {
			return "", err
		}
	}
	if _, err := c.Write(inductionRequest()); err != nil {
		return "", err
	}

	answer := make([]byte, 1500)
	n, err := c.Read(answer)
	if err != nil {
		return "", err
	}
	return inductionAnswer(answer[:n])
}

// inductionRequest is the packet an SRT caller opens with.
//
// The fields left zero are zero on a first packet: the destination socket, which the listener names
// in its answer, the SYN cookie it answers with, and the peer address it reads off the datagram.
func inductionRequest() []byte {
	p := make([]byte, srtPacketBytes)

	binary.BigEndian.PutUint32(p[0:], srtControlHandshake)
	// Version 4 opens every handshake whatever the caller speaks, and a listener on version 5 answers
	// with 5.
	binary.BigEndian.PutUint32(p[16:], 4)
	binary.BigEndian.PutUint32(p[20:], srtDatagram)
	binary.BigEndian.PutUint32(p[24:], srtInitialSequence)
	binary.BigEndian.PutUint32(p[28:], srtMTU)
	binary.BigEndian.PutUint32(p[32:], srtFlowWindow)
	binary.BigEndian.PutUint32(p[36:], srtInduction)
	binary.BigEndian.PutUint32(p[40:], srtProbeSocket)

	assert.Assert(len(p) == srtPacketBytes, "an induction request is a whole handshake packet", len(p))
	return p
}

// inductionAnswer reads a listener's reply, which is a handshake packet naming the version it
// speaks.
// Anything else on the port is something other than SRT, and the row says so rather than counting
// the datagram as the leg answering.
func inductionAnswer(p []byte) (string, error) {
	if len(p) < srtPacketBytes {
		return "", fmt.Errorf("answered %d bytes, which is no SRT handshake", len(p))
	}
	if leading := binary.BigEndian.Uint32(p); leading != srtControlHandshake {
		return "", fmt.Errorf("answered %#08x, which is no SRT handshake", leading)
	}
	return fmt.Sprintf("handshake answered, version %d", binary.BigEndian.Uint32(p[16:])), nil
}
