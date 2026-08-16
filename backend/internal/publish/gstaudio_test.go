package publish

import (
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The second track is mixed from a list, and that list decides the branch's shape: one chain per
// source into one mixer, and one track out of it.

// audioStream is settings this engine publishes with the given sources in the mix.
func audioStream(sources ...settings.AudioSource) settings.Settings {
	s := baseStream()
	s.Publish.Capture, s.Publish.Transport = "ximagesrc", "rtsp"
	s.Publish.UseCodec("libx264")
	s.Publish.Mode, s.Publish.Chroma = capabilities.ModeCbr, "yuv420p"
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec(), s.Publish.Mode)
	s.Publish.AudioSources = sources
	return s
}

// Every source is a chain of its own into one mixer, and the encoder reads the mixer.
// One track rather than several is carriage: RTMP carries one audio track and the relay re-serves
// every ingest on all of its listeners, so a two-track stream would be unplayable on the narrowest
// leg while the form said it published.
func TestEverySourceIsAChainIntoOneMixer(t *testing.T) {
	s := audioStream(
		settings.AudioSource{Source: platform.AudioSourceDesktop, Gain: settings.GainUnity},
		settings.AudioSource{Source: platform.AudioSourceApplication, Device: "Firefox", Gain: 150},
	)

	branch, err := gstAudioBranch(s)
	if err != nil {
		t.Fatalf("building the audio branch: %v", err)
	}
	line := strings.Join(branch, " ")

	if got := strings.Count(line, gstAudioMixName); got != 1 {
		t.Errorf("the branch carries %d mixers, want exactly one: %s", got, line)
	}
	// A kind is opened by the element its row names, and the desktop entry names no device, so it
	// opens the one its kind's default names.
	for _, want := range []string{
		"pulsesrc",
		"device=" + platform.AudioMonitorDevice,
		"pipewiresrc",
		"target-object=Firefox",
	} {
		if !slices.Contains(branch, want) {
			t.Errorf("the branch opens no %s: %s", want, line)
		}
	}
}

// A gain reaches the mixer as a multiplier on that source's own branch, and a mute reaches it as
// zero.
// Both are one value to an element that multiplies, which keeps unmuting a write to a running
// pipeline rather than a rebuild of the graph.
func TestTheGainAndTheMuteReachTheSourcesOwnVolume(t *testing.T) {
	s := audioStream(
		settings.AudioSource{Source: platform.AudioSourceDesktop, Gain: 50},
		settings.AudioSource{Source: platform.AudioSourceApplication, Gain: 150, Mute: true},
	)

	branch, err := gstAudioBranch(s)
	if err != nil {
		t.Fatalf("building the audio branch: %v", err)
	}
	for i, want := range []string{"volume=0.500", "volume=0.000"} {
		if !slices.Contains(branch, want) {
			t.Errorf("source %d is mixed at %v, want %s", i, branch, want)
		}
		if !slices.Contains(branch, "name="+gstAudioVolumeName(i)) {
			t.Errorf("source %d's volume carries no name for a live write to address: %v", i, branch)
		}
	}
}

// An entry naming no kind is how a source is turned off, and what the row past the end of the list
// holds.
// Neither records, so neither is a branch and neither makes the stream carry a track.
func TestAnEntryWithNoKindIsNoBranch(t *testing.T) {
	s := audioStream(settings.DefaultAudioSource())

	branch, err := gstAudioBranch(s)
	if err != nil {
		t.Fatalf("building the audio branch: %v", err)
	}
	if len(branch) != 0 {
		t.Errorf("an entry naming no kind yields %v, want no branch", branch)
	}
	if track := s.Publish.AudioTrack(); track != capabilities.AudioNone {
		t.Errorf("a list of entries that record nothing carries track %q, want none", track)
	}
}

// A list of nothing but muted sources still carries a track.
// Mute is a level and not a removal: the mixer keeps the branch and the stream keeps its track,
// so unmuting is a value written to a pipeline that is already running.
func TestAMutedSourceStillCarriesATrack(t *testing.T) {
	s := audioStream(settings.AudioSource{Source: platform.AudioSourceDesktop, Mute: true})

	if track := s.Publish.AudioTrack(); track == capabilities.AudioNone {
		t.Error("a muted source carries no track, so unmuting it would be a relaunch")
	}
	branch, err := gstAudioBranch(s)
	if err != nil {
		t.Fatalf("building the audio branch: %v", err)
	}
	if !slices.Contains(branch, "volume=0.000") {
		t.Errorf("a muted source is mixed at %v, want silence", branch)
	}
}

// An entry naming its own device opens that one rather than the kind's default, which is what the
// enumeration is for: a machine playing into a headset and a pair of speakers has an entry per
// output and neither of them is "the default monitor".
func TestAnEntryOpensTheDeviceItNames(t *testing.T) {
	s := audioStream(settings.AudioSource{
		Source: platform.AudioSourceDesktop,
		Device: "alsa_output.usb-Scarlett.analog-stereo.monitor",
		Gain:   settings.GainUnity,
	})

	branch, err := gstAudioBranch(s)
	if err != nil {
		t.Fatalf("building the audio branch: %v", err)
	}
	if !slices.Contains(branch, "device=alsa_output.usb-Scarlett.analog-stereo.monitor") {
		t.Errorf("the branch opens %v, want the device the entry names", branch)
	}
}

// The levels reach a running pipeline, so moving one costs no viewer a reconnect.
// Adding or taking off a source is a different graph and a relaunch, and the line between the two
// is what this pins.
func TestTheLevelsAreLiveAndTheListIsNot(t *testing.T) {
	running := audioStream(
		settings.AudioSource{Source: platform.AudioSourceDesktop, Gain: settings.GainUnity},
	)

	louder := running
	louder.Publish.AudioSources = []settings.AudioSource{
		{Source: platform.AudioSourceDesktop, Gain: 150},
	}
	if live, err := LiveOnly(running, louder); err != nil || !live {
		t.Errorf("a level change asked for a relaunch (%v, %v)", live, err)
	}

	muted := running
	muted.Publish.AudioSources = []settings.AudioSource{
		{Source: platform.AudioSourceDesktop, Gain: settings.GainUnity, Mute: true},
	}
	if live, err := LiveOnly(running, muted); err != nil || !live {
		t.Errorf("a mute asked for a relaunch (%v, %v)", live, err)
	}

	added := running
	added.Publish.AudioSources = []settings.AudioSource{
		{Source: platform.AudioSourceDesktop, Gain: settings.GainUnity},
		{Source: platform.AudioSourceApplication, Gain: settings.GainUnity},
	}
	if live, _ := LiveOnly(running, added); live {
		t.Error("a source added to the mix was treated as live, and it is a different graph")
	}

	// LiveOnly holds the running levels against a proposal and answers, so it writes nothing:
	// the two settings share the entry slice, and a probe writing through it would move the caller's
	// own sources.
	if added.Publish.AudioSources[0].Gain != settings.GainUnity {
		t.Error("asking whether a change is live changed the settings it was asked about")
	}
}

// An application is a PipeWire node and not a sound device, so the element that speaks to nodes
// opens it.
// PulseAudio cannot record one program's stream at all, which is why the kind is this engine's and
// why the source element differs per kind rather than the device string alone.
func TestAnApplicationIsOpenedThroughPipeWire(t *testing.T) {
	s := audioStream(settings.AudioSource{
		Source: platform.AudioSourceApplication,
		Device: "spotify",
		Gain:   settings.GainUnity,
	})

	branch, err := gstAudioBranch(s)
	if err != nil {
		t.Fatalf("building the audio branch: %v", err)
	}
	line := strings.Join(branch, " ")
	if !slices.Contains(branch, "pipewiresrc") {
		t.Errorf("an application is opened by %s, want the element that takes a node", line)
	}
	if !slices.Contains(branch, "target-object=spotify") {
		t.Errorf("the branch targets nothing named: %s", line)
	}
	if slices.Contains(branch, "pulsesrc") {
		t.Errorf("an application is opened through PulseAudio, which cannot record one: %s", line)
	}
}

// The kind is refused on the engine with nothing to open it with, which is a second question from
// whether the platform serves it: the platform answers about the machine, this about the pipeline
// that would run there.
func TestTheApplicationKindIsRefusedOnTheEngineThatCannotOpenIt(t *testing.T) {
	if available, _ := AudioAvailable("ximagesrc", platform.AudioSourceApplication); !available {
		t.Error("the GStreamer engine cannot open an application, and it has the element for it")
	}
	available, reason := AudioAvailable("x11grab", platform.AudioSourceApplication)
	if available {
		t.Error("the ffmpeg engine offers per-application audio, and nothing there records one")
	}
	if reason == nil {
		t.Error("the refusal says nothing, so a greyed entry teaches nothing")
	}
}
