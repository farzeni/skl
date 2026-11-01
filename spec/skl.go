package spec

import (
	"os"

	"github.com/farzeni/skl/internal/cli"
)

// runSkl is the test binary acting as skl. It lives outside the _test files
// so that the package has a statement to instrument: a test binary built by
// "go test -cover ./spec" with nothing to count prints "program not built with
// -cover" at startup, on the stderr of every skl a script runs.
func runSkl() { os.Exit(cli.Run()) }
