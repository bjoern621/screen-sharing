package publish

import (
	"slices"
	"strings"
	"testing"

	"bjoernblessin.de/screenshare/internal/capabilities"
	"bjoernblessin.de/screenshare/internal/platform"
	"bjoernblessin.de/screenshare/internal/settings"
)

// The second track is mixed from a list, and the list is what decides the audio branch's
// shape: one chain per source into one mixer, and one track out of it.

// audioStream is settings this engine publishes with the given sources in the mix.
func audioStream(sources ...settings.AudioSource) settings.Settings {
	s := baseStream()
	s.Publish.Capture, s.Publish.Transport = "ximagesrc", "rtsp"
	s.Publish.Codec, s.Publish.Mode, s.Publish.Chroma = "libx264", capabilities.ModeCbr, "yuv420p"
	s.Publish.Effort, s.Publish.Tune = settings.LadderSteps(s.Publish.Codec, s.Publish.Mode)
	s.Publish.AudioSources = sources
	return s
}

// Every source is its own chain into one mixer, and the encoder reads the mixer. One track
// and not several is carriage: RTMP carries one audio track and the relay re-serves every
// ingest on all of its listeners, so a two-track stream would be unplayable on the narrowest
// leg while the form said it published.
func TestEverySourceIsAChainIntoOneMixer(t *testing.T) {
	s := audioStream(
		settings.AudioSource{Source: platform.AudioSourceDesktop, Gain: settings.GainUnity},
		settings.AudioSource{Source: platform.AudioSourceMic, Gain: 150},
	)

	branch, err := gstAudioBranch(s)
	if err != nil {
		t.Fatalf("building the audio branch: %v", err)
	}
	line := strings.Join(branch, " ")

	if got := strings.Count(line, "pulsesrc"); got != 2 {
		t.Errorf("the branch opens %d devices for two sources: %s", got, line)
	}
	if got := strings.Count(line, gstAudioMixName); got != 1 {
		t.Errorf("the branch carries %d mixers, want exactly one: %s", got, line)
	}
	// Each source opens the device its kind's default names, since neither entry names one
	// of its own.
	for _, want := range []string{
		"device=" + platform.AudioMonitorDevice,
		"device=" + platform.AudioInputDevice,
	} {
		if !slices.Contains(branch, want) {
			t.Errorf("the branch opens no %s: %s", want, line)
		}
	}
}

// A gain reaches the mixer as a multiplier on that source's own branch, and a muted source
// reaches it as zero. Both are one value to an element that multiplies, which is what keeps
// unmuting a write to a running pipeline rather than a rebuild of the graph.
func TestTheGainAndTheMuteReachTheSourcesOwnVolume(t *testing.T) {
	s := audioStream(
		settings.AudioSource{Source: platform.AudioSourceDesktop, Gain: 50},
		settings.AudioSource{Source: platform.AudioSourceMic, Gain: 150, Mute: true},
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

// An entry naming no kind is what a reader turns a source off by, and what the row at the
// end of the list holds. Neither records, so neither is a branch and neither makes the stream
// carry a track.
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

// A source muted at every entry still carries a track. Mute is a level and not a removal:
// the mixer keeps the branch and the stream keeps its track, so unmuting is a value written
// to a pipeline that is already running.
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

// An entry naming its own device opens that one rather than the kind's default, which is
// what the enumeration is for: a machine with two microphones has one entry per microphone
// and neither is "the default input".
func TestAnEntryOpensTheDeviceItNames(t *testing.T) {
	s := audioStream(settings.AudioSource{
		Source: platform.AudioSourceMic,
		Device: "alsa_input.usb-Yeti.analog-stereo",
		Gain:   settings.GainUnity,
	})

	branch, err := gstAudioBranch(s)
	if err != nil {
		t.Fatalf("building the audio branch: %v", err)
	}
	if !slices.Contains(branch, "device=alsa_input.usb-Yeti.analog-stereo") {
		t.Errorf("the branch opens %v, want the device the entry names", branch)
	}
}

// The levels reach a running pipeline, so moving one costs nobody watching a reconnect.
// Adding or taking off a source is a different graph and a relaunch, which is the line
// between the two that this holds.
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
		{Source: platform.AudioSourceMic, Gain: settings.GainUnity},
	}
	if live, _ := LiveOnly(running, added); live {
		t.Error("a source added to the mix was treated as live, and it is a different graph")
	}

	// The question is asked and never answered by changing the settings it was asked
	// about: LiveOnly holds the running levels against a proposal, and a probe that wrote
	// through the shared list would move the caller's own entries.
	if added.Publish.AudioSources[0].Gain != settings.GainUnity {
		t.Error("asking whether a change is live changed the settings it was asked about")
	}
}
