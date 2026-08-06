package app

import (
	"fmt"

	"bjoernblessin.de/screenshare/internal/moq"
)

// MoqCert reads the relay's Media-over-QUIC certificate fingerprint so a web grid
// tile can pin it through WebTransport's serverCertificateHashes.
//
// It exists as a backend call because the webview cannot make it: the app's own
// origin has never accepted a self-signed relay certificate, so the fetch the
// relay's own page makes fails here before WebTransport is reached (see the moq
// package comment).
//
// The relay location comes from current settings rather than from the caller, on
// the rule the rest of the viewer side follows: a host or port change reaches the
// next tile without the frontend holding a copy of either.
func (a *App) MoqCert(streamName string) (moq.Cert, error) {
	if streamName == "" {
		return moq.Cert{}, fmt.Errorf("no stream named for the MoQ certificate")
	}

	a.settingsMu.Lock()
	host, port := a.settings.RelayHost, a.settings.MoqPort
	a.settingsMu.Unlock()

	return moq.Fetch(a.ctx, host, port, streamName)
}
