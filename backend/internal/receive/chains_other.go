//go:build !windows

package receive

// What a viewer renders through where nothing chose a chain, Windows aside.
//
// The GL row: the only one that both leaves frames on the GPU and states the colour it produces,
// what a chain nobody picked has to do.
const defaultChain = "gl"
