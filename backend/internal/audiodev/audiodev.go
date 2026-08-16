// Package audiodev enumerates what is inside each audio capture kind on this machine.
//
// Which kinds exist is declared (platform.AudioSources) and is the same on every machine of one
// operating system.
// What is inside a kind is not: which output this machine plays into and which application is
// playing are facts about it at this moment, so they are read off it rather than listed anywhere.
//
// The read is separated from the resolve for the reason the encoder probe's is.
// A form resolves on every keystroke and a resolve is a pure function of the draft and what the
// machine last answered, so it cannot pay for a subprocess.
// The answer is taken once, cached for the process lifetime and read back through Cached
// (docs/development-principles.md).
//
// PipeWire answers, through pw-dump, and it is the right server to ask rather than the convenient
// one.
// It alone reports the applications playing as nodes of their own, which is what the
// per-application kind is.
// Its sinks carry the names its Pulse server serves them under, so one enumeration
// describes what either engine opens: a pulsesrc takes a sink's monitor by name, and only a
// pipewiresrc takes one application's output.
//
// A machine with no PipeWire enumerates nothing and every kind keeps its own default, which is what
// an entry naming no device takes.
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

// pwDumpExe writes every object the daemon holds as JSON, the one form that carries a node's
// properties rather than a line written for a reader.
const pwDumpExe = "pw-dump"

// pwDumpArgs drops the ANSI colouring pw-dump wraps its JSON in for a terminal, which no parser
// reads.
var pwDumpArgs = []string{"--no-colors"}

// The media class a node carries, which is what sorts it into a kind.
// A sink is what the machine plays into, and an output stream one application's own sound on its way
// to a sink.
// A capture node, "Audio/Source", is nothing either kind holds: what a device hears is not what the
// screen shows.
const (
	classSink   = "Audio/Sink"
	classStream = "Stream/Output/Audio"
)

// monitorSuffix names a sink's recordable side: recording what the machine plays is recording the
// monitor of the sink it plays into.
// The handle is the sink's name plus this, which is how the Pulse server and both engines spell it.
const monitorSuffix = ".monitor"

// cached is the enumeration this process took and once guards taking it.
// A sync.Once rather than a nil check: two surfaces may resolve at the same moment, and the answer
// is the machine's rather than one caller's.
var (
	once   sync.Once
	cached []platform.AudioDevice
)

// Cached is what this machine offers inside each kind, enumerated on the first call and answered
// from memory afterwards.
//
// The cache is a departure from reading through, and the enumeration's cost is what pays for it:
// a subprocess against a form that resolves on every keystroke.
// Every caller gets the same slice, which callers read and do not write, and a machine whose
// devices changed since the first call is answered with what was there then.
// PipeWire's own add and remove events are what replaces the cache rather than a shorter lifetime:
// the application that just launched is the one worth selecting, and that is the case a cache gets
// wrong every time.
func Cached(ctx context.Context) []platform.AudioDevice {
	assert.IsNotNil(ctx, "an enumeration runs under a context")

	once.Do(func() { cached = enumerate(ctx) })
	return cached
}

// node is the part of a PipeWire object the enumeration reads; the daemon reports much more.
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
// Each is an Umgebungsfehler and none of them fails the app: a kind with nothing enumerated keeps
// its own default.
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
			// The monitor rather than the sink: what is recordable is what the machine plays into it.
			devices = append(devices, platform.AudioDevice{
				Kind: platform.AudioSourceDesktop,
				ID:   name + monitorSuffix,
				Name: describe(o.Info.Props, name),
			})
		case classStream:
			devices = append(devices, platform.AudioDevice{
				Kind: platform.AudioSourceApplication,
				ID:   name,
				Name: application(o.Info.Props, name),
			})
		}
	}

	// One order per machine, on every run: an option list is built from this, and a list that
	// reshuffled between two resolves would move an entry out from under the cursor.
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

// text is one string property, empty where the node carries none or carries it as another type.
func text(props map[string]any, key string) string {
	s, _ := props[key].(string)
	return s
}

// describe is what a device is called on screen: the daemon's own description, or the handle where
// it wrote none.
func describe(props map[string]any, name string) string {
	if description := text(props, "node.description"); description != "" {
		return description
	}
	return name
}

// application is what one application's stream is called on screen: the name it wrote for itself,
// its binary where it wrote none, and the node's handle where it carries neither.
//
// A changeable name is safe here because it is the description alone.
// Identity is the node's handle, since two windows of one program are one binary and a name a
// program writes for itself is one it can change halfway through a stream.
func application(props map[string]any, name string) string {
	if written := text(props, "application.name"); written != "" {
		return written
	}
	if binary := text(props, "application.process.binary"); binary != "" {
		return binary
	}
	return name
}
