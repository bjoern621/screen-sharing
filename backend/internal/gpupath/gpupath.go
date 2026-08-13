// Package gpupath declares the capture backend and encoder family pairs whose frames reach the
// encoder without a trip through system memory, and resolves the memory setting against them.
//
// A capture backend producing GPU frames and an encoder reading GPU surfaces link directly: the
// conversion to the encoder's layout runs on the device and no frame crosses the bus.
// Where either end speaks system memory, every frame is downloaded, converted on the CPU and
// uploaded again for a surface encoder.
// That is a full round trip per frame at capture resolution, which is why the pair decides the
// shape of the whole capture chain rather than one filter in it.
//
// Which pairs have a device path is a fact about the two frameworks' elements and filters, declared
// once here and read by both publish engines.
// An entry states that a path exists, and each engine builds it in its own vocabulary.
//
// The table answers the pair question and nothing else.
// Whether the machine captures and encodes on the same GPU is a second precondition, checked by the
// engine that can name its devices, and a machine that cannot be held to one is refused rather than
// demoted (Undetermined).
package gpupath

import (
	"fmt"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
)

// The memory setting: where a run's frames reach the encoder, in the user's own terms.
// A resolved value is MemoryGpu, MemoryGpuEncoderColor or MemorySystem.
// MemoryAuto is a question rather than an answer.
const (
	// MemoryAuto takes the device path where the pair has one and system memory where it does not.
	// Every combination satisfies it, which is what makes it the default: a codec or capture backend
	// with no device path is not a setting the user got wrong.
	MemoryAuto = "auto"
	// MemoryGpu demands the device path and refuses a pair that has none, so a machine meant to
	// publish without the round trip says so rather than paying a frame cost no control names.
	// It demands the colour with it: a pair whose only device path lets the encoder convert is refused
	// here, since a setting naming the memory alone is no consent to a colour nobody chose.
	MemoryGpu = "gpu"
	// MemoryGpuEncoderColor demands a device path and pays the encoder's colour for it, the only way
	// to ask for a path where the two cannot both be had.
	// A value of its own rather than a relaxation of MemoryGpu, so the trade is recorded in settings
	// that can be read back, and so a pair growing an exact-colour path stops trading without the
	// setting changing meaning: the demand is for a device path, and this one takes a cost where there
	// is one.
	MemoryGpuEncoderColor = "gpu-encoder-color"
	// MemorySystem is the round trip: capture downloads, the CPU converts, a surface encoder uploads.
	// It is what a pair with no row does, and the answer for a machine whose second GPU puts the
	// import out of reach.
	MemorySystem = "system"
)

// Memories is the values the setting takes, in display order.
var Memories = []string{MemoryAuto, MemoryGpu, MemoryGpuEncoderColor, MemorySystem}

// Path is a capture backend and an encoder family whose frames reach the encoder without leaving
// the device.
type Path struct {
	// Engine runs the pair, one of capabilities.Engines.
	Engine string `json:"engine"`
	// Capture is the capture backend, keyed as publish.Captures keys it.
	Capture string `json:"capture"`
	// Family is the encoder family, one of capabilities.Families declares.
	Family string `json:"family"`
	// Import names what carries the frames from the capture end to the encode end, so a row states
	// which mechanism has to work rather than only that one does.
	// A code and not a sentence (api/proto/screenshare/v1/text.proto), one per row: the mechanism is
	// what differs between the rows, and a statement general enough to cover all of them describes
	// none.
	Import screensharev1.TextCode `json:"import"`
	// Colour is what this path does to the colour the settings name, and so whether a memory setting
	// may resolve to it without asking twice.
	Colour Colour `json:"colour"`
	// Cost is what a ColourEncoder path takes in place of the settings' colour.
	// The refusal under MemoryGpu carries it, and so does every field the run overrides, so a greyed
	// control states why at the control rather than in a log line.
	// A ColourExact row leaves it unset, having taken nothing, and what the encoder signals instead is
	// Signalled.
	Cost screensharev1.TextCode `json:"cost"`
	// Signalled is the colour a ColourEncoder path's stream carries, unset on a ColourExact row.
	Signalled Signalled `json:"signalled"`
}

// Paths is the table of pairs.
// A pair absent from it encodes from system memory, the only path where either end has no GPU
// frames to offer or to read.
//
// The rows are pairs rather than two lists intersected, since neither end implies the other.
// The GStreamer engine's va elements import the portal's DMABuf where its nvcodec elements take
// system memory from the same source, and on the ffmpeg side nvenc reads CUDA frames no grabber
// here can hand it: the one CUDA filter converting a captured texture to the encoder's layout
// states neither colour matrix nor range, and a conversion that cannot say what it produced is not
// one the publish leg can encode against.
//
// An AMF or a Vulkan encoder reaches the same AMD block VAAPI reaches and still has no row.
// hwmap derives no Vulkan device from kmsgrab's DRM frames, and AMF derives none from them at all,
// so both families download every frame whatever the pair looks like.
//
// The portal row alone is held against a device check.
// A row whose engine half derives its device from the captured frames encodes on the GPU those
// frames came off by construction (internal/ffmpeg, gpuConverts).
// The portal names no device at all and the va elements open their own, so the two are one device
// only on a machine that has one (Undetermined).
//
// The colour verdict rides on the row rather than following from membership, because a pair can
// have a device path with no converter to put on it.
// Such a row belongs in this table too: leaving it out would hide a path that works, and adding it
// unlabelled would let a default take a colour nobody asked for.
var Paths = []Path{
	{
		Engine:  capabilities.EngineGst,
		Capture: "portal",
		Family:  capabilities.FamilyVaapi,
		Import:  screensharev1.TextCode_TEXT_CODE_IMPORT_GST_PORTAL_VAAPI,
		Colour:  ColourExact,
	},
	{
		Engine:  capabilities.EngineGst,
		Capture: "d3d11screencapturesrc",
		Family:  capabilities.FamilyNvenc,
		Import:  screensharev1.TextCode_TEXT_CODE_IMPORT_GST_D3D11_NVENC,
		Colour:  ColourExact,
	},
	{
		Engine:  capabilities.EngineFfmpeg,
		Capture: "kmsgrab",
		Family:  capabilities.FamilyVaapi,
		Import:  screensharev1.TextCode_TEXT_CODE_IMPORT_FFMPEG_KMSGRAB_VAAPI,
		Colour:  ColourExact,
	},
	{
		Engine:  capabilities.EngineFfmpeg,
		Capture: "ddagrab",
		Family:  capabilities.FamilyQsv,
		Import:  screensharev1.TextCode_TEXT_CODE_IMPORT_FFMPEG_DDAGRAB_QSV,
		Colour:  ColourExact,
	},
	// The row whose colour is the encoder's.
	// Nothing converts between the two ends: hwmap derives no CUDA and no Vulkan device from a
	// Direct3D11 frame, and scale_d3d11 builds no encoder layout out of the captured BGRA, so nvenc
	// reads the texture on its own device and converts it there.
	// It signals what it chose, so the stream is described truthfully and described as something other
	// than the form shows, which is the whole of what this row costs.
	{
		Engine:  capabilities.EngineFfmpeg,
		Capture: "ddagrab",
		Family:  capabilities.FamilyNvenc,
		Import:  screensharev1.TextCode_TEXT_CODE_IMPORT_FFMPEG_DDAGRAB_NVENC,
		Colour:  ColourEncoder,
		Cost:    screensharev1.TextCode_TEXT_CODE_COST_ENCODER_SIGNALS_ITS_OWN_COLOUR,
		Signalled: Signalled{
			Matrix: "bt470bg",
			Range:  "tv",
			Chroma: "yuv420p",
		},
	},
}

// For returns the device path this engine runs the pair over, and false where the pair has none.
func For(engine, capture, family string) (Path, bool) {
	assert.Assert(engine != "", "a GPU path lookup names a publish engine")

	for _, p := range Paths {
		if p.Engine == engine && p.Capture == capture && p.Family == family {
			return p, true
		}
	}
	return Path{}, false
}

// Resolve returns the memory the frames reach the encoder in, MemoryGpu, MemoryGpuEncoderColor or
// MemorySystem, and refuses a demand the pair cannot meet.
//
// The single arbiter: the pair table and the colour verdict on the row it finds are read here and
// nowhere else, so both publish engines and the form take one answer from the same two facts.
//
// The two demands differ in what they pay.
// MemoryGpu wants the device path and the colour, so a row offering only the first is refused, with
// what it would have cost and both values that get the user publishing again.
// MemoryGpuEncoderColor wants the device path and pays the colour for it, so it takes an
// encoder-colour row and still answers MemoryGpu where the row converts on the device: there is
// nothing to trade, and refusing a run that gives more than was asked for is a refusal with no
// cause.
//
// Auto answers with whichever of the two costs nothing.
// A pair whose only device path lets the encoder convert falls to system memory under auto, since
// auto is what a settings file with no frame memory is filled with and the colour fields it would
// override were set on purpose.
// Trading them is a choice, and a default does not make choices.
//
// A value outside Memories is refused, never read as auto.
// The setting decides whether the capture chain downloads every frame, so substituting one runs a
// pipeline the form does not show.
func Resolve(engine, capture, family, memory string) (string, error) {
	assert.Assert(engine != "", "a memory resolution names a publish engine")

	path, ok := For(engine, capture, family)
	switch memory {
	case MemoryAuto:
		if ok && !path.Colour.TradesColour() {
			return MemoryGpu, nil
		}
		return MemorySystem, nil
	case MemoryGpu:
		if !ok {
			return "", noPath(engine, capture, family)
		}
		if path.Colour.TradesColour() {
			// The refusal names the pair and the two values that get the user publishing again.
			// What the path costs is the row's Cost code, which the form states on the greyed control:
			// an operational error is read once, a greyed control is read where the choice is made.
			return "", fmt.Errorf(
				"%s capture into a %s encoder on the %s engine reaches the encoder on the GPU but converts nothing on the way. Pick %q to publish on the GPU at that cost, or %q to convert on the CPU and keep the colour selected",
				capture, family, engine, MemoryGpuEncoderColor, MemorySystem)
		}
		return MemoryGpu, nil
	case MemoryGpuEncoderColor:
		if !ok {
			return "", noPath(engine, capture, family)
		}
		if path.Colour.TradesColour() {
			return MemoryGpuEncoderColor, nil
		}
		return MemoryGpu, nil
	case MemorySystem:
		return MemorySystem, nil
	default:
		return "", fmt.Errorf("frame memory %q is not one of %s", memory, strings.Join(Memories, ", "))
	}
}

// noPath is what both device demands answer for a pair the table has no row for.
// The pair is why either fails, so they fail with one sentence: what colour the caller was willing
// to pay changes nothing about frames that have no way onto the device.
func noPath(engine, capture, family string) error {
	return fmt.Errorf(
		"%s capture into a %s encoder has no GPU path on the %s engine: its frames reach the encoder through system memory, so pick %q",
		capture, family, engine, MemorySystem)
}

// OnDevice reports whether a resolved frame memory keeps the frames on the GPU, the question a
// capture chain has to answer.
// Both device values link capture to encoder with no download and differ only in who converts, so a
// check written against MemoryGpu alone builds the round trip for the other one in silence.
//
// It takes a resolved value, which is why MemoryAuto is not among them: Resolve has answered that
// question before any chain is built, and reading it here means a caller asked where the frames
// live before deciding.
func OnDevice(memory string) bool {
	switch memory {
	case MemoryGpu, MemoryGpuEncoderColor:
		return true
	case MemorySystem:
		return false
	default:
		assert.Never("a resolved frame memory is one of the two device values or system memory", memory)
		return false
	}
}

// Undetermined is the refusal for a machine where which GPU captures can be read off nothing.
// The import is correct on one device only, so a machine offering several is refused with the
// candidates named rather than started against a guess.
func Undetermined(what string, candidates []string) error {
	return fmt.Errorf(
		"%s, and this machine carries several: %s. Frames can only be imported within one GPU, so pick frame memory %q to download them",
		what, strings.Join(candidates, ", "), MemorySystem)
}
