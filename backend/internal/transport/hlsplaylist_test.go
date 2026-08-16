package transport

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"bjoernblessin.de/screenshare/internal/settings"
)

// masterWithAudio is what the relay answers for a stream carrying sound: the audio rendition is a
// tag with its URI inside, and the video one is the line under EXT-X-STREAM-INF.
const masterWithAudio = `#EXTM3U
#EXT-X-VERSION:10
#EXT-X-INDEPENDENT-SEGMENTS

#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="audio2",AUTOSELECT=YES,DEFAULT=YES,URI="audio2_stream.m3u8?session=s1"

#EXT-X-STREAM-INF:BANDWIDTH=124432,CODECS="avc1.42c01e,opus",RESOLUTION=640x360,AUDIO="audio"
video1_stream.m3u8?session=s1
`

// gapPlaylist is a muxer that has started and cut nothing: placeholders in place of every segment.
const gapPlaylist = `#EXTM3U
#EXT-X-VERSION:10
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:1
#EXT-X-MAP:URI="init.mp4?session=s1"
#EXT-X-GAP
#EXTINF:1.00000,
gap.mp4
#EXT-X-GAP
#EXTINF:1.00000,
gap.mp4
`

// segmentPlaylist is the same playlist once a segment exists.
const segmentPlaylist = `#EXTM3U
#EXT-X-VERSION:10
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:90
#EXT-X-MAP:URI="init.mp4?session=s1"
#EXTINF:1.00000,
video1_seg90.mp4?session=s1
`

// The audio rendition is the one hlsdemux2 stalls on, so what a receive source opens is the video
// media playlist and never the master that names both.
func TestHLSVariantURIIsTheVideoRendition(t *testing.T) {
	got, ok := hlsVariantURI(masterWithAudio)
	if !ok {
		t.Fatal("hlsVariantURI found no rendition in a master playlist that names one")
	}
	if want := "video1_stream.m3u8?session=s1"; got != want {
		t.Errorf("hlsVariantURI = %q, want %q", got, want)
	}
}

func TestHLSVariantURIRefusesAMasterWithNoStream(t *testing.T) {
	if _, ok := hlsVariantURI("#EXTM3U\n#EXT-X-VERSION:10\n"); ok {
		t.Error("hlsVariantURI found a rendition in a playlist that names none")
	}
}

// A gap is a placeholder a client is not to download, and a playlist made of them is a muxer with
// nothing to serve yet.
func TestHLSHasSegmentReadsPastGaps(t *testing.T) {
	if hlsHasSegment(gapPlaylist) {
		t.Error("hlsHasSegment took a gap for a segment")
	}
	if !hlsHasSegment(segmentPlaylist) {
		t.Error("hlsHasSegment missed the segment in a playlist that carries one")
	}
	// A gap applies to the URI under it and to no later one.
	if !hlsHasSegment(gapPlaylist + "#EXTINF:1.00000,\nvideo1_seg91.mp4\n") {
		t.Error("hlsHasSegment missed a segment that follows a gap")
	}
}

// hlsRelay answers the two playlists a resolve reads, counting the reads of the media one and
// serving gaps until gapsUntil of them have gone by.
func hlsRelay(t *testing.T, gapsUntil int32) (*httptest.Server, *atomic.Int32, *atomic.Value) {
	t.Helper()

	var reads atomic.Int32
	var authorization atomic.Value
	authorization.Store("")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		if strings.HasSuffix(r.URL.Path, "/index.m3u8") {
			_, _ = w.Write([]byte(masterWithAudio))
			return
		}
		if reads.Add(1) <= gapsUntil {
			_, _ = w.Write([]byte(gapPlaylist))
			return
		}
		_, _ = w.Write([]byte(segmentPlaylist))
	}))
	t.Cleanup(server.Close)
	return server, &reads, &authorization
}

// relayAt points settings at a test server, the address being built from host and port.
func relayAt(t *testing.T, server *httptest.Server) settings.Settings {
	t.Helper()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("test server address: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("test server port: %v", err)
	}
	return settings.Settings{Relay: settings.Relay{Host: u.Hostname(), HlsPort: port}}
}

// The source is the video rendition the master named, session and all: that address is what the
// relay answers, and the one built from settings alone is answered 401.
func TestHLSResolveGstSourceOpensTheRendition(t *testing.T) {
	server, _, _ := hlsRelay(t, 0)

	source, err := HLS{}.ResolveGstSource(relayAt(t, server), "bob")
	if err != nil {
		t.Fatalf("ResolveGstSource: %v", err)
	}
	if len(source) != 2 || source[0] != "urisourcebin" {
		t.Fatalf("ResolveGstSource = %v, want urisourcebin and a uri", source)
	}
	if want := "uri=" + server.URL + "/bob/video1_stream.m3u8?session=s1"; source[1] != want {
		t.Errorf("ResolveGstSource uri = %q, want %q", source[1], want)
	}
}

// The wait is the whole reason the resolve is a read rather than a written address: a playlist of
// gaps is one hlsdemux2 fails the pipeline on.
func TestHLSResolveWaitsForASegment(t *testing.T) {
	server, reads, _ := hlsRelay(t, 1)

	if _, err := (HLS{}).ResolveGstSource(relayAt(t, server), "bob"); err != nil {
		t.Fatalf("ResolveGstSource: %v", err)
	}
	if got := reads.Load(); got < 2 {
		t.Errorf("the media playlist was read %d times, want the gap one read again", got)
	}
}

// Two forms of one token: the header the relay's HTTP servers read, and the userinfo souphttpsrc
// builds a Basic pair out of, a launch line setting no header.
func TestHLSResolveCarriesTheCredentialBothWays(t *testing.T) {
	server, _, authorization := hlsRelay(t, 0)

	s := relayAt(t, server)
	s.Relay.Token = "tok-1"

	source, err := HLS{}.ResolveGstSource(s, "bob")
	if err != nil {
		t.Fatalf("ResolveGstSource: %v", err)
	}
	if got := authorization.Load().(string); got != "Bearer tok-1" {
		t.Errorf("the playlist was read with %q, want a bearer token", got)
	}
	if !strings.Contains(source[1], "jwt:tok-1@") {
		t.Errorf("ResolveGstSource uri = %q, want the token as userinfo", source[1])
	}
}

// A relay that refuses the read is an Umgebungsfehler, and what comes back names the address rather
// than a fragment nothing can open.
func TestHLSResolveRefusesWhatTheRelayRefuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	source, err := HLS{}.ResolveGstSource(relayAt(t, server), "bob")
	if err == nil {
		t.Fatalf("ResolveGstSource = %v, want a refusal", source)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("ResolveGstSource error = %q, want the relay's answer in it", err)
	}
}

// ReceiveSource is the one call the receiving side makes, and it covers both ways a source is
// arrived at.
func TestReceiveSourceCoversBothWays(t *testing.T) {
	server, _, _ := hlsRelay(t, 0)

	s := relayAt(t, server)
	s.Relay.RtspPort = 8554
	s.Viewer.RtspWatchProtocol = "tcp"

	written, err := ReceiveSource("rtsp", s, "bob")
	if err != nil {
		t.Fatalf("ReceiveSource(rtsp): %v", err)
	}
	if written[0] != "rtspsrc" {
		t.Errorf("ReceiveSource(rtsp) = %v, want the fragment rtsp writes from settings", written)
	}

	resolved, err := ReceiveSource("hls", s, "bob")
	if err != nil {
		t.Fatalf("ReceiveSource(hls): %v", err)
	}
	if resolved[0] != "urisourcebin" {
		t.Errorf("ReceiveSource(hls) = %v, want the fragment hls resolves", resolved)
	}

	if _, err := ReceiveSource("moq", s, "bob"); err == nil {
		t.Error("ReceiveSource(moq) must refuse, no source element here subscribing MoQ tracks")
	}
}
