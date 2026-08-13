// The transport, and the one decision in it: this service is reachable over a local socket and over
// nothing else.
//
// No TCP listener, not even on loopback.
// A loopback port is reachable by every process on the machine and by anything a browser can be
// persuaded to fetch, and the surface behind this one starts screen captures:
// a page that found the port could ask this backend to publish the user's screen to a relay of its
// choosing.
// A named pipe and a Unix socket carry an identity the operating system enforces instead - a DACL
// naming the owning user, a directory and a file mode naming them - which a port number does not.
// That is also why the socket path is the whole discovery mechanism: there is nothing to scan for,
// and a shell that cannot open the path reports that the backend is not running (docs/ipc-api.md,
// "The format, and why this one").
//
// Listen and Endpoint are per platform, in listen_windows.go and listen_other.go.
// What is here is the half that is the same on both: registering the service on a gRPC server,
// running it, and stopping it.

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

// stopGrace is how long a call already in flight has when the service is stopped,
// before the connections are cut.
//
// A grace period exists so that an effect running as the process quits finishes and answers,
// rather than reaching the shell as a connection that broke.
// It is bounded because the one streaming method never ends on its own: Subscribe stays open for as
// long as its shell holds it, so waiting for every call to return would be waiting on a call whose
// whole job is not to return.
const stopGrace = time.Second

// Service is a running control service.
//
// It exists so that package main can start and stop the contract without naming gRPC.
// What crosses to that package is a listener, a Start and a Stop, which is the entire lifecycle;
// how the calls are carried stays on this side of the line, where changing it costs one package
// instead of two.
type Service struct {
	server *grpc.Server
	// endpoint is the address the listener was opened on, kept so a service that has already stopped
	// can still say what it was serving.
	endpoint string
	// once makes Stop idempotent, so a shutdown path that runs twice is one shutdown.
	once sync.Once
}

// Start opens this platform's local socket and serves the contract on it.
//
// It is the whole of what package main needs: the address, the permissions and the stale-endpoint
// handling are the platform files' business, and a caller that had to choose between them would be
// a caller holding a fact about the transport.
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
// It returns as soon as the accepting has begun, and the accepting itself runs on its own
// goroutine, because the caller is package main on its way to a window: a control service that had
// to be waited on would make the shell's startup wait for a socket nothing has connected to yet.
func Serve(listener net.Listener, srv *Server) *Service {
	assert.IsNotNil(listener, "a control service accepts on a listener")
	assert.IsNotNil(srv, "a control service serves a server")

	server := grpc.NewServer()
	screensharev1.RegisterControlServiceServer(server, srv)
	// The frame channel rides the same socket, which is what avoids reinventing framing,
	// versioning and cancellation for a stream of handle metadata.
	// It changes nothing about the boundary rule: one service carries control and description,
	// the other carries handles, and neither carries pixels (docs/ipc-api.md).
	screensharev1.RegisterFrameServiceServer(server, NewFrames(srv.backend))

	service := &Service{server: server, endpoint: listener.Addr().String()}

	go func() {
		logger.Infof("control: serving %s and %s on %s",
			screensharev1.ControlService_ServiceDesc.ServiceName,
			screensharev1.FrameService_ServiceDesc.ServiceName,
			service.endpoint)
		// A stopped server ends its accept loop without an error, so anything reported here is the
		// listener itself failing and is worth a line in the log.
		// It is a warning and not a fatal: the backend keeps capturing and publishing with no shell
		// attached, which is what the contract says a backend without a shell does.
		if err := server.Serve(listener); err != nil {
			logger.Warnf("control: stopped accepting on %s: %v", service.endpoint, err)
		}
	}()

	return service
}

// Stop ends the service.
//
// Calls in flight get stopGrace to finish and then the connections are cut,
// for the reason the constant states.
// Cutting them costs nothing at this point: a shell learns what the backend became from the event
// stream, and a backend that has quit is exactly the case the contract has a shell report as "the
// backend is not running" rather than try to recover from.
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
			// Stop closes the open connections, which is what lets the graceful stop above finish;
			// waiting for it afterwards is what makes this function's return mean the listener is closed
			// and, on Unix, the socket file unlinked.
			s.server.Stop()
			<-stopped
		}

		logger.Infof("control: stopped serving on %s", s.endpoint)
	})
}
