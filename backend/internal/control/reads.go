package control

import (
	"context"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/form"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The reads: hand the shell something to draw, compute, change nothing
// (docs/ipc-api.md, "The rule").
//
// None states a precondition of its own.
// Each forwards to the backend and shapes the answer for the wire,
// so the contract belongs to what it forwards to.
// The one condition under all of them, the backend the service holds, is asserted in server.go.
// None reads a request field either: a read takes no argument it could refuse,
// so every one is safe to call at any moment.
//
// A shell resolves a form on every keystroke and reads the publish state on every mount,
// so a read that saved, started or probed would fire at a rate nobody chose.
// ResolveForm is the sharp case, and its contract states both halves: no save,
// and the probe result read rather than taken.
//
// No sentence for the screen is built here:
// labels, greyings and derived figures are form's, and the shapes on the wire are wire's.
// Here is which of the two to ask, and what to hand it.

// GetCatalog answers with every fixed fact known about this machine and the encoding model.
//
// It reads the probe result.
// A flag that started the probe would replace what another shell's next resolve answers from,
// with nothing on the wire announcing it.
// Probing is ProbeEncoders, and what it finds reaches every shell on the event stream.
func (s *Server) GetCatalog(ctx context.Context, req *screensharev1.GetCatalogRequest) (*screensharev1.GetCatalogResponse, error) {
	return &screensharev1.GetCatalogResponse{Catalog: s.catalog()}, nil
}

// catalog builds the reference set from what is known.
// One function, because this read and the event ProbeEncoders announces carry the same catalog.
// Two builders would draw one table for the shell that asked for the probe,
// another for the shell that was told about it.
func (s *Server) catalog() *screensharev1.Catalog {
	catalog := wire.Catalog(wire.CatalogInput{
		Platform:     s.backend.Platform(),
		Monitors:     s.backend.Monitors(),
		Encoders:     s.backend.CachedEncoders(),
		AudioDevices: s.backend.AudioDevices(),
	})

	assert.IsNotNil(catalog, "a read of the reference set answers with one")
	return catalog
}

// GetSettings answers with the settings the backend holds:
// the draft a form starts from, and the values a fresh shell opens on.
//
// The store notice rides here rather than on the catalog.
// Why the persisted settings could not be restored is a fact about the settings,
// so it belongs beside the defaults it explains.
func (s *Server) GetSettings(ctx context.Context, req *screensharev1.GetSettingsRequest) (*screensharev1.GetSettingsResponse, error) {
	return &screensharev1.GetSettingsResponse{
		Settings:    wire.Settings(s.backend.Settings()),
		StoreNotice: s.backend.StoreNotice(),
	}, nil
}

// ResolveForm turns a settings draft into the complete description of the screen.
//
// It reads CachedEncoders.
// The contract promises a resolve cheap enough to call on every keystroke,
// and the probe takes seconds on its first call:
// taking one here would charge those seconds to the first character typed,
// with every keystroke until it returned queued behind it.
// The cached result costs the smaller and honest thing instead:
// on a machine nothing has probed, no engine is greyed for a missing encoder,
// a form that has not been told rather than a form saying there is nothing.
//
// It saves nothing either.
// A form is resolved far more often than a value is committed,
// so a resolve that wrote would make every intermediate keystroke a persisted setting.
//
// A request carrying no settings resolves the empty draft instead of being refused,
// unlike the effects.
// Repair is what a resolve is for:
// it walks a draft the tables forbid to the first legal value and names what it moved,
// and an empty draft is the far end of that same walk.
func (s *Server) ResolveForm(ctx context.Context, req *screensharev1.ResolveFormRequest) (*screensharev1.ResolveFormResponse, error) {
	// Brokered ahead of the resolve, so the audience diagnostic reads Discord mode's membership.
	draft := s.backend.Brokered(wire.ToSettings(req.GetSettings()))
	return &screensharev1.ResolveFormResponse{Form: form.Resolve(s.formDeps(), draft)}, nil
}

// formDeps is what a resolve reads off this machine, one builder for every caller:
// the form read and the start that resolves a followed preset (effects.go)
// answer from the same machine.
func (s *Server) formDeps() form.Deps {
	return form.Deps{
		Monitors:     s.backend.Monitors(),
		Platform:     s.backend.Platform(),
		Device:       s.backend.Device(),
		Encoders:     s.backend.CachedEncoders(),
		AudioDevices: s.backend.AudioDevices(),
		Portal:       s.backend.PortalCapabilities(),
	}
}

// ListPresets answers with the user's saved configurations.
//
// A store that could not be read is an Umgebungsfehler.
// The list comes back empty carrying the notice,
// because nothing-readable-remained and nothing-was-saved are different facts about the machine,
// and the difference is the user's to see:
// the unreadable file has been moved aside by then, and the notice names where the presets went.
// A status error would replace both facts with "the call failed".
//
// The store is reached directly rather than through Backend.
// Presets are a file, held by nothing in the running app and announced by no event,
// so a Backend method would forward and nothing else.
func (s *Server) ListPresets(ctx context.Context, req *screensharev1.ListPresetsRequest) (*screensharev1.ListPresetsResponse, error) {
	presets, err := settings.LoadPresets()

	var notice *screensharev1.Text
	if err != nil {
		logger.Warnf("control: presets not restored: %v", err)
		notice = settings.StoreNotice(
			screensharev1.TextCode_TEXT_CODE_PRESET_STORE_UNREADABLE, err)
	}

	return &screensharev1.ListPresetsResponse{Presets: wire.Presets(presets), Notice: notice}, nil
}

// GetPublishState says whether a stream is in force,
// and whether the held settings have moved off it.
// A shell reads it on mount and receives the same message on the event stream after that,
// so a window that has just opened and one that has been open show the same thing.
func (s *Server) GetPublishState(ctx context.Context, req *screensharev1.GetPublishStateRequest) (*screensharev1.PublishState, error) {
	return wire.PublishState(s.backend.PublishState()), nil
}

// GetRelayStatus answers with the latest relay snapshot, and always succeeds.
//
// An unreachable relay is a snapshot whose reachable is false, carrying the reason
// (errors.go, and docs/ipc-api.md, "Errors").
// "The relay is down" is a thing the screen has to say,
// and a call that failed would leave the shell nothing to say it with.
//
// The backend owns the polling,
// so several shells reading do not multiply the requests to the relay,
// and the byte-delta bitrates stay computed against one steady interval
// rather than whatever cadence each shell chose.
func (s *Server) GetRelayStatus(ctx context.Context, req *screensharev1.GetRelayStatusRequest) (*screensharev1.RelayStatus, error) {
	return wire.RelayStatus(s.backend.RelayStatus()), nil
}

// GetViewerState answers with the open external viewers,
// one entry per stream and transport it is received over.
// A shell reads it on mount and receives the same message on the event stream after that.
func (s *Server) GetViewerState(ctx context.Context, req *screensharev1.GetViewerStateRequest) (*screensharev1.ViewerState, error) {
	return wire.ViewerState(s.backend.Watching()), nil
}

// GetReceiveState answers with the streams being decoded,
// one entry per stream and leg it is received over.
// A shell reads it on mount and receives the same message on the event stream after that.
func (s *Server) GetReceiveState(ctx context.Context, req *screensharev1.GetReceiveStateRequest) (*screensharev1.ReceiveState, error) {
	return wire.ReceiveState(s.backend.ReceiveState()), nil
}

// GetMonitorPreviewState answers with the monitors being read,
// what a shell that has just connected converges against.
// A preview outlives the window that asked for it,
// so a shell that crashed leaves screens captured for nobody.
func (s *Server) GetMonitorPreviewState(ctx context.Context, req *screensharev1.GetMonitorPreviewStateRequest) (*screensharev1.MonitorPreviewState, error) {
	return wire.MonitorPreviewState(s.backend.MonitorPreviewState()), nil
}

// GetTestStreamState counts the synthetic publishers alive:
// one that died on its own drops out.
// The slots travel beside the count, so a set with one dead publisher names the slot.
func (s *Server) GetTestStreamState(ctx context.Context, req *screensharev1.GetTestStreamStateRequest) (*screensharev1.TestStreamState, error) {
	running, slots := s.backend.TestStreamState()
	return wire.TestStreamState(running, slots...), nil
}

// GetMembersState answers with who this machine shares a group with,
// as the presence loop last read it.
//
// A read of a reading, stating no presence of its own:
// the loop polling the relay is the heartbeat,
// and a shell asking would be a second thing deciding when this machine is in its group.
// A shell reads it on mount and receives the same message on the event stream after that.
func (s *Server) GetMembersState(ctx context.Context, req *screensharev1.GetMembersStateRequest) (*screensharev1.MembersState, error) {
	return wire.MembersState(s.backend.MembersState()), nil
}

// GetDiscordState answers Discord mode as the last manager pass landed it, the event's read twin.
func (s *Server) GetDiscordState(ctx context.Context, req *screensharev1.GetDiscordStateRequest) (*screensharev1.DiscordState, error) {
	return wire.DiscordState(s.backend.DiscordState()), nil
}
