package receive

// The chain a Windows viewer renders through when nothing chose one.
//
// It is the D3D11 chain because that is the only memory the frame channel can hand
// this platform's window (share_windows.go): a DXGI shared texture is what the
// compositor imports, and it is exported from a Direct3D 11 resource and from
// nothing else. The GL chain is not a second option here - GStreamer's OpenGL on
// Windows is WGL, and the shell's GL is ANGLE over Direct3D, so a texture from one
// is not a texture the other can open.
const defaultChain = "d3d11"
