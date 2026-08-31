package control

import (
	"context"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/wire"
)

// The measurements.
// Both run the real thing instead of predicting it, both take seconds,
// and both are refused while a stream is publishing.
//
// That refusal is the whole of this file.
// A measurement earns its seconds only where what it measures is idle:
// an uplink probe beside a live stream measures the line minus the stream,
// and an encoder timing beside a live encode measures the silicon minus the encode.
// Either answers with a figure that reads as a property of the machine and is one of the moment.
// A bitrate or a frame rate is then set from it.
// So the competition is the reason given,
// and the code is FAILED_PRECONDITION, in the terms of the contract's table:
// the request is well formed and the world is not ready for it.
//
// The publish state decides, not a flag of this package's own,
// which leaves a pipeline waiting out a retry backoff on the refused side.
// That is still the stream the user asked for and it returns by itself,
// so a measurement started in the gap would be running when it does.

// MeasureUplink probes this machine's real upload throughput,
// so a guessed uplink figure can be replaced with a measured one.
func (s *Server) MeasureUplink(ctx context.Context, req *screensharev1.MeasureUplinkRequest) (*screensharev1.MeasureUplinkResponse, error) {
	if s.backend.PublishState().Publishing() {
		return nil, failedPrecondition(
			"a stream is publishing, and an uplink measurement would compete with it for the line; stop the stream to measure")
	}

	mbps, err := s.backend.MeasureUplink(ctx)
	if err != nil {
		return nil, fromBackend("cannot measure the uplink", err)
	}
	return &screensharev1.MeasureUplinkResponse{Mbps: mbps}, nil
}

// MeasureEncodeRate times the configured encoder on generated frames of the captured monitor's size,
// so a target frame rate can be held against what this machine encodes at these settings.
//
// The settings decide what is timed,
// so a request carrying none is refused by the gate the effects use (draftOf, in effects.go):
// the empty draft would time an encoder nobody chose,
// and the figure would read as an answer about the one they did.
func (s *Server) MeasureEncodeRate(ctx context.Context, req *screensharev1.MeasureEncodeRateRequest) (*screensharev1.MeasureEncodeRateResponse, error) {
	draft, err := draftOf(req.GetSettings(), "time")
	if err != nil {
		return nil, err
	}

	if s.backend.PublishState().Publishing() {
		return nil, failedPrecondition(
			"a stream is publishing, and an encode-rate measurement would compete with it for the encoder; stop the stream to measure")
	}

	rate, err := s.backend.MeasureEncodeRate(ctx, draft)
	if err != nil {
		return nil, fromBackend("cannot measure the encode rate", err)
	}
	return &screensharev1.MeasureEncodeRateResponse{Rate: wire.EncodeRate(rate)}, nil
}
