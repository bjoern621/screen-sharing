//go:build windows

package control

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os/user"

	"github.com/Microsoft/go-winio"

	"bjoernblessin.de/go-utils/util/assert"
)

// pipeName is the endpoint docs/ipc-api.md names for Windows.
//
// The contract version sits in the name instead of being negotiated on the connection, which is
// what makes a v2 a second pipe: two backends on different majors run side by side, and a shell
// opening the wrong one fails to connect rather than connecting and being turned away at Hello.
const pipeName = `\\.\pipe\screenshare-control-v1`

// Listen opens the named pipe this backend serves the control contract on.
//
// No stale endpoint to clean up, unlike the Unix socket: a named pipe lives only while a process
// holds it and disappears when that process dies, however it dies.
// The same property fails a second backend's Listen rather than letting it steal the name, so the
// error returned here is also the app's "another instance is already running" signal.
func Listen() (net.Listener, error) {
	descriptor, err := pipeSecurity()
	if err != nil {
		return nil, err
	}

	listener, err := winio.ListenPipe(pipeName, &winio.PipeConfig{SecurityDescriptor: descriptor})
	if err != nil {
		// A name already held answers ERROR_ACCESS_DENIED, which the syscall package maps to
		// fs.ErrPermission: the first instance of a pipe owns the name, and a second creation of it is
		// refused rather than queued.
		// A name held by another user's process refuses identically, and the conclusion is the same:
		// something else serves this endpoint and nothing will reach this process (ErrAddressInUse).
		if errors.Is(err, fs.ErrPermission) {
			return nil, fmt.Errorf("%w: %s", ErrAddressInUse, pipeName)
		}
		return nil, fmt.Errorf("cannot listen on %s: %w", pipeName, err)
	}
	return listener, nil
}

// Endpoint is the address this platform serves on, for the backend's log and for a shell's "the
// backend is not running" message.
func Endpoint() string {
	return pipeName
}

// pipeSecurity builds the pipe's security descriptor: the user running this backend may open it,
// nobody else may, and not from another machine.
//
// The SDDL is "D:P(D;;GA;;;NU)(A;;GA;;;<this user's SID>)", left to right:
//
//	D:            the discretionary ACL, which decides who may open the pipe.
//	P             protected, so no ACE is inherited into it. Inherited, the pipe would
//	              carry a list this code neither chose nor can predict.
//	(D;;GA;;;NU)  denies everything to NETWORK, the logon type a session that arrived
//	              over SMB carries. A named pipe is reachable remotely as
//	              \\host\pipe\name and the surface behind this one starts screen
//	              captures, so a remote logon as this same account is refused rather
//	              than taken for the user sitting at the machine. Written first because
//	              a deny ACE binds only what follows it in evaluation order, and SDDL
//	              keeps explicit ACEs in the order they were written.
//	(A;;GA;;;SID) grants everything to one account: the one running this process.
//
// The SID is looked up rather than written as a well-known alias, because the aliases that look
// right are not.
// CREATOR OWNER is substituted through inheritance alone and grants nothing when named directly on
// an object created with it, and any group this user belongs to admits every other member of that
// group.
//
// No Administrators ACE and no SYSTEM ACE, deliberately.
// An administrator can take ownership of the pipe whether or not they are named, so naming them
// buys nothing and widens the list to every account that can elevate on this machine.
func pipeSecurity() (string, error) {
	owner, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("cannot name the user the control pipe belongs to: %w", err)
	}
	assert.Assert(owner.Uid != "", "the user running the backend has a security identifier", owner.Username)

	return "D:P(D;;GA;;;NU)(A;;GA;;;" + owner.Uid + ")", nil
}
