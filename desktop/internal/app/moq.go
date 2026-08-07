package app

import (
	"context"
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
// The endpoint the pin belongs to is dropped here and kept by moqCert. This frontend
// composes the WebTransport URL itself from the settings it holds, which is a second
// definition of the endpoint and the reason the control contract carries the URL with
// the certificate instead.
func (a *App) MoqCert(streamName string) (moq.Cert, error) {
	cert, _, err := a.moqCert(a.ctx, streamName)
	return cert, err
}

// moqCert reads the fingerprint under ctx and names the endpoint it belongs to.
//
// The relay location comes from current settings rather than from the caller, on
// the rule the rest of the viewer side follows: a host or port change reaches the
// next tile without any surface holding a copy of either. The URL is built from the
// same host and port the fingerprint was fetched against, in the same call, so the
// pin and the endpoint it is good for cannot name two different relays.
func (a *App) moqCert(ctx context.Context, streamName string) (moq.Cert, string, error) {
	if streamName == "" {
		return moq.Cert{}, "", fmt.Errorf("no stream named for the MoQ certificate")
	}

	a.settingsMu.Lock()
	host, port := a.settings.RelayHost, a.settings.MoqPort
	a.settingsMu.Unlock()

	cert, err := moq.Fetch(ctx, host, port, streamName)
	if err != nil {
		return moq.Cert{}, "", err
	}
	return cert, moq.WatchURL(host, port, streamName), nil
}
