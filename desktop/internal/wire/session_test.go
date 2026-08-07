package wire

import (
	"reflect"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
	"bjoernblessin.de/screenshare/internal/ffmpeg"
	"bjoernblessin.de/screenshare/internal/relay"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The names PublishStats.missing carries, in the spelling the proto message
// declares its fields under. They are written out here rather than read from the
// conversion's own table, because a test that derived them from the thing under test
// would agree with it however it was spelled.
var missingNames = []string{
	"fps", "capture_fps", "size_kib", "time_sec", "speed", "inst_mbps", "avg_mbps",
}

// allMissing is a sample that measured nothing at all: every flag the domain has,
// set. It is built by reflection so that a figure added to ffmpeg.Missing arrives in
// this test without anyone remembering to put it here.
func allMissing() ffmpeg.Missing {
	var m ffmpeg.Missing
	v := reflect.ValueOf(&m).Elem()
	for i := 0; i < v.NumField(); i++ {
		v.Field(i).SetBool(true)
	}
	return m
}

// presentFigures reports which of the figures a run can leave unmeasured the sample
// actually carries, read off the message rather than off a second list.
func presentFigures(stats *screensharev1.PublishStats) map[protoreflect.Name]bool {
	m := stats.ProtoReflect()
	present := map[protoreflect.Name]bool{}
	for _, name := range missingNames {
		field := m.Descriptor().Fields().ByName(protoreflect.Name(name))
		if field == nil {
			continue
		}
		present[protoreflect.Name(name)] = m.Has(field)
	}
	return present
}

// A figure with no measurement is not a measured zero: ffmpeg reports nothing until
// the first packet is muxed, a per-interval figure has no value on the first sample
// of a run, and each engine instruments only what its own pipeline exposes. The
// distinction survives the crossing as proto3 presence, so a shell draws the figure as
// absent instead of as a stalled encoder.
func TestAnUnmeasuredFigureIsAbsentRatherThanZeroed(t *testing.T) {
	present := presentFigures(PublishStats(ffmpeg.Stats{Missing: allMissing()}))

	if len(present) != len(missingNames) {
		t.Fatalf("PublishStats declares %d of the %d unmeasurable figures", len(present), len(missingNames))
	}
	for name, carried := range present {
		if carried {
			t.Errorf("a sample that measured nothing carries %s, want it absent", name)
		}
	}
}

// The zero Missing marks nothing missing, which is what an engine that measured every
// figure leaves behind. Every figure is then present, including the ones whose
// measurement happens to be zero - which is the distinction that would be lost if
// absence were spelled as a value.
func TestAMeasuredSampleCarriesEveryFigure(t *testing.T) {
	present := presentFigures(PublishStats(ffmpeg.Stats{Fps: 60, InstMbps: 12}))

	for name, carried := range present {
		if !carried {
			t.Errorf("a fully measured sample leaves %s absent, want it carried", name)
		}
	}
}

// Every figure a run can leave unmeasured needs a field that can be absent, or it
// crosses as a measured zero with nothing to mark it.
//
// Both halves are read rather than restated: the flags off ffmpeg.Missing, the
// presence off the generated descriptor. A figure added to the domain without an
// optional field to carry it fails here rather than in a shell's numbers, and a field
// that lost its presence fails here rather than by silently reading as zero.
func TestEveryUnmeasurableFigureHasAFieldThatCanBeAbsent(t *testing.T) {
	flags := reflect.TypeOf(ffmpeg.Missing{}).NumField()
	if flags != len(missingNames) {
		t.Errorf("the domain can leave %d figures unmeasured and the contract carries %d", flags, len(missingNames))
	}

	fields := (&screensharev1.PublishStats{}).ProtoReflect().Descriptor().Fields()
	optional := 0
	for i := 0; i < fields.Len(); i++ {
		if fields.Get(i).HasPresence() {
			optional++
		}
	}
	if optional != len(missingNames) {
		t.Errorf("PublishStats carries presence on %d fields, and %d figures can go unmeasured", optional, len(missingNames))
	}
	for _, name := range missingNames {
		field := fields.ByName(protoreflect.Name(name))
		if field == nil {
			t.Errorf("PublishStats does not declare %q", name)
			continue
		}
		if !field.HasPresence() {
			t.Errorf("%q cannot be absent, so an unmeasured one crosses as zero", name)
		}
	}
}

// The contract says a publish state carries a live stream only while one is in force,
// and that a live one always carries the settings it was built from. Both are message
// presence, so neither is a rule anything has to check: a state with nothing publishing
// has no arm to put settings in, and one that is publishing cannot omit them.
func TestNothingPublishingCarriesNoLiveStream(t *testing.T) {
	state := PublishState(PublishSnapshot{})

	if state.GetLive() != nil {
		t.Errorf("a state with nothing publishing carries a live stream: %v", state.GetLive())
	}
}

// A pipeline waiting out a backoff is still the stream the user asked for, so it stays
// live and the retry hangs off it. The attempt and the budget exist only inside that
// retry, which is what makes "an attempt belongs to a retry" unrepresentable rather
// than asserted.
func TestARetryHangsOffTheLiveStream(t *testing.T) {
	live := PublishState(PublishSnapshot{Live: &LiveSnapshot{
		Settings: settings.Stream{Name: "bob"},
		Retry:    &RetrySnapshot{Attempt: 2, Budget: 3},
	}})

	if live.GetLive() == nil {
		t.Fatal("a pipeline waiting out a backoff carries no live stream, want one")
	}
	if live.GetLive().GetSettings() == nil {
		t.Error("a live stream carries no settings, want the ones it was built from")
	}
	retry := live.GetLive().GetRetry()
	if retry == nil {
		t.Fatal("a pending relaunch carries no retry")
	}
	if retry.GetAttempt() != 2 || retry.GetBudget() != 3 {
		t.Errorf("a pending relaunch carries attempt %d of %d, want 2 of 3", retry.GetAttempt(), retry.GetBudget())
	}

	carrying := PublishState(PublishSnapshot{Live: &LiveSnapshot{Settings: settings.Stream{Name: "bob"}}})
	if carrying.GetLive().GetRetry() != nil {
		t.Error("a stream carrying frames carries a retry, want none")
	}
}

// An unreachable relay is an environment condition and not a call failure: "the
// relay is down" is a thing the screen has to say rather than a thing the call
// failed at. The conversion therefore has no error path, and the reason rides in
// the snapshot it produces.
func TestAnUnreachableRelayIsASnapshotRatherThanAFailure(t *testing.T) {
	status := RelayStatus(relay.Status{Reachable: false, Error: "connection refused"})

	if status == nil {
		t.Fatal("an unreachable relay produced no snapshot")
	}
	if status.GetReachable() {
		t.Error("an unreachable relay converted to a reachable snapshot")
	}
	if status.GetError() != "connection refused" {
		t.Errorf("an unreachable relay carries the reason %q, want %q", status.GetError(), "connection refused")
	}
	// A relay that never answered reports no paths, which is a snapshot with an empty
	// list and not one the conversion has to be kept away from.
	if len(status.GetPaths()) != 0 {
		t.Errorf("an unreachable relay carries %d paths, want none", len(status.GetPaths()))
	}
}

// The relay re-serves each stream on all its listeners, so a stream can be watched
// over several transports at once and the name alone is not an identity. Both halves
// have to cross, or a viewer that ended would clear every viewer of its stream.
func TestAViewerIsIdentifiedByBothItsHalves(t *testing.T) {
	want := []WatchKey{
		{StreamName: "desk", Transport: "srt"},
		{StreamName: "desk", Transport: "rtsp"},
	}
	keys := ViewerState(want).GetViewers()

	if len(keys) != 2 {
		t.Fatalf("two open viewers converted to %d keys", len(keys))
	}
	for i, w := range want {
		if keys[i].GetStreamName() != w.StreamName || keys[i].GetTransport() != w.Transport {
			t.Errorf("viewer %d crossed as (%q, %q), want (%q, %q)",
				i, keys[i].GetStreamName(), keys[i].GetTransport(), w.StreamName, w.Transport)
		}
	}

	// No viewer open is an empty list rather than a nil slice to be guarded against.
	if got := ViewerState(nil).GetViewers(); len(got) != 0 {
		t.Errorf("no open viewer converted to %d keys", len(got))
	}

	// The identity survives a round trip, which is what lets the same key open a viewer
	// and close it: two spellings of one pair is how a name and a stream_name end up
	// naming one thing.
	if got := WatchKeyOf(WatchKeyMessage(want[0])); got != want[0] {
		t.Errorf("a viewer's identity round-tripped to %+v, want %+v", got, want[0])
	}
}
