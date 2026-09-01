// Package decode runs every receive pipeline in a child process.
// It holds both halves of the contract between that child and the backend.
//
// A GPU reset takes down whichever process was submitting to the ring.
// The kernel marks every context on the reset ring lost, innocent ones included, and Mesa aborts
// a context that did not ask for robustness, which no libva or GStreamer OpenGL context does.
// A decode in the backend therefore costs the control socket, the group membership and the publish
// supervision along with the picture.
// In the child it costs the decodes, and the backend reports each one ending the way it reports
// a pipeline that ended by itself (docs/viewer-architecture.md, "The decode host").
//
// One child for every decode rather than one per stream.
// A ring reset marks the innocent contexts lost too, so a child per stream would abort together
// and buy nothing for the OpenGL context and the VA display each of them holds.
//
// Both halves are the same executable, so the wire carries the receive package's own types.
// A second declaration of receive.Stats is one that drifts from it.
package decode

import (
	"fmt"

	"bjoernblessin.de/screenshare/internal/receive"
)

// Kind is which of the three things a decode is.
// The three share no identity: a tile names a stream and a leg, the publish preview names nothing,
// and a monitor preview names one of this machine's outputs.
type Kind uint8

const (
	KindStream Kind = iota + 1
	// KindPreview is the publish's own local preview, of which there is one.
	KindPreview
	KindMonitor
)

// ID addresses one decode on the host.
// Comparable, so one type keys the host's set and the backend's.
type ID struct {
	Kind Kind
	// Name and Transport are set on KindStream alone.
	Name      string
	Transport string
	// Monitor is the output index, set on KindMonitor alone.
	Monitor int
}

// StreamID names the decode of one stream on one watch leg.
func StreamID(name, transport string) ID {
	return ID{Kind: KindStream, Name: name, Transport: transport}
}

// PreviewID names the publish's local preview.
func PreviewID() ID { return ID{Kind: KindPreview} }

// MonitorID names the preview of one of this machine's outputs.
func MonitorID(monitor int) ID { return ID{Kind: KindMonitor, Monitor: monitor} }

// String is what a log line and an assert message carry.
func (id ID) String() string {
	switch id.Kind {
	case KindStream:
		return fmt.Sprintf("'%s' over %s", id.Name, id.Transport)
	case KindPreview:
		return "the local preview"
	case KindMonitor:
		return fmt.Sprintf("monitor %d", id.Monitor)
	default:
		return fmt.Sprintf("an unnamed decode (kind %d)", id.Kind)
	}
}

// State is everything a reader asks about one running decode, in one answer.
//
// One message for every figure rather than a call per field.
// The backend reads these together, on a ticker and while assembling the receive state,
// so a round trip per field would be dozens where one does.
//
// Ended is a decode whose pipeline stopped by itself, and it stays in the host's set carrying
// the reason until the backend stops it.
// A decode dropped the moment it ended would take the reason with it, and the reason is what
// the tile shows.
type State struct {
	Stats   receive.Stats
	ToneMap bool

	Volume   float64
	Muted    bool
	HasAudio bool

	PeakDB   float64
	RMSDB    float64
	HasLevel bool

	Ended      bool
	EndMessage string
}

// Events are one decode's lifecycle callbacks, fired on the backend's side from what the host
// reports.
//
// They arrive on a goroutine of the client's, so a callback touching the caller's own state guards
// it itself, as receive.Events does.
//
// Neither is the only report of what it carries.
// A snapshot carries the same two facts continuously, so a callback that never arrives costs
// promptness and never correctness.
// Both are therefore safe to deliver twice, which the app's own idempotency already covers.
type Events struct {
	// OnLive fires once the decode has produced a frame.
	OnLive func()
	// OnEnd fires when the pipeline ended by itself, with the reason a reader is shown.
	OnEnd func(message string)
}
