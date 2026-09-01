package framestamp

// formatMedia is the media type a publish format's bitstream is carried under,
// for the formats a stamp can be written into.
//
// Keyed by the settings' own format names, so a consumer holding one asks about it directly.
// A format absent here has no user-data unit this writes, which is the whole of what Carries
// answers.
var formatMedia = map[string]string{
	"h264": MediaH264,
	"hevc": MediaH265,
}

// Carries reports whether a stream published in this format has a unit a stamp goes into.
//
// Every string is legal input: the value comes off settings or another process,
// so unknown is an answer rather than a broken contract.
func Carries(format string) bool {
	media, known := formatMedia[format]
	if !known {
		return false
	}
	_, writable := headers[media]
	return writable
}
