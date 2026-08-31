package relay

import (
	"encoding/json"
	"fmt"
	"net/http"

	"bjoernblessin.de/go-utils/util/assert"
)

// Reader is one connection the relay is serving a path to.
//
// The path list names a reader by type and id alone, so every figure below comes
// from the per-protocol list that type is served by, joined on that id.
// Hence a pointer per figure: a protocol reporting none leaves it absent, and absent is not
// a measured zero.
// SRT alone is measured for a round trip and a loss rate, the rest reporting what was sent to them
// and little more (readerKinds).
type Reader struct {
	// Type is the relay's own token for what this reader is: "srtConn".
	// Kept as the relay wrote it, being the join key's other half and what finds the connection
	// a snapshot row came from.
	Type string `json:"type"`
	ID   string `json:"id"`
	// Transport is Type in this app's vocabulary: "srt", "rtsp", "rtmp", "webrtc", "hls", "moq".
	// A type with no row here keeps the relay's own token, so an unknown reader reads as whatever
	// the relay called it rather than as nothing.
	// A reader the relay named nothing leaves this empty, the same "unknown" an unrecognised track
	// format leaves on the path beside it.
	Transport string `json:"transport"`

	// RemoteAddr is host:port as the relay saw it, empty where the join found no connection: a reader
	// that ended between the two calls, or one on a protocol whose list this relay does not serve.
	RemoteAddr string `json:"remoteAddr,omitempty"`
	// Joined is when the relay accepted this reader, in the relay's own RFC 3339 spelling and clock.
	// Passed through unparsed: only the relay's clock dates its connections, and reformatting here
	// would put a second opinion about the time on the wire.
	Joined string `json:"joined,omitempty"`

	BytesSent *uint64 `json:"bytesSent,omitempty"`
	// RttMs is the smoothed round trip to this reader, in milliseconds.
	// SRT alone reports one.
	RttMs *float64 `json:"rttMs,omitempty"`
	// LossPercent is SRT's own send-side loss rate, percent of resent data against sent data.
	// The relay's figure rather than one computed from two counters, a rate off lifetime counters
	// being a run's average wearing the look of a current reading.
	LossPercent *float64 `json:"lossPercent,omitempty"`

	PacketsSent *uint64 `json:"packetsSent,omitempty"`
	// PacketsLost were lost on the way to this reader, as the sender counted them or as the receiver
	// reported them back.
	// Cumulative over the connection, as are PacketsDropped and FramesDiscarded.
	PacketsLost *uint64 `json:"packetsLost,omitempty"`
	// PacketsDropped were given up on by the sender rather than lost in transit.
	PacketsDropped *uint64 `json:"packetsDropped,omitempty"`
	// FramesDiscarded were dropped by the relay itself on a full outgoing queue for this reader.
	// The one congestion signal protocols with no round trip and no loss rate still give, and a fact
	// about the relay rather than the line, so it is counted apart from the loss counters.
	FramesDiscarded *uint64 `json:"framesDiscarded,omitempty"`
}

// readerKind states, per kind of reader the relay can report, which per-protocol list describes it
// and what this app calls the leg.
//
// The move trackFormats makes for codec names: the relay names a thing in its own vocabulary, one
// table converts it, and every consumer reads the table instead of restating the rule.
// The list segment is the other half, the path list giving a type and an id alone, so the type
// is the only thing that can say where the figures for that id live.
//
// Which figures each list carries, measured against a MediaMTX v1.20.0 relay and its OpenAPI
// document:
//
//	kind                      list              sent  rtt  loss  pkts  lost  drop  discarded
//	srtConn                   srtconns           y     y    y     y     y     y     y
//	rtspSession/rtspsSession  rtsp(s)sessions    y     -    -     y     y     y     -
//	rtspConn/rtspsConn        rtsp(s)conns       y     -    -     -     -     -     -
//	rtmpConn/rtmpsConn        rtmp(s)conns       y     -    -     -     -     -     y
//	webRTCSession             webrtcsessions     y     -    -     y     -     -     y
//	hlsSession                hlssessions        y     -    -     -     -     -     -
//	moqSession                moqsessions        y     -    -     -     -     -     -
//	hidden                    -                  -     -    -     -     -     -     -
//
// Nothing enforces that table beyond the field names in apiConn: a list carrying no field
// for a figure leaves it absent.
// A reader type with no row here is no error either: a relay serving a protocol this build never
// heard of reads honestly as a reader named and unmeasured rather than a reader dropped.
type readerKind struct {
	list      string
	transport string
	// kick is whether that list takes a kick beside it, what membership enforcement sweeps
	// (sessions.go).
	//
	// Measured against v1.20.0: rtspconns and rtspsconns answer a list and have no kick of their own,
	// a connection being closed by kicking the session running on it.
	kick bool
}

var readerKinds = map[string]readerKind{
	"srtConn":       {list: "srtconns", transport: "srt", kick: true},
	"rtspConn":      {list: "rtspconns", transport: "rtsp"},
	"rtspSession":   {list: "rtspsessions", transport: "rtsp", kick: true},
	"rtmpConn":      {list: "rtmpconns", transport: "rtmp", kick: true},
	"webRTCSession": {list: "webrtcsessions", transport: "webrtc", kick: true},
	"hlsSession":    {list: "hlssessions", transport: "hls", kick: true},
	"moqSession":    {list: "moqsessions", transport: "moq", kick: true},

	// TLS variants are the same protocol to a viewer, one name each in this app's vocabulary.
	// Which listener the relay was reached over is a fact about the relay rather than the leg, and no
	// consumer asks it.
	"rtspsConn":    {list: "rtspsconns", transport: "rtsp"},
	"rtspsSession": {list: "rtspssessions", transport: "rtsp", kick: true},
	"rtmpsConn":    {list: "rtmpsconns", transport: "rtmp", kick: true},

	// A reader the relay declines to describe.
	// Listed rather than left out, so a type with no list is visible beside the ones that have one.
	// Empty segment takes the path an unknown type takes.
	"hidden": {list: "", transport: ""},
}

// apiConn is one item of a per-protocol connection or session list, decoded into the union
// of fields this app reads across all of them.
//
// One struct rather than one per endpoint, so absence falls out of decoding: a list carrying no
// msRTT leaves MsRTT nil without anything here knowing which lists those are.
// That also fixes the direction a mistake fails in: a wrong field name decodes to nil and reads
// as unmeasured, where a name right for the wrong endpoint decodes to a zero and reads
// as a measurement.
// A measured zero and an absence are the two things this snapshot may never confuse.
//
// The paired fields are two spellings of one figure, never two figures.
// Every list states the bytes it sent as outboundBytes and repeats it as the deprecated bytesSent,
// except the SRT list, which carries bytesSent alone.
// Packet counters are SRT's sender-side names beside the RTSP session's RTCP-reported ones.
type apiConn struct {
	ID         string `json:"id"`
	RemoteAddr string `json:"remoteAddr"`
	Created    string `json:"created"`

	OutboundBytes *uint64 `json:"outboundBytes"`
	BytesSent     *uint64 `json:"bytesSent"`

	MsRTT               *float64 `json:"msRTT"`
	PacketsSendLossRate *float64 `json:"packetsSendLossRate"`

	PacketsSent        *uint64 `json:"packetsSent"`
	OutboundRTPPackets *uint64 `json:"outboundRTPPackets"`

	PacketsSendLoss                *uint64 `json:"packetsSendLoss"`
	OutboundRTPPacketsReportedLost *uint64 `json:"outboundRTPPacketsReportedLost"`

	PacketsSendDrop             *uint64 `json:"packetsSendDrop"`
	OutboundRTPPacketsDiscarded *uint64 `json:"outboundRTPPacketsDiscarded"`

	OutboundFramesDiscarded *uint64 `json:"outboundFramesDiscarded"`
}

// apiConnList is the envelope every per-protocol list answers with.
type apiConnList struct {
	Items []apiConn `json:"items"`
}

// fetchConnLists reads every per-protocol list the given readers are described by, once each, keyed
// by list segment and then by connection id.
//
// Once each and not once per path, the lists being relay-wide: two paths served over SRT are two
// slices of one answer, and a fetch per path would ask twice and get two moments.
// A protocol no reader is on is not asked about, so a relay with one SRT viewer costs one extra
// call and a relay with none costs zero.
//
// Nothing here can make a reachable relay unreachable.
// A list that refuses, times out or answers undecodably is left out and its readers stay named and
// unmeasured.
// A protocol whose listener is switched off has no list at all: a relay running without MoQ answers
// 404 for moqsessions, a fact about its configuration and not its health.
func fetchConnLists(httpClient *http.Client, host string, apiPort int, named []apiReader) map[string]map[string]apiConn {
	assert.IsNotNil(httpClient, "a roster is joined over the client the snapshot was read with")
	assert.Assert(apiPort > 0, "apiPort comes from validated settings", apiPort)

	lists := map[string]map[string]apiConn{}
	for _, named := range named {
		segment := readerKinds[named.Type].list
		if segment == "" {
			continue
		}
		if _, asked := lists[segment]; asked {
			continue
		}

		lists[segment] = fetchConns(httpClient, host, apiPort, segment)
	}

	return lists
}

// joinReaders turns the readers a path named into rows with figures on them, against lists already
// fetched.
// The whole interpretation this package does to a reader, and a pure function of the two answers so
// a test can state both.
func joinReaders(lists map[string]map[string]apiConn, named []apiReader) []Reader {
	readers := make([]Reader, 0, len(named))
	for _, named := range named {
		kind := readerKinds[named.Type]

		reader := Reader{Type: named.Type, ID: named.ID, Transport: kind.transport}
		if reader.Transport == "" {
			reader.Transport = named.Type
		}

		if conn, joined := lists[kind.list][named.ID]; joined {
			reader.RemoteAddr = conn.RemoteAddr
			reader.Joined = conn.Created
			reader.BytesSent = firstPresent(conn.OutboundBytes, conn.BytesSent)
			reader.RttMs = conn.MsRTT
			reader.LossPercent = conn.PacketsSendLossRate
			reader.PacketsSent = firstPresent(conn.PacketsSent, conn.OutboundRTPPackets)
			reader.PacketsLost = firstPresent(conn.PacketsSendLoss, conn.OutboundRTPPacketsReportedLost)
			reader.PacketsDropped = firstPresent(conn.PacketsSendDrop, conn.OutboundRTPPacketsDiscarded)
			reader.FramesDiscarded = conn.OutboundFramesDiscarded
		}

		readers = append(readers, reader)
	}

	assert.Assert(len(readers) == len(named), "a row per reader the path named", len(readers), len(named))
	return readers
}

// fetchConns reads one per-protocol list and keys it by connection id, answering nil for a list
// that could not be read.
// Nil rather than an error, every caller's response to a missing list being to leave the figures
// absent.
func fetchConns(httpClient *http.Client, host string, apiPort int, segment string) map[string]apiConn {
	assert.IsNotNil(httpClient, "a list is fetched over a client")
	assert.Assert(segment != "", "a list is fetched from a named endpoint")

	url := fmt.Sprintf("http://%s:%d/v3/%s/list", host, apiPort, segment)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var list apiConnList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil
	}

	conns := make(map[string]apiConn, len(list.Items))
	for _, item := range list.Items {
		conns[item.ID] = item
	}

	return conns
}

// firstPresent answers the first spelling of one figure the list carried, nil where it carried
// none.
func firstPresent[T any](values ...*T) *T {
	for _, value := range values {
		if value != nil {
			return value
		}
	}

	return nil
}
