package main

import (
	"context"
	"fmt"
	"math"
	"time"

	v1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// What the broadcast screen shows while a stream runs, held to what the samples behind it carry.
//
// A figure carries presence because no measurement is not a measured zero, and the screen prints an
// absent one as an ellipsis and holds the last value that was measured
// (avalonia/.../Broadcast/Model/HeldFigures.cs).
// So a figure no sample of a whole run states is not a gap: it is a row that reads empty for the
// session, on a screen whose neighbouring rows carry numbers.

// The figures the broadcast screen draws out of an encoder sample, and the bounds outside which one
// prints as something a reader would report rather than read.
//
// Exactly these four: the header promotes the first three and the pill counts on the fourth, and
// the egress plot needs a rate and a clock on one sample to place a point
// (avalonia/.../Broadcast/Plots/Model/PlotSeries.cs).
var overlayFigures = []struct {
	name  string
	shown string
	has   func(*v1.PublishStats) bool
	read  func(*v1.PublishStats) float64
	floor float64
	limit float64
}{
	{"fps", "the frame rate in the header", func(s *v1.PublishStats) bool { return s.Fps != nil },
		func(s *v1.PublishStats) float64 { return s.GetFps() }, 0, 1000},
	{"inst_mbps", "the rate in the header and the egress plot", func(s *v1.PublishStats) bool { return s.InstMbps != nil },
		func(s *v1.PublishStats) float64 { return s.GetInstMbps() }, 0, 10000},
	{"transit_ms", "the encode delay in the header", func(s *v1.PublishStats) bool { return s.TransitMs != nil },
		func(s *v1.PublishStats) float64 { return s.GetTransitMs() }, 0, 60000},
	{"time_sec", "the running clock on the sharing pill", func(s *v1.PublishStats) bool { return s.TimeSec != nil },
		func(s *v1.PublishStats) float64 { return s.GetTimeSec() }, 0, 86400},
}

// checkOverlay states which promoted figure the screen would have drawn as an ellipsis for the
// whole run, and which one carried a value it cannot print.
func (s *session) checkOverlay(stats []*v1.PublishStats, attempts int32, fields map[string]string, settings *v1.Settings) {
	if len(stats) == 0 {
		return
	}
	engine := fields["capture"] + "/" + fields["transport"]

	for _, figure := range overlayFigures {
		measured := 0
		for _, sample := range stats {
			if !figure.has(sample) {
				continue
			}
			measured++
			value := figure.read(sample)
			switch {
			case math.IsNaN(value) || math.IsInf(value, 0):
				s.report.report("publish.figure_not_finite", "publish/figure-nan/"+figure.name+"/"+engine,
					fmt.Sprintf("%s arrived as %v, which is %s", figure.name, value, figure.shown),
					fields, settings)
			case value < figure.floor || value > figure.limit:
				s.report.report("publish.figure_out_of_bounds", "publish/figure-bounds/"+figure.name+"/"+engine,
					fmt.Sprintf("%s arrived as %.2f, outside the %.0f..%.0f a reader could act on",
						figure.name, value, figure.floor, figure.limit), fields, settings)
			}
		}
		// A per-interval figure is measured against the sample before it, so the first sample of a run
		// states none (api/proto/screenshare/v1/session.proto, PublishStats).
		// A run that relaunched is first samples all the way down, and what that says is that the
		// stream kept dying rather than anything about what this engine measures.
		if measured == 0 && attempts == 0 {
			s.report.report("publish.figure_never_measured", "publish/figure-absent/"+figure.name+"/"+engine,
				fmt.Sprintf("no sample of %d carried %s, so %s reads as an ellipsis for the run",
					len(stats), figure.name, figure.shown), fields, settings)
		}
	}

	// A point on the egress plot needs both figures off one sample, so a run stating each of them on
	// samples of its own draws no curve at all.
	if attempts == 0 && !carries(stats, func(sample *v1.PublishStats) bool { return sample.InstMbps != nil && sample.TimeSec != nil }) {
		s.report.report("publish.plot_has_no_point", "publish/no-plot-point/"+engine,
			"no sample carried a rate and a clock together, so the egress plot draws nothing",
			fields, settings)
	}

	checkCounters(s, stats, attempts, fields, settings)
}

// checkCounters holds the running totals to the one thing a reader assumes of a counter, which is
// that it counts up.
//
// A figure that falls between two samples is a readout that jumps backwards on screen, and the
// clock falling is a pill that starts its stream again in front of somebody watching it.
//
// A relaunched pipeline counts from zero again, and the stream it belongs to is one the reader never
// stopped, so the two are reported apart: what the samples of one child did, and what a relaunch does
// to a screen that keeps showing the same stream.
func checkCounters(s *session, stats []*v1.PublishStats, attempts int32, fields map[string]string, settings *v1.Settings) {
	engine := fields["capture"] + "/" + fields["transport"]
	kind, signature := "publish.counter_went_backwards", "publish/counter-back/"
	if attempts > 0 {
		kind, signature = "publish.relaunch_resets_readout", "publish/relaunch-reset/"
	}

	for i := 1; i < len(stats); i++ {
		previous, sample := stats[i-1], stats[i]
		for _, counter := range []struct {
			name string
			was  int64
			is   int64
		}{
			{"frame_count", previous.GetFrameCount(), sample.GetFrameCount()},
			{"dropped_frames", previous.GetDroppedFrames(), sample.GetDroppedFrames()},
			{"duplicated_frames", previous.GetDuplicatedFrames(), sample.GetDuplicatedFrames()},
		} {
			if counter.is < counter.was {
				s.report.report(kind, signature+counter.name+"/"+engine,
					fmt.Sprintf("%s fell from %d to %d between two samples of one run",
						counter.name, counter.was, counter.is), fields, settings)
			}
		}
		if previous.TimeSec != nil && sample.TimeSec != nil && sample.GetTimeSec() < previous.GetTimeSec() {
			s.report.report(kind, signature+"clock/"+engine,
				fmt.Sprintf("the stream clock fell from %.1f s to %.1f s", previous.GetTimeSec(), sample.GetTimeSec()),
				fields, settings)
		}
	}

	// The pill counts a stream's life off this figure, so one that stands still is a timer frozen
	// while the frames beside it keep arriving.
	first, last := firstTimed(stats), lastTimed(stats)
	if first != nil && last != nil && last.GetFrameCount() > first.GetFrameCount() &&
		last.GetTimeSec() <= first.GetTimeSec() {
		s.report.report("publish.clock_stalled", "publish/clock-stalled/"+engine,
			fmt.Sprintf("%d frames arrived and the clock stayed at %.1f s",
				last.GetFrameCount()-first.GetFrameCount(), last.GetTimeSec()), fields, settings)
	}
}

func firstTimed(stats []*v1.PublishStats) *v1.PublishStats {
	for _, sample := range stats {
		if sample.TimeSec != nil {
			return sample
		}
	}
	return nil
}

func lastTimed(stats []*v1.PublishStats) *v1.PublishStats {
	for i := len(stats) - 1; i >= 0; i-- {
		if stats[i].TimeSec != nil {
			return stats[i]
		}
	}
	return nil
}

// checkRelayView holds what the relay says about the running stream to what the screen reads off it.
//
// The viewer count, the worst round trip and the worst loss are all looked up by the path the
// stream publishes to, so a relay naming no such path leaves three figures absent at once and a
// panel with nothing in it (avalonia/.../Broadcast/Model/BroadcastSnapshot.cs, PathOf).
func (s *session) checkRelayView(ctx context.Context, stream string, fields map[string]string, settings *v1.Settings) {
	var status *v1.RelayStatus
	err := withTimeout(ctx, 15*time.Second, func(call context.Context) error {
		answer, err := s.control.GetRelayStatus(call, &v1.GetRelayStatusRequest{})
		status = answer
		return err
	})
	if err != nil {
		s.report.report("publish.relay_status_failed", "publish/relay-status/"+stream, err.Error(), fields, settings)
		return
	}
	if !status.GetReachable() {
		s.report.report("publish.relay_unreachable", "publish/relay-unreachable",
			fmt.Sprintf("the relay was unreachable while a stream ran: %s", status.GetError()), fields, settings)
		return
	}

	for _, path := range status.GetPaths() {
		if path.GetName() != stream {
			continue
		}
		if !path.GetReady() {
			s.report.report("publish.relay_path_not_ready", "publish/relay-not-ready/"+fields["transport"],
				fmt.Sprintf("the relay carries %q and does not serve it", stream), fields, settings)
		}
		if path.GetTracks() == "" {
			s.report.report("publish.relay_path_without_tracks", "publish/relay-no-tracks/"+fields["transport"],
				fmt.Sprintf("the relay carries %q and names no track on it", stream), fields, settings)
		}
		return
	}

	s.report.report("publish.relay_path_absent", "publish/relay-no-path/"+fields["transport"],
		fmt.Sprintf("a stream ran and the relay named no path %q, leaving the viewer count, the round trip and the loss absent at once", stream),
		fields, settings)
}
