// Package colour reads the transfer characteristic out of a colorimetry
// and says which curves are HDR (docs/video-stack.md, "The transfer functions").
//
// Read on three sides: publish narrows encoder input to the capture's colour,
// the backend refuses an HDR capture in an eight-bit format,
// and a receive pipeline decides whether a stream needs tone mapping.
// A second spelling of "which part of the colorimetry is the transfer" would let one side call a surface HDR
// while another coded it as something else.
//
// Converts nothing.
// What a chain does about an HDR stream: internal/receive.
// What a publish does about an HDR capture: internal/publish.
package colour

import "strings"

// TransferOfColorimetry is the transfer characteristic of one colorimetry value,
// as GstVideoTransferFunction nicks it, and "" where the value carries none.
//
// Two forms answer alike, callers holding one against the other.
// A capture negotiates one of GStreamer's names for a common combination,
// a capsfilter pins "range:matrix:transfer:primaries" and the third is read.
// A number for one form and a nick for the other would make every comparison between them false.
//
// Every string is legal input.
// An unrecognised value is answered with itself:
// a colorimetry this table does not know is a fact about GStreamer's enum.
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

// The HDR transfer characteristics, as GstVideoTransferFunction names them:
// PQ the absolute curve mastered content carries, HLG the broadcast one.
//
// Every other curve in the enum is standard range whatever its primaries.
// A wide-gamut SDR desktop is not HDR, and the primaries are read nowhere.
const (
	TransferPQ  = "smpte2084"
	TransferHLG = "arib-std-b67"
)

// IsHDR reports whether a transfer characteristic is one of the HDR curves.
//
// An unrecognised or ambiguous transfer reads as standard range, never guessed HDR.
// Guessing upward is the worse failure: the tag travels with the stream, every viewer trusts it,
// and the picture is wrong on all of them.
func IsHDR(transfer string) bool {
	return transfer == TransferPQ || transfer == TransferHLG
}

// namedTransfers is the transfer behind each colorimetry name GStreamer prints in place of the four components.
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
