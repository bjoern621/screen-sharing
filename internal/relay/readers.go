package relay

import (
	"encoding/json"
	"fmt"
	"net/http"

	"bjoernblessin.de/go-utils/util/assert"
)

// Reader is one connection the relay is serving a path to.
//
// The path list names a reader by type and id and says nothing else about it;
// every figure below comes from the per-protocol list that type is served by, joined on that id.
// Which is why a figure is a pointer: a protocol that does not report one leaves it absent,
// and absent is not a measured zero.
// An SRT reader is the only one the relay measures a round trip and a loss rate for;
// the rest report what was sent to them and little more (see readerKinds).
type Reader struct {
	// Type is the relay's own token for what this reader is, such as srtConn.
	// It is kept as the relay wrote it, because it is the join key's other half and a reader of a
	// snapshot should be able to find the connection it came from.
	Type string `json:"type"`
	ID   string `json:"id"`
	// Transport is Type in this app's transport vocabulary, srt, rtsp, rtmp, webrtc, hls or moq,
	// which is what a viewer's leg is named everywhere else in this codebase.
	// A type this build has no row for keeps the relay's own token, so an unknown reader reads as
	// whatever the relay called it rather than as nothing.
	// A reader the relay named nothing at all leaves this empty, the same "unknown" an unrecognised
	// track format leaves on the path beside it, and consumers read it the same way.
	Transport string `json:"transport"`

	// RemoteAddr is host:port as the relay saw it, and empty where the join found no connection:
	// a reader that ended between the two calls, or one on a protocol whose list this relay does not
	// serve.
	RemoteAddr string `json:"remoteAddr,omitempty"`
	// Joined is when the relay accepted this reader, in the relay's own RFC 3339 spelling and its own
	// clock.
	// It is passed through unparsed: the relay's clock is the only one that can date its connections,
	// and reformatting it here would put a second opinion about the time on the wire.
	Joined string `json:"joined,omitempty"`

	BytesSent *uint64 `json:"bytesSent,omitempty"`
	// RttMs is the smoothed round trip to this reader.
	// SRT alone reports one.
	RttMs *float64 `json:"rttMs,omitempty"`
	// LossPercent is SRT's own send-side loss rate, which it defines as the percentage of resent data
	// against sent data.
	// It is the relay's figure and not one computed here from two counters, because a rate computed
	// from lifetime counters would be a run's average wearing the look of a current reading.
	LossPercent *float64 `json:"lossPercent,omitempty"`

	PacketsSent *uint64 `json:"packetsSent,omitempty"`
	// PacketsLost were lost on the way to this reader, as the sender counted them or as the receiver
	// reported them back.
	// Cumulative over the connection.
	PacketsLost *uint64 `json:"packetsLost,omitempty"`
	// PacketsDropped were given up on by the sender rather than lost in transit.
	PacketsDropped *uint64 `json:"packetsDropped,omitempty"`
	// FramesDiscarded were dropped by the relay itself because this reader's outgoing queue was full.
	// It is the one congestion signal the protocols with no round trip and no loss rate still give,
	// and it is a fact about the relay rather than about the line, which is why it is counted apart
	// from the two above.
	FramesDiscarded *uint64 `json:"framesDiscarded,omitempty"`
}

// readerKind states, for one kind of reader the relay can report, which per-protocol list describes
// it and what this app calls the leg it is watching over.
//
// It is the same move trackFormats makes for codec names: the relay names a thing in its own
// vocabulary, one table converts it to this app's, and every consumer reads the table instead of
// restating the rule.
// The list segment is the other half: the path list gives a type and an id and nothing else,
// so the type is the only thing that can say where the figures for that id live.
//
// Which figures each list actually carries, verified against a MediaMTX v1.20.0 relay and its
// OpenAPI document rather than assumed:
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
// Nothing enforces that table beyond the field names in apiConn: a list that carries no field for a
// figure leaves it absent, which is what the whole shape is for.
// A reader type this build has no row for is not an error either.
// A newer relay may serve a protocol this one has never heard of, and the honest snapshot of that
// is a reader named and unmeasured rather than a reader dropped.
type readerKind struct {
	list      string
	transport string
}

var readerKinds = map[string]readerKind{
	"srtConn":       {list: "srtconns", transport: "srt"},
	"rtspConn":      {list: "rtspconns", transport: "rtsp"},
	"rtspSession":   {list: "rtspsessions", transport: "rtsp"},
	"rtmpConn":      {list: "rtmpconns", transport: "rtmp"},
	"webRTCSession": {list: "webrtcsessions", transport: "webrtc"},
	"hlsSession":    {list: "hlssessions", transport: "hls"},
	"moqSession":    {list: "moqsessions", transport: "moq"},

	// The TLS variants are the same protocol to a viewer, and this app's transport vocabulary has one
	// name for each.
	// What the relay was reached over is a fact about the relay's listeners rather than about the leg,
	// and no consumer here asks it.
	"rtspsConn":    {list: "rtspsconns", transport: "rtsp"},
	"rtspsSession": {list: "rtspssessions", transport: "rtsp"},
	"rtmpsConn":    {list: "rtmpsconns", transport: "rtmp"},

	// A reader the relay declines to describe.
	// It is listed rather than left out so that the one type with deliberately no list is visible
	// beside the ones that have one; the empty segment takes the same path an unknown type takes.
	"hidden": {list: "", transport: ""},
}

// apiConn is one item of a per-protocol connection or session list, decoded into the union of the
// fields this app reads across all of them.
//
// One struct rather than one per endpoint, because absence then falls out of decoding:
// a list that carries no msRTT leaves MsRTT nil without anything here having to know which lists
// those are.
// That also fixes the direction a mistake fails in.
// A field name that is wrong decodes to nil and the figure reads as unmeasured,
// where a name that is right for the wrong endpoint would decode to a zero and read as a
// measurement, and a measured zero and an absence are the two things this snapshot may never
// confuse.
//
// The pairs below are two spellings of one figure, never two figures.
// Every list states the bytes it sent as outboundBytes and repeats it as the deprecated bytesSent,
// except the SRT list, which carries bytesSent alone; and the packet counters are SRT's sender-side
// names beside the RTSP session's RTCP-reported ones.
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

// apiConnList mirrors the envelope every per-protocol list answers with.
type apiConnList struct {
	Items []apiConn `json:"items"`
}

// fetchConnLists reads every per-protocol list the given readers are described by, once each,
// keyed by list segment and then by connection id.
//
// Once each and not once per path, because the lists are relay-wide: two paths served over SRT are
// two slices of one answer, and a fetch per path would ask the same question twice and get two
// moments.
// A protocol no reader is on is not asked about at all, so a relay with one SRT viewer costs one
// extra call and a relay with none costs zero.
//
// Nothing here can make a reachable relay unreachable.
// A list that refuses, times out or answers with something undecodable is left out,
// its readers stay named and unmeasured, and that is the honest reading rather than a failure.
// The relay answered the question this snapshot is about, and a protocol whose listener is switched
// off has no list at all: a relay running without MoQ answers 404 for moqsessions,
// which is a fact about its configuration and not about its health.
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

// joinReaders turns the readers a path named into rows with figures on them,
// against lists already fetched.
// It is the whole of the interpretation this package does to a reader, and it is a pure function of
// the two answers so that a test can state both.
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

// fetchConns reads one per-protocol list and keys it by connection id, and answers nil for a list
// that could not be read.
// Nil rather than an error: every caller's response to a list it did not get is to leave the
// figures absent, so there is one thing to do with the answer and no branch that could do something
// else with it.
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

// firstPresent answers with the first of two spellings of one figure that the list carried,
// and nil where it carried neither.
func firstPresent[T any](values ...*T) *T {
	for _, value := range values {
		if value != nil {
			return value
		}
	}

	return nil
}
