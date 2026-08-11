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
// The contract version is in the name rather than negotiated on the connection, which
// is what makes a v2 a second pipe: two backends on different majors can then run side
// by side, and a shell that opens the wrong one fails to connect instead of connecting
// and being turned away at Hello.
const pipeName = `\\.\pipe\screenshare-control-v1`

// Listen opens the named pipe this backend serves the control contract on.
//
// There is no stale endpoint to clean up here, unlike on Unix: a named pipe exists only
// while a process holds it and disappears when that process dies, however it dies. The
// same property is what makes a second backend's Listen fail rather than steal the
// name, so the error this returns is also the app's "another instance is already
// running" signal.
func Listen() (net.Listener, error) {
	descriptor, err := pipeSecurity()
	if err != nil {
		return nil, err
	}

	listener, err := winio.ListenPipe(pipeName, &winio.PipeConfig{SecurityDescriptor: descriptor})
	if err != nil {
		// A name already held answers ERROR_ACCESS_DENIED, which the syscall package maps
		// to this: the first instance of a pipe owns the name, and a second creation of it
		// is refused rather than queued. A name held by another user's process refuses the
		// same way, and the conclusion is the same one - something else is serving this
		// endpoint and this process will not be reached (ErrAddressInUse).
		if errors.Is(err, fs.ErrPermission) {
			return nil, fmt.Errorf("%w: %s", ErrAddressInUse, pipeName)
		}
		return nil, fmt.Errorf("cannot listen on %s: %w", pipeName, err)
	}
	return listener, nil
}

// Endpoint names the address this platform serves on, for the backend's log and for a
// shell's "the backend is not running" message.
func Endpoint() string {
	return pipeName
}

// pipeSecurity builds the pipe's security descriptor: nobody but the user running this
// backend may open it, and not from another machine.
//
// The SDDL is "D:P(D;;GA;;;NU)(A;;GA;;;<this user's SID>)", read left to right:
//
//	D:            this is the discretionary ACL, the list that decides who may open the
//	              pipe.
//	P             makes it protected, so no ACE is inherited into it. Without it the
//	              pipe would also carry whatever it inherits, which is not a list this
//	              code chose and not one it can predict.
//	(D;;GA;;;NU)  denies everything to NETWORK, the logon type a session that arrived
//	              over SMB carries. A named pipe is reachable remotely as
//	              \\host\pipe\name, and the surface behind this one starts screen
//	              captures, so a remote logon as this same account is refused rather
//	              than treated as the user sitting at the machine. It is written first
//	              because a deny ACE only binds what follows it in evaluation order, and
//	              SDDL keeps explicit ACEs in the order they are written.
//	(A;;GA;;;SID) grants everything to exactly one account: the one running this
//	              process.
//
// The SID is looked up rather than written as a well-known alias, because the aliases
// that look right are not. CREATOR OWNER is substituted only through inheritance and
// grants nothing on an object created with it named directly, and any group this user
// belongs to would also admit every other member of that group.
//
// Nothing else appears - no Administrators ACE, no SYSTEM ACE - and the omission is
// deliberate. An administrator can take ownership of the pipe whether or not they are
// named, so naming them buys nothing and widens the list to every account that can
// elevate on this machine.
func pipeSecurity() (string, error) {
	owner, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("cannot name the user the control pipe belongs to: %w", err)
	}
	assert.Assert(owner.Uid != "", "the user running the backend has a security identifier", owner.Username)

	return "D:P(D;;GA;;;NU)(A;;GA;;;" + owner.Uid + ")", nil
}
