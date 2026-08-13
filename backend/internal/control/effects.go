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

// The effects.
// Each does the one thing its name says and answers with its empty response message.
//
// None returns the state it produced (docs/ipc-api.md, "Events"): what the state became arrives on
// the event stream.
// One path into the display is what keeps the window that pressed the button and the window that
// did not from showing different things; the shell that read an answer and the shell that only
// listened would disagree the first time an effect and an event crossed.
//
// The same rule forbids reading state back here to build a richer answer.
// A state read after the effect is already one event out of date, in a message the contract says
// carries nothing.

// fromBackend turns an untyped backend error into UNAVAILABLE.
//
// One helper rather than a judgement per call site: the backend's errors carry no type, so deciding
// the code by matching on the text would derive the contract's error model from a sentence written
// for a person, which is the one input that changes without anything failing to compile.
//
// UNAVAILABLE is what is left once the other kinds are excluded.
// Every refusal the request itself earns is made above the backend call, against the arguments or
// the state: an empty name, a settings message carrying nothing, a count that is not positive,
// a stream already in force, nothing to apply settings to, a log file that is not there.
// An Entwicklungsfehler never arrives either, since assert panics in the backend.
// What remains is the world failing at something it was legal to ask for - a child process that
// would not start, a store that could not be written, a relay that could not be reached.
//
// The cost: a settings combination no engine can build also comes back untyped from StartPublish,
// and INVALID_ARGUMENT would describe it better.
// Telling the two apart needs a typed refusal on Backend, and a guess off the sentence that landed
// wrong would tell a shell to blame the user for a disk that is full.
func fromBackend(what string, err error) error {
	assert.IsNotNil(err, "a classified failure is a failure", what)

	return unavailable("%s: %v", what, err)
}

// draftOf reads the settings off a request that needs them.
//
// An absent message and one carrying nothing are the same request, and both are INVALID_ARGUMENT
// rather than a stream with no name, no relay and no codec: a shell that forgot to attach its draft
// would otherwise persist that emptiness over the user's settings and find out on the next launch.
//
// Emptiness is tested against the zero message rather than one chosen field, so a field added to
// the contract stays covered without an edit here.
func draftOf(m *screensharev1.Settings, verb string) (settings.Settings, error) {
	// The verb is this file's word for what the caller was doing, never the request's,
	// so an empty one is a call site leaving a hole in the sentence a person reads.
	assert.Assert(verb != "", "a refused draft names what it was going to be used for")

	if m == nil || proto.Equal(m, &screensharev1.Settings{}) {
		return settings.Settings{}, invalidArgument("no settings to %s", verb)
	}
	return wire.ToSettings(m), nil
}

// presetOf reads one saved way of publishing off a request, refusing an empty one for the reason
// draftOf refuses an empty draft.
//
// A preset is a PublishSettings and nothing else: where the relay is belongs to a deployment and
// how this machine watches belongs to a viewer, so neither is part of what applying one means.
func presetOf(m *screensharev1.PublishSettings, verb string) (settings.Publish, error) {
	assert.Assert(verb != "", "a refused preset names what it was going to be used for")

	if m == nil || proto.Equal(m, &screensharev1.PublishSettings{}) {
		return settings.Publish{}, invalidArgument("no settings to %s", verb)
	}
	return wire.ToPublish(m), nil
}

// SaveSettings persists the settings the shell holds, and touches no running stream.
//
// Both engines run a child built from an argv and neither takes a value back afterwards,
// so what reaches a live pipeline is ApplyToStream's business.
// Two methods because the user's two intentions differ: keep this for next time, and put this on
// the air now.
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
// A request with no name is INVALID_ARGUMENT.
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

// DeletePreset removes a named preset, and answers NOT_FOUND where no preset carries the name.
//
// The store is read before the delete because it does not distinguish the two answers the contract
// does: settings.DeletePreset rewrites the file without the name, and a name that was never in it
// is removed as successfully as one that was.
// NOT_FOUND is decided here, off the same list the delete walks.
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
// A start naming a *different* pipeline while a stream is in force is FAILED_PRECONDITION,
// and a pipeline waiting out a retry backoff is in force: the user asked for it, has not stopped
// it, it comes back on its own, and the one call that ends a running pipeline ends it too.
// A different start let through in that gap would put two encoders on one relay path seconds apart.
//
// A start naming the pipeline already publishing succeeds and does nothing.
// It is a request for a state that already holds (docs/development-principles.md, "Effects across a
// process boundary"): a shell whose answer went missing cannot tell "not done" from "done, answer
// lost", and asking again is the only move that resolves it.
// publish.SamePipeline decides sameness here and at the backend, because "these two settings are
// one stream" is one fact.
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
// It names a transition rather than a state, which is the documented departure from the
// idempotency the rest of these effects hold to (docs/development-principles.md, "Effects across a
// process boundary"): a second apply is a second restart.
//
// Nothing publishing is FAILED_PRECONDITION and not a quiet start: an apply asks for the live
// stream to change, not for a stream the user had stopped to come back.
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
// It refuses nothing, a stop with nothing publishing included.
// A stop names what is to be true afterwards, and where that already holds no precondition is left
// to fail on.
func (s *Server) StopPublish(ctx context.Context, req *screensharev1.StopPublishRequest) (*screensharev1.StopPublishResponse, error) {
	s.backend.StopPublish()

	return &screensharev1.StopPublishResponse{}, nil
}

// StartWatch opens an external viewer for one stream over one transport.
// Half a pair is INVALID_ARGUMENT, and a leg that cannot carry the stream's format is
// FAILED_PRECONDITION.
//
// The transport is per viewer and independent of the publish leg, so one stream can be watched over
// any leg the relay serves it on.
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
		// The carriage refusal, which is the world not being ready rather than the request being
		// malformed: the relay re-serves a stream only on the listeners whose protocol has a payload
		// mapping for its bitstream, so an SRT viewer opened on a VP9 stream connects and receives
		// nothing.
		// Both halves of the pair exist - the leg is one this build serves, the stream one the relay
		// carries - and what stands in the way is the format the stream is published in at this instant.
		// Republished as H.264, the same request succeeds.
		// The backend's sentence names the format and the legs that do carry it, so the reason reaches
		// the user intact.
		//
		// A transport this build has no viewer for is INVALID_ARGUMENT under the contract's table, and
		// the backend says which of the two it is by returning a Refused for it.
		if refused(err) {
			return nil, invalidArgument("cannot watch '%s' over %s: %v", name, leg, err)
		}
		return nil, failedPrecondition("cannot watch '%s' over %s: %v", name, leg, err)
	}
	return &screensharev1.StartWatchResponse{}, nil
}

// StopWatch closes one open viewer.
//
// The pair is checked for the reason WatchKey exists: one stream is watched over several transports
// at once, so half an identity names a viewer that cannot exist and is INVALID_ARGUMENT.
// A complete pair with no viewer open succeeds, on the ground StopPublish does.
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

// OpenInBrowser opens the relay's player page for one stream in the machine's default browser.
//
// The pair and its refusals are StartWatch's, for the same reasons: the leg is per reader, and one
// the stream's format does not cross opens a page that connects and shows nothing.
//
// A second call opens a second page.
// That is the documented departure from idempotency (docs/development-principles.md, "Effects
// across a process boundary"): the effect lands in a program this process does not own, so there is
// no state to read back, no stop to write, and nothing here reaches the viewer state.
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
// A decode already open on that pair is the state the call names and succeeds.
//
// The empty-argument refusals and the carriage one are StartWatch's, for the same reasons: the pair
// is the identity a decode is keyed by, and a leg that cannot carry the stream's format is a
// request the world is not ready for rather than a malformed one.
// What differs is the engine the carriage is asked about, a receive pipeline reaching WHEP where no
// player does, and the backend is what asks.
//
// Tone mapping is not checked here.
// Whether the stream carries more range than a display shows is known once the decoder negotiates
// and not before, and whether this machine can roll it down is the backend's registry,
// so a refusal written here would guess at both.
func (s *Server) StartReceive(ctx context.Context, req *screensharev1.StartReceiveRequest) (*screensharev1.StartReceiveResponse, error) {
	key := wire.WatchKeyOf(req.GetStream())
	name, leg := key.StreamName, key.Transport
	if name == "" {
		return nil, invalidArgument("no stream named to receive")
	}
	if leg == "" {
		return nil, invalidArgument("no transport named to receive '%s' over", name)
	}

	if err := s.backend.StartReceive(key, req.GetToneMap()); err != nil {
		return nil, failedPrecondition("cannot receive '%s' over %s: %v", name, leg, err)
	}
	return &screensharev1.StartReceiveResponse{}, nil
}

// StopReceive closes one running decode.
// The pair is checked, and a pair nothing is decoding succeeds, both for the reasons StopWatch
// gives.
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

// StartMonitorPreview reads one of this machine's screens, so the wizard can offer it by its
// picture rather than by its number.
// A monitor already being previewed is the state the call names and succeeds.
//
// The index is not checked here, unlike the pair the receive methods take.
// An empty stream name shows as a hole from the message alone.
// Every integer is a monitor index somewhere, and whether it is one of this machine's is a fact
// about the machine.
// The backend answers that, and a Refused is what separates the index naming no output of this
// machine (INVALID_ARGUMENT) from a session that cannot read one screen apart from another
// (FAILED_PRECONDITION).
func (s *Server) StartMonitorPreview(ctx context.Context, req *screensharev1.StartMonitorPreviewRequest) (*screensharev1.StartMonitorPreviewResponse, error) {
	monitor := int(req.GetMonitor())

	if err := s.backend.StartMonitorPreview(monitor); err != nil {
		if refused(err) {
			return nil, invalidArgument("cannot preview monitor %d: %v", monitor, err)
		}
		return nil, failedPrecondition("cannot preview monitor %d: %v", monitor, err)
	}
	return &screensharev1.StartMonitorPreviewResponse{}, nil
}

// StopMonitorPreview closes one monitor's preview.
// A monitor nothing is previewing succeeds, for the reason StopReceive takes a decode already
// closed.
func (s *Server) StopMonitorPreview(ctx context.Context, req *screensharev1.StopMonitorPreviewRequest) (*screensharev1.StopMonitorPreviewResponse, error) {
	s.backend.StopMonitorPreview(int(req.GetMonitor()))

	return &screensharev1.StopMonitorPreviewResponse{}, nil
}

// SetReceiveAudio sets how loud one decode plays and whether it plays at all.
//
// The pair is checked for the reason every receive method checks it.
// The refusal differs: a decode that is not running is NOT_FOUND rather than a quiet success,
// this being a request about something absent rather than for a state that already holds.
// A repeat of a loudness is that state, and succeeds.
//
// The volume is not range-checked here.
// The bound lives at the backend, which brings a figure past the end of the range back,
// and a refusal would turn a slider that overshot into an error a reader has to read.
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

// ProbeEncoders test-encodes on every engine, records what this machine can really run,
// and announces the catalog that result changed.
//
// The announcement is why this is a method rather than a flag on GetCatalog: every shell learns on
// the stream, including the ones that never asked, instead of one shell's read silently moving what
// a different shell's next resolve says.
//
// The catalog is announced unconditionally rather than only where the result moved.
// The probe is cached for the process lifetime, so a second call is answered from the cache and
// publishes a state its subscribers already hold; a duplicate whole-state event is harmless by
// construction, and comparing availabilities to avoid one would be a second definition of when two
// probe results are the same.
func (s *Server) ProbeEncoders(ctx context.Context, req *screensharev1.ProbeEncodersRequest) (*screensharev1.ProbeEncodersResponse, error) {
	s.backend.Encoders(ctx)
	s.events.Publish(wire.CatalogEvent(s.catalog()))

	return &screensharev1.ProbeEncodersResponse{}, nil
}

// StartTestStreams launches synthetic publishers, replacing a running set.
//
// A count that is not positive is INVALID_ARGUMENT: it names a set of publishers that cannot exist,
// and stopping them has a method of its own.
// A count above MaxTestStreams is RESOURCE_EXHAUSTED: each test stream runs its own software
// encoder, so a large set saturates the machine without testing anything new.
//
// Both refusals are made above the call, where every request-earned refusal in this file is made.
// Inferring the bound from the backend's error would put a viewer binary that could not be found
// under RESOURCE_EXHAUSTED as well, and a missing binary is not a saturated machine.
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

// StopTestStreams stops every synthetic publisher, and refuses nothing.
func (s *Server) StopTestStreams(ctx context.Context, req *screensharev1.StopTestStreamsRequest) (*screensharev1.StopTestStreamsResponse, error) {
	s.backend.StopTestStreams()

	return &screensharev1.StopTestStreamsResponse{}, nil
}

// ForgetPortalConsent drops the stored screen-capture consent, so the next capture asks the
// compositor to pick again.
// How a share aimed at the wrong window or monitor is corrected.
func (s *Server) ForgetPortalConsent(ctx context.Context, req *screensharev1.ForgetPortalConsentRequest) (*screensharev1.ForgetPortalConsentResponse, error) {
	if err := s.backend.ForgetPortalConsent(); err != nil {
		return nil, fromBackend("cannot forget the screen-capture consent", err)
	}
	return &screensharev1.ForgetPortalConsentResponse{}, nil
}

// OpenLog opens one run log in the machine's default application.
//
// The path comes off an ExitInfo the backend handed out and a shell constructs none, so an empty
// path is INVALID_ARGUMENT, the exit having carried no log, and a path that is not there is
// NOT_FOUND.
// The second check is made here rather than left to the default handler, which cannot tell the two
// apart: run logs are rotated, so a log named by an older exit is a file that existed and has since
// gone, and "no log file at this path" says so.
//
// A second call opens a second window, the departure OpenInBrowser documents.
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

// OpenLogsFolder opens the directory holding the run logs in the machine's file browser,
// and opens a second window on a second call, as OpenLog does.
func (s *Server) OpenLogsFolder(ctx context.Context, req *screensharev1.OpenLogsFolderRequest) (*screensharev1.OpenLogsFolderResponse, error) {
	if err := s.backend.OpenLogsFolder(); err != nil {
		return nil, fromBackend("cannot open the logs folder", err)
	}
	return &screensharev1.OpenLogsFolderResponse{}, nil
}
