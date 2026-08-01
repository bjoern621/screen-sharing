// Package stats renders what a player reports as the tile's nerd-stats overlay:
// a monospace card of key/value rows, blocked by where each figure comes from.
//
// The rows are a table (blocks), which both the widget building and every refresh
// walk, so a row is described once: its key, whether it disappears while its
// figure is missing, and how it reads the poll. What the transport's own elements
// count is not in that table; the player reports those rows already labelled and
// the card grows a block per element.
package stats

import (
	"time"

	"bjoernblessin.de/screenshare-nativegrid/internal/player"
	"bjoernblessin.de/screenshare-nativegrid/internal/roster"
)

// View is one poll of a player's figures, plus the rates only two polls can give
// and the one figure the player cannot see.
type View struct {
	Stream roster.Stream
	Stats  player.Stats
	// FPS is the measured frame rate, "" until a second poll can measure it.
	FPS string
	// VideoRate and AudioRate are the measured bitrates, in Mbps.
	VideoRate, AudioRate float64
	// Renderer is the GSK renderer drawing the window, which is the last link in
	// the render path and the view's own knowledge: the pipeline hands over a
	// texture and has no say in what draws it. The tile fills it the way it reports
	// the size it draws at, and it is "" while the widget is not in a window.
	Renderer string
}

// Poller turns the monotonic counters a player reports into rates. The player
// reports counters only, so fps is measured off the frames that reached the
// surface rather than off the negotiated framerate, and bitrate off the bytes that
// reached the decoder.
type Poller struct {
	at         time.Time
	frames     uint64
	videoBytes uint64
	audioBytes uint64
}

// Reset drops the previous poll, so the next View reports no rates. The overlay
// resets when it opens: the counters kept running while it was closed, and the
// delta across that gap is not a rate anybody wants to read.
func (p *Poller) Reset() {
	*p = Poller{}
}

// Poll takes one sample and returns the view of it.
func (p *Poller) Poll(st roster.Stream, s player.Stats) View {
	v := View{Stream: st, Stats: s}

	now := time.Now()
	if !p.at.IsZero() {
		if dt := now.Sub(p.at).Seconds(); dt > 0 {
			if s.Frames > 0 {
				v.FPS = fpsText(float64(s.Frames-p.frames) / dt)
			}
			v.VideoRate = mbps(s.VideoBytes-p.videoBytes, dt)
			v.AudioRate = mbps(s.AudioBytes-p.audioBytes, dt)
		}
	}
	p.at, p.frames = now, s.Frames
	p.videoBytes, p.audioBytes = s.VideoBytes, s.AudioBytes
	return v
}
