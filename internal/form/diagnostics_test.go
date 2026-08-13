package form

import (
	"google.golang.org/protobuf/proto"

	"slices"
	"testing"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/display"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The machine every draft below is judged against: one 1080p output at 60 Hz,
// which is what makes a 120 fps target a statement about the monitor rather than about an
// enumeration that failed.
func diagnosticTestDeps() Deps {
	return Deps{Monitors: []display.Monitor{
		{Index: 0, Width: 1920, Height: 1080, Primary: true, RefreshHz: 60},
	}}
}

// diagnosticTestStream is a stream the publish path builds a command for on any machine:
// the software H.264 encoder over SRT at a bitrate target, on the one capture backend whose command
// asks nothing of the host it is rendered on, so a healthy configuration stays healthy on every
// platform this test runs on.
func diagnosticTestStream() settings.Settings {
	s := settings.Defaults()
	s.Publish.Capture = "gdigrab"
	s.Publish.Codec = "libx264"
	s.Publish.Mode = "cbr"
	s.Publish.Chroma = "yuv420p"
	s.Publish.Transport = "srt"
	s.Publish.BitrateM = 20
	s.Publish.Fps = 60
	s.Publish.Monitor = 0
	s.Publish.UplinkMbps = 100
	// The two ladder steps this codec declares for this mode, which is what a draft naming it holds
	// after the migration or the repair.
	// The defaults carry the default codec's, and the builder refuses a step off the ladder,
	// which would make this stream one the publish path cannot build for a reason no case here is
	// about.
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec, s.Publish.Mode)
	return s
}

// diagnosticTestOfRank returns the diagnostics of one rank, so a case can say how many of them it
// expects rather than filtering at each site.
func diagnosticTestOfRank(diags []*screensharev1.Diagnostic, rank screensharev1.Severity) []*screensharev1.Diagnostic {
	var out []*screensharev1.Diagnostic
	for _, w := range diags {
		if w.GetSeverity() == rank {
			out = append(out, w)
		}
	}
	return out
}

// diagnosticTestDrafts is the spread every whole-list property below is held against.
// Each one reaches a different diagnostic, so a rule that fires on one of them is a rule the
// invariants have seen.
func diagnosticTestDrafts() map[string]settings.Settings {
	drafts := map[string]settings.Settings{}

	drafts["a healthy configuration"] = diagnosticTestStream()

	unencodable := diagnosticTestStream()
	unencodable.Publish.Chroma = "gbrp"
	drafts["a pixel format the codec cannot encode"] = unencodable

	// VP8 is absent from the FLV muxer's tag set, so the bitstream has no RTMP form.
	// The colour range moves with it because libvpx codes tv range alone, and a gap on that axis would
	// refuse the command before the leg was ever asked about.
	uncarriable := diagnosticTestStream()
	uncarriable.Publish.Codec = "libvpx"
	uncarriable.Publish.ColorRange = "tv"
	uncarriable.Publish.Transport = "rtmp"
	drafts["a leg that cannot carry the bitstream"] = uncarriable

	noLine := diagnosticTestStream()
	noLine.Publish.UplinkMbps = 0
	drafts["no uplink stated"] = noLine

	narrowLine := diagnosticTestStream()
	narrowLine.Publish.UplinkMbps = 5
	drafts["an uplink under the prediction"] = narrowLine

	burst := diagnosticTestStream()
	burst.Publish.Mode = "crf"
	burst.Publish.Cq = 18
	burst.Publish.UplinkMbps = 30
	drafts["a burst above the line"] = burst

	fast := diagnosticTestStream()
	fast.Publish.Fps = 120
	drafts["a rate above the monitor"] = fast

	software := diagnosticTestStream()
	software.Publish.Chroma = "yuv444p"
	drafts["a pixel format no GPU decodes"] = software

	partial := diagnosticTestStream()
	partial.Publish.Codec = "libx265"
	partial.Publish.Chroma = "gbrp"
	drafts["a pixel format some GPUs decode"] = partial

	gone := diagnosticTestStream()
	gone.Publish.Monitor = 4
	drafts["a monitor the machine does not have"] = gone

	return drafts
}

// A combination no engine can build is refused once, and the sentence that refuses it is the
// sentence the summary shows.
// Two answers to one question is the fork the whole contract exists to remove:
// a form that says one thing and a start button that fails with another leaves the user with
// nothing to act on.
func TestAnUnbuildableCombinationRefusesAndSaysWhy(t *testing.T) {
	d := diagnosticTestDeps()
	s := diagnosticTestStream()
	// x264 codes no planar RGB: the format is absent from this codec's chroma list,
	// so the combination is one no engine can build rather than one a machine cannot run.
	s.Publish.Chroma = "gbrp"

	est := estimate(d, s)
	diags := diagnostics(d, s, est)
	errors := diagnosticTestOfRank(diags, screensharev1.Severity_SEVERITY_ERROR)
	if len(errors) != 1 {
		t.Fatalf("errors = %d, and settings that cannot be published are refused once: %v", len(errors), diags)
	}
	if errors[0].GetFieldKey() != "" {
		t.Errorf("field key = %q, and a refusal of the combination names no single control", errors[0].GetFieldKey())
	}
	if publishable(diags) {
		t.Error("an error diagnostic is what turns publishable false")
	}

	// The diagnostic states that the publish was refused and the summary carries the refusal itself,
	// which is the one raw string on this contract a diagnostic points at rather than repeating
	// (api/proto/screenshare/v1/text.proto).
	sum := summarize(d, s, est)
	if codeOf(errors[0].GetText()) != publishRefused {
		t.Errorf("the error diagnostic reads %v, want the statement that the publish was refused",
			codeOf(errors[0].GetText()))
	}
	if sum.GetCommandError() == "" {
		t.Error("the diagnostic says the publish was refused and the summary carries no refusal")
	}
	if sum.GetCommand() != "" {
		t.Errorf("command = %q, and a combination no engine builds has none", sum.GetCommand())
	}
}

// The healthy case is the one worth stating explicitly: nothing in the settings refuses the
// publish, so the start button is live and the summary carries the line it would run.
func TestAHealthyConfigurationRefusesNothing(t *testing.T) {
	d := diagnosticTestDeps()
	s := diagnosticTestStream()
	est := estimate(d, s)

	sum := summarize(d, s, est)
	if sum.GetCommandError() != "" {
		t.Fatalf("these settings are meant to build a command, and the engine refused them: %s", sum.GetCommandError())
	}
	if sum.GetCommand() == "" {
		t.Error("a summary that refuses nothing carries the command it would run")
	}

	diags := diagnostics(d, s, est)
	if errors := diagnosticTestOfRank(diags, screensharev1.Severity_SEVERITY_ERROR); len(errors) != 0 {
		t.Errorf("errors = %v, and these settings publish as they stand", errors)
	}
	if !publishable(diags) {
		t.Error("settings the publish path builds a command for are publishable")
	}
}

// A diagnostic anchors beside a control or beside nothing, and a key no shell has a widget for
// would render the diagnostic nowhere at all.
// The failure is silent, which is why the whole spread is held against the declared set rather than
// each rule against its own.
func TestEveryDiagnosticAnchorsOnADeclaredKey(t *testing.T) {
	d := diagnosticTestDeps()
	for name, s := range diagnosticTestDrafts() {
		t.Run(name, func(t *testing.T) {
			for _, w := range diagnostics(d, s, estimate(d, s)) {
				if w.GetFieldKey() == "" {
					continue
				}
				if !slices.Contains(warningAnchors, w.GetFieldKey()) {
					t.Errorf("diagnostic %v anchors on %q, which no field key declares",
						codeOf(w.GetText()), w.GetFieldKey())
				}
			}
		})
	}
}

// Every diagnostic is ranked and says something.
// An unranked one sorts as though it were the least important and reads as though the backend had
// no opinion, and an empty one occupies a line in the form to say nothing.
func TestEveryDiagnosticIsRankedAndSaysSomething(t *testing.T) {
	d := diagnosticTestDeps()
	for name, s := range diagnosticTestDrafts() {
		t.Run(name, func(t *testing.T) {
			for _, w := range diagnostics(d, s, estimate(d, s)) {
				if w.GetSeverity() == screensharev1.Severity_SEVERITY_UNSPECIFIED {
					t.Errorf("diagnostic %v carries no rank", codeOf(w.GetText()))
				}
				if codeOf(w.GetText()) == screensharev1.TextCode_TEXT_CODE_UNSPECIFIED {
					t.Errorf("a diagnostic anchored on %q says nothing", w.GetFieldKey())
				}
			}
		})
	}
}

// The list is ranked so a shell renders it in the order it is given.
// A shell that had to sort it would be ranking diagnostics itself, which is the judgement the
// backend is here to make.
func TestTheListIsRankedByWhatIgnoringItCosts(t *testing.T) {
	d := diagnosticTestDeps()
	for name, s := range diagnosticTestDrafts() {
		t.Run(name, func(t *testing.T) {
			diags := diagnostics(d, s, estimate(d, s))
			for i := 1; i < len(diags); i++ {
				if diags[i-1].GetSeverity() < diags[i].GetSeverity() {
					t.Errorf("diagnostic %d ranks %v and the one before it %v",
						i, diags[i].GetSeverity(), diags[i-1].GetSeverity())
				}
			}
		})
	}
}

// A line under the prediction is the failure that announces itself least: nothing slows down,
// the transport drops what it cannot ship, and the viewer sees a stall.
// It is a diagnostic and not a refusal, since the stream does run.
func TestALineUnderThePredictionWarnsWithoutRefusing(t *testing.T) {
	d := diagnosticTestDeps()
	s := diagnosticTestStream()
	s.Publish.UplinkMbps = 5

	diags := diagnostics(d, s, estimate(d, s))
	if !publishable(diags) {
		t.Error("a line too narrow for the stream does not make the settings unbuildable")
	}
	found := false
	for _, w := range diagnosticTestOfRank(diags, screensharev1.Severity_SEVERITY_WARNING) {
		if w.GetFieldKey() == KeyUplinkMbps {
			found = true
		}
	}
	if !found {
		t.Errorf("a 5 Mbit/s line under a 20 Mbit/s target diags beside the uplink field: %v", diags)
	}
}

// A pixel format no GPU decodes costs every viewer a software decode and nothing else:
// every format has a CPU decoder, so it is never a reason to refuse the publish.
func TestAPixelFormatNoGpuDecodesCostsRatherThanRefuses(t *testing.T) {
	d := diagnosticTestDeps()
	s := diagnosticTestStream()
	// No vendor put H.264's High 4:4:4 Predictive profile in silicon, so this pair has no hardware
	// decoder anywhere.
	s.Publish.Chroma = "yuv444p"

	diags := diagnostics(d, s, estimate(d, s))
	if !publishable(diags) {
		t.Errorf("a software decode at the viewer is a cost and not a refusal: %v", diags)
	}
	found := false
	for _, w := range diagnosticTestOfRank(diags, screensharev1.Severity_SEVERITY_WARNING) {
		if w.GetFieldKey() == KeyChroma {
			found = true
		}
	}
	if !found {
		t.Errorf("a pixel format no GPU decodes diags beside the pixel format field: %v", diags)
	}
}

// The estimate the summary shows is the one the diagnostics were ranked against.
// Computing it a second time would be a second answer, and the two would part the first time either
// side of the model moved.
func TestTheSummaryCarriesTheEstimateItWasHanded(t *testing.T) {
	d := diagnosticTestDeps()
	s := diagnosticTestStream()
	est := estimate(d, s)

	sum := summarize(d, s, est)
	if sum.GetEstimate() != est {
		t.Errorf("the summary carries %v where it was handed %v", sum.GetEstimate(), est)
	}
}

// The summary carries a command or the reason there is none, and never both or neither.
// It is the whole of what it states now: the shorthand for the configuration and the one per group
// used to be composed here, and both were screen copy - a separator, an abbreviation and a length,
// chosen where none of the three is visible.
func TestTheSummaryCarriesACommandOrTheReasonThereIsNone(t *testing.T) {
	d := diagnosticTestDeps()
	s := diagnosticTestStream()

	sum := summarize(d, s, estimate(d, s))
	if (sum.GetCommand() == "") == (sum.GetCommandError() == "") {
		t.Errorf("the summary carries command %q and refusal %q, and it states exactly one",
			sum.GetCommand(), sum.GetCommandError())
	}
}

// Resolve runs on every keystroke, so a diagnostic list that moved between two identical drafts
// would reorder the form under a user who changed nothing.
func TestTheSameDraftWarnsTheSameWayTwice(t *testing.T) {
	d, s := diagnosticTestDeps(), diagnosticTestStream()
	est := estimate(d, s)

	first, second := diagnostics(d, s, est), diagnostics(d, s, est)
	if len(first) != len(second) {
		t.Fatalf("one draft warned %d times and then %d", len(first), len(second))
	}
	for i := range first {
		if first[i].GetSeverity() != second[i].GetSeverity() ||
			!proto.Equal(first[i].GetText(), second[i].GetText()) ||
			first[i].GetFieldKey() != second[i].GetFieldKey() {
			t.Errorf("diagnostic %d read %v and then %v", i, first[i], second[i])
		}
	}
}
