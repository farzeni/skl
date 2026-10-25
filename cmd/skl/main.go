// Command skl keeps your agent skills in one git repo and loads them on
// every machine. See https://github.com/farzeni/skl.
package main

import (
	"os"

	"github.com/farzeni/skl/internal/cli"
)

func main() { os.Exit(cli.Run()) }
