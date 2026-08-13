// Package colour reads the transfer characteristic of a video signal and says which ones are HDR.
//
// Both facts are read on three sides.
// The publish child narrows its encoder input to the colour the capture produces,
// the backend refuses an HDR capture in an eight-bit format, and a receive pipeline decides whether
// a stream needs tone mapping.
// A second spelling of "which part of the colorimetry is the transfer" would let one side call a
// surface HDR while another coded it as something else.
//
// Nothing here converts anything.
// What a chain does about an HDR stream belongs with the elements (internal/receive),
// and what a publish does about an HDR capture belongs with the encoder (internal/publish).
package colour

import "strings"

// TransferOfColorimetry is the transfer characteristic in one colorimetry value,
// as GstVideoTransferFunction nicks it, and the empty string where the value carries none.
//
// The field takes two forms and both have to answer alike, because callers hold one against the
// other.
// A capture negotiates one of GStreamer's names for a common combination, and a capsfilter pins
// four numbers separated by colons: range, matrix, transfer, primaries.
// Answering with the number for one and the nick for the other would make every comparison between
// them false.
//
// Every string is a legal input, so there is no precondition to state.
// An unrecognised value is answered with itself rather than refused, because a colorimetry this
// table does not know is a fact about GStreamer's enum and not a bug in this code.
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

// The two transfer characteristics that make a picture HDR, spelled as GstVideoTransferFunction
// names them: PQ is the absolute curve mastered content carries and HLG the broadcast one.
//
// Everything else in that enum describes a standard-range picture whatever its primaries are.
// A wide-gamut SDR desktop is therefore not HDR, and the primaries are deliberately not read
// anywhere.
const (
	TransferPQ  = "smpte2084"
	TransferHLG = "arib-std-b67"
)

// IsHDR reports whether a transfer characteristic is one of the two HDR curves.
//
// A signal carrying no transfer at all is standard range, never "probably HDR".
// Guessing upward is the worse failure of the two: the tag travels with the stream,
// every viewer trusts it, and the picture is then wrong on all of them.
func IsHDR(transfer string) bool {
	return transfer == TransferPQ || transfer == TransferHLG
}

// namedTransfers is the transfer characteristic behind each colorimetry name GStreamer prints in
// place of the four components.
var namedTransfers = map[string]string{
	"bt601":      "bt601",
	"bt709":      "bt709",
	"bt2020":     "bt2020-10",
	"smpte240m":  "smpte240m",
	"sRGB":       "srgb",
	"bt2100-pq":  TransferPQ,
	"bt2100-hlg": TransferHLG,
}

// transferNicks is the GstVideoTransferFunction enum as the colorimetry field spells it in each of
// its two forms: the value a capsfilter pins, and the nick everything else prints.
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
