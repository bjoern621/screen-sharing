package wire

import (
	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/relay"
)

// The event constructors sit here rather than at each producer,
// so the oneof wrapper a payload belongs in is written once.
// A producer building the envelope itself would choose the event kind alongside the state,
// and a payload in the wrong wrapper is a state arriving under another name with nothing to say so.
//
// A state payload carries that state whole and never a delta,
// so a duplicate changes nothing and a dropped connection is recovered by reading state again
// rather than by replaying history.
//
// Every constructor leaves Sequence zero.
// Each subscription stamps its own numbers as it sends: a subscription that named kinds is not sent
// the ones it filtered out, and a number shared across subscribers would show a gap for every event
// a filter dropped.

// PublishStateEvent announces the publish state after any change, whoever made it.
// Carries the message GetPublishState answers with,
// so a window that has just mounted and one that has been open cannot be told different things.
func PublishStateEvent(p PublishSnapshot) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_PublishState{PublishState: PublishState(p)},
	}
}

// PublishStatsEvent announces one progress sample from the running encoder.
// The high-rate kind, at roughly one a second per running pipeline.
func PublishStatsEvent(s ffmpeg.Stats) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_PublishStats{PublishStats: PublishStats(s)},
	}
}

// PublishExitEvent announces that the publish pipeline ended, with why and where the log is.
// What the backend did about it is the publish state event that follows,
// so no retry is described here.
//
// cause is optional, for the reason stated at oneCause.
func PublishExitEvent(message, logPath string, cause ...*screensharev1.Text) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_PublishExit{PublishExit: ExitInfo(message, logPath, oneCause(cause))},
	}
}

// RelayStatusEvent announces a relay snapshot at the backend's poll interval.
// Pushed rather than polled per shell,
// so the byte-delta bitrates it carries are computed against one steady interval instead
// of whatever cadence each shell chose.
func RelayStatusEvent(s relay.Status) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_RelayStatus{RelayStatus: RelayStatus(s)},
	}
}

// ViewerStateEvent announces the open external viewers whenever one opens or closes.
//
// StartWatch and StopWatch answer with an empty message,
// and an effect whose result reaches no event is one only the calling shell learns the outcome of.
func ViewerStateEvent(refs []StreamRef) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_ViewerState{ViewerState: ViewerState(refs)},
	}
}

// ViewerExitEvent announces that one external viewer ended.
// The whole ref travels for the reason StreamRef exists:
// one stream can be watched over several transports at once,
// so the name alone would clear the wrong viewer.
func ViewerExitEvent(ref StreamRef, message, logPath string, cause ...*screensharev1.Text) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_ViewerExit{ViewerExit: &screensharev1.ViewerExit{
			Viewer: StreamRefMessage(ref),
			Exit:   ExitInfo(message, logPath, oneCause(cause)),
		}},
	}
}

// TestStreamStateEvent announces the synthetic set, for the reason ViewerStateEvent exists:
// StartTestStreams and StopTestStreams answer with an empty message,
// and one that died on its own moves the set with nothing having been called.
//
// The slots travel beside the count,
// so a set with one dead publisher says which slot rather than only that it got smaller.
func TestStreamStateEvent(running int, slots ...TestStreamSlot) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_TestStreamState{TestStreamState: TestStreamState(running, slots...)},
	}
}

func TestStreamExitEvent(message, logPath string, cause ...*screensharev1.Text) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_TestStreamExit{TestStreamExit: ExitInfo(message, logPath, oneCause(cause))},
	}
}

// CatalogEvent announces the whole reference set again, after the encoder probe has filled in.
//
// The catalog and not the probe result alone,
// a shell holding a catalog having nothing to merge a half-state into.
// Every shell learns what the probe found, not only the one that asked:
// a resolve on any other would otherwise start greying codecs with nothing having said why.
func CatalogEvent(c *screensharev1.Catalog) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_Catalog{Catalog: c},
	}
}

// SettingsChangedEvent announces that the backend's held settings moved for a reason that did not
// come from the shell receiving it.
//
// Carries no values.
// The shell re-reads the settings and re-resolves its form,
// so there is one way to learn what they became rather than two able to disagree.
func SettingsChangedEvent() *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_SettingsChanged{SettingsChanged: &screensharev1.SettingsChanged{}},
	}
}

// ReceiveStateEvent announces the streams the backend is decoding, whole.
//
// The receive-side counterpart of ViewerStateEvent, carrying decodes rather than tiles:
// how a shell arranges what it receives is the shell's, and is on no message this package writes.
func ReceiveStateEvent(streams []ReceiveStream) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_ReceiveState{ReceiveState: ReceiveState(streams)},
	}
}

// ReceiveStatsEvent announces one sample of every running decode.
//
// The receive-side counterpart of the publish's progress samples,
// and a second event rather than a fuller ReceiveState for the reason the publish has two:
// what a decode is is announced when it changes, and what a decode is doing is read on a clock.
// One message for both would push everything a tile knows at sampling rate.
func ReceiveStatsEvent(streams []ReceiveStreamStats) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_ReceiveStats{ReceiveStats: ReceiveStats(streams)},
	}
}

// MonitorPreviewStateEvent announces the monitors the backend is previewing, whole.
//
// No exit event beside it, unlike the receive pair: a preview that ended leaves the set,
// with no log to open, no viewer to account for and no retry to explain.
func MonitorPreviewStateEvent(monitors []PreviewedMonitor) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_MonitorPreviewState{
			MonitorPreviewState: MonitorPreviewState(monitors),
		},
	}
}

// ReceiveExitEvent announces that one receive pipeline ended, and why.
//
// No log path, unlike the publish and viewer exits:
// a receive pipeline runs inside this process rather than as a child, so it has no run log
// to offer.
func ReceiveExitEvent(stream StreamRef, message string, cause ...*screensharev1.Text) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_ReceiveExit{ReceiveExit: &screensharev1.ReceiveExit{
			Stream:  StreamRefMessage(stream),
			Message: message,
			Cause:   oneCause(cause),
		}},
	}
}

// MembersStateEvent announces who this machine shares a group with, whole.
//
// Refusals travel too:
// a machine that cannot state its presence is one whose connections the relay closes,
// and the refusal reaches a reader instead of a stream that ends with nothing said.
func MembersStateEvent(m MembersSnapshot) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_MembersState{MembersState: MembersState(m)},
	}
}

// DiscordSnapshot is Discord mode as the backend's last manager pass read it.
type DiscordSnapshot struct {
	Linked      bool
	AccountName string
	Refused     bool
	InChannel   bool
	GuildName   string
	ChannelName string
	Stale       bool
}

// DiscordState is the snapshot in the contract's shape, the read and the event sharing it.
func DiscordState(d DiscordSnapshot) *screensharev1.DiscordState {
	return &screensharev1.DiscordState{
		Linked:      d.Linked,
		AccountName: d.AccountName,
		LinkRefused: d.Refused,
		InChannel:   d.InChannel,
		GuildName:   d.GuildName,
		ChannelName: d.ChannelName,
		Stale:       d.Stale,
	}
}

// DiscordStateEvent announces Discord mode's own state: link, channel and freshness.
func DiscordStateEvent(d DiscordSnapshot) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_DiscordState{DiscordState: DiscordState(d)},
	}
}

// oneCause reads the statement off a constructor's trailing argument.
//
// Optional and trailing, so one constructor serves a producer that can name what ended a run
// and one that cannot, the second passing nothing rather than a nil it had to spell.
// Two would be two answers to one question.
func oneCause(cause []*screensharev1.Text) *screensharev1.Text {
	assert.Assert(len(cause) <= 1, "an ending has one cause at most", len(cause))

	if len(cause) == 0 {
		return nil
	}
	return cause[0]
}
