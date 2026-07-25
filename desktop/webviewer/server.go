// Package webviewer serves the browser viewer path for codecs WebRTC cannot
// negotiate, VP9 4:4:4 above all. It subscribes to the relay over RTSP through
// an ffmpeg child, remuxes to IVF, and pushes each encoded frame to the page
// over a WebSocket; the page decodes with WebCodecs and draws to a canvas.
//
// The service runs in-process, supervised by the app, so it starts and stops
// with the window. The web grid connects to it on localhost; other machines
// on the LAN reach the standalone page it serves.
package webviewer

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"bjoernblessin.de/go-utils/util/logger"
	"bjoernblessin.de/screenshare/ffmpeg"
)

// DefaultPort is the fixed listen port of the viewer service, mirrored by the
// frontend's WebCodecs sink. Fixed like the relay's own ports rather than a
// setting, so no configuration is needed to reach it.
const DefaultPort = 8899

// Config supplies the listen port and a lookup for the current relay location,
// read per connection so a settings change takes effect on the next viewer.
type Config struct {
	Port  int
	Relay func() (host string, rtspPort int)
}

// Server is the viewer HTTP/WebSocket service.
type Server struct {
	cfg     Config
	httpSrv *http.Server
}

func New(cfg Config) *Server {
	if cfg.Port == 0 {
		cfg.Port = DefaultPort
	}
	return &Server{cfg: cfg}
}

var upgrader = websocket.Upgrader{
	// The viewer page may be served from another origin (an external browser) or
	// the Wails window; frames carry no credentials, so any origin may subscribe.
	CheckOrigin: func(*http.Request) bool { return true },
}

// Start binds the listener and serves in the background. It returns an error
// only if the port cannot be bound; serving errors are logged.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/", s.handleWS)
	mux.HandleFunc("/", s.handlePage)

	addr := fmt.Sprintf(":%d", s.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("viewer service listen on %s: %w", addr, err)
	}
	s.httpSrv = &http.Server{Handler: mux}
	go func() {
		if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logger.Warnf("viewer service stopped: %v", err)
		}
	}()
	logger.Infof("viewer service on %s", addr)
	return nil
}

// Stop shuts the service down, ending any open viewer connections.
func (s *Server) Stop() {
	if s.httpSrv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.httpSrv.Shutdown(ctx)
}

// handleWS upgrades the connection and streams one stream's encoded frames until
// the client leaves or the source ends. One ffmpeg child serves one viewer; a
// second viewer of the same stream starts its own, which the relay allows.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/ws/")
	if name == "" {
		http.Error(w, "stream name required", http.StatusBadRequest)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// A client sends nothing; reading only detects its disconnect, which cancels
	// the context and kills the ffmpeg child.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()

	host, rtspPort := s.cfg.Relay()
	if err := s.stream(ctx, conn, host, rtspPort, name); err != nil {
		logger.Infof("viewer stream %q ended: %v", name, err)
	}
}

// stream runs ffmpeg to pull the named stream from the relay over RTSP, remux it
// to IVF, and forward each VP9 frame to the WebSocket.
func (s *Server) stream(ctx context.Context, conn *websocket.Conn, host string, rtspPort int, name string) error {
	exe, err := ffmpeg.FindExe("ffmpeg")
	if err != nil {
		return err
	}
	url := fmt.Sprintf("rtsp://%s:%d/%s", host, rtspPort, name)
	cmd := exec.CommandContext(ctx, exe,
		"-hide_banner", "-loglevel", "error",
		"-rtsp_transport", "tcp",
		"-i", url,
		"-map", "0:v:0",
		"-c:v", "copy",
		"-f", "ivf", "pipe:1",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	defer func() { _ = cmd.Wait() }()

	return readIVF(stdout, func(payload []byte, ptsUs uint64, keyframe bool) error {
		return writeFrame(conn, payload, ptsUs, keyframe)
	})
}

// writeFrame serializes one encoded frame per the WebCodecs sink's contract: a
// 1-byte flag (bit 0 = keyframe), an 8-byte big-endian PTS in microseconds, then
// the VP9 payload.
func writeFrame(conn *websocket.Conn, payload []byte, ptsUs uint64, keyframe bool) error {
	msg := make([]byte, 9+len(payload))
	if keyframe {
		msg[0] = 0x01
	}
	binary.BigEndian.PutUint64(msg[1:9], ptsUs)
	copy(msg[9:], payload)
	return conn.WriteMessage(websocket.BinaryMessage, msg)
}

// handlePage serves the standalone viewer page for external browsers. The page
// derives the stream name from its own path and opens the matching WebSocket.
func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(viewerPage))
}
