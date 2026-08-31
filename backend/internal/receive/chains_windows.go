package receive

// What a Windows viewer renders through where nothing chose a chain.
//
// D3D11, that being the only memory the frame channel can hand this platform's window
// (share_windows.go): the compositor imports a DXGI shared texture,
// which is exported from a Direct3D 11 resource and from nothing else.
// GL is no second option here,
// GStreamer's OpenGL on Windows being WGL against the shell's ANGLE over Direct3D,
// and neither opens the other's texture.
const defaultChain = "d3d11"
