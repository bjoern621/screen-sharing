//go:build !windows

package gtkenv

// Pin states the renderer, which is the only one of the look's variables this
// platform does not already answer the way the window wants it.
//
// Pango here builds a fontconfig font map on its own, so the family the pinned
// font names resolves against the machine's fontconfig and rasterizes through
// FreeType, which is exactly what the Windows side is pushed onto. The font
// itself is a dependency of the shell rather than a bundled file (flake.nix),
// because every channel this ships through installs fonts rather than carrying
// them (docs/packaging.md).
func Pin() {
	setIfUnset(rendererVar, renderer)
}
