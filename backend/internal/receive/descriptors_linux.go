package receive

import (
	"net"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// The lending of a pool's descriptors.
//
// A file descriptor indexes one process's table, which is not a value another process can be told:
// the number naming a frame here names something else, or nothing, over there.
// SCM_RIGHTS over a Unix socket is the kernel's own way to move one,
// installing a descriptor of the receiver's own against the same file,
// so a pool announces a socket path where every other handle kind announces a number
// (api/proto/screenshare/v1/frame.proto, FramePool.fd_socket).
//
// Every connection is answered with the same set, in slot order, for as long as the pool lives.
// Reconnecting reads the same descriptors again,
// which makes reading them a repeatable step rather than a one-shot handshake.

// descriptorSocket answers with one pool's descriptors.
type descriptorSocket struct {
	// dir is the private directory holding the socket, removed with it.
	// Permissions ride on the directory: it belongs to the user alone,
	// so no second user reaches the socket under whatever umask the socket file itself carries.
	dir      string
	listener *net.UnixListener
	// fds are this process's own descriptors, in slot order.
	// Lent and not handed over: a send duplicates the descriptor into the receiver,
	// and closing them remains the pool's job.
	fds []int
}

// lendDescriptors begins answering with these descriptors, in the order given.
func lendDescriptors(fds []int) (*descriptorSocket, error) {
	assert.Assert(len(fds) > 0, "a lent pool holds a descriptor per slot", len(fds))
	for i, fd := range fds {
		assert.Assert(fd >= 0, "an exported slot holds a descriptor", i, fd)
	}

	dir, err := os.MkdirTemp(socketRoot(), "screenshare-frames-")
	if err != nil {
		return nil, err
	}

	listener, err := net.ListenUnix("unix", &net.UnixAddr{
		Name: filepath.Join(dir, "pool.sock"),
		Net:  "unix",
	})
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	socket := &descriptorSocket{dir: dir, listener: listener, fds: append([]int(nil), fds...)}
	go socket.serve()
	return socket, nil
}

// path is where a consumer connects, and what travels on the pool.
func (s *descriptorSocket) path() string { return s.listener.Addr().String() }

// serve answers connections until close ends the loop:
// a closed listener fails the accept, and there is nothing left to answer for.
func (s *descriptorSocket) serve() {
	for {
		conn, err := s.listener.AcceptUnix()
		if err != nil {
			return
		}
		s.send(conn)
		conn.Close()
	}
}

// send writes the whole set to one consumer, a message per slot.
//
// One per message rather than all at once, the payload being the slot's index:
// descriptor and slot number arrive in a single read,
// so no import pairs a descriptor with the wrong slot however the messages turn up.
//
// A failed write is a consumer that has gone,
// costing it the frames it never imported and the pool nothing.
func (s *descriptorSocket) send(conn *net.UnixConn) {
	for i, fd := range s.fds {
		if _, _, err := conn.WriteMsgUnix([]byte{byte(i)}, unix.UnixRights(fd), nil); err != nil {
			logger.Debugf("a frame consumer stopped reading the descriptors at slot %d: %v", i, err)
			return
		}
	}
}

// close stops answering and removes the socket.
// Runs ahead of the pool closing its descriptors, so nobody is lent a number about to name nothing.
func (s *descriptorSocket) close() {
	s.listener.Close()
	os.RemoveAll(s.dir)
}

// socketRoot is the directory the sockets are created under.
//
// The runtime directory suits one: the session's own, cleaned out when the session ends,
// and on a filesystem that never persists.
// The temporary directory stands in for a session that has none,
// a login without a systemd user instance and not an error.
func socketRoot() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	return os.TempDir()
}
