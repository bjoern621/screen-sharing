package wire

import (
	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/relay"
)

// The event constructors are here rather than at each producer so that the oneof
// wrapper a payload belongs in is written once. A producer that built the envelope
// itself would be choosing the event kind alongside the state, and a payload put in
// the wrong wrapper is a state that arrives under another name with nothing on
// either side to say so.
//
// Every constructor leaves Sequence zero. Each subscription stamps its own numbers as
// it sends, because a subscription that named kinds is not sent the ones it filtered
// out, and a number shared across subscribers would show a gap for every event a
// filter dropped.

// PublishStateEvent announces the publish state after any change to it, whoever made
// the change. It carries the same message GetPublishState answers with, so a window
// that has just mounted and one that has been open cannot be told different things.
func PublishStateEvent(p PublishSnapshot) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_PublishState{PublishState: PublishState(p)},
	}
}

// PublishStatsEvent announces one progress sample from the running encoder. It is
// the high-rate kind, at roughly one per second per running pipeline.
func PublishStatsEvent(s ffmpeg.Stats) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_PublishStats{PublishStats: PublishStats(s)},
	}
}

// PublishExitEvent announces that the publish pipeline ended. It says why and where
// the log is; what the backend then did about it is the publish state event that
// follows, which is why a retry is not described here.
func PublishExitEvent(message, logPath string) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_PublishExit{PublishExit: ExitInfo(message, logPath)},
	}
}

// RelayStatusEvent announces a relay snapshot at the backend's poll interval. It is
// pushed rather than polled by a shell so that the byte-delta bitrates it carries
// are computed against one steady interval instead of against whatever cadence each
// shell chose.
func RelayStatusEvent(s relay.Status) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_RelayStatus{RelayStatus: RelayStatus(s)},
	}
}

// ViewerStateEvent announces the open external viewers whenever one opens or closes.
//
// It exists because StartWatch and StopWatch answer with an empty message, and an
// effect whose result reaches no event is an effect only the shell that called it
// learns the outcome of. That is the one rule the event stream is built on.
func ViewerStateEvent(keys []WatchKey) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_ViewerState{ViewerState: ViewerState(keys)},
	}
}

// ViewerExitEvent announces that one external viewer ended. It carries the whole key
// for the reason WatchKey exists: a stream can be watched over several transports at
// once, so the name alone would clear the wrong viewer.
func ViewerExitEvent(key WatchKey, message, logPath string) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_ViewerExit{ViewerExit: &screensharev1.ViewerExit{
			Viewer: WatchKeyMessage(key),
			Exit:   ExitInfo(message, logPath),
		}},
	}
}

// TestStreamStateEvent announces how many synthetic publishers are alive, for the
// reason ViewerStateEvent exists: StartTestStreams and StopTestStreams answer with an
// empty message, and one that died on its own changes the count with nothing having
// been called at all.
func TestStreamStateEvent(running int) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_TestStreamState{TestStreamState: TestStreamState(running)},
	}
}

// TestStreamExitEvent announces that a synthetic test publisher ended.
func TestStreamExitEvent(message, logPath string) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_TestStreamExit{TestStreamExit: ExitInfo(message, logPath)},
	}
}

// CatalogEvent announces the whole reference set again, after the encoder probe has
// filled in.
//
// It carries the catalog and not the probe result alone, because a shell holding a
// catalog has nothing to merge a half-state into. It exists so that the shell that
// asked for the probe is not the only one that learns what it found: a resolve on any
// other shell would otherwise start greying codecs with nothing having told it why.
func CatalogEvent(c *screensharev1.Catalog) *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_Catalog{Catalog: c},
	}
}

// SettingsChangedEvent announces that the backend's held settings moved for a reason
// that did not come from the shell receiving this.
//
// It carries no values. The shell re-reads the settings and re-resolves its form, so
// there is one way to learn what they became rather than two that can disagree.
func SettingsChangedEvent() *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_SettingsChanged{SettingsChanged: &screensharev1.SettingsChanged{}},
	}
}

// ShowSettingsEvent asks a shell to bring its own configuration surface to the
// front. It is the one event that asks for something rather than reporting
// something, and it is a user intent routed rather than a UI command: something
// outside the window reports that the user asked to configure, and which surface that
// is remains the shell's answer.
func ShowSettingsEvent() *screensharev1.Event {
	return &screensharev1.Event{
		Payload: &screensharev1.Event_ShowSettings{ShowSettings: &screensharev1.ShowSettings{}},
	}
}
