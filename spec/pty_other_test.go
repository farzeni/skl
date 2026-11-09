//go:build !(linux || darwin)

package spec

import (
	"fmt"
	"os"
)

// ptyrun needs a pseudo-terminal, which the harness opens only on Linux and macOS,
// where CI runs.
func ptyrun() int {
	fmt.Fprintln(os.Stderr, "ptyrun: no pseudo-terminal on this system")
	return 2
}
