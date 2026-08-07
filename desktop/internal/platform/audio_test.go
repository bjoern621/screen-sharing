package platform

import (
	"google.golang.org/protobuf/proto"

	"slices"
	"testing"
)

// audioTestPlatforms are the machines the table is asked about: every operating system
// it answers for, plus one it has never heard of. The last is the case worth naming -
// a table that only ever runs on the three it declares would never show what it does
// with a fourth.
var audioTestPlatforms = []Info{
	{OS: "windows"},
	{OS: "linux", Display: "wayland"},
	{OS: "linux", Display: "x11"},
	{OS: "darwin"},
	{OS: "plan9"},
	{},
}

// The contract every consumer of the table depends on: a source this platform does not
// serve says what the platform is missing, and one it serves says nothing.
//
// It is the same contract form.go asserts on a rendered option and the same one the
// catalog filters on, which is why it is checked here rather than three times downstream:
// a greyed entry with no sentence teaches nothing, and a sentence beside a live entry is
// a reason for a refusal that never happened.
func TestAnUnservedAudioSourceSaysWhatTheMachineIsMissing(t *testing.T) {
	for _, info := range audioTestPlatforms {
		for _, s := range AudioSources(info) {
			if s.Available && s.Reason != nil {
				t.Errorf("%s serves %q and still carries a reason: %v", info.OS, s.ID, s.Reason)
			}
			if !s.Available && s.Reason == nil {
				t.Errorf("%s serves no %q and says nothing about why", info.OS, s.ID)
			}
			if !s.Available && s.Server != nil {
				t.Errorf("%s serves no %q and still names %v as what serves it",
					info.OS, s.ID, s.Server)
			}
		}
	}
}

// Every platform is answered for on every declared source, in one order.
//
// The order is part of the answer: it is the order a form presents the choice in, and a
// list that reshuffled per machine would move the entry under the user's cursor when
// nothing about the machine changed. The absent source leads it because a stream that
// carries no second track asks nothing of the machine, so it is the one entry no platform
// can refuse and the value a fresh stream holds.
func TestEveryPlatformIsAnsweredForOnEverySource(t *testing.T) {
	want := AudioSourceIDs(Info{})
	if len(want) == 0 {
		t.Fatal("the audio source table declares nothing")
	}
	if want[0] != AudioSourceNone {
		t.Errorf("the source table leads with %q, want the absent source %q", want[0], AudioSourceNone)
	}

	for _, info := range audioTestPlatforms {
		got := AudioSourceIDs(info)
		if !slices.Equal(got, want) {
			t.Errorf("%s answers %v, want every declared source in table order %v",
				info.OS, got, want)
		}
		sources := AudioSources(info)
		if !sources[0].Available {
			t.Errorf("%s refuses the absent source: %s", info.OS, sources[0].Reason)
		}
	}
}

// A resolve reads this table on every keystroke, so it has to be a table read: the same
// platform in, the same ordered list out, with nothing about the machine touched on the
// way. A second call that answered differently would be a probe hiding in a lookup, and
// the form would grey a source on one keystroke and offer it on the next.
func TestTheSamePlatformAlwaysAnswersTheSame(t *testing.T) {
	for _, info := range audioTestPlatforms {
		first := AudioSources(info)
		second := AudioSources(info)
		if len(first) != len(second) {
			t.Fatalf("%s answers %d sources and then %d", info.OS, len(first), len(second))
		}
		// Compared field by field rather than by value, because two rows differ in the
		// two statements they carry and a protobuf message is compared by what it says
		// rather than by the pointer holding it.
		for i := range first {
			if first[i].ID != second[i].ID || first[i].Available != second[i].Available ||
				!proto.Equal(first[i].Reason, second[i].Reason) ||
				!proto.Equal(first[i].Server, second[i].Server) {
				t.Errorf("%s answers %v and then %v", info.OS, first[i], second[i])
			}
		}
	}
}

// What each platform actually says, which is the point of the table being one: the same
// source is served on one operating system and out of reach on the others, and the row is
// where that difference is stated rather than in whichever consumer asked.
//
// Desktop audio is Linux-only because both publish engines open it as the PulseAudio or
// PipeWire monitor of the default sink and neither has anything to open on the other two
// (ffmpeg/args.go, publish/gstpipeline.go). The day one gains a loopback the row gains a
// platform, and this case is what says so out loud.
func TestDesktopAudioIsServedWhereAServerServesIt(t *testing.T) {
	cases := []struct {
		info   Info
		served bool
	}{
		{Info{OS: "linux", Display: "wayland"}, true},
		{Info{OS: "linux", Display: "x11"}, true},
		{Info{OS: "windows"}, false},
		{Info{OS: "darwin"}, false},
	}
	for _, c := range cases {
		available, reason := AudioSourceAvailable(AudioSourceDesktop, c.info)
		if available != c.served {
			t.Errorf("%s serves desktop audio: %t, want %t (%s)", c.info.OS, available, c.served, reason)
		}

		source := audioTestSource(t, c.info, AudioSourceDesktop)
		if c.served && source.Server == nil {
			t.Errorf("%s serves desktop audio and names nothing as what serves it", c.info.OS)
		}
	}

	// The two refusals carry the operating system that refused, which is what lets a
	// surface say what that machine in particular is missing: a user who reads what
	// Windows lacks cannot act on what macOS lacks, and "Linux only" would name neither.
	_, windows := AudioSourceAvailable(AudioSourceDesktop, Info{OS: "windows"})
	_, darwin := AudioSourceAvailable(AudioSourceDesktop, Info{OS: "darwin"})
	if proto.Equal(windows, darwin) {
		t.Errorf("Windows and macOS are refused desktop audio with one statement: %v", windows)
	}
}

// A machine the table never named keeps every source rather than losing all of them.
//
// The reasons are written per operating system, so an unnamed one has no sentence to be
// refused under, and refusing it anyway would grey a control with somebody else's
// machine's explanation. It is the same conclusion publish.Available reaches from the
// opposite direction, and the reason both tables state their platforms rather than
// inferring them.
func TestAnUnknownPlatformKeepsEverySource(t *testing.T) {
	for _, info := range []Info{{OS: "plan9"}, {}} {
		for _, s := range AudioSources(info) {
			if !s.Available {
				t.Errorf("%q takes %q away from a platform it has nothing to say about: %s",
					info.OS, s.ID, s.Reason)
			}
		}
	}
}

// A source no consumer can name is a source no control can offer, so the declaration and
// the constants that spell it are held together. A row losing its constant would leave
// the settings and the form matching on a literal the table no longer produces.
func TestTheDeclaredSourcesAreTheNamedOnes(t *testing.T) {
	ids := AudioSourceIDs(Info{})
	for _, want := range []string{AudioSourceNone, AudioSourceDesktop} {
		if !slices.Contains(ids, want) {
			t.Errorf("the table declares %v, which does not carry the named source %q", ids, want)
		}
	}
	if len(ids) != len(audioSourceNeeds) {
		t.Errorf("the table answers with %d sources and declares %d", len(ids), len(audioSourceNeeds))
	}
}

// A row the platform serves names what serves it, and one it does not names nothing. The
// note a form puts beside the entry is that name, so an empty one on a served source is a
// screen that offers a capture and cannot say where it reads from.
func TestAServedSourceNamesWhatServesIt(t *testing.T) {
	for _, info := range audioTestPlatforms {
		for _, s := range AudioSources(info) {
			if s.ID == AudioSourceNone && s.Server != nil {
				t.Errorf("%s names %v as what serves the absent source", info.OS, s.Server)
			}
		}
	}
}

// audioTestSource reads one row out of a platform's answer, failing where the table
// stopped declaring it.
func audioTestSource(t *testing.T, info Info, id string) AudioSource {
	t.Helper()
	for _, s := range AudioSources(info) {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("%s answers with no %q source", info.OS, id)
	return AudioSource{}
}
