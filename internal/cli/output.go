package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/skillrig/cli/pkg/skillcore"
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

// addResultJSON is the complete, untruncated --json view of an add. Keys are
// always present (contract add.md): ok,name,version,path,commit,treeSha,action,
// dryRun. It is the presentation-layer projection of skillcore.AddResult (which
// carries no JSON tags — skillcore stays presentation-free).
type addResultJSON struct {
	OK      bool   `json:"ok"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Path    string `json:"path"`
	Commit  string `json:"commit"`
	TreeSha string `json:"treeSha"`
	Action  string `json:"action"`
	DryRun  bool   `json:"dryRun"`
}

// addFooterHint is the next-step footer for a human add summary (cli.md
// Principle 3: confirmation + the command to run next).
const addFooterHint = "→ commit it, then run: skillrig verify"

// renderAddResult writes an add outcome to w. With jsonOut it emits one complete
// JSON object (all keys present); otherwise a compact human summary (≤2 lines
// incl. the footer hint). Data goes to stdout (the caller passes
// cmd.OutOrStdout()).
func renderAddResult(w io.Writer, r skillcore.AddResult, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)

		return enc.Encode(addResultJSON{
			OK:      true,
			Name:    r.Name,
			Version: r.Version,
			Path:    r.Path,
			Commit:  r.Commit,
			TreeSha: r.TreeSha,
			Action:  string(r.Action),
			DryRun:  r.DryRun,
		})
	}

	_, err := io.WriteString(w, addSummary(r)+"\n"+addFooterHint+"\n")

	return err
}

// addSummary builds the one-line human confirmation for an add. The verb tracks
// the Action (and --dry-run): a fresh/forced placement names the destination +
// short tree-SHA; an idempotent re-add reports no change.
func addSummary(r skillcore.AddResult) string {
	switch r.Action {
	case skillcore.ActionUnchanged:
		return fmt.Sprintf("%s@%s already vendored (no change)", r.Name, r.Version)
	case skillcore.ActionOverwritten:
		return fmt.Sprintf("overwrote %s@%s → %s (treeSha %s)",
			r.Name, r.Version, r.Path, shortSha(r.TreeSha))
	case skillcore.ActionVendored:
		fallthrough
	default:
		verb := "vendored"
		if r.DryRun {
			verb = "would vendor"
		}

		return fmt.Sprintf("%s %s@%s → %s (treeSha %s)",
			verb, r.Name, r.Version, r.Path, shortSha(r.TreeSha))
	}
}

// searchResultJSON is the complete, untruncated --json view of a search. It
// carries the resolved origin and every matching skill with all the fields add
// needs (name, version, namespace, description, topics, path, requires). It is
// the presentation projection of the matched skillcore.CatalogEntry slice (which
// carries JSON tags of its own, reused here for completeness).
type searchResultJSON struct {
	Origin string                   `json:"origin"`
	Skills []skillcore.CatalogEntry `json:"skills"`
}

// searchDescWidth is the human-output truncation width for a skill's one-line
// description (cli.md Principle 3: compact human output, complete --json).
const searchDescWidth = 60

// renderSearchResult writes a search outcome to w. With jsonOut it emits one
// complete JSON object (origin + every matching skill, all fields, [] not null
// when empty); otherwise a compact human list — one line per match (name,
// version, truncated description) plus a summary/footer hint — whose line count
// is bounded by the number of matches plus a small constant (Constitution §II).
// An empty result is "no skills matched" and is still success (exit 0). Data goes
// to stdout (the caller passes cmd.OutOrStdout()).
func renderSearchResult(w io.Writer, origin string, matches []skillcore.CatalogEntry, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)

		skills := matches
		if skills == nil {
			skills = []skillcore.CatalogEntry{}
		}

		return enc.Encode(searchResultJSON{Origin: origin, Skills: skills})
	}

	if len(matches) == 0 {
		_, err := io.WriteString(w, "no skills matched\n"+searchEmptyFooter+"\n")

		return err
	}

	var b strings.Builder

	for _, e := range matches {
		fmt.Fprintf(&b, "%s  %s  — %s\n", e.Name, e.Version, truncateDesc(e.Description))
	}

	fmt.Fprintf(&b, "%d skill(s) · run: skillrig add <name>\n", len(matches))

	_, err := io.WriteString(w, b.String())

	return err
}

// searchEmptyFooter is the next-step hint for an empty search result (still exit 0).
const searchEmptyFooter = "→ broaden the query, or run skillrig search with no filter to list all"

// truncateDesc collapses a description's newlines to spaces and clips it to
// searchDescWidth for compact human output (the complete text is in --json).
func truncateDesc(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= searchDescWidth {
		return s
	}

	return s[:searchDescWidth-1] + "…"
}

// indexResult is the presentation-layer view of an index generation: where the
// catalog was written, how many skills it carries, and the convention it
// declares. It is the single struct both renderers consume.
type indexResult struct {
	Out        string `json:"out"`
	Skills     int    `json:"skills"`
	Convention int    `json:"convention"`
}

// indexFooterHint is the next-step footer for a human index summary.
const indexFooterHint = "→ commit it so search reads the current catalog"

// renderIndexResult writes an index outcome to w. With jsonOut it emits one
// complete JSON object (out, skills, convention — all keys present); otherwise a
// compact human summary (≤2 lines incl. the footer hint). Data goes to stdout
// (the caller passes cmd.OutOrStdout()).
func renderIndexResult(w io.Writer, r indexResult, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)

		return enc.Encode(r)
	}

	summary := fmt.Sprintf("indexed %d skill(s) → %s\n", r.Skills, r.Out)

	_, err := io.WriteString(w, summary+indexFooterHint+"\n")

	return err
}

// verifyReportJSON is the complete, untruncated --json view of a verify report.
// Top-level keys ok,counts,verdicts are always present; counts carries all five
// fields and verdicts every checked skill with all six fields. It is the
// presentation projection of skillcore.Report (which carries no JSON tags).
type verifyReportJSON struct {
	OK       bool          `json:"ok"`
	Counts   countsJSON    `json:"counts"`
	Verdicts []verdictJSON `json:"verdicts"`
}

type countsJSON struct {
	Verified int `json:"verified"`
	Mismatch int `json:"mismatch"`
	Orphan   int `json:"orphan"`
	Missing  int `json:"missing"`
	Dirty    int `json:"dirty"`
}

type verdictJSON struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	Status          string `json:"status"`
	ExpectedTreeSha string `json:"expectedTreeSha"`
	ActualTreeSha   string `json:"actualTreeSha"`
	Reason          string `json:"reason"`
}

// verifyOKFooter / verifyFailFooter are the two-level-output footer hints.
const (
	verifyOKFooter   = "→ all match their recorded version"
	verifyFailFooter = "→ inspect with: skillrig verify --json"
)

// renderVerifyReport writes a verify report to w. With jsonOut it emits one
// complete JSON object (every checked skill present, all keys); otherwise a
// compact human summary whose line count is bounded by the number of findings
// plus a small constant. Data goes to stdout (the caller passes
// cmd.OutOrStdout()).
func renderVerifyReport(w io.Writer, r skillcore.Report, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)

		return enc.Encode(verifyReportJSON{
			OK:       r.OK,
			Counts:   countsJSON(r.Counts),
			Verdicts: verdictsJSON(r.Verdicts),
		})
	}

	if r.OK {
		_, err := io.WriteString(w, fmt.Sprintf("verified %d skills ✓\n%s\n",
			r.Counts.Verified, verifyOKFooter))

		return err
	}

	return renderVerifyFailure(w, r)
}

// renderVerifyFailure writes the compact failing report: a header line, one line
// per failing verdict (passing ones are summarized by the header count), and the
// footer. Bounded by the number of findings (Constitution II).
func renderVerifyFailure(w io.Writer, r skillcore.Report) error {
	total := len(r.Verdicts)
	failed := total - r.Counts.Verified

	var b strings.Builder

	fmt.Fprintf(&b, "verify FAILED: %d of %d skills\n", failed, total)

	for _, v := range r.Verdicts {
		if v.Status == skillcore.StatusOK {
			continue
		}

		fmt.Fprintf(&b, "  ✗ %s  %s\n", v.Name, verdictLine(v))
	}

	b.WriteString(verifyFailFooter + "\n")

	_, err := io.WriteString(w, b.String())

	return err
}

// verdictLine is the compact human explanation for one failing verdict. Mismatch
// shows the recorded vs on-disk short tree-SHAs; the rest use the skillcore
// reason verbatim (already a human-readable phrase).
func verdictLine(v skillcore.Verdict) string {
	switch v.Status {
	case skillcore.StatusMismatch:
		return fmt.Sprintf("content mismatch (recorded %s, on-disk %s)",
			shortSha(v.ExpectedTreeSha), shortSha(v.ActualTreeSha))
	case skillcore.StatusOrphan:
		return "untracked (no lock entry)"
	case skillcore.StatusMissing:
		return "missing (locked but absent on disk and from HEAD)"
	case skillcore.StatusDirty:
		return "uncommitted or locally modified — commit before verifying"
	default:
		return v.Reason
	}
}

// verdictsJSON projects skillcore verdicts into the JSON view. It returns a
// non-nil empty slice so an empty repo serializes verdicts as [] (not null),
// matching the contract.
func verdictsJSON(vs []skillcore.Verdict) []verdictJSON {
	out := make([]verdictJSON, 0, len(vs))
	for _, v := range vs {
		out = append(out, verdictJSON{
			Name:            v.Name,
			Path:            v.Path,
			Status:          v.Status,
			ExpectedTreeSha: v.ExpectedTreeSha,
			ActualTreeSha:   v.ActualTreeSha,
			Reason:          v.Reason,
		})
	}

	return out
}

// shortSha trims a tree/commit SHA to git's conventional 7-char prefix for
// compact human output. Shorter strings (incl. empty) are returned unchanged.
func shortSha(sha string) string {
	const short = 7
	if len(sha) <= short {
		return sha
	}

	return sha[:short]
}
