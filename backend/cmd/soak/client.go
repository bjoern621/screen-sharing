package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	v1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// The endpoint the backend serves, derived the way the backend places it.
func socketPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir, _ = os.UserConfigDir()
	}
	return filepath.Join(dir, "screenshare", "control-v1.sock")
}

func dial(sock string) (*grpc.ClientConn, v1.ControlServiceClient, error) {
	conn, err := grpc.NewClient("passthrough:///"+sock,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		}))
	if err != nil {
		return nil, nil, err
	}
	return conn, v1.NewControlServiceClient(conn), nil
}

// waitReady dials until the backend answers a handshake,
// so a run can start the backend and the probe together.
func waitReady(ctx context.Context, c v1.ControlServiceClient, within time.Duration) error {
	deadline := time.Now().Add(within)
	var last error
	for time.Now().Before(deadline) {
		call, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, err := c.Hello(call, &v1.HelloRequest{Client: "soak", ProtocolMajor: 1})
		cancel()
		if err == nil {
			return nil
		}
		last = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return last
}

func withTimeout(ctx context.Context, d time.Duration, fn func(context.Context) error) error {
	call, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return fn(call)
}
