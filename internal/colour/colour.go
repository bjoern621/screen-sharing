// Package colour reads the transfer characteristic out of a colorimetry and says which curves are
// HDR (docs/video-stack.md, "The transfer functions").
//
// Both facts are read on three sides: the publish child narrows its encoder input to the colour the
// capture produces, the backend refuses an HDR capture in an eight-bit format, and a receive
// pipeline decides whether a stream needs tone mapping.
// A second spelling of "which part of the colorimetry is the transfer" would let one side call a
// surface HDR while another coded it as something else.
//
// Nothing here converts anything.
// What a chain does about an HDR stream belongs with the elements (internal/receive),
// and what a publish does about an HDR capture belongs with the encoder (internal/publish).
package colour

import "strings"

// TransferOfColorimetry is the transfer characteristic of one colorimetry value,
// as GstVideoTransferFunction nicks it, and the empty string where the value carries none.
//
// The field takes two forms and both answer alike, because callers hold one against the other.
// A capture negotiates one of GStreamer's names for a common combination; a capsfilter pins four
// colon-separated numbers, range:matrix:transfer:primaries, of which the third is read.
// Answering with the number for one form and the nick for the other would make every comparison
// between them false.
//
// Every string is a legal input, so there is no precondition to state.
// An unrecognised value is answered with itself rather than refused: a colorimetry this table does
// not know is a fact about GStreamer's enum and not a bug here.
func TransferOfColorimetry(value string) string {
	if value == "" {
		return ""
	}
	if named, ok := namedTransfers[value]; ok {
		return named
	}
	if parts := strings.Split(value, ":"); len(parts) == 4 {
		if nick, ok := transferNicks[parts[2]]; ok {
			return nick
		}
		return parts[2]
	}
	return value
}

// The transfer characteristics that make a picture HDR, spelled as GstVideoTransferFunction names
// them: PQ the absolute curve mastered content carries, HLG the broadcast one.
//
// Every other curve in that enum describes a standard-range picture whatever its primaries are.
// A wide-gamut SDR desktop is therefore not HDR, and the primaries are read nowhere.
const (
	TransferPQ  = "smpte2084"
	TransferHLG = "arib-std-b67"
)

// IsHDR reports whether a transfer characteristic is one of the HDR curves.
//
// A signal carrying no transfer is standard range, never "probably HDR".
// Guessing upward is the worse failure: the tag travels with the stream, every viewer trusts it,
// and the picture is then wrong on all of them.
func IsHDR(transfer string) bool {
	return transfer == TransferPQ || transfer == TransferHLG
}

// namedTransfers is the transfer behind each colorimetry name GStreamer prints in place of the four
// components.
var namedTransfers = map[string]string{
	"bt601":      "bt601",
	"bt709":      "bt709",
	"bt2020":     "bt2020-10",
	"smpte240m":  "smpte240m",
	"sRGB":       "srgb",
	"bt2100-pq":  TransferPQ,
	"bt2100-hlg": TransferHLG,
}

// transferNicks is the GstVideoTransferFunction enum in both spellings of the colorimetry field:
// the number a capsfilter pins, and the nick everything else prints.
var transferNicks = map[string]string{
	"1":  "gamma10",
	"2":  "gamma18",
	"3":  "gamma20",
	"4":  "gamma22",
	"5":  "bt709",
	"6":  "smpte240m",
	"7":  "srgb",
	"8":  "gamma28",
	"9":  "log100",
	"10": "log316",
	"11": "bt2020-12",
	"12": "adobergb",
	"13": "bt2020-10",
	"14": TransferPQ,
	"15": TransferHLG,
	"16": "bt601",
}
