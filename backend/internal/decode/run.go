package decode

import (
	"net"
	"os"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// Run is the child process, and what Subcommand dispatches to.
//
// It serves the backend on socket and ends when the control connection does, taking every pipeline
// down through GStreamer on the way out.
// The socket is the backend's to create a directory for and to remove, so nothing is cleaned up
// here.
func Run(socket string) error {
	assert.Assert(socket != "", "the decode host names the socket it serves")

	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()

	host := NewReceiveHost()
	defer host.StopAll()

	go func() {
		if err := host.Serve(listener); err != nil {
			logger.Debugf("the decode host stopped serving: %v", err)
		}
	}()

	logger.Infof("decode host serving on %s", socket)
	<-host.Done()
	return nil
}

// Main runs the child and ends the process with it.
// Separate from Run so a caller that wants the error keeps it.
func Main(args []string) {
	if len(args) != 1 {
		logger.Warnf("the decode host takes the socket to serve and nothing else")
		os.Exit(2)
	}
	if err := Run(args[0]); err != nil {
		logger.Warnf("the decode host stopped: %v", err)
		os.Exit(1)
	}
}
