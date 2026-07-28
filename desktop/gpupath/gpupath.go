// Package gpupath declares which capture backend and encoder family pairs hand
// frames to the encoder without a trip through system memory, and resolves the
// memory setting against that table.
//
// A capture backend that produces GPU frames and an encoder that reads GPU surfaces
// can be linked directly: the conversion to the encoder's layout runs on the device
// and no frame crosses the bus. Where either end speaks system memory, every frame is
// downloaded, converted on the CPU, and uploaded again for a surface encoder. The
// difference is a full round trip per frame at capture resolution, which is why the
// pair decides the shape of the whole capture chain rather than one filter in it.
//
// Which pairs have a GPU path is a fact about the two frameworks' elements and
// filters, so it is declared once here and both publish engines read it. An entry
// states that a path exists; each engine builds it in its own vocabulary.
//
// The table answers the pair question alone. Whether the machine running the pair
// captures and encodes on the same GPU is a second precondition, checked by the
// engine that knows how to name its devices, and a mismatch is a refusal rather than
// a demotion (Mismatch, Undetermined).
package gpupath

import (
	"fmt"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/capabilities"
)

// The memory setting: where a run's frames reach the encoder, as the user asks for
// it. The resolved value is one of MemoryGpu and MemorySystem; MemoryAuto is a
// question, not an answer.
const (
	// MemoryAuto takes the GPU path where the pair has one and system memory where it
	// does not. It is the one value every combination satisfies, which is what makes
	// it the default: a codec or capture backend with no GPU path is a setting the
	// user did not get wrong.
	MemoryAuto = "auto"
	// MemoryGpu demands the GPU path and refuses a pair that has none, so a machine
	// that was meant to publish without the round trip says so instead of running at
	// a frame cost nothing on the form names.
	MemoryGpu = "gpu"
	// MemorySystem is the round trip: capture downloads, the CPU converts, and a
	// surface encoder uploads. It is what every pair without a row does, and the
	// answer for a machine whose two GPUs make the import unavailable.
	MemorySystem = "system"
)

// Memories lists the values the setting takes, in UI display order.
var Memories = []string{MemoryAuto, MemoryGpu, MemorySystem}

// Path is one capture backend and encoder family whose frames reach the encoder
// without leaving the GPU.
type Path struct {
	// Engine is the publish engine that runs the pair, one of capabilities.Engines.
	Engine string `json:"engine"`
	// Capture is the capture backend, keyed as publish.Captures names it.
	Capture string `json:"capture"`
	// Family is the encoder family, one of capabilities.Families.
	Family string `json:"family"`
	// Import names what carries the frames from the capture end to the encode end, so
	// a row states which mechanism has to work rather than only that one does.
	Import string `json:"import"`
}

// Paths is the pair table. A pair absent from it encodes from system memory, which is
// the only path when either end has no GPU frames to offer or to read.
//
// The rows are pairs rather than two lists intersected, because neither end implies
// the other. The GStreamer engine's va elements import the portal's DMABuf where its
// nvcodec elements read system memory from the same source, and on the ffmpeg side the
// nvenc encoder reads CUDA frames that no grabber here can hand it: the only CUDA
// filter converting a captured texture to the encoder's layout states no colour
// matrix or range, and a conversion that cannot say what it produced is not one the
// publish leg can encode against.
//
// Only the portal row is held against a device check. The two ffmpeg rows map the
// captured frames onto a device derived from the frames themselves, so the encoder
// runs on the GPU the capture came off by construction. The portal names no device at
// all, and the va elements open their own, so the two are the same one only on a
// machine that has one (Undetermined).
var Paths = []Path{
	{
		Engine:  capabilities.EngineGst,
		Capture: "portal",
		Family:  capabilities.FamilyVaapi,
		Import:  "the va elements import the portal's DMABuf and vapostproc converts on the GPU",
	},
	{
		Engine:  capabilities.EngineFfmpeg,
		Capture: "kmsgrab",
		Family:  capabilities.FamilyVaapi,
		Import:  "the scanout buffer maps to a VAAPI surface and scale_vaapi converts on the GPU",
	},
	{
		Engine:  capabilities.EngineFfmpeg,
		Capture: "ddagrab",
		Family:  capabilities.FamilyQsv,
		Import:  "the Desktop Duplication texture maps to a QSV frame and vpp_qsv converts on the GPU",
	},
}

// For returns the GPU path this engine runs the pair over, and false when the pair
// has none.
func For(engine, capture, family string) (Path, bool) {
	assert.Assert(engine != "", "a GPU path lookup names a publish engine")

	for _, p := range Paths {
		if p.Engine == engine && p.Capture == capture && p.Family == family {
			return p, true
		}
	}
	return Path{}, false
}

// Resolve returns the memory the frames reach the encoder in, MemoryGpu or
// MemorySystem, and refuses a demand the pair cannot meet.
//
// A value outside Memories is refused rather than read as auto. The setting decides
// whether the capture chain downloads every frame, so substituting one would run a
// pipeline the form does not show, and the difference between the two is the whole
// reason the option exists.
func Resolve(engine, capture, family, memory string) (string, error) {
	assert.Assert(engine != "", "a memory resolution names a publish engine")

	_, ok := For(engine, capture, family)
	switch memory {
	case MemoryAuto:
		if ok {
			return MemoryGpu, nil
		}
		return MemorySystem, nil
	case MemoryGpu:
		if !ok {
			return "", fmt.Errorf(
				"%s capture into a %s encoder has no GPU path on the %s engine: its frames reach the encoder through system memory, so pick %q",
				capture, family, engine, MemorySystem)
		}
		return MemoryGpu, nil
	case MemorySystem:
		return MemorySystem, nil
	default:
		return "", fmt.Errorf("frame memory %q is not one of %s", memory, strings.Join(Memories, ", "))
	}
}

// Mismatch is the refusal for a machine that captures on one GPU and encodes on
// another. The import needs one device holding both ends, so the frames would have to
// travel through system memory to reach the encoder, which is the path the setting
// names and not one this takes on its own.
//
// It refuses under MemoryAuto as well. Auto answers whether the pair has a GPU path,
// which this one has; a second GPU is a property of the machine, and demoting for it
// would hand back the round trip the setting was meant to avoid without saying so.
func Mismatch(captureDevice, encodeDevice string) error {
	return fmt.Errorf(
		"capture runs on %s and the encoder on %s: frames cannot be imported across two GPUs, so pick frame memory %q to download them, or move both ends onto one device",
		captureDevice, encodeDevice, MemorySystem)
}

// Undetermined is the refusal for a machine where which GPU captures cannot be read
// off anything. The import is only correct on one device, and a machine offering
// several has no answer this can take, so the run is refused with the candidates
// named rather than started against a guess.
func Undetermined(what string, candidates []string) error {
	return fmt.Errorf(
		"%s, and this machine carries several: %s. Frames can only be imported within one GPU, so pick frame memory %q to download them",
		what, strings.Join(candidates, ", "), MemorySystem)
}
