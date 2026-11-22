//go:build linux

package handoff

import (
	"os"
	"syscall"
)

// inheritable lets a file survive an exec: Go opens everything close-on-exec.
func inheritable(f *os.File) error {
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_SETFD, 0)
	if errno != 0 {
		return errno
	}
	return nil
}
