// Package netspeed measures the machine's upload capacity so the UI can warn
// when a stream needs more bandwidth than the line actually carries.
package netspeed

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

const (
	// uploadURL is Cloudflare's public speed-test upload endpoint. It swallows an
	// arbitrary POST body and echoes nothing back, so it needs no account or API
	// key - ideal for a one-directional uplink probe.
	uploadURL = "https://speed.cloudflare.com/__up"

	// payloadBytes trades measurement accuracy against test duration. 20 MiB is
	// enough for TCP to leave slow-start on a typical home line yet stays a few
	// seconds even on a slow uplink.
	payloadBytes = 20 << 20

	// measureTimeout bounds the whole probe so an absent or stalled network fails
	// fast instead of pinning the button's loading state open forever.
	measureTimeout = 30 * time.Second
)

// MeasureUplink uploads a fixed payload to a public speed-test endpoint and
// returns the observed throughput in Mbit/s. It is an environment operation: a
// missing network, a blocked endpoint or a timeout all return an error the
// caller surfaces to the user, not an invariant violation.
func MeasureUplink(ctx context.Context) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, measureTimeout)
	defer cancel()

	payload := make([]byte, payloadBytes)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(payload))

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Warnf("uplink probe failed: %v", err)
		return 0, fmt.Errorf("upload: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // drain so the connection can be reused
	elapsed := time.Since(start).Seconds()

	assert.Assert(elapsed > 0, "elapsed upload time is positive because a completed multi-megabyte network upload cannot take zero monotonic-clock time")

	mbps := float64(payloadBytes) * 8 / elapsed / 1e6
	logger.Infof("uplink measured: %.1f Mbit/s (%d MiB in %.2fs)", mbps, payloadBytes>>20, elapsed)
	return mbps, nil
}
