//go:build !linux

package handoff

import (
	"errors"
	"os"
)

// inheritable is only known to work on Linux, which is where the server runs and
// updates itself. Anywhere else the socket is not handed over and the port closes
// for a moment, as it did before.
func inheritable(*os.File) error {
	return errors.New("handing a socket to the next process is only supported on Linux")
}
