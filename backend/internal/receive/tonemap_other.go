//go:build !linux

package receive

// Every platform but Linux rolls an HDR stream down through the shader rung and no other.
//
// VA-API is the one driver interface this app can reach that states a tone-mapping filter,
// and it is Linux's.
// What Windows converts with, d3d11convert, states two conversion modes, gamma and primaries,
// and neither is a luminance rolloff,
// so naming it as a rung would offer a tone map on a property that does something else.
//
// The shader rung carries its own conversion instead of asking for one,
// so it is as available here as anywhere OpenGL is.
// What it costs here is a round trip: the default chain on Windows is Direct3D 11,
// so a tone-mapped decode leaves GL for system memory and is uploaded again by d3d11upload.
// Only a tile that asked to tone-map pays it, and what it replaces is a tile that cannot.
var toneMapRungs = []toneMapRung{glToneMapRung}
