package audiodev

import (
	"encoding/json"
	"io"
	"os/exec"
	"time"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/platform"
)

// pwDumpExe writes the daemon's objects as JSON,
// the one form carrying a node's properties rather than a line written for a reader.
//
// Under --monitor it writes the whole state as one array
// and then one array per batch of changes, for as long as it runs.
// So the process is the subscription: no poll, and nothing spent between events.
const pwDumpExe = "pw-dump"

// pwDumpArgs subscribe to changes,
// and drop the ANSI colouring written for a terminal, which no parser reads.
var pwDumpArgs = []string{"--monitor", "--no-colors"}

// retryWait is how long a watcher waits before running the tool again.
// A daemon that restarted takes a moment to answer, and a machine that has none
// would otherwise spawn a process in a loop.
const retryWait = 5 * time.Second

// The media class a node carries, which is what sorts it into a kind.
// A sink is what the machine plays into,
// and an output stream one application's own sound on its way to a sink.
// A capture node, "Audio/Source", is nothing either kind holds:
// what a device hears is not what the screen shows.
const (
	classSink   = "Audio/Sink"
	classStream = "Stream/Output/Audio"
)

// monitorSuffix names a sink's recordable side:
// recording what the machine plays is recording the monitor of the sink it plays into.
// The handle is the sink's name plus this, how the Pulse server and both engines spell it.
const monitorSuffix = ".monitor"

// node is the part of a PipeWire object the enumeration reads.
// The daemon reports much more.
//
// Info is a pointer because its absence is the fact a removal carries:
// an object the daemon destroyed is reported as an id and a null.
type node struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Info *struct {
		Props map[string]any `json:"props"`
	} `json:"info"`
}

// watch keeps the held set current for as long as this process runs.
//
// The tool is restarted where it exits, which is what a daemon restart looks like from here.
// Each run re-seeds from the whole state it opens with,
// so a set that went stale while nothing was listening is replaced rather than merged into.
//
// Every failure is an Umgebungsfehler: a machine with no daemon and no tool enumerates nothing,
// every kind keeps its own default, and the first read is released either way.
func watch() {
	for {
		if err := stream(); err != nil {
			logger.Warnf("audio device watch: %v", err)
		}
		markSeeded()
		time.Sleep(retryWait)
	}
}

// stream runs the tool once and folds what it writes into the held set until it exits.
//
// The first array is the daemon's whole state and replaces the set.
// Every array after it is a batch of changes and is folded in.
func stream() error {
	cmd := exec.Command(pwDumpExe, pwDumpArgs...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	decoder := json.NewDecoder(out)
	for first := true; ; first = false {
		var batch []node
		if err := decoder.Decode(&batch); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if first {
			reseed(batch)
			markSeeded()
			continue
		}
		apply(batch)
	}
}

// sortNode is the device one object stands for.
//
// sorted reports whether it is inside a kind this app records.
// classified reports whether the object said what class it is,
// which separates an object that is not ours from an update that did not say.
func sortNode(o node) (device platform.AudioDevice, sorted, classified bool) {
	if o.Info == nil || !isNode(o.Type) {
		return platform.AudioDevice{}, false, false
	}
	class := text(o.Info.Props, "media.class")
	if class == "" {
		return platform.AudioDevice{}, false, false
	}
	name := text(o.Info.Props, "node.name")
	if name == "" {
		return platform.AudioDevice{}, false, true
	}

	switch class {
	case classSink:
		// The monitor rather than the sink: what is recordable is what the machine plays into it.
		return platform.AudioDevice{
			Kind: platform.AudioSourceDesktop,
			ID:   name + monitorSuffix,
			Name: describe(o.Info.Props, name),
		}, true, true
	case classStream:
		return platform.AudioDevice{
			Kind: platform.AudioSourceApplication,
			ID:   name,
			Name: application(o.Info.Props, name),
		}, true, true
	}
	return platform.AudioDevice{}, false, true
}

// isNode reports whether an object is a node, the one interface carrying a media class.
func isNode(kind string) bool {
	const suffix = "Node"
	return len(kind) >= len(suffix) && kind[len(kind)-len(suffix):] == suffix
}

// text is one string property, empty where the node carries none or carries it as another type.
func text(props map[string]any, key string) string {
	s, _ := props[key].(string)
	return s
}

// describe is what a device is called on screen:
// the daemon's own description, or the handle where it wrote none.
func describe(props map[string]any, name string) string {
	if description := text(props, "node.description"); description != "" {
		return description
	}
	return name
}

// application is what one application's stream is called on screen:
// the name it wrote for itself, its binary where it wrote none,
// and the node's handle where it carries neither.
//
// A changeable name is safe here, being the description alone.
// Identity is the node's handle:
// two windows of one program are one binary,
// and a name a program writes for itself is one it can change halfway through a stream.
func application(props map[string]any, name string) string {
	if written := text(props, "application.name"); written != "" {
		return written
	}
	if binary := text(props, "application.process.binary"); binary != "" {
		return binary
	}
	return name
}
