// Package netspeed measures the machine's upload capacity so the UI can warn
// when a stream needs more bandwidth than the line actually carries.
package netspeed

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync/atomic"
	"time"

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
// missing network, a blocked endpoint, a rejected upload or a timeout all return
// an error the caller surfaces to the user, not an invariant violation. The
// figure every bitrate warning is judged against is worth nothing if it is a
// guess, so a probe that cannot be timed refuses instead of returning one.
func MeasureUplink(ctx context.Context) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, measureTimeout)
	defer cancel()

	payload := make([]byte, payloadBytes)

	// The clock covers the body transfer alone. DNS, the TCP and TLS handshakes
	// and the response are round trips of their own: on a fast line they outweigh
	// the transfer, and counting them as upload time halves the capacity the
	// warnings are judged against. The transport writes the body on its own
	// goroutine, so the two instants cross goroutines as offsets from one base,
	// which also keeps the interval between them on the monotonic clock.
	base := time.Now()
	var bodyStart, bodyEnd atomic.Int64
	trace := &httptrace.ClientTrace{
		WroteHeaders: func() { bodyStart.Store(int64(time.Since(base))) },
		WroteRequest: func(httptrace.WroteRequestInfo) { bodyEnd.Store(int64(time.Since(base))) },
	}

	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodPost, uploadURL, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(payload))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Warnf("uplink probe failed: %v", err)
		return 0, fmt.Errorf("upload: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // drain so the connection can be reused

	// An endpoint that refuses the body answers before it has all of it, so the
	// transfer that was timed is the rejection rather than the payload.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logger.Warnf("uplink probe rejected: %s", resp.Status)
		return 0, fmt.Errorf("upload rejected: %s", resp.Status)
	}

	start, end := bodyStart.Load(), bodyEnd.Load()
	if start == 0 || end <= start {
		return 0, fmt.Errorf("upload finished with no measurable transfer time")
	}
	elapsed := time.Duration(end - start).Seconds()

	mbps := float64(payloadBytes) * 8 / elapsed / 1e6
	logger.Infof("uplink measured: %.1f Mbit/s (%d MiB in %.2fs)", mbps, payloadBytes>>20, elapsed)
	return mbps, nil
}
