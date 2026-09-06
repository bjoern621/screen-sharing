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

	"bjoernblessin.de/screenshare/internal/form"
	"bjoernblessin.de/screenshare/internal/publish"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The effects.
// Each answers with its empty response message:
// what the state became arrives on the event stream (docs/ipc-api.md, "Events").
// One path into the display keeps the shell that pressed the button
// and the one that only listened from showing different things.
//
// Reading state back here for a richer answer is the same fault:
// a read after the effect is already one event out of date.

// fromBackend turns an untyped backend error into UNAVAILABLE.
//
// One helper rather than a judgement per call site.
// Backend errors carry no type, so coding them off the text would derive the error model
// from a sentence written for a person, the one input that changes without breaking the build.
//
// UNAVAILABLE is the residue.
// Every refusal the request itself earns is made above the backend call, against the arguments
// or the state: an empty name, a settings message carrying nothing, a count that is not positive,
// a stream already in force, nothing to apply settings to, a log file that is not there.
// An Entwicklungsfehler never arrives either, assert panicking in the backend.
// What is left is the world failing at something legal to ask for: a child that would not start,
// a store that could not be written, a relay that could not be reached.
//
// Cost: a settings combination no engine can build also comes back untyped from StartPublish,
// where INVALID_ARGUMENT would fit better.
// Telling the two apart needs a typed refusal on Backend.
// A guess off the sentence, landing wrong, would tell a shell to blame the user for a full disk.
func fromBackend(what string, err error) error {
	assert.IsNotNil(err, "a classified failure is a failure", what)

	return unavailable("%s: %v", what, err)
}

// draftOf reads the settings off a request that needs them.
//
// An absent message and one carrying nothing are the same request:
// both INVALID_ARGUMENT, rather than a stream with no name, no relay and no codec.
// A shell that forgot to attach its draft would otherwise persist that emptiness over the settings
// and find out on the next launch.
//
// Emptiness is tested against the zero message rather than one chosen field,
// so a field added to the contract stays covered without an edit here.
func draftOf(m *screensharev1.Settings, verb string) (settings.Settings, error) {
	// Verb is this file's word for what the caller was doing, fixed at the call site,
	// so an empty one leaves a hole in the sentence a person reads.
	assert.Assert(verb != "", "a refused draft names what it was going to be used for")

	if m == nil || proto.Equal(m, &screensharev1.Settings{}) {
		return settings.Settings{}, invalidArgument("no settings to %s", verb)
	}
	return wire.ToSettings(m), nil
}

// presetOf reads one saved way of publishing off a request.
// An empty one is refused for the reason draftOf refuses an empty draft.
//
// A preset is a PublishSettings and nothing else: where the relay is belongs to a deployment,
// how this machine watches to a viewer, so neither is part of applying one.
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
// so reaching a live pipeline is ApplyToStream's business.
// Two methods because the user's two intentions differ: keep this for next time,
// and put this on the air.
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
// The store is read before the delete, settings.DeletePreset rewriting the file without the name
// and removing an absent name as successfully as a present one.
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

// resolveFollowed turns a draft that follows a built-in preset into the configuration
// that preset produces on this machine at this moment, and leaves a detached draft alone.
//
// At the press rather than at the form:
// the machine may have changed since the form was drawn, and what was asked for is the promise,
// so the search runs against the machine as it stands.
// A preset nothing here reaches is FAILED_PRECONDITION,
// the verdict the form already blocks the start with,
// so the refusal repeats what the screen says rather than surprising it.
func (s *Server) resolveFollowed(draft settings.Settings) (settings.Settings, error) {
	if !form.Followed(draft) {
		return draft, nil
	}

	resolved, ok := form.ResolveBuiltin(s.formDeps(), draft)
	if !ok {
		return settings.Settings{}, failedPrecondition(
			"no configuration on this machine delivers the selected preset (%s). Open the stream settings to see what stands in the way",
			draft.Publish.Preset)
	}
	return resolved, nil
}

// StartPublish persists the settings and starts the encoder on them.
//
// A start naming a *different* pipeline while a stream is in force is FAILED_PRECONDITION,
// and a pipeline waiting out a retry backoff is in force:
// the user asked for it, has not stopped it, it comes back on its own,
// and the one call that ends a running pipeline ends it too.
// A different start let through in that gap would put two encoders on one relay path seconds apart.
//
// A start naming the pipeline already publishing succeeds and does nothing.
// It is a request for a state that already holds (docs/development-principles.md,
// "Effects across a process boundary"): a shell whose answer went missing cannot tell "not done"
// from "done, answer lost", and asking again is the only move that resolves it.
// publish.SamePipeline decides sameness here and at the backend,
// "these two settings are one stream" being one fact.
func (s *Server) StartPublish(ctx context.Context, req *screensharev1.StartPublishRequest) (*screensharev1.StartPublishResponse, error) {
	draft, err := draftOf(req.GetSettings(), "publish")
	if err != nil {
		return nil, err
	}
	draft, err = s.resolveFollowed(draft)
	if err != nil {
		return nil, err
	}

	if state := s.backend.PublishState(); state.Publishing() {
		if same, err := publish.SamePipeline(state.Live.Settings, draft); err == nil && same {
			return &screensharev1.StartPublishResponse{}, nil
		}
		if retry := state.Retry(); retry != nil {
			return nil, failedPrecondition(
				"a stream is already publishing and is waiting out attempt %d of %d after its pipeline died. Stop it before starting another",
				retry.Attempt, retry.Budget)
		}
		return nil, failedPrecondition("a stream is already publishing. Stop it before starting another")
	}

	if err := s.backend.StartPublish(draft); err != nil {
		return nil, fromBackend("cannot start publishing", err)
	}
	return &screensharev1.StartPublishResponse{}, nil
}

// ApplyToStream restarts the running stream on the settings the request carries.
//
// It names a transition rather than a state,
// the documented departure from the idempotency of every other effect here
// (docs/development-principles.md, "Effects across a process boundary").
// A second apply is a second restart.
//
// Nothing publishing is FAILED_PRECONDITION:
// an apply asks for the live stream to change, and StartPublish is what brings a stopped one back.
func (s *Server) ApplyToStream(ctx context.Context, req *screensharev1.ApplyToStreamRequest) (*screensharev1.ApplyToStreamResponse, error) {
	draft, err := draftOf(req.GetSettings(), "apply")
	if err != nil {
		return nil, err
	}
	draft, err = s.resolveFollowed(draft)
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

// StopPublish ends the stream, running or waiting out a backoff.
//
// It refuses nothing, a stop with nothing publishing included.
// A stop names what is to be true afterwards,
// and where that already holds no precondition is left to fail on.
func (s *Server) StopPublish(ctx context.Context, req *screensharev1.StopPublishRequest) (*screensharev1.StopPublishResponse, error) {
	s.backend.StopPublish()

	return &screensharev1.StopPublishResponse{}, nil
}

// StartWatch opens an external viewer for one stream over one transport.
// Half a pair is INVALID_ARGUMENT, a leg that cannot carry the stream's format FAILED_PRECONDITION.
//
// The transport is per viewer and independent of the publish leg,
// so one stream is watchable over any leg the relay serves it on.
func (s *Server) StartWatch(ctx context.Context, req *screensharev1.StartWatchRequest) (*screensharev1.StartWatchResponse, error) {
	ref := wire.StreamRefOf(req.GetViewer())
	name, leg := ref.StreamName, ref.Transport
	if name == "" {
		return nil, invalidArgument("no stream named to watch")
	}
	if leg == "" {
		return nil, invalidArgument("no transport named to watch '%s' over", name)
	}

	if err := s.backend.StartWatch(ref); err != nil {
		// Carriage refusal: the world not ready.
		// The relay re-serves a stream only on listeners whose protocol carries a payload mapping
		// for its bitstream, so an SRT viewer opened on a VP9 stream connects and receives nothing.
		// Both halves exist: the leg one this build serves, the stream one the relay carries.
		// What stands in the way is the publish format at this instant.
		// Republished as H.264, the same request succeeds.
		// The backend's sentence names the format and the legs that do carry it,
		// so the reason reaches the user intact.
		//
		// A transport this build has no viewer for is INVALID_ARGUMENT under the contract's table,
		// and the backend separates the two by returning a Refused for it.
		if refused(err) {
			return nil, invalidArgument("cannot watch '%s' over %s: %v", name, leg, err)
		}
		return nil, failedPrecondition("cannot watch '%s' over %s: %v", name, leg, err)
	}
	return &screensharev1.StartWatchResponse{}, nil
}

// StopWatch closes one open viewer.
//
// The pair is checked for the reason StreamRef exists: one stream is watched over several transports
// at once, so half an identity names a viewer that cannot exist, and is INVALID_ARGUMENT.
// A complete pair with no viewer open succeeds, on the ground StopPublish does.
func (s *Server) StopWatch(ctx context.Context, req *screensharev1.StopWatchRequest) (*screensharev1.StopWatchResponse, error) {
	ref := wire.StreamRefOf(req.GetViewer())
	if ref.StreamName == "" {
		return nil, invalidArgument("no stream named to stop watching")
	}
	if ref.Transport == "" {
		return nil, invalidArgument("no transport named to stop watching '%s' over", ref.StreamName)
	}

	s.backend.StopWatch(ref)

	return &screensharev1.StopWatchResponse{}, nil
}

// OpenInBrowser opens the relay's player page for one stream in the machine's default browser.
//
// The pair and its refusals are StartWatch's, for the same reasons: the leg is per reader,
// and one the stream's format does not cross opens a page that connects and shows nothing.
//
// A second call opens a second page, the documented departure from idempotency
// (docs/development-principles.md, "Effects across a process boundary"):
// the effect lands in a program this process does not own, so there is no state to read back,
// no stop to write, and nothing here reaches the viewer state.
func (s *Server) OpenInBrowser(ctx context.Context, req *screensharev1.OpenInBrowserRequest) (*screensharev1.OpenInBrowserResponse, error) {
	ref := wire.StreamRefOf(req.GetViewer())
	name, leg := ref.StreamName, ref.Transport
	if name == "" {
		return nil, invalidArgument("no stream named to open in the browser")
	}
	if leg == "" {
		return nil, invalidArgument("no transport named to open '%s' over", name)
	}

	if err := s.backend.OpenInBrowser(ref); err != nil {
		return nil, failedPrecondition("cannot open '%s' over %s in the browser: %v", name, leg, err)
	}
	return &screensharev1.OpenInBrowserResponse{}, nil
}

// StartReceive opens a decode for one stream on one leg, inside the backend.
// A decode already open on that pair is the state the call names, and succeeds.
//
// The empty-argument refusals and the carriage one are StartWatch's, for the same reasons:
// the pair is the identity a decode is keyed by, and a leg that cannot carry the stream's format
// is the world not being ready rather than a malformed request.
// What differs is the engine the carriage is asked about,
// a receive pipeline reaching WHEP where no player does, and the backend is what asks.
//
// Tone mapping is not checked here.
// Whether the stream carries more range than a display shows is known once the decoder negotiates
// and not before, and whether this machine can roll it down is the backend's registry,
// so a refusal here would guess at both.
func (s *Server) StartReceive(ctx context.Context, req *screensharev1.StartReceiveRequest) (*screensharev1.StartReceiveResponse, error) {
	ref := wire.StreamRefOf(req.GetStream())
	name, leg := ref.StreamName, ref.Transport
	if name == "" {
		return nil, invalidArgument("no stream named to receive")
	}
	if leg == "" {
		return nil, invalidArgument("no transport named to receive '%s' over", name)
	}

	if err := s.backend.StartReceive(ref, req.GetToneMap()); err != nil {
		return nil, failedPrecondition("cannot receive '%s' over %s: %v", name, leg, err)
	}
	return &screensharev1.StartReceiveResponse{}, nil
}

// StopReceive closes one running decode.
// The pair is checked, and a pair nothing is decoding succeeds, both for StopWatch's reasons.
func (s *Server) StopReceive(ctx context.Context, req *screensharev1.StopReceiveRequest) (*screensharev1.StopReceiveResponse, error) {
	ref := wire.StreamRefOf(req.GetStream())
	if ref.StreamName == "" {
		return nil, invalidArgument("no stream named to stop receiving")
	}
	if ref.Transport == "" {
		return nil, invalidArgument("no transport named to stop receiving '%s' over", ref.StreamName)
	}

	s.backend.StopReceive(ref)

	return &screensharev1.StopReceiveResponse{}, nil
}

// StartMonitorPreview reads one of this machine's screens, so the wizard offers it by its picture
// rather than by its number.
// A monitor already being previewed is the state the call names, and succeeds.
//
// The index is not checked here, unlike the pair the receive methods take.
// An empty stream name shows as a hole from the message alone, while every integer is a monitor
// index somewhere, and whether it is one of this machine's is a fact about the machine.
// The backend answers that, a Refused separating the index naming no output of this machine
// (INVALID_ARGUMENT) from a session that cannot read one screen apart from another
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
// A monitor nothing is previewing succeeds, as StopReceive takes a decode already closed.
func (s *Server) StopMonitorPreview(ctx context.Context, req *screensharev1.StopMonitorPreviewRequest) (*screensharev1.StopMonitorPreviewResponse, error) {
	s.backend.StopMonitorPreview(int(req.GetMonitor()))

	return &screensharev1.StopMonitorPreviewResponse{}, nil
}

// SetReceiveAudio sets how loud one decode plays and whether it plays at all.
//
// The pair is checked for the reason every receive method checks it.
// The refusal differs: a decode that is not running is NOT_FOUND rather than a quiet success,
// being a request about something absent rather than for a state that already holds.
// A repeat of a loudness is that state, and succeeds.
//
// The volume is not range-checked here.
// The bound lives at the backend, which brings a figure past the end of the range back,
// and a refusal would turn a slider that overshot into an error a reader has to read.
func (s *Server) SetReceiveAudio(ctx context.Context, req *screensharev1.SetReceiveAudioRequest) (*screensharev1.SetReceiveAudioResponse, error) {
	ref := wire.StreamRefOf(req.GetStream())
	if ref.StreamName == "" {
		return nil, invalidArgument("no stream named to set the audio of")
	}
	if ref.Transport == "" {
		return nil, invalidArgument("no transport named to set the audio of '%s' over", ref.StreamName)
	}

	if err := s.backend.SetReceiveAudio(ref, req.GetVolume(), req.GetMuted()); err != nil {
		return nil, notFound("cannot set the audio of '%s' over %s: %v", ref.StreamName, ref.Transport, err)
	}

	return &screensharev1.SetReceiveAudioResponse{}, nil
}

// ProbeEncoders test-encodes on every engine, records what this machine can really run,
// and announces the catalog that result changed.
//
// A method rather than a flag on GetCatalog, for the announcement: every shell learns on the stream,
// the ones that never asked included, instead of one shell's read moving what a different shell's
// next resolve says.
//
// The catalog is announced unconditionally rather than only where the result moved.
// The probe is cached for the process lifetime, so a second call is answered from the cache
// and publishes a state its subscribers already hold.
// A duplicate whole-state event is harmless by construction, and comparing availabilities to avoid
// one would be a second definition of when two probe results are the same.
func (s *Server) ProbeEncoders(ctx context.Context, req *screensharev1.ProbeEncodersRequest) (*screensharev1.ProbeEncodersResponse, error) {
	s.backend.Encoders(ctx)
	s.events.Publish(wire.CatalogEvent(s.catalog()))

	return &screensharev1.ProbeEncodersResponse{}, nil
}

// StartTestStreams launches synthetic publishers, replacing a running set.
//
// A count that is not positive is INVALID_ARGUMENT, naming a set of publishers that cannot exist,
// and stopping them has a method of its own.
// A count above MaxTestStreams is RESOURCE_EXHAUSTED: each test stream runs its own software
// encoder, so a large set saturates the machine and tests nothing further.
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

// ForgetPortalConsent drops the stored screen-capture consent,
// so the next capture asks the compositor to pick again.
// How a share aimed at the wrong window or monitor is corrected.
func (s *Server) ForgetPortalConsent(ctx context.Context, req *screensharev1.ForgetPortalConsentRequest) (*screensharev1.ForgetPortalConsentResponse, error) {
	if err := s.backend.ForgetPortalConsent(); err != nil {
		return nil, fromBackend("cannot forget the screen-capture consent", err)
	}
	return &screensharev1.ForgetPortalConsentResponse{}, nil
}

// CreateGroup draws a group key at the relay named in the request and answers it.
//
// Nothing is stored: the group key goes back to the shell.
// The shell writes it to the group key field like a value the user typed,
// so the one write moving a machine between groups stays where every other settings write is.
// A relay with no group service and an unreachable one are both the caller's to fix,
// so each leaves as the backend's own sentence rather than as a group key drawn nowhere.
func (s *Server) CreateGroup(ctx context.Context, req *screensharev1.CreateGroupRequest) (*screensharev1.CreateGroupResponse, error) {
	groupKey, groupID, err := s.backend.CreateGroup(wire.ToRelay(req.GetRelay()))
	if err != nil {
		return nil, fromBackend("cannot draw a group key", err)
	}
	return &screensharev1.CreateGroupResponse{Key: groupKey, Id: groupID}, nil
}

// LinkDiscord runs the consent flow and answers when the link landed or did not.
// The secret lands in the stored settings, so the answer carries nothing to show.
func (s *Server) LinkDiscord(ctx context.Context, req *screensharev1.LinkDiscordRequest) (*screensharev1.LinkDiscordResponse, error) {
	if err := s.backend.LinkDiscord(ctx, wire.ToRelay(req.GetRelay())); err != nil {
		return nil, fromBackend("cannot link Discord", err)
	}
	return &screensharev1.LinkDiscordResponse{}, nil
}

// OpenLog opens one run log in the machine's default application.
//
// The path comes off an ExitInfo the backend handed out and a shell constructs none,
// so an empty path is INVALID_ARGUMENT, the exit having carried no log,
// and a path that is not there is NOT_FOUND.
// The second check is made here rather than left to the default handler, which cannot tell the two
// apart: run logs are rotated, so a log named by an older exit is a file that has since gone,
// and "no log file at this path" says so.
//
// A second call opens a second window, the departure OpenInBrowser documents.
func (s *Server) OpenLog(ctx context.Context, req *screensharev1.OpenLogRequest) (*screensharev1.OpenLogResponse, error) {
	path := req.GetPath()
	if path == "" {
		return nil, invalidArgument("no log file for this run")
	}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, notFound("no log file at %s. The run logs are rotated, so one named by an older exit may already be gone", path)
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

// SendReport bundles this machine's facts and run logs and delivers them
// to the group service beside the stored relay.
// The name it landed under answers to the caller, the measurements' exception (control.proto).
// A second call stores a second report, the departure OpenInBrowser documents.
func (s *Server) SendReport(ctx context.Context, req *screensharev1.SendReportRequest) (*screensharev1.SendReportResponse, error) {
	id, err := s.backend.SendReport()
	if err != nil {
		return nil, fromBackend("cannot send the report", err)
	}

	assert.Assert(id != "", "a delivered report carries the name it was stored under")
	return &screensharev1.SendReportResponse{ReportId: id}, nil
}

// CheckUpdate reads the published release, and fetches it where this install replaces its own files.
//
// Returns as soon as the work is under way, the download outliving the call.
// What it finds arrives on the event stream, so the shell that asked and the shell that did not
// are told the same thing.
//
// Refused where the install asks the release service nothing at all,
// which the state says before a shell draws a control to ask with.
func (s *Server) CheckUpdate(ctx context.Context, req *screensharev1.CheckUpdateRequest) (*screensharev1.CheckUpdateResponse, error) {
	if err := s.backend.CheckUpdate(); err != nil {
		// A well-formed request whose moment is wrong: this install asks nothing,
		// which UpdateState.unchecked stated before any control was drawn.
		return nil, failedPrecondition("cannot check for updates: %v", err)
	}
	return &screensharev1.CheckUpdateResponse{}, nil
}

// InstallUpdate starts the staged release and leaves the running app to close.
//
// It answers while the app is still up: the applier is a process of its own that waits for this
// one to exit before it replaces anything (internal/update).
// Refused where nothing is staged and verified.
func (s *Server) InstallUpdate(ctx context.Context, req *screensharev1.InstallUpdateRequest) (*screensharev1.InstallUpdateResponse, error) {
	if err := s.backend.InstallUpdate(); err != nil {
		// FAILED_PRECONDITION for the reason CheckUpdate answers with one:
		// nothing is staged, which the state says before the dialog offers a restart.
		return nil, failedPrecondition("cannot install the staged release: %v", err)
	}
	return &screensharev1.InstallUpdateResponse{}, nil
}
