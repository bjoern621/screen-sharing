package control

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"

	"google.golang.org/protobuf/proto"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The effects. Each does the one thing its name says and answers with its empty
// response message.
//
// Not one of them returns the state it produced, and that is the contract's rule
// rather than an economy (docs/ipc-api.md, "Events"): what the state became arrives on
// the event stream. One path into the display is what stops the window that pressed
// the button and the window that did not from showing different things, and an effect
// that answered with the new state would be a second path - the shell that read the
// answer and the shell that only listened would be two ways of learning one fact, and
// they would disagree the first time an effect and an event crossed.
//
// The same rule forbids reading state back here to build a richer answer. An effect
// that read the publish state after starting a publish would be reporting a state that
// is already one event out of date, in a message the contract says carries nothing.

// fromBackend turns a plain error the backend returned into a status.
//
// It exists once rather than per call site because a per-site judgement would be a
// guess. The backend hands back an untyped error, so a helper that decided the code by
// matching on its text would be deriving the contract's error model from a sentence
// written for a person - the one input that changes without anything failing to
// compile.
//
// The rule it applies is a consequence of how the effects below are written. Every
// refusal the request itself earns is made above the backend call, against the
// arguments or against the state: a name that is empty, a settings message that
// carries nothing, a count that is not positive, a stream already in force, nothing to
// apply settings to, a log file that is not there. An Entwicklungsfehler does not
// reach here either, because assert panics in the backend as it does everywhere else.
// What is left is the world failing to do something it was legal to ask for - a child
// process that would not start, a store that could not be written, a relay that could
// not be reached - and the contract's table calls that UNAVAILABLE.
//
// The cost is worth naming rather than hiding. A settings combination no engine can
// build also comes back from StartPublish as an untyped error, and INVALID_ARGUMENT
// would describe it better than UNAVAILABLE does. Telling the two apart needs a typed
// refusal on Backend; matching on the sentence here would not be telling them apart,
// it would be guessing, and a guess that lands wrong tells a shell to blame the user
// for a disk that is full.
func fromBackend(what string, err error) error {
	assert.IsNotNil(err, "a classified failure is a failure", what)

	return unavailable("%s: %v", what, err)
}

// draftOf reads the settings off a request that needs them.
//
// A message that is absent and a message that carries nothing are the same request:
// one that named no settings at all. Both are INVALID_ARGUMENT rather than a stream
// with no name, no relay and no codec, because the alternative is that a shell which
// forgot to attach its draft persists that emptiness over the user's settings and
// finds out on the next launch.
//
// The emptiness is tested against the zero message rather than against one chosen
// field, so a field added to the contract keeps the check exact without an edit here.
func draftOf(m *screensharev1.Settings, verb string) (settings.Settings, error) {
	if m == nil || proto.Equal(m, &screensharev1.Settings{}) {
		return settings.Settings{}, invalidArgument("no settings to %s", verb)
	}
	return wire.ToSettings(m), nil
}

// presetOf reads one saved way of publishing off a request, and refuses an empty one
// for the reason draftOf refuses an empty draft.
//
// A preset is a PublishSettings and nothing else: where the relay is belongs to a
// deployment and how this machine watches belongs to a viewer, so neither is part of
// what applying a preset means.
func presetOf(m *screensharev1.PublishSettings, verb string) (settings.Publish, error) {
	if m == nil || proto.Equal(m, &screensharev1.PublishSettings{}) {
		return settings.Publish{}, invalidArgument("no settings to %s", verb)
	}
	return wire.ToPublish(m), nil
}

// SaveSettings persists the settings the shell holds.
//
// It does not touch a running stream. Both engines run a child built from an argv and
// neither takes a value back afterwards, so what reaches a live pipeline is
// ApplyToStream's business and the two are separate methods because the user's two
// intentions are different: keep this for next time, and put this on the air now.
func (s *Server) SaveSettings(ctx context.Context, req *screensharev1.SaveSettingsRequest) (*screensharev1.SaveSettingsResponse, error) {
	draft, err := draftOf(req.GetSettings(), "save")
	if err != nil {
		return nil, err
	}

	if err := s.backend.SaveSettings(draft); err != nil {
		return nil, fromBackend("cannot save the settings", err)
	}
	return &screensharev1.SaveSettingsResponse{}, nil
}

// SavePreset stores the settings under a name, replacing a same-named preset.
func (s *Server) SavePreset(ctx context.Context, req *screensharev1.SavePresetRequest) (*screensharev1.SavePresetResponse, error) {
	name := req.GetName()
	if name == "" {
		return nil, invalidArgument("a preset is saved under a name, and none was given")
	}

	preset, err := presetOf(req.GetSettings(), "save as a preset")
	if err != nil {
		return nil, err
	}

	if err := settings.SavePreset(name, preset); err != nil {
		return nil, fromBackend("cannot save the preset '"+name+"'", err)
	}
	return &screensharev1.SavePresetResponse{}, nil
}

// DeletePreset removes a named preset.
//
// The store is read before the delete because the store does not distinguish the two
// answers the contract does: settings.DeletePreset rewrites the file without the name,
// and a name that was never in it is removed just as successfully as one that was.
// NOT_FOUND is therefore decided here, off the same list the delete itself walks.
func (s *Server) DeletePreset(ctx context.Context, req *screensharev1.DeletePresetRequest) (*screensharev1.DeletePresetResponse, error) {
	name := req.GetName()
	if name == "" {
		return nil, invalidArgument("a preset is deleted by name, and none was given")
	}

	presets, err := settings.LoadPresets()
	if err != nil {
		return nil, fromBackend("cannot read the presets", err)
	}
	if !slices.ContainsFunc(presets, func(p settings.Preset) bool { return p.Name == name }) {
		return nil, notFound("no preset named '%s'", name)
	}

	if err := settings.DeletePreset(name); err != nil {
		return nil, fromBackend("cannot delete the preset '"+name+"'", err)
	}
	return &screensharev1.DeletePresetResponse{}, nil
}

// StartPublish persists the settings and starts the encoder on them.
//
// A stream already in force refuses a start naming a *different* pipeline, and a pipeline
// waiting out a retry backoff is in force: it is the stream the user asked for and has not
// stopped, it will come back on its own, and the one call that ends a running pipeline ends
// it too. Letting a different start through in that gap would put two encoders on one relay
// path seconds apart.
//
// A start naming the pipeline that is already publishing is not that case. It is a request
// for a state that already holds, and a state that already holds is a success
// (docs/development-principles.md, "Effects across a process boundary"): a shell whose
// answer went missing cannot tell "not done" from "done, answer lost", and asking again is
// the only move that resolves it. publish.SamePipeline decides sameness here and at the
// backend, because "these two settings are one stream" is one fact.
func (s *Server) StartPublish(ctx context.Context, req *screensharev1.StartPublishRequest) (*screensharev1.StartPublishResponse, error) {
	draft, err := draftOf(req.GetSettings(), "publish")
	if err != nil {
		return nil, err
	}

	if state := s.backend.PublishState(); state.Publishing() {
		if same, err := publish.SamePipeline(state.Live.Settings, draft); err == nil && same {
			return &screensharev1.StartPublishResponse{}, nil
		}
		if retry := state.Retry(); retry != nil {
			return nil, failedPrecondition(
				"a stream is already publishing and is waiting out attempt %d of %d after its pipeline died; stop it before starting another",
				retry.Attempt, retry.Budget)
		}
		return nil, failedPrecondition("a stream is already publishing; stop it before starting another")
	}

	if err := s.backend.StartPublish(draft); err != nil {
		return nil, fromBackend("cannot start publishing", err)
	}
	return &screensharev1.StartPublishResponse{}, nil
}

// ApplyToStream restarts the running stream on new settings.
//
// With nothing publishing there is no pipeline to apply them to, which is
// FAILED_PRECONDITION and not a quiet start: a user who edited settings and pressed
// apply asked for the live stream to change, and starting one they had stopped would
// be a different thing than the one they asked for.
func (s *Server) ApplyToStream(ctx context.Context, req *screensharev1.ApplyToStreamRequest) (*screensharev1.ApplyToStreamResponse, error) {
	draft, err := draftOf(req.GetSettings(), "apply")
	if err != nil {
		return nil, err
	}

	if !s.backend.PublishState().Publishing() {
		return nil, failedPrecondition("nothing is publishing, so there is no pipeline to apply the settings to")
	}

	if err := s.backend.ApplyToStream(draft); err != nil {
		return nil, fromBackend("cannot apply the settings to the running stream", err)
	}
	return &screensharev1.ApplyToStreamResponse{}, nil
}

// StopPublish ends the stream, whether it is running or waiting out a backoff.
//
// It refuses nothing, including a stop with nothing publishing. A stop is a statement
// about what the user wants to be true afterwards, and it is already true, so there is
// no condition left for a precondition to fail on.
func (s *Server) StopPublish(ctx context.Context, req *screensharev1.StopPublishRequest) (*screensharev1.StopPublishResponse, error) {
	s.backend.StopPublish()

	return &screensharev1.StopPublishResponse{}, nil
}

// StartWatch opens an external viewer for one stream over one transport.
//
// The transport is per viewer and independent of the publish leg, so the same stream
// can be watched over any leg the relay serves it on.
func (s *Server) StartWatch(ctx context.Context, req *screensharev1.StartWatchRequest) (*screensharev1.StartWatchResponse, error) {
	key := wire.WatchKeyOf(req.GetViewer())
	name, leg := key.StreamName, key.Transport
	if name == "" {
		return nil, invalidArgument("no stream named to watch")
	}
	if leg == "" {
		return nil, invalidArgument("no transport named to watch '%s' over", name)
	}

	if err := s.backend.StartWatch(key); err != nil {
		// FAILED_PRECONDITION rather than INVALID_ARGUMENT, and the difference is what the
		// request is wrong about. The refusal the contract names for this method is the
		// carriage one: the relay re-serves a stream only on the listeners whose protocol
		// has a payload mapping for its bitstream, so an SRT viewer opened on a VP9 stream
		// connects and receives nothing, and the backend refuses it with the format named.
		// Everything about that pair exists - the transport is a leg this build serves, the
		// stream is one the relay is carrying - and what stands in the way is the format
		// that stream is being published in at this instant. Republish it as H.264 and the
		// same request succeeds. That is precisely the table's "the request is well formed
		// and the world is not ready for it", and the backend's sentence names both the
		// format and the legs that do carry it, so the reason reaches the user intact.
		//
		// A transport this build has no viewer for would be INVALID_ARGUMENT under the same
		// table and arrives here under this code instead, because the backend answers both
		// with an untyped error and separating them would mean matching on its text, which
		// is the guess fromBackend exists not to make. A typed refusal on Backend is what
		// would fix it; the two empty-argument cases above are the part this side can see
		// for itself.
		return nil, failedPrecondition("cannot watch '%s' over %s: %v", name, leg, err)
	}
	return &screensharev1.StartWatchResponse{}, nil
}

// StopWatch closes one open viewer.
//
// The pair is checked for the reason WatchKey exists: a stream is watched over several
// transports at once, so a stop naming only half the identity would be a stop naming a
// viewer that cannot exist. A complete pair with no viewer open is not refused, on the
// same ground StopPublish is not.
func (s *Server) StopWatch(ctx context.Context, req *screensharev1.StopWatchRequest) (*screensharev1.StopWatchResponse, error) {
	key := wire.WatchKeyOf(req.GetViewer())
	if key.StreamName == "" {
		return nil, invalidArgument("no stream named to stop watching")
	}
	if key.Transport == "" {
		return nil, invalidArgument("no transport named to stop watching '%s' over", key.StreamName)
	}

	s.backend.StopWatch(key)

	return &screensharev1.StopWatchResponse{}, nil
}

// OpenInBrowser opens the relay's player page for one stream in the machine's default
// browser.
//
// The pair and its two refusals are StartWatch's, for the same reasons: the leg is per
// reader, and one whose format the stream does not cross would open a page that
// connects and shows nothing. What it does not have is a counterpart: the tab belongs
// to the browser, so there is no stop to write and no viewer state for this to move.
func (s *Server) OpenInBrowser(ctx context.Context, req *screensharev1.OpenInBrowserRequest) (*screensharev1.OpenInBrowserResponse, error) {
	key := wire.WatchKeyOf(req.GetViewer())
	name, leg := key.StreamName, key.Transport
	if name == "" {
		return nil, invalidArgument("no stream named to open in the browser")
	}
	if leg == "" {
		return nil, invalidArgument("no transport named to open '%s' over", name)
	}

	if err := s.backend.OpenInBrowser(key); err != nil {
		return nil, failedPrecondition("cannot open '%s' over %s in the browser: %v", name, leg, err)
	}
	return &screensharev1.OpenInBrowserResponse{}, nil
}

// StartReceive opens a decode for one stream on one leg, inside the backend.
//
// The two empty-argument refusals and the carriage one are StartWatch's, for the same
// reasons: the pair is the identity a decode is keyed by, and a leg that cannot carry
// the stream's format is a request the world is not ready for rather than one that is
// malformed. What differs is the engine the carriage is asked about - a receive pipeline
// reaches WHEP and no player does - and the backend is what asks.
func (s *Server) StartReceive(ctx context.Context, req *screensharev1.StartReceiveRequest) (*screensharev1.StartReceiveResponse, error) {
	key := wire.WatchKeyOf(req.GetStream())
	name, leg := key.StreamName, key.Transport
	if name == "" {
		return nil, invalidArgument("no stream named to receive")
	}
	if leg == "" {
		return nil, invalidArgument("no transport named to receive '%s' over", name)
	}

	if err := s.backend.StartReceive(key); err != nil {
		return nil, failedPrecondition("cannot receive '%s' over %s: %v", name, leg, err)
	}
	return &screensharev1.StartReceiveResponse{}, nil
}

// StopReceive closes one running decode. The pair is checked and a pair nothing is
// decoding is not refused, both for the reasons StopWatch gives.
func (s *Server) StopReceive(ctx context.Context, req *screensharev1.StopReceiveRequest) (*screensharev1.StopReceiveResponse, error) {
	key := wire.WatchKeyOf(req.GetStream())
	if key.StreamName == "" {
		return nil, invalidArgument("no stream named to stop receiving")
	}
	if key.Transport == "" {
		return nil, invalidArgument("no transport named to stop receiving '%s' over", key.StreamName)
	}

	s.backend.StopReceive(key)

	return &screensharev1.StopReceiveResponse{}, nil
}

// StartMonitorPreview reads one of this machine's screens so the wizard can offer it by
// its picture rather than by its number.
//
// The index is not checked here, unlike the pair the receive methods take. An empty
// stream name is a request with a hole in it and can be recognised as one from the
// message alone; every integer is a monitor index somewhere, and whether it is one of
// this machine's is a fact about the machine. So the backend answers that, and the
// refusal is the one it gives.
func (s *Server) StartMonitorPreview(ctx context.Context, req *screensharev1.StartMonitorPreviewRequest) (*screensharev1.StartMonitorPreviewResponse, error) {
	monitor := int(req.GetMonitor())

	if err := s.backend.StartMonitorPreview(monitor); err != nil {
		return nil, failedPrecondition("cannot preview monitor %d: %v", monitor, err)
	}
	return &screensharev1.StartMonitorPreviewResponse{}, nil
}

// StopMonitorPreview closes one monitor's preview. A monitor nothing is previewing is
// not refused, for the reason StopReceive takes a decode that is already closed.
func (s *Server) StopMonitorPreview(ctx context.Context, req *screensharev1.StopMonitorPreviewRequest) (*screensharev1.StopMonitorPreviewResponse, error) {
	s.backend.StopMonitorPreview(int(req.GetMonitor()))

	return &screensharev1.StopMonitorPreviewResponse{}, nil
}

// SetReceiveAudio sets how loud one decode plays and whether it plays at all.
//
// The pair is checked for the reason every receive method checks it. What differs from
// the two above is the refusal: a decode that is not running is NOT_FOUND rather than
// a quiet success, because this is a request about something absent and not a request
// for a state that already holds. A repeat of a volume, by contrast, is exactly that
// state and succeeds - the backend holds what it was asked for and writes it again.
//
// The volume is not range-checked here. A figure past the end of the range is brought
// back by the backend, which is where the bound lives, and a refusal would make a
// slider that overshot into an error a reader has to read.
func (s *Server) SetReceiveAudio(ctx context.Context, req *screensharev1.SetReceiveAudioRequest) (*screensharev1.SetReceiveAudioResponse, error) {
	key := wire.WatchKeyOf(req.GetStream())
	if key.StreamName == "" {
		return nil, invalidArgument("no stream named to set the audio of")
	}
	if key.Transport == "" {
		return nil, invalidArgument("no transport named to set the audio of '%s' over", key.StreamName)
	}

	if err := s.backend.SetReceiveAudio(key, req.GetVolume(), req.GetMuted()); err != nil {
		return nil, notFound("cannot set the audio of '%s' over %s: %v", key.StreamName, key.Transport, err)
	}

	return &screensharev1.SetReceiveAudioResponse{}, nil
}

// ProbeEncoders test-encodes on every engine and records what this machine can really
// run, then announces the catalog that result changed.
//
// The announcement is the reason this is a method at all. The probe used to be a flag
// on GetCatalog, which made one shell's read change what a different shell's next
// resolve would say - a codec greyed for missing hardware, with nothing having told
// that shell why its form moved. Every shell now learns on the stream, including the
// ones that never asked.
//
// It is announced unconditionally rather than only where the result moved. The probe is
// cached for the process lifetime, so a second call is answered from the cache and the
// event it publishes is a state a subscriber already holds; a duplicate whole-state
// event is harmless by construction, and comparing availabilities to avoid one would be
// a second definition of when two probe results are the same.
func (s *Server) ProbeEncoders(ctx context.Context, req *screensharev1.ProbeEncodersRequest) (*screensharev1.ProbeEncodersResponse, error) {
	s.backend.Encoders(ctx)
	s.events.Publish(wire.CatalogEvent(s.catalog()))

	return &screensharev1.ProbeEncodersResponse{}, nil
}

// StartTestStreams launches synthetic publishers, replacing a running set.
//
// A count that is not positive is INVALID_ARGUMENT: it names a set of publishers that
// cannot exist, and there is a separate method for stopping them. A count above the
// bound is RESOURCE_EXHAUSTED: each test stream runs its own software encoder, so a
// large set saturates the machine without testing anything new.
//
// Both refusals are made here, above the call, which is where every request-earned
// refusal in this file is made. Inferring the bound from the backend's error instead
// would put a viewer binary that could not be found under RESOURCE_EXHAUSTED as well,
// and a missing binary is not a saturated machine.
func (s *Server) StartTestStreams(ctx context.Context, req *screensharev1.StartTestStreamsRequest) (*screensharev1.StartTestStreamsResponse, error) {
	count := int(req.GetCount())
	if count <= 0 {
		return nil, invalidArgument("a test-stream count is at least one, and %d was asked for", count)
	}

	max := s.backend.MaxTestStreams()
	assert.Assert(max > 0, "a backend that runs test streams runs at least one", max)
	if count > max {
		return nil, resourceExhausted("this machine runs at most %d test streams at once, and %d were asked for", max, count)
	}

	if err := s.backend.StartTestStreams(count); err != nil {
		return nil, fromBackend(fmt.Sprintf("cannot start %d test streams", count), err)
	}
	return &screensharev1.StartTestStreamsResponse{}, nil
}

// StopTestStreams stops every synthetic publisher.
func (s *Server) StopTestStreams(ctx context.Context, req *screensharev1.StopTestStreamsRequest) (*screensharev1.StopTestStreamsResponse, error) {
	s.backend.StopTestStreams()

	return &screensharev1.StopTestStreamsResponse{}, nil
}

// ForgetPortalConsent drops the stored screen-capture consent, so the next capture asks
// the compositor to pick again. It is how a share aimed at the wrong window or monitor
// is corrected.
func (s *Server) ForgetPortalConsent(ctx context.Context, req *screensharev1.ForgetPortalConsentRequest) (*screensharev1.ForgetPortalConsentResponse, error) {
	if err := s.backend.ForgetPortalConsent(); err != nil {
		return nil, fromBackend("cannot forget the screen-capture consent", err)
	}
	return &screensharev1.ForgetPortalConsentResponse{}, nil
}

// OpenLog opens one run log in the machine's default application.
//
// The path is one the backend handed out on an ExitInfo and a shell does not construct
// one, so an empty path is INVALID_ARGUMENT - the exit carried no log - and a path that
// is not there is NOT_FOUND. The second check is made here rather than left to the
// backend because the difference matters to the user and the default handler cannot
// tell them: run logs are rotated, so a log named by an older exit is a file that
// existed and has since gone, and "no log file at this path" says that where a handler
// that refused to open it would not.
func (s *Server) OpenLog(ctx context.Context, req *screensharev1.OpenLogRequest) (*screensharev1.OpenLogResponse, error) {
	path := req.GetPath()
	if path == "" {
		return nil, invalidArgument("no log file for this run")
	}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, notFound("no log file at %s; the run logs are rotated, so one named by an older exit may already be gone", path)
	}

	if err := s.backend.OpenLog(path); err != nil {
		return nil, fromBackend("cannot open "+path, err)
	}
	return &screensharev1.OpenLogResponse{}, nil
}

// OpenLogsFolder opens the directory holding the run logs in the machine's file
// browser.
func (s *Server) OpenLogsFolder(ctx context.Context, req *screensharev1.OpenLogsFolderRequest) (*screensharev1.OpenLogsFolderResponse, error) {
	if err := s.backend.OpenLogsFolder(); err != nil {
		return nil, fromBackend("cannot open the logs folder", err)
	}
	return &screensharev1.OpenLogsFolderResponse{}, nil
}
