package control

import (
	"context"

	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/form"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// The reads. They hand the shell something to draw, they compute, and they change
// nothing (docs/ipc-api.md, "The rule").
//
// That is not a matter of taste. A shell resolves a form on every keystroke and reads
// the publish state on every mount, so a read that saved, started or probed something
// would fire at a rate nobody chose. Two of them are the sharp cases and the contract
// states both: ResolveForm does not save, and it reads what has been probed rather
// than probing.
//
// None of them formats a sentence for the screen either. The labels, the greyings and
// the derived figures are form's, and the shapes on the wire are wire's; what is here
// is which of the two to ask and what to hand it.

// GetCatalog returns every fixed fact about this machine and the encoding model, as
// it is known now.
//
// It reads the probe result and never takes one, which is what makes it the read the
// contract says it is. It used to carry a probe_encoders flag, and the flag was the
// defect: one shell's request replaced the result a different shell's next resolve
// would answer from, with nothing on the wire to announce that it had. Probing is
// ProbeEncoders, and what it finds reaches every shell on the event stream.
func (s *Server) GetCatalog(ctx context.Context, req *screensharev1.GetCatalogRequest) (*screensharev1.GetCatalogResponse, error) {
	return &screensharev1.GetCatalogResponse{Catalog: s.catalog()}, nil
}

// catalog builds the reference set from what is known now. It is one function because
// the read above and the event ProbeEncoders announces have to be the same catalog:
// two builders would be two answers to one question, and the shell that asked for the
// probe and the shell that was told about it would draw different tables.
func (s *Server) catalog() *screensharev1.Catalog {
	return wire.Catalog(wire.CatalogInput{
		Platform: s.backend.Platform(),
		Monitors: s.backend.Monitors(),
		Encoders: s.backend.CachedEncoders(),
	})
}

// GetSettings returns the settings the backend holds: the starting draft for a form
// and the values a fresh shell opens on.
//
// The store notice rides here rather than on the catalog. Why the persisted settings
// could not be restored is a fact about the settings, and it belongs beside the
// defaults it explains rather than inside a message the contract calls every fixed
// fact about this machine.
func (s *Server) GetSettings(ctx context.Context, req *screensharev1.GetSettingsRequest) (*screensharev1.GetSettingsResponse, error) {
	return &screensharev1.GetSettingsResponse{
		Settings:    wire.Settings(s.backend.Settings()),
		StoreNotice: s.backend.StoreNotice(),
	}, nil
}

// ResolveForm turns a settings draft into the complete description of the screen.
//
// It reads CachedEncoders and never Encoders, and that is the one line in this file
// worth reading twice. The contract promises a resolve is cheap enough to call on
// every keystroke; the probe takes seconds on its first call. Reaching for the probe
// here would make the first character a user typed cost those seconds, and every
// keystroke until it returned would queue behind it. What the cached result costs
// instead is honest and small: on a machine nothing has probed yet, no engine is
// greyed for a missing encoder, which is the difference between a form that has not
// been told and a form that says there is nothing.
//
// It also does not save. A form is resolved far more often than a value is committed,
// and a resolve that wrote would make every intermediate keystroke a persisted
// setting.
//
// A request that carries no settings resolves the empty draft rather than being
// refused, unlike the effects below. Repair is what a resolve is for: it walks a draft
// the tables forbid to the first legal value and names what it moved, and a draft with
// nothing in it is the far end of that same walk rather than a different question.
func (s *Server) ResolveForm(ctx context.Context, req *screensharev1.ResolveFormRequest) (*screensharev1.ResolveFormResponse, error) {
	deps := form.Deps{
		Monitors: s.backend.Monitors(),
		Platform: s.backend.Platform(),
		Encoders: s.backend.CachedEncoders(),
	}
	draft := wire.StreamSettingsOnto(s.backend.Settings(), req.GetSettings())
	return &screensharev1.ResolveFormResponse{Form: form.Resolve(deps, draft)}, nil
}

// ListPresets returns the user's saved configurations.
//
// A store that could not be read is not a failed call. The list comes back empty with
// the notice saying why, because empty-because-nothing-readable-remained and
// empty-because-nothing-was-saved are different facts about the user's machine and the
// difference is theirs to see - the unreadable file has been moved aside by then, and
// the sentence names where their presets went. A status error here would replace both
// facts with "the call failed".
//
// The store is reached directly rather than through Backend. Presets are a file and
// not state the backend owns: nothing in the running app holds them, no event
// announces them, and a Backend method for them would be a method that only forwards.
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

// GetPublishState reports whether a stream is in force and whether the held settings
// have moved off it. A shell reads it when it mounts and receives the same message on
// the event stream thereafter, which is what stops a window that has just opened and
// one that has been open from showing different things.
func (s *Server) GetPublishState(ctx context.Context, req *screensharev1.GetPublishStateRequest) (*screensharev1.PublishState, error) {
	return wire.PublishState(s.backend.PublishState()), nil
}

// GetRelayStatus returns the latest relay snapshot, and always succeeds.
//
// An unreachable relay is a snapshot whose reachable is false carrying the reason, and
// never a status error. The contract states it on its own for a reason (errors.go, and
// docs/ipc-api.md, "Errors"): "the relay is down" is a thing the screen has to say, and
// a call that failed instead would leave the shell with nothing to say it with.
//
// The snapshot is read rather than fetched. The backend owns the polling, so several
// shells reading this do not multiply the requests to the relay, and the byte-delta
// bitrates it carries stay computed against one steady interval instead of against
// whatever cadence each shell chose.
func (s *Server) GetRelayStatus(ctx context.Context, req *screensharev1.GetRelayStatusRequest) (*screensharev1.RelayStatus, error) {
	return wire.RelayStatus(s.backend.RelayStatus()), nil
}

// GetViewerState returns the external viewers currently open, one entry per stream and
// transport it is received over. A shell reads it when it mounts and receives the same
// message on the event stream thereafter.
func (s *Server) GetViewerState(ctx context.Context, req *screensharev1.GetViewerStateRequest) (*screensharev1.ViewerState, error) {
	return wire.ViewerState(s.backend.Watching()), nil
}

// GetTestStreamState reports how many synthetic publishers are alive, which is not the
// count that was asked for: one that died on its own drops out of it.
func (s *Server) GetTestStreamState(ctx context.Context, req *screensharev1.GetTestStreamStateRequest) (*screensharev1.TestStreamState, error) {
	return wire.TestStreamState(s.backend.TestStreamsRunning()), nil
}

// GetMoqCert reads the relay's Media-over-QUIC certificate fingerprint for one stream,
// so a browser tile can pin it through WebTransport's serverCertificateHashes.
//
// An empty stream name is INVALID_ARGUMENT: the fingerprint endpoint is per stream, so
// a request without one names a certificate that cannot exist. A relay that could not
// be reached for it is UNAVAILABLE, which is the one place in the reads where an
// unreachable relay is a call failure rather than a snapshot - GetRelayStatus describes
// the relay and can describe it as down, while this one has a single value to return
// and no honest way to return it.
//
// The endpoint the pin belongs to travels with the certificate rather than being
// composed here, on the rule the rest of the viewer side follows: a host or port the
// user changed reaches the next tile without any shell holding a copy of it, and
// without this service holding a second one either.
func (s *Server) GetMoqCert(ctx context.Context, req *screensharev1.GetMoqCertRequest) (*screensharev1.GetMoqCertResponse, error) {
	name := req.GetStreamName()
	if name == "" {
		return nil, invalidArgument("no stream named for the MoQ certificate")
	}

	cert, url, err := s.backend.MoqCert(ctx, name)
	if err != nil {
		return nil, unavailable("cannot read the MoQ certificate for '%s': %v", name, err)
	}

	return &screensharev1.GetMoqCertResponse{Cert: wire.MoqCert(cert, url)}, nil
}
