package control

import (
	"context"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/wire"
)

// The measurements.
// Both run the real thing rather than predicting it, both take seconds, and both are refused while
// a stream is publishing.
//
// The refusal is what this file is for.
// A measurement is worth its seconds only if what it measures is idle: an uplink probe run beside a
// live stream measures the line minus the stream, and an encoder timing run beside a live encode
// measures the silicon minus the encode.
// Either answers with a number that looks like a property of the machine and is really a property
// of the moment, and the user then sets a bitrate or a frame rate from it.
// So the competition is the reason given, in the words of the contract's table:
// the request is well formed and the world is not ready for it, which is FAILED_PRECONDITION.
//
// The state that decides is the publish state and not a flag of this package's own,
// which is what keeps a pipeline waiting out a retry backoff on the right side of the line.
// It is still the stream the user asked for and it will come back on its own,
// so a measurement started in the gap would be running when it does.

// MeasureUplink probes this machine's real upload throughput, so a shell can replace the user's
// guessed uplink figure with a measured one.
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

// MeasureEncodeRate times the configured encoder on generated frames of the captured monitor's
// size, so a shell can say whether the target frame rate is above what this machine encodes at
// these settings.
//
// The settings decide what is timed, so a request that carries none is refused by the same gate the
// effects use (draftOf, in effects.go): timing the empty draft would answer about an encoder nobody
// chose, and the figure would look like an answer about the one they did.
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
