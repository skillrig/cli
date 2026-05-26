package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

// bindResult is the presentation-layer view of an init outcome. It is the
// single struct both renderers consume; it carries no business logic.
type bindResult struct {
	OK         bool   `json:"ok"`
	Origin     string `json:"origin"`
	Scope      string `json:"scope"`
	ConfigPath string `json:"configPath"`
	Written    bool   `json:"written"`
}

// resolveOrderHint is the footer line that teaches the resolution precedence.
// docs/design/cli.md Principle 3 (two-level output: confirmation + next step).
const resolveOrderHint = "→ resolve order: SKILLRIG_ORIGIN > ./.skillrig/config.toml > ~/.config/skillrig/config.toml"

// renderBindResult writes an init result to w. With jsonOut it emits a single
// complete JSON object (all keys present); otherwise a compact human summary
// (≤2 lines including the footer hint). Data goes to stdout (the caller passes
// cmd.OutOrStdout()).
func renderBindResult(w io.Writer, r bindResult, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)

		return enc.Encode(r)
	}

	summary := fmt.Sprintf("bound origin %s (%s: %s)\n", r.Origin, r.Scope, r.ConfigPath)
	if !r.Written {
		summary = fmt.Sprintf("already bound to %s (no change)\n", r.Origin)
	}

	_, err := io.WriteString(w, summary+resolveOrderHint+"\n")

	return err
}
