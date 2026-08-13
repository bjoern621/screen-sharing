// Package audiodev enumerates what is inside each audio capture kind on this machine.
//
// Which kinds exist is declared (platform.AudioSources) and the same on every machine of one
// operating system.
// What is inside a kind is not: which microphone is plugged in and which application is playing are
// facts about this machine at this moment, so they are read off it rather than listed anywhere.
//
// The read is separated from the resolve for the reason the encoder probe's is.
// A form resolves on every keystroke and a resolve is a pure function of the draft and what the
// machine last answered, so it cannot pay for a subprocess.
// The answer is taken once, cached for the process lifetime, and read back through Cached
// (docs/development-principles.md).
//
// PipeWire is what answers, through pw-dump, and it is the right server to ask rather than the
// convenient one.
// It is the only one that reports the applications playing as nodes of their own,
// which is what the per-application kind is.
// Its sinks and sources carry the same names its Pulse server serves them under,
// so one enumeration describes what either engine can open: a pulsesrc takes a sink's monitor by
// name, and only a pipewiresrc can take one application's output.
//
// A machine with no PipeWire enumerates nothing and every kind keeps its own default,
// which is the entry that needs no enumeration and what every build before this recorded through.
// Every failure on that path is an Umgebungsfehler, so nothing here asserts on what the daemon
// says.
package audiodev

import (
	"context"
	"encoding/json"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/platform"
)

// pwDumpExe is the tool the enumeration reads, and pwDumpArgs what it is asked for:
// every object the daemon holds, as JSON, which is the one form that carries a node's properties
// rather than a line written for a reader.
const pwDumpExe = "pw-dump"

var pwDumpArgs = []string{"--no-colors"}

// The media classes a node carries, which is how one kind is told from another.
// A sink is something the machine plays into, a source something it hears,
// and an output stream one application's own sound on its way to a sink.
const (
	classSink   = "Audio/Sink"
	classSource = "Audio/Source"
	classStream = "Stream/Output/Audio"
)

// monitorSuffix is what a sink's own recordable side is called.
// Recording what the machine plays is recording the monitor of the sink it plays into,
// and the name is the sink's plus this, which is what both the Pulse server and the engines that
// open it use.
const monitorSuffix = ".monitor"

// cached is the enumeration this process took, and once guards taking it.
// A sync.Once rather than a nil check because two surfaces may resolve at the same moment,
// and the answer is one machine's rather than one caller's.
var (
	once   sync.Once
	cached []platform.AudioDevice
)

// Cached is what this machine offers inside each kind, enumerated on the first call and answered
// from memory afterwards.
//
// Every caller gets the same slice, which callers read and do not write.
// A machine whose devices changed since the first call is answered with what was there then.
// Following PipeWire's own add and remove events is what replaces that, and it is worth its own
// mechanism rather than a shorter cache: the application just launched is the one worth selecting,
// and that is the case a cache gets wrong every time.
func Cached(ctx context.Context) []platform.AudioDevice {
	assert.IsNotNil(ctx, "an enumeration runs under a context")

	once.Do(func() { cached = enumerate(ctx) })
	return cached
}

// node is the part of one PipeWire object this reads.
type node struct {
	Type string `json:"type"`
	Info struct {
		Props map[string]any `json:"props"`
	} `json:"info"`
}

// enumerate reads the daemon's objects and sorts the audio nodes into kinds.
//
// A daemon that is not running, a tool that is not installed and output that does not parse all
// yield nothing rather than an error.
// The absence of an enumeration is not a failure of the app: every kind keeps its own default,
// which is what an entry naming no device takes.
func enumerate(ctx context.Context) []platform.AudioDevice {
	assert.IsNotNil(ctx, "an enumeration runs under a context")

	out, err := exec.CommandContext(ctx, pwDumpExe, pwDumpArgs...).Output()
	if err != nil {
		return nil
	}
	var objects []node
	if err := json.Unmarshal(out, &objects); err != nil {
		return nil
	}

	var devices []platform.AudioDevice
	for _, o := range objects {
		if !strings.HasSuffix(o.Type, "Node") {
			continue
		}
		name := text(o.Info.Props, "node.name")
		if name == "" {
			continue
		}

		switch text(o.Info.Props, "media.class") {
		case classSink:
			// The sink's monitor and not the sink: what is recordable is what the machine plays,
			// and a sink is where it plays it.
			devices = append(devices, platform.AudioDevice{
				Kind: platform.AudioSourceDesktop,
				ID:   name + monitorSuffix,
				Name: describe(o.Info.Props, name),
			})
		case classSource:
			// A source that is itself a monitor is the desktop kind arriving by another route,
			// and listing it under both would offer one device twice.
			if strings.HasSuffix(name, monitorSuffix) {
				continue
			}
			devices = append(devices, platform.AudioDevice{
				Kind: platform.AudioSourceMic,
				ID:   name,
				Name: describe(o.Info.Props, name),
			})
		case classStream:
			// One application's own output.
			// It is named by its binary first and its own name second, because two windows of one program
			// are one binary and a name a program writes for itself is a name it can change while it runs.
			devices = append(devices, platform.AudioDevice{
				Kind: platform.AudioSourceApplication,
				ID:   name,
				Name: application(o.Info.Props, name),
			})
		}
	}

	// Sorted so the same machine answers the same order on every run: the enumeration is what an
	// option list is built from, and a list that reshuffled between resolves would move a reader's
	// entry under their cursor.
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Kind != devices[j].Kind {
			return devices[i].Kind < devices[j].Kind
		}
		return devices[i].ID < devices[j].ID
	})

	for _, d := range devices {
		assert.Assert(d.Kind != "" && d.ID != "", "an enumerated device names its kind and its handle", d.Kind, d.ID)
	}
	return devices
}

// text is one string property, and the empty string where the node carries none.
func text(props map[string]any, key string) string {
	s, _ := props[key].(string)
	return s
}

// describe is what a device is called on screen: the description the daemon wrote for a reader,
// and the handle itself where there is none.
func describe(props map[string]any, name string) string {
	if description := text(props, "node.description"); description != "" {
		return description
	}
	return name
}

// application is what one application's stream is called: its own name where it wrote one,
// and its binary otherwise.
//
// The binary is the identity and the name is the description, which is the order the design asks
// for: two windows of one program are one binary, and a name a program writes for itself is one it
// can change halfway through a stream.
func application(props map[string]any, name string) string {
	if written := text(props, "application.name"); written != "" {
		return written
	}
	if binary := text(props, "application.process.binary"); binary != "" {
		return binary
	}
	return name
}
