// The transport, and the one decision in it:
// this service is reachable over a local socket and over nothing else.
//
// No TCP listener, not even on loopback.
// A loopback port is reachable by every process on the machine,
// and by anything a browser can be talked into fetching,
// and the surface behind this one starts screen captures:
// a page that found the port could have this backend publish the user's screen to a relay it chose.
// A named pipe and a Unix socket carry an identity the operating system enforces instead,
// a DACL naming the owning user or a directory and a file mode naming them,
// which a port number does not.
// The path is also the whole discovery mechanism:
// nothing is scanned for,
// and a shell that cannot open it reports that the backend is not running
// (docs/ipc-api.md, "The format, and why this one").
//
// Listen and Endpoint are per platform, in listen_windows.go and listen_other.go.
// Here is the half both share: registering the service on a gRPC server, running it, stopping it.

package control

import (
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"
)

// stopGrace is what a call in flight has when the service stops, before the connections are cut.
//
// An effect running as the process quits finishes and answers,
// rather than reaching the shell as a connection that broke.
// Bounded because the streaming methods never end on their own:
// Subscribe stays open for as long as its shell holds it,
// so waiting for every call to return would be waiting on calls whose whole job is not returning.
const stopGrace = time.Second

// Service is a running control service.
//
// Package main starts and stops the contract without naming gRPC:
// a Start and a Stop cross that line, and how the calls are carried stays on this side,
// where changing it costs one package instead of two.
type Service struct {
	server *grpc.Server
	// endpoint the listener opened on, kept so a stopped service still names what it served.
	endpoint string
	// once makes Stop idempotent: a shutdown path that runs twice is one shutdown.
	once sync.Once
}

// Start opens this platform's local socket and serves the contract on it.
//
// The whole of what package main needs.
// The address, the permissions and the stale-endpoint handling belong to the platform files,
// so no caller is made to choose between them and hold a fact about the transport.
func Start(srv *Server) (*Service, error) {
	assert.IsNotNil(srv, "a control service serves a server")

	listener, err := Listen()
	if err != nil {
		return nil, err
	}
	return Serve(listener, srv), nil
}

// Serve registers srv on a new gRPC server and accepts on listener.
//
// The accept loop owns a goroutine and this returns as soon as it has started:
// the caller is package main on its way to a window,
// and a service that had to be waited on would block startup on a socket nothing has connected to.
func Serve(listener net.Listener, srv *Server) *Service {
	assert.IsNotNil(listener, "a control service accepts on a listener")
	assert.IsNotNil(srv, "a control service serves a server")

	server := grpc.NewServer()
	screensharev1.RegisterControlServiceServer(server, srv)
	// The frame channel rides the same socket,
	// so framing, versioning and cancellation are not reinvented for a stream of handle metadata.
	// The boundary rule holds across it: one service carries control and description,
	// the other carries handles, and neither carries pixels (docs/ipc-api.md).
	screensharev1.RegisterFrameServiceServer(server, NewFrames(srv.backend))

	service := &Service{server: server, endpoint: listener.Addr().String()}

	go func() {
		logger.Infof("control: serving %s and %s on %s",
			screensharev1.ControlService_ServiceDesc.ServiceName,
			screensharev1.FrameService_ServiceDesc.ServiceName,
			service.endpoint)
		// A stopped server ends its accept loop with no error,
		// so anything arriving here is the listener itself failing.
		// An Umgebungsfehler, so Warnf and never Errorf:
		// the backend goes on capturing and publishing with no shell attached.
		if err := server.Serve(listener); err != nil {
			logger.Warnf("control: stopped accepting on %s: %v", service.endpoint, err)
		}
	}()

	return service
}

// Stop ends the service, and a second Stop is the same one.
//
// Calls in flight get stopGrace to finish, then the connections are cut.
// Cutting them costs nothing here:
// a backend that has quit is the case a shell reports as "the backend is not running",
// rather than one it recovers from.
func (s *Service) Stop() {
	s.once.Do(func() {
		stopped := make(chan struct{})
		go func() {
			s.server.GracefulStop()
			close(stopped)
		}()

		select {
		case <-stopped:
		case <-time.After(stopGrace):
			// Stop closes the open connections, which lets the graceful stop finish.
			// Waiting for it afterwards makes a return from here mean the listener is closed
			// and, on Unix, the socket file unlinked.
			s.server.Stop()
			<-stopped
		}

		logger.Infof("control: stopped serving on %s", s.endpoint)
	})
}
