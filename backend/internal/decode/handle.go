package decode

// Handle addresses one decode on the host.
//
// toneMap is what the open built with and never changes, so a caller comparing a new request
// against it needs no round trip.
// Every other figure is read through the client, the host owning what a decode is doing.
type Handle struct {
	client  *Client
	id      ID
	toneMap bool
}

// ToneMap is what the pipeline was built with rather than what was asked for.
func (h *Handle) ToneMap() bool { return h.toneMap }

// Stop closes the decode, and succeeds where it has already stopped.
func (h *Handle) Stop() {
	if h == nil {
		return
	}
	h.client.Stop(h.id)
}

// SetAudio sets how loud the decode plays, and refuses a decode that is not open.
func (h *Handle) SetAudio(volume float64, muted bool) error {
	return h.client.SetAudio(h.id, volume, muted)
}

// Subscribe opens one consumer's view of this decode's frames.
func (h *Handle) Subscribe() (Subscription, error) { return h.client.Subscribe(h.id) }
