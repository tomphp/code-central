// The listen errors serve tells apart, on Windows, where Winsock's codes
// aren't syscall's EADDRINUSE and EADDRNOTAVAIL.

package cli

import (
	"errors"

	"golang.org/x/sys/windows"
)

// addrInUse is whether err says another program holds the address.
func addrInUse(err error) bool { return errors.Is(err, windows.WSAEADDRINUSE) }

// addrNotAvail is whether err says this machine doesn't have the address.
func addrNotAvail(err error) bool { return errors.Is(err, windows.WSAEADDRNOTAVAIL) }
