<!-- >>> specledger-generated -->
<!-- Auto-managed by specledger - do not edit this section -->
## Active Technologies

- Go 1.24+ (toolchain in this environment is 1.24.4; 1.25 also fine) — single static binary; cross-OS/arch via goreleaser later, out of scope here
- Go standard `go test`. Two tiers — (a) in-process Cobra unit tests via `SetArgs`/`SetOut`/`SetErr` + table-driven resolver tests; (b) `TestQuickstart_*` integration tests that build and exec the real binary (Constitution II/III).
- Local files only — project `.skillrig/config.toml`, global `~/.config/skillrig/config.toml` (XDG-aware). No database, no network.
- `github.com/spf13/cobra` (command tree); `github.com/pelletier/go-toml/v2` (config read/write — see research.md). Dependencies kept minimal (consume-only
- static binary).
<!-- <<< specledger-generated -->
