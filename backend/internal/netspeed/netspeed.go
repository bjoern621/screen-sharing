// Package netspeed measures uplink capacity, the figure a bitrate warning is judged against.
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
	// uploadURL swallows any POST body and echoes nothing back, so an upload probe needs no account or
	// key.
	uploadURL = "https://speed.cloudflare.com/__up"

	// payloadBytes trades accuracy against duration.
	// 20 MiB leaves TCP slow-start on a home line and still takes a few seconds on a slow uplink.
	payloadBytes = 20 << 20

	// measureTimeout bounds the whole probe, so a stalled network fails instead of pinning a loading
	// state open.
	measureTimeout = 30 * time.Second
)

// MeasureUplink uploads payloadBytes to a public endpoint and answers throughput in Mbit/s.
//
// Every failure is an Umgebungsfehler carried as an error: no network, blocked endpoint, rejected
// upload, timeout.
// A probe that cannot be timed refuses rather than guessing.
func MeasureUplink(ctx context.Context) (float64, error) {
	assert.IsNotNil(ctx, "a probe runs under a context, since its whole bound is a deadline")

	ctx, cancel := context.WithTimeout(ctx, measureTimeout)
	defer cancel()

	payload := make([]byte, payloadBytes)

	// Clock covers the body transfer alone.
	// DNS, the handshakes and the response are round trips of their own and outweigh the transfer
	// on a fast line, so counting them as upload time halves the measured capacity.
	// Transport writes the body on its own goroutine, so both instants are offsets from one base,
	// keeping the interval on the monotonic clock.
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

	// Refusal answers before the whole body arrives, so what was timed is the rejection, not
	// the payload.
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
