// Package netspeed measures uplink capacity, which is the figure a bitrate warning is judged
// against.
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

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

const (
	// uploadURL swallows any POST body and echoes nothing back, so a one-directional probe needs
	// neither an account nor a key.
	uploadURL = "https://speed.cloudflare.com/__up"

	// payloadBytes trades accuracy against duration.
	// 20 MiB leaves TCP slow-start on a home line and still takes a few seconds on a slow uplink.
	payloadBytes = 20 << 20

	// measureTimeout bounds the whole probe, so an absent or stalled network fails instead of pinning
	// a loading state open.
	measureTimeout = 30 * time.Second
)

// MeasureUplink uploads payloadBytes to a public endpoint and answers the throughput in Mbit/s.
//
// Every failure is an Umgebungsfehler and leaves as an error: no network, a blocked endpoint,
// a rejected upload, a timeout.
// A probe that cannot be timed refuses rather than answering with a guess.
func MeasureUplink(ctx context.Context) (float64, error) {
	assert.IsNotNil(ctx, "a probe runs under a context, since its whole bound is a deadline")

	ctx, cancel := context.WithTimeout(ctx, measureTimeout)
	defer cancel()

	payload := make([]byte, payloadBytes)

	// The clock covers the body transfer alone.
	// DNS, the TCP and TLS handshakes and the response are round trips of their own, they outweigh the
	// transfer on a fast line, and counting them as upload time halves the measured capacity.
	// The transport writes the body on its own goroutine, so both instants are offsets from one base,
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
	_, _ = io.Copy(io.Discard, resp.Body) // drained so the connection stays reusable

	// A refused body is answered before all of it arrives, so what was timed is the rejection rather
	// than the payload.
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
	assert.Assert(mbps > 0, "a measured uplink is a positive rate", mbps, elapsed)

	logger.Infof("uplink measured: %.1f Mbit/s (%d MiB in %.2fs)", mbps, payloadBytes>>20, elapsed)
	return mbps, nil
}
