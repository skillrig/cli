// Command skillrig is the SpecLedger skill-management CLI. main is a thin shim:
// all command logic lives in internal/cli, and we map the returned exit code
// to os.Exit so the binary's status is load-bearing for scripts and agents.
package main

import (
	"os"

	"github.com/skillrig/cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
