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

// pipeStem is the endpoint docs/ipc-api.md names for Windows, before EnvInstance.
//
// The contract version sits in the name instead of being negotiated on the connection,
// so a v2 is a second pipe: two backends on different majors run side by side,
// and a shell opening the wrong one fails to connect rather than being turned away at Hello.
const pipeStem = `\\.\pipe\mirrorme-control-v1`

// pipeName is the endpoint this run serves, EnvInstance included.
func pipeName() string {
	return pipeStem + instanceSuffix()
}

// Listen opens the named pipe this backend serves the control contract on.
//
// No stale endpoint to clean up, unlike the Unix socket: a named pipe lives only while a process
// holds it and disappears when that process dies, however it dies.
// The same property fails a second backend's Listen rather than letting it steal the name,
// so the error returned here is also the app's "another instance is already running" signal.
func Listen() (net.Listener, error) {
	descriptor, err := pipeSecurity()
	if err != nil {
		return nil, err
	}

	name := pipeName()
	listener, err := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: descriptor})
	if err != nil {
		// A name already held answers ERROR_ACCESS_DENIED, mapped to fs.ErrPermission by syscall.
		// The first instance of a pipe owns the name, and a second creation of it is refused.
		// A name held by another user's process refuses identically, and the conclusion is the same:
		// something else serves this endpoint and nothing will reach this process (ErrAddressInUse).
		if errors.Is(err, fs.ErrPermission) {
			return nil, fmt.Errorf("%w: %s", ErrAddressInUse, name)
		}
		return nil, fmt.Errorf("cannot listen on %s: %w", name, err)
	}
	return listener, nil
}

// Endpoint is the address this platform serves on,
// for the backend's log and for a shell's "the backend is not running" message.
func Endpoint() string {
	return pipeName()
}

// pipeSecurity builds the pipe's security descriptor:
// the user running this backend may open it, nobody else, and not from another machine.
//
// The SDDL is "D:P(D;;GA;;;NU)(A;;GA;;;<this user's SID>)", left to right:
//
//	D:            discretionary ACL, deciding who may open the pipe.
//	P             protected, so no ACE is inherited into it.
//	              Inherited, the pipe would carry a list this code neither chose
//	              nor can predict.
//	(D;;GA;;;NU)  denies everything to NETWORK, the logon type a session arriving
//	              over SMB carries.
//	              A named pipe is reachable remotely as \\host\pipe\name,
//	              and the surface behind this one starts screen captures,
//	              The account is the same either way, so the logon type is what
//	              separates a remote session from the user sitting at the machine.
//	              Written first because a deny ACE binds only what follows it
//	              in evaluation order, and SDDL keeps explicit ACEs in the order
//	              they were written.
//	(A;;GA;;;SID) grants everything to one account: the one running this process.
//
// The SID is looked up rather than written as a well-known alias,
// because the aliases that look right are not.
// CREATOR OWNER is substituted through inheritance alone,
// and grants nothing when named directly on an object created with it.
// Any group this user belongs to admits every other member of that group.
//
// No Administrators ACE and no SYSTEM ACE.
// An administrator can take ownership of the pipe whether or not they are named,
// so naming them buys nothing,
// and widens the list to every account that can elevate on this machine.
func pipeSecurity() (string, error) {
	owner, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("cannot name the user the control pipe belongs to: %w", err)
	}
	assert.Assert(owner.Uid != "", "the user running the backend has a security identifier", owner.Username)

	return "D:P(D;;GA;;;NU)(A;;GA;;;" + owner.Uid + ")", nil
}
