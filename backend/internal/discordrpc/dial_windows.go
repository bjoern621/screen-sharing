//go:build windows

package discordrpc

import (
	"errors"
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
)

// dial opens the named pipe a running Discord client serves.
// Numbered as the Unix socket is, one per instance running (dial_other.go).
func dial() (net.Conn, error) {
	timeout := dialTimeout
	for i := range socketsPerHost {
		name := fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, i)
		conn, err := winio.DialPipe(name, &timeout)
		if err == nil {
			return conn, nil
		}
	}
	return nil, errors.New(noClientRunning)
}
