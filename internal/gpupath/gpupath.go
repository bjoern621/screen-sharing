// Package gpupath declares which capture backend and encoder family pairs hand frames to the
// encoder without a trip through system memory, and resolves the memory setting against that table.
//
// A capture backend that produces GPU frames and an encoder that reads GPU surfaces can be linked
// directly: the conversion to the encoder's layout runs on the device and no frame crosses the bus.
// Where either end speaks system memory, every frame is downloaded, converted on the CPU,
// and uploaded again for a surface encoder.
// The difference is a full round trip per frame at capture resolution, which is why the pair
// decides the shape of the whole capture chain rather than one filter in it.
//
// Which pairs have a GPU path is a fact about the two frameworks' elements and filters,
// so it is declared once here and both publish engines read it.
// An entry states that a path exists; each engine builds it in its own vocabulary.
//
// The table answers the pair question alone.
// Whether the machine running the pair captures and encodes on the same GPU is a second
// precondition, checked by the engine that knows how to name its devices, and a mismatch is a
// refusal rather than a demotion (Mismatch, Undetermined).
package gpupath

import (
	"fmt"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/capabilities"
)

// The memory setting: where a run's frames reach the encoder, as the user asks for it.
// The resolved value is one of MemoryGpu, MemoryGpuEncoderColor and MemorySystem;
// MemoryAuto is a question, not an answer.
const (
	// MemoryAuto takes the GPU path where the pair has one and system memory where it does not.
	// It is the one value every combination satisfies, which is what makes it the default:
	// a codec or capture backend with no GPU path is a setting the user did not get wrong.
	MemoryAuto = "auto"
	// MemoryGpu demands the GPU path and refuses a pair that has none, so a machine that was meant to
	// publish without the round trip says so instead of running at a frame cost nothing on the form
	// names.
	// It demands the colour as well: a pair whose only device path lets the encoder convert is refused
	// here rather than served, because a setting that named the memory alone cannot stand for consent
	// to a colour the user did not choose.
	MemoryGpu = "gpu"
	// MemoryGpuEncoderColor demands a device path and accepts the encoder's colour in exchange,
	// which is the only way to ask for the one path where the two cannot both be had.
	// It is a value of its own rather than a relaxation of MemoryGpu so that the trade is recorded in
	// the settings the user can read back, and so that a pair which later grows an exact-colour path
	// stops trading without the setting changing meaning: the demand is for a device path,
	// and this one takes the cost if there is one to take.
	MemoryGpuEncoderColor = "gpu-encoder-color"
	// MemorySystem is the round trip: capture downloads, the CPU converts, and a surface encoder
	// uploads.
	// It is what every pair without a row does, and the answer for a machine whose two GPUs make the
	// import unavailable.
	MemorySystem = "system"
)

// Memories lists the values the setting takes, in UI display order.
var Memories = []string{MemoryAuto, MemoryGpu, MemoryGpuEncoderColor, MemorySystem}

// Path is one capture backend and encoder family whose frames reach the encoder without leaving the
// GPU.
type Path struct {
	// Engine is the publish engine that runs the pair, one of capabilities.Engines.
	Engine string `json:"engine"`
	// Capture is the capture backend, keyed as publish.Captures names it.
	Capture string `json:"capture"`
	// Family is the encoder family, one of capabilities.Families.
	Family string `json:"family"`
	// Import names what carries the frames from the capture end to the encode end,
	// so a row states which mechanism has to work rather than only that one does.
	// It is a code and not a sentence (api/proto/screenshare/v1/text.proto): one per row,
	// because the mechanism is exactly what differs between the rows and a statement general enough to
	// cover all of them would describe none.
	Import screensharev1.TextCode `json:"import"`
	// Colour is what this path does to the colour the settings name, which decides whether a memory
	// setting may resolve to it without being asked twice.
	Colour Colour `json:"colour"`
	// Cost names what a ColourEncoder path takes instead of the settings' colour.
	// The refusal under MemoryGpu carries it, and so does every field the run overrides,
	// so a greyed control states why it is greyed at the control rather than in a log line.
	// A ColourExact row leaves it unset, having taken nothing; what the encoder signals instead is
	// Signalled below, which is what a surface names in the statement's place.
	Cost screensharev1.TextCode `json:"cost"`
	// Signalled is the colour a ColourEncoder path's stream carries, empty on a ColourExact row.
	Signalled Signalled `json:"signalled"`
}

// Paths is the pair table.
// A pair absent from it encodes from system memory, which is the only path when either end has no
// GPU frames to offer or to read.
//
// The rows are pairs rather than two lists intersected, because neither end implies the other.
// The GStreamer engine's va elements import the portal's DMABuf where its nvcodec elements read
// system memory from the same source, and on the ffmpeg side the nvenc encoder reads CUDA frames
// that no grabber here can hand it: the only CUDA filter converting a captured texture to the
// encoder's layout states no colour matrix or range, and a conversion that cannot say what it
// produced is not one the publish leg can encode against.
//
// Only the portal row is held against a device check.
// The two ffmpeg rows map the captured frames onto a device derived from the frames themselves,
// so the encoder runs on the GPU the capture came off by construction.
// The portal names no device at all, and the va elements open their own, so the two are the same
// one only on a machine that has one (Undetermined).
//
// Every row here converts on the device and states the colour it produces,
// so no pair the table currently carries asks the user to choose between hardware and colour.
// The verdict rides on the row rather than following from membership because a pair can have a
// device path with no converter to put on it, and such a row belongs in this table too:
// leaving it out would hide a path that works, and adding it unlabelled would let a default take a
// colour nobody asked for.
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
	// The one row whose colour is the encoder's.
	// Nothing converts between the two ends: hwmap derives neither a CUDA nor a Vulkan device from a
	// Direct3D11 frame, and scale_d3d11 cannot create the encoder's layout from the captured BGRA,
	// so nvenc reads the texture on its own device and converts it itself.
	// What it chose it also signals, so the stream is described truthfully and described as something
	// other than the form shows, which is the whole of what this row costs.
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

// For returns the GPU path this engine runs the pair over, and false when the pair has none.
func For(engine, capture, family string) (Path, bool) {
	assert.Assert(engine != "", "a GPU path lookup names a publish engine")

	for _, p := range Paths {
		if p.Engine == engine && p.Capture == capture && p.Family == family {
			return p, true
		}
	}
	return Path{}, false
}

// Resolve returns the memory the frames reach the encoder in, one of MemoryGpu,
// MemoryGpuEncoderColor and MemorySystem, and refuses a demand the pair cannot meet.
//
// It is the single arbiter: the pair table and the colour verdict on the row it finds are read here
// and nowhere else, so both publish engines and the form take the same answer from the same two
// facts and cannot disagree about what a run will do.
//
// The two demands differ in what they will pay.
// MemoryGpu wants the device path and the colour, and a row offering only the first is a refusal
// naming what it would have cost and both values that get the user publishing again.
// MemoryGpuEncoderColor wants the device path and will pay the colour for it,
// so it takes an encoder-colour row and still answers MemoryGpu where the row converts on the
// device: there is nothing to trade, and refusing a run that gives more than was asked for would be
// a refusal with no cause.
//
// Auto answers whichever of the two costs nothing.
// A pair whose only device path lets the encoder convert resolves to system memory under auto,
// because auto is the value a settings file with no frame memory is filled with and the colour
// fields it would override are ones the user set on purpose.
// Trading them is a choice, and a default is not where a choice gets made.
//
// A value outside Memories is refused rather than read as auto.
// The setting decides whether the capture chain downloads every frame, so substituting one would
// run a pipeline the form does not show, and the difference between the two is the whole reason the
// option exists.
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
			// The refusal names identifiers and the two values that get the user publishing again.
			// What the path costs is the row's Cost code, which the form states on the greyed control
			// itself: an operational error is read once and a greyed control is read where the choice is
			// made.
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

// noPath is the refusal both device demands give a pair the table has no row for.
// The pair is the reason either one fails, so they fail with one sentence:
// nothing about which colour the caller was willing to pay changes that the frames have no way onto
// the device in the first place.
func noPath(engine, capture, family string) error {
	return fmt.Errorf(
		"%s capture into a %s encoder has no GPU path on the %s engine: its frames reach the encoder through system memory, so pick %q",
		capture, family, engine, MemorySystem)
}

// OnDevice reports whether a resolved frame memory keeps the frames on the GPU,
// which is the question a capture chain actually has to answer.
// Both device values link capture to encoder with no download and differ only in who converts,
// so a check written against MemoryGpu alone would silently build the round trip for the other one.
//
// It takes a resolved value, which is why MemoryAuto is not among them: auto is a question Resolve
// has already answered by the time any chain is built, and reading it here would mean a caller
// asked where the frames live before deciding.
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

// Mismatch is the refusal for a machine that captures on one GPU and encodes on another.
// The import needs one device holding both ends, so the frames would have to travel through system
// memory to reach the encoder, which is the path the setting names and not one this takes on its
// own.
//
// It refuses under MemoryAuto as well.
// Auto answers whether the pair has a GPU path, which this one has; a second GPU is a property of
// the machine, and demoting for it would hand back the round trip the setting was meant to avoid
// without saying so.
func Mismatch(captureDevice, encodeDevice string) error {
	return fmt.Errorf(
		"capture runs on %s and the encoder on %s: frames cannot be imported across two GPUs, so pick frame memory %q to download them, or move both ends onto one device",
		captureDevice, encodeDevice, MemorySystem)
}

// Undetermined is the refusal for a machine where which GPU captures cannot be read off anything.
// The import is only correct on one device, and a machine offering several has no answer this can
// take, so the run is refused with the candidates named rather than started against a guess.
func Undetermined(what string, candidates []string) error {
	return fmt.Errorf(
		"%s, and this machine carries several: %s. Frames can only be imported within one GPU, so pick frame memory %q to download them",
		what, strings.Join(candidates, ", "), MemorySystem)
}
