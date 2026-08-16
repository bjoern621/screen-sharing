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

// The reads: they hand the shell something to draw, they compute, and they change nothing
// (docs/ipc-api.md, "The rule").
//
// None states a precondition of its own.
// Each forwards to the backend and shapes the answer for the wire, so the contract belongs to what
// it forwards to, and the single condition under all of them is the backend the service holds,
// asserted where the service is built (server.go).
// The other half is that none reads a request field: a read takes no argument it could refuse,
// which is what leaves every one of them safe to call at any moment.
//
// A shell resolves a form on every keystroke and reads the publish state on every mount, so a read
// that saved, started or probed would fire at a rate nobody chose.
// ResolveForm is the sharp case, and the contract states both halves of it: no save, and the probe
// result read rather than taken.
//
// No sentence for the screen is built here either.
// Labels, greyings and derived figures are form's, and the shapes on the wire are wire's; what is
// here is which of the two to ask, and what to hand it.

// GetCatalog answers with every fixed fact about this machine and the encoding model, as far as
// they are known.
//
// It reads the probe result and never takes one, which is what keeps it the read the contract says
// it is: a flag that started the probe would let one shell's request replace what a different
// shell's next resolve answers from, with nothing on the wire announcing it.
// Probing is ProbeEncoders, and what it finds reaches every shell on the event stream.
func (s *Server) GetCatalog(ctx context.Context, req *screensharev1.GetCatalogRequest) (*screensharev1.GetCatalogResponse, error) {
	return &screensharev1.GetCatalogResponse{Catalog: s.catalog()}, nil
}

// catalog builds the reference set from what is known.
// One function, because the read above and the event ProbeEncoders announces carry the same
// catalog: two builders would be two answers to one question, and the shell that asked for the
// probe would draw a different table from the shell that was told about it.
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

// GetSettings answers with the settings the backend holds: the draft a form starts from, and the
// values a fresh shell opens on.
//
// The store notice rides here rather than on the catalog.
// Why the persisted settings could not be restored is a fact about the settings, so it belongs
// beside the defaults it explains rather than inside the message the contract calls every fixed
// fact about this machine.
func (s *Server) GetSettings(ctx context.Context, req *screensharev1.GetSettingsRequest) (*screensharev1.GetSettingsResponse, error) {
	return &screensharev1.GetSettingsResponse{
		Settings:    wire.Settings(s.backend.Settings()),
		StoreNotice: s.backend.StoreNotice(),
	}, nil
}

// ResolveForm turns a settings draft into the complete description of the screen.
//
// It reads CachedEncoders and never Encoders.
// The contract promises a resolve cheap enough to call on every keystroke, and the probe takes
// seconds on its first call: taking one here would charge those seconds to the first character
// typed, with every keystroke until it returned queued behind it.
// The cached result costs the honest and smaller thing instead: on a machine nothing has probed, no
// engine is greyed for a missing encoder, which is a form that has not been told rather than a form
// saying there is nothing.
//
// It saves nothing either.
// A form is resolved far more often than a value is committed, and a resolve that wrote would make
// every intermediate keystroke a persisted setting.
//
// A request carrying no settings resolves the empty draft instead of being refused, unlike the
// effects.
// Repair is what a resolve is for: it walks a draft the tables forbid to the first legal value and
// names what it moved, and an empty draft is the far end of that same walk rather than a different
// question.
func (s *Server) ResolveForm(ctx context.Context, req *screensharev1.ResolveFormRequest) (*screensharev1.ResolveFormResponse, error) {
	deps := form.Deps{
		Monitors:     s.backend.Monitors(),
		Platform:     s.backend.Platform(),
		Device:       s.backend.Device(),
		Encoders:     s.backend.CachedEncoders(),
		AudioDevices: s.backend.AudioDevices(),
	}
	draft := wire.ToSettings(req.GetSettings())
	return &screensharev1.ResolveFormResponse{Form: form.Resolve(deps, draft)}, nil
}

// ListPresets answers with the user's saved configurations.
//
// A store that could not be read is an Umgebungsfehler, and not a failed call.
// The list comes back empty carrying the notice, because nothing-readable-remained and
// nothing-was-saved are different facts about the user's machine and the difference is theirs to
// see: the unreadable file has been moved aside by then, and the notice names where the presets
// went.
// A status error would replace both facts with "the call failed".
//
// The store is reached directly rather than through Backend.
// Presets are a file and not state the backend owns - nothing in the running app holds them and no
// event announces them - so a Backend method would forward and nothing else.
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

// GetPublishState says whether a stream is in force, and whether the held settings have moved off
// it.
// A shell reads it on mount and receives the same message on the event stream after that, which is
// what stops a window that has just opened and one that has been open showing different things.
func (s *Server) GetPublishState(ctx context.Context, req *screensharev1.GetPublishStateRequest) (*screensharev1.PublishState, error) {
	return wire.PublishState(s.backend.PublishState()), nil
}

// GetRelayStatus answers with the latest relay snapshot, and always succeeds.
//
// An unreachable relay is a snapshot whose reachable is false, carrying the reason, and never a
// status error (errors.go, and docs/ipc-api.md, "Errors").
// "The relay is down" is a thing the screen has to say, and a call that failed instead would leave
// the shell nothing to say it with.
//
// The snapshot is read and not fetched.
// The backend owns the polling, so several shells reading do not multiply the requests to the
// relay, and the byte-delta bitrates stay computed against one steady interval instead of whatever
// cadence each shell chose.
func (s *Server) GetRelayStatus(ctx context.Context, req *screensharev1.GetRelayStatusRequest) (*screensharev1.RelayStatus, error) {
	return wire.RelayStatus(s.backend.RelayStatus()), nil
}

// GetViewerState answers with the open external viewers, one entry per stream and transport it is
// received over.
// A shell reads it on mount and receives the same message on the event stream after that.
func (s *Server) GetViewerState(ctx context.Context, req *screensharev1.GetViewerStateRequest) (*screensharev1.ViewerState, error) {
	return wire.ViewerState(s.backend.Watching()), nil
}

// GetReceiveState answers with the streams being decoded, one entry per stream and leg it is
// received over.
// A shell reads it on mount and receives the same message on the event stream after that.
func (s *Server) GetReceiveState(ctx context.Context, req *screensharev1.GetReceiveStateRequest) (*screensharev1.ReceiveState, error) {
	return wire.ReceiveState(s.backend.ReceiveState()), nil
}

// GetMonitorPreviewState answers with the monitors being read, which is what a shell that has just
// connected converges against.
// A preview outlives the window that asked for it, so a shell that crashed leaves screens captured
// for nobody.
func (s *Server) GetMonitorPreviewState(ctx context.Context, req *screensharev1.GetMonitorPreviewStateRequest) (*screensharev1.MonitorPreviewState, error) {
	return wire.MonitorPreviewState(s.backend.MonitorPreviewState()), nil
}

// GetTestStreamState counts the synthetic publishers alive, which is not the count that was asked
// for: one that died on its own drops out.
func (s *Server) GetTestStreamState(ctx context.Context, req *screensharev1.GetTestStreamStateRequest) (*screensharev1.TestStreamState, error) {
	return wire.TestStreamState(s.backend.TestStreamsRunning()), nil
}
