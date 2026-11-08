//go:build unix

// The listen errors serve tells apart, on Unix.

package cli

import (
	"errors"
	"syscall"
)

// addrInUse is whether err says another program holds the address.
func addrInUse(err error) bool { return errors.Is(err, syscall.EADDRINUSE) }

// addrNotAvail is whether err says this machine doesn't have the address.
func addrNotAvail(err error) bool { return errors.Is(err, syscall.EADDRNOTAVAIL) }
