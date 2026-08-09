//go:build !windows

package receive

// The chain a viewer renders through when nothing chose one, everywhere but Windows.
//
// It is the GL chain: the one row that both keeps the frames on the GPU and states
// the colour it produces, which is what a default nobody picked has to do.
const defaultChain = "gl"
