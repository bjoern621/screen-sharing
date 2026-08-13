package relay

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

// Which protocols can carry a stream follows its bitstream format, and the relay names a track
// after the format rather than the encoder.
// A track name with no entry leaves the format empty, which the callers read as "unknown" and act
// on by not refusing anything: a snapshot older than the stream must not block a viewer that would
// have worked.
func TestFormatOfTracks(t *testing.T) {
	cases := []struct {
		tracks []string
		want   string
	}{
		{tracks: []string{"H264"}, want: "h264"},
		{tracks: []string{"AVC"}, want: "h264"},
		{tracks: []string{"H265"}, want: "hevc"},
		{tracks: []string{"HEVC"}, want: "hevc"},
		{tracks: []string{"VP9"}, want: "vp9"},
		{tracks: []string{"AV1"}, want: "av1"},
		{tracks: []string{"VP8"}, want: "vp8"},
		// The audio track of a muxed stream is not the format the transport question is about,
		// so the video one answers wherever it sits.
		{tracks: []string{"Opus", "AV1"}, want: "av1"},
		{tracks: []string{"h264"}, want: "h264"},
		{tracks: []string{"Opus"}, want: ""},
		{tracks: nil, want: ""},
	}
	for _, c := range cases {
		if got := formatOfTracks(c.tracks); got != c.want {
			t.Errorf("formatOfTracks(%v) = %q, want %q", c.tracks, got, c.want)
		}
	}
}

// The fixtures below are a MediaMTX v1.20.0 relay's own answers, trimmed to the items under test
// and otherwise left in the spelling it wrote them in.
// They were taken off a running relay - one ffmpeg publishing over SRT and three reading the same
// path over SRT, RTSP and RTMP - rather than composed from the field names this package hopes for,
// because a fixture written to match the decoder proves only that the decoder matches itself.
const (
	pathsWithThreeReaders = `{"itemCount":1,"pageCount":1,"items":[{
		"name":"teststream","confName":"all_others","ready":true,
		"source":{"type":"srtConn","id":"18404ade-9c49-46fa-818b-e822a27de51a"},
		"tracks":["H264"],
		"readers":[
			{"type":"rtmpConn","id":"ea726a88-471a-4806-8fdc-9844c8f7074b"},
			{"type":"rtspSession","id":"b07b1063-6bd3-4b1d-9a6a-e44f3159e077"},
			{"type":"srtConn","id":"8905cef7-0e3b-4452-b0fa-7fcb6f71201d"}],
		"inboundBytes":581390,"outboundBytes":995983,"bytesReceived":581390,"bytesSent":995983}]}`

	// Two items, and only the second is a reader: the first is the publisher, which the same list
	// reports.
	// Nothing filters on state here - the path named which ids are readers, so the publisher is left
	// out by not being asked for rather than by a second rule that could disagree with the first.
	srtConns = `{"itemCount":2,"pageCount":1,"items":[
		{"id":"18404ade-9c49-46fa-818b-e822a27de51a","created":"2026-08-09T22:01:04.141901+02:00",
		 "remoteAddr":"127.0.0.1:52152","state":"publish","path":"teststream",
		 "packetsSent":0,"packetsSendLoss":0,"packetsSendDrop":0,"bytesSent":0,
		 "msRTT":4.0153188907292235e-77,"packetsSendLossRate":0,"outboundFramesDiscarded":0},
		{"id":"8905cef7-0e3b-4452-b0fa-7fcb6f71201d","created":"2026-08-09T22:01:12.4445876+02:00",
		 "remoteAddr":"127.0.0.1:52157","state":"read","path":"teststream",
		 "packetsSent":385,"packetsSendLoss":2,"packetsSendDrop":1,"bytesSent":373004,
		 "msRTT":14.5,"packetsSendLossRate":0.5,"outboundFramesDiscarded":3}]}`

	rtspSessions = `{"itemCount":1,"pageCount":1,"items":[
		{"id":"b07b1063-6bd3-4b1d-9a6a-e44f3159e077","created":"2026-08-09T22:01:13.8614943+02:00",
		 "remoteAddr":"127.0.0.1:55371","state":"read","path":"teststream","transport":"TCP",
		 "outboundBytes":491878,"outboundRTPPackets":647,"outboundRTPPacketsReportedLost":4,
		 "outboundRTPPacketsDiscarded":0,"bytesSent":491878,"rtpPacketsSent":647}]}`

	rtmpConns = `{"itemCount":1,"pageCount":1,"items":[
		{"id":"ea726a88-471a-4806-8fdc-9844c8f7074b","created":"2026-08-09T22:01:15.5006717+02:00",
		 "remoteAddr":"127.0.0.1:55372","state":"read","path":"teststream","userAgent":"LNX 9,0,124,2",
		 "inboundBytes":3428,"outboundBytes":358672,"outboundFramesDiscarded":0,
		 "bytesReceived":3428,"bytesSent":358672}]}`
)

// relayServing answers the given endpoints and 404s every other one, which is what a real relay
// does for a protocol whose listener is switched off.
// It hands back the host and port to point a client at.
func relayServing(t *testing.T, answers map[string]string) (string, int) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, served := answers[r.URL.Path]
		if !served {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	address, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("the test server named an address that does not parse: %v", err)
	}

	port, err := strconv.Atoi(address.Port())
	if err != nil {
		t.Fatalf("the test server named a port that is not a number: %v", err)
	}

	return address.Hostname(), port
}

func readerByType(t *testing.T, path Path, kind string) Reader {
	t.Helper()

	for _, reader := range path.Roster {
		if reader.Type == kind {
			return reader
		}
	}

	t.Fatalf("the roster names no %s reader: %+v", kind, path.Roster)
	return Reader{}
}

// A path's readers are named by the path list and measured by the per-protocol list each one's type
// points at.
// What each protocol reports differs, and the roster says which figures those are by carrying the
// ones it was told and leaving the rest absent.
func TestFetchJoinsEachReaderToItsProtocolsList(t *testing.T) {
	host, port := relayServing(t, map[string]string{
		"/v3/paths/list":        pathsWithThreeReaders,
		"/v3/srtconns/list":     srtConns,
		"/v3/rtspsessions/list": rtspSessions,
		"/v3/rtmpconns/list":    rtmpConns,
	})

	status := New().Fetch(host, port)

	if !status.Reachable {
		t.Fatalf("a relay that answered is reachable: %+v", status)
	}
	if len(status.Paths) != 1 {
		t.Fatalf("one path was reported, got %d", len(status.Paths))
	}

	path := status.Paths[0]
	if path.Readers != 3 || len(path.Roster) != 3 {
		t.Fatalf("three readers were reported, got count %d and roster %d", path.Readers, len(path.Roster))
	}

	// SRT is the one leg the relay times and states a loss rate on.
	srt := readerByType(t, path, "srtConn")
	if srt.Transport != "srt" {
		t.Errorf("an srtConn watches over srt, got %q", srt.Transport)
	}
	if srt.RemoteAddr != "127.0.0.1:52157" {
		t.Errorf("the roster carries the address the relay saw, got %q", srt.RemoteAddr)
	}
	if srt.Joined != "2026-08-09T22:01:12.4445876+02:00" {
		t.Errorf("the roster carries the relay's own join time, got %q", srt.Joined)
	}
	if srt.RttMs == nil || *srt.RttMs != 14.5 {
		t.Errorf("an SRT reader reports a round trip, got %v", srt.RttMs)
	}
	if srt.LossPercent == nil || *srt.LossPercent != 0.5 {
		t.Errorf("an SRT reader reports a loss rate, got %v", srt.LossPercent)
	}
	if srt.BytesSent == nil || *srt.BytesSent != 373004 {
		t.Errorf("an SRT list states its bytes as bytesSent alone, got %v", srt.BytesSent)
	}
	if srt.PacketsLost == nil || *srt.PacketsLost != 2 {
		t.Errorf("an SRT reader counts the packets lost to it, got %v", srt.PacketsLost)
	}
	if srt.PacketsDropped == nil || *srt.PacketsDropped != 1 {
		t.Errorf("an SRT reader counts the packets given up on, got %v", srt.PacketsDropped)
	}
	if srt.FramesDiscarded == nil || *srt.FramesDiscarded != 3 {
		t.Errorf("an SRT reader counts what the relay's queue discarded, got %v", srt.FramesDiscarded)
	}

	// RTSP reports what the receiver said came up missing, and no round trip at all.
	rtsp := readerByType(t, path, "rtspSession")
	if rtsp.Transport != "rtsp" {
		t.Errorf("an rtspSession watches over rtsp, got %q", rtsp.Transport)
	}
	if rtsp.RttMs != nil || rtsp.LossPercent != nil {
		t.Errorf("nothing times an RTSP reader, got rtt %v and loss %v", rtsp.RttMs, rtsp.LossPercent)
	}
	if rtsp.BytesSent == nil || *rtsp.BytesSent != 491878 {
		t.Errorf("an RTSP session states its bytes as outboundBytes, got %v", rtsp.BytesSent)
	}
	if rtsp.PacketsSent == nil || *rtsp.PacketsSent != 647 {
		t.Errorf("an RTSP session counts the packets it sent, got %v", rtsp.PacketsSent)
	}
	if rtsp.PacketsLost == nil || *rtsp.PacketsLost != 4 {
		t.Errorf("an RTSP session carries what the receiver reported lost, got %v", rtsp.PacketsLost)
	}

	// RTMP reports bytes and the relay's own discards, and nothing about the line.
	rtmp := readerByType(t, path, "rtmpConn")
	if rtmp.Transport != "rtmp" {
		t.Errorf("an rtmpConn watches over rtmp, got %q", rtmp.Transport)
	}
	if rtmp.BytesSent == nil || *rtmp.BytesSent != 358672 {
		t.Errorf("an RTMP connection states its bytes, got %v", rtmp.BytesSent)
	}
	if rtmp.RttMs != nil || rtmp.LossPercent != nil || rtmp.PacketsSent != nil || rtmp.PacketsLost != nil {
		t.Errorf("an RTMP connection is measured by nothing but its bytes: %+v", rtmp)
	}
	if rtmp.FramesDiscarded == nil || *rtmp.FramesDiscarded != 0 {
		t.Errorf("a discard count of zero is a measurement and not an absence, got %v", rtmp.FramesDiscarded)
	}
}

// A per-protocol list that cannot be read leaves its readers named and unmeasured.
// It does not fail the snapshot: the relay answered the question the snapshot is about,
// and a protocol whose listener is off has no list at all - which is exactly the 404 below.
func TestAListThatRefusesLeavesItsReadersUnmeasured(t *testing.T) {
	host, port := relayServing(t, map[string]string{
		"/v3/paths/list":        pathsWithThreeReaders,
		"/v3/rtspsessions/list": rtspSessions,
		// No srtconns and no rtmpconns: both 404.
	})

	status := New().Fetch(host, port)

	if !status.Reachable || status.Error != "" {
		t.Fatalf("a list that 404s does not make a relay unreachable: %+v", status)
	}

	path := status.Paths[0]
	if path.Readers != 3 || len(path.Roster) != 3 {
		t.Fatalf("every reader the path named is still on the roster, got %d of %d", len(path.Roster), path.Readers)
	}

	srt := readerByType(t, path, "srtConn")
	if srt.Transport != "srt" || srt.ID != "8905cef7-0e3b-4452-b0fa-7fcb6f71201d" {
		t.Errorf("a reader with no list is still named by the path list: %+v", srt)
	}
	if srt.RemoteAddr != "" || srt.Joined != "" || srt.RttMs != nil || srt.BytesSent != nil {
		t.Errorf("a reader whose list did not answer carries no figures: %+v", srt)
	}

	// The one list that did answer is unaffected by the two that did not.
	rtsp := readerByType(t, path, "rtspSession")
	if rtsp.BytesSent == nil || *rtsp.BytesSent != 491878 {
		t.Errorf("a list that answered still measures its own readers, got %v", rtsp.BytesSent)
	}
}

// A reader on a protocol this build has no row for is named and left unmeasured rather than
// dropped: a newer relay may serve a leg this one has never heard of, and a roster that hid it
// would disagree with the count beside it.
func TestAnUnknownReaderKeepsTheRelaysOwnWords(t *testing.T) {
	host, port := relayServing(t, map[string]string{
		"/v3/paths/list": `{"itemCount":1,"pageCount":1,"items":[{
			"name":"teststream","ready":true,"tracks":["H264"],
			"readers":[{"type":"hidden","id":"1"},{"type":"quicheConn","id":"2"}],
			"bytesReceived":10}]}`,
	})

	status := New().Fetch(host, port)

	path := status.Paths[0]
	if path.Readers != 2 || len(path.Roster) != 2 {
		t.Fatalf("a reader on an unknown leg is counted and named, got %d of %d", len(path.Roster), path.Readers)
	}
	for _, reader := range path.Roster {
		if reader.Transport != reader.Type {
			t.Errorf("a leg with no row keeps the relay's own token, got %q for %q", reader.Transport, reader.Type)
		}
		if reader.BytesSent != nil || reader.RttMs != nil || reader.LossPercent != nil {
			t.Errorf("a reader nothing describes carries no figures: %+v", reader)
		}
	}
}

// A path nobody is watching has an empty roster and a zero count, and asks no per-protocol list
// anything.
func TestAPathWithNoReadersAsksNoList(t *testing.T) {
	asked := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		if r.URL.Path != "/v3/paths/list" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"itemCount":1,"pageCount":1,"items":[
			{"name":"teststream","ready":true,"tracks":["H264"],"readers":[],"bytesReceived":10}]}`))
	}))
	defer server.Close()

	address, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(address.Port())

	status := New().Fetch(address.Hostname(), port)

	if len(status.Paths) != 1 || status.Paths[0].Readers != 0 || len(status.Paths[0].Roster) != 0 {
		t.Fatalf("a path nobody watches has an empty roster: %+v", status.Paths)
	}
	if asked != 1 {
		t.Errorf("a snapshot with no readers costs one call, got %d", asked)
	}
}
