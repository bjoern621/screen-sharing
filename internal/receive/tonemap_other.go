//go:build !linux

package receive

// Every platform but Linux declares no rung, so a viewer there is offered no tone mapping
// and is told that rather than being offered a conversion nothing performs.
//
// VA-API is the one interface this app can reach that states a tone-mapping filter. What
// Windows converts with, d3d11convert, states two conversion modes - gamma and primaries -
// which are the two the software converter states as well, and neither of them is a
// luminance rolloff. Naming it here would offer a tone map on the strength of a property
// that does something else.
var toneMapping = toneMapRung{}
