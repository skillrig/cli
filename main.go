// Command skillrig is the SpecLedger skill-management CLI. main is a thin shim:
// all command logic lives in internal/cli, and we map the returned exit code
// to os.Exit so the binary's status is load-bearing for scripts and agents.
package main

import (
	"os"

	"github.com/skillrig/cli/internal/cli"
)

// Build metadata. Defaults describe a local (non-release) build; GoReleaser
// overrides them at release time via -ldflags -X (see .goreleaser.yaml).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(cli.Execute(cli.BuildInfo{Version: version, Commit: commit, Date: date}))
}
