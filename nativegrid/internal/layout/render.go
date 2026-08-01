package layout

// Render is the render-chain choice carried across runs: the chain the window
// renders through, and the streams that render on one of their own.
//
// Streams are keyed by name, for the reason Layout keys them by name: a stream
// index only means something within one run. An entry for a stream the current
// roster does not carry is kept, the rule the display order follows, so a machine
// that goes away and comes back renders the way it was left.
//
// An empty chain is a window that chose nothing, which is the backend's own
// default. It is omitted rather than written as the name that default resolves to,
// so a later build moving its default still reaches a window that never picked one.
type Render struct {
	Chain   string            `json:"renderChain,omitempty"`
	Streams map[string]string `json:"renderChainByStream,omitempty"`
}
