package receive

import (
	"net"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"
)

// Where a pool's descriptors are lent from.
//
// A file descriptor is an index into one process's table, so it is not a value another
// process can be told: the number that names a frame here names something else, or nothing,
// over there. The kernel's own way to move one is SCM_RIGHTS over a Unix socket, which
// installs a descriptor of the receiver's own naming the same file - and that is why the
// pool announces a socket path where every other handle kind announces a number
// (api/proto/screenshare/v1/frame.proto, FramePool.fd_socket).
//
// It answers every connection with the same set, in slot order, for as long as the pool
// lives. A consumer that reconnects is a consumer that reads the same descriptors again,
// which is what makes reading them a repeatable step rather than a one-shot handshake.

// descriptorSocket lends one pool's descriptors.
type descriptorSocket struct {
	// dir is the private directory the socket sits in, removed with it. The directory is
	// what carries the permissions: it is the user's alone, so no second user reaches the
	// socket whatever umask the socket file itself is created under.
	dir      string
	listener *net.UnixListener
	// fds are this process's own descriptors, in slot order. They are lent rather than
	// handed over: a send duplicates the descriptor into the receiver, and closing them
	// stays the pool's job.
	fds []int
}

// lendDescriptors starts answering with these descriptors, in the order they are given.
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

// path is where a consumer connects, which is what travels on the pool.
func (s *descriptorSocket) path() string { return s.listener.Addr().String() }

// serve answers connections until the socket is closed, which is what ends the loop: a
// listener that has been closed fails the accept, and there is nothing left to answer for.
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

// send writes the whole set to one consumer, one message per slot.
//
// One per message rather than all in one, because the payload is the slot's index: the
// consumer reads a descriptor and the number of the slot it belongs to in the same read,
// so an import cannot pair a descriptor with the wrong slot however the messages arrive.
//
// A failed write is the consumer having gone, which costs it the frames it never imported
// and costs the pool nothing.
func (s *descriptorSocket) send(conn *net.UnixConn) {
	for i, fd := range s.fds {
		if _, _, err := conn.WriteMsgUnix([]byte{byte(i)}, unix.UnixRights(fd), nil); err != nil {
			logger.Debugf("a frame consumer stopped reading the descriptors at slot %d: %v", i, err)
			return
		}
	}
}

// close stops answering and takes the socket with it. It is what runs before the pool's
// descriptors are closed, so nobody is lent a number that is about to name nothing.
func (s *descriptorSocket) close() {
	s.listener.Close()
	os.RemoveAll(s.dir)
}

// socketRoot is where the sockets are made.
//
// The runtime directory is the right place for one: it is the session's own, it is cleaned
// up when the session ends, and it is on a filesystem that never persists. The temporary
// directory is the fallback for a session that has none, which is a login without a
// systemd user instance rather than an error.
func socketRoot() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	return os.TempDir()
}
