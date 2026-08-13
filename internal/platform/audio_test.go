package platform

import (
	"google.golang.org/protobuf/proto"

	"slices"
	"testing"
)

// audioTestPlatforms are the machines the table is asked about: every operating system it answers
// for, plus ones it never heard of.
// The unnamed ones are the case worth covering, since a table asked only about what it declares
// never shows what it does with anything else.
var audioTestPlatforms = []Info{
	{OS: "windows"},
	{OS: "linux", Display: "wayland"},
	{OS: "linux", Display: "x11"},
	{OS: "darwin"},
	{OS: "plan9"},
	{},
}

// The contract every consumer depends on, checked here rather than at each of them: a greyed entry
// with no sentence teaches nothing, and a sentence beside a live entry is a reason for a refusal
// that never happened.
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

// The order is part of the answer: a list reshuffled per machine would move the entry under the
// user's cursor when nothing about the machine changed.
// The absent source leads it, since a stream carrying no second track asks nothing of the machine,
// so it is the one entry no platform can refuse and the value a fresh stream holds.
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

// A resolve reads this table on every keystroke, so it has to stay a table read: same platform in,
// same ordered list out.
// A second call answering differently would be a probe hiding in a lookup,
// and the form would grey a source on one keystroke and offer it on the next.
func TestTheSamePlatformAlwaysAnswersTheSame(t *testing.T) {
	for _, info := range audioTestPlatforms {
		first := AudioSources(info)
		second := AudioSources(info)
		if len(first) != len(second) {
			t.Fatalf("%s answers %d sources and then %d", info.OS, len(first), len(second))
		}
		// Compared field by field, because a row carries protobuf messages, which are equal by what
		// they say rather than by the pointer holding them.
		for i := range first {
			if first[i].ID != second[i].ID || first[i].Available != second[i].Available ||
				!proto.Equal(first[i].Reason, second[i].Reason) ||
				!proto.Equal(first[i].Server, second[i].Server) {
				t.Errorf("%s answers %v and then %v", info.OS, first[i], second[i])
			}
		}
	}
}

// Desktop audio is Linux-only because both publish engines open it as the PulseAudio or PipeWire
// monitor of the default sink, and neither has anything to open on the other two (ffmpeg/args.go,
// publish/gstpipeline.go).
// An engine gaining a loopback is a platform gained on the row, which is the change this case is
// here to notice.
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

	// Each refusal carries the operating system it is about, so a surface can say what that machine in
	// particular is missing: a user reading what Windows lacks cannot act on what macOS lacks, and
	// "Linux only" would name neither.
	_, windows := AudioSourceAvailable(AudioSourceDesktop, Info{OS: "windows"})
	_, darwin := AudioSourceAvailable(AudioSourceDesktop, Info{OS: "darwin"})
	if proto.Equal(windows, darwin) {
		t.Errorf("Windows and macOS are refused desktop audio with one statement: %v", windows)
	}
}

// A machine the table never named keeps every source this app opens somewhere.
//
// Reasons are written per operating system, so an unnamed one has no sentence to be refused under,
// and refusing it anyway would grey a control with somebody else's machine's explanation.
//
// A kind no platform serves is the one exception, and it is not an inference about the machine:
// nothing opens it anywhere, so an unknown operating system is not one it might work on.
func TestAnUnknownPlatformKeepsEverySourceSomethingOpens(t *testing.T) {
	for _, info := range []Info{{OS: "plan9"}, {}} {
		for _, s := range AudioSources(info) {
			if !s.Available {
				t.Errorf("%q takes %q away from a platform it has nothing to say about: %s",
					info.OS, s.ID, s.Reason)
			}
		}
	}
}

// The declaration and the constants that spell it are held together, since a source no consumer can
// name is one no control can offer.
// A row losing its constant leaves the settings and the form matching on a literal the table no
// longer produces.
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

// The absent source names nothing as serving it, on any platform.
// The name is the note a form puts beside an entry, and a stream capturing nothing reads from
// nowhere.
func TestAServedSourceNamesWhatServesIt(t *testing.T) {
	for _, info := range audioTestPlatforms {
		for _, s := range AudioSources(info) {
			if s.ID == AudioSourceNone && s.Server != nil {
				t.Errorf("%s names %v as what serves the absent source", info.OS, s.Server)
			}
		}
	}
}

// audioTestSource reads one row out of a platform's answer, failing the test where the table
// declares no such source.
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
