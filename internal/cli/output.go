package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode"

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
// description (cli.md Principle 3: compact human output ~80 chars, complete
// --json).
const searchDescWidth = 80

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

	// Align name/version/description into fixed-width columns so the ragged,
	// unreadable output of issue #21 becomes a clean table. The table is rendered
	// before the footer so the summary line is not drawn into the columns.
	rows := make([][]string, 0, len(matches))
	for _, e := range matches {
		// Catalog text is untrusted (fetched per call): strip terminal control
		// bytes from each cell. truncateDesc already collapses newlines, so the
		// single-line form is requested here too.
		rows = append(rows, []string{
			sanitizeTerminal(e.Name, false),
			sanitizeTerminal(e.Version, false),
			"— " + sanitizeTerminal(truncateDesc(e.Description), false),
		})
	}

	if err := writeAlignedColumns(&b, rows); err != nil {
		return err
	}

	fmt.Fprintf(&b, "%d skill(s) · run: skillrig add <name>\n", len(matches))

	_, err := io.WriteString(w, b.String())

	return err
}

// writeAlignedColumns renders rows as space-padded, fixed-width columns via the
// stdlib text/tabwriter (elastic tabstops) so columns line up across rows in
// compact human output. It is the one shared table renderer for search and the
// verify failure list — no duplicated tabwriter setup. Each row's cells are
// tab-joined; the final cell is newline-terminated, so it is never right-padded
// (no trailing whitespace). Cell width is counted in runes (tabwriter assumes
// equal-width code points), matching truncateDesc's rune budget so the column
// math and the clip agree. An empty rows slice writes nothing.
func writeAlignedColumns(w io.Writer, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, cells := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(cells, "\t")); err != nil {
			return err
		}
	}

	return tw.Flush()
}

// showResultJSON is the complete, untruncated --json view of a show: the
// resolved origin and the single matched skill with every field add needs. It
// reuses the skillcore.CatalogEntry JSON tags, so the record is byte-for-byte the
// same shape search emits per entry — one entry, named `skill`.
type showResultJSON struct {
	Origin string                 `json:"origin"`
	Skill  skillcore.CatalogEntry `json:"skill"`
}

// showFooterPrefix is the next-step footer for a human show — the skill name is
// appended so the hint is a runnable command (cli.md Principle 3).
const showFooterPrefix = "→ vendor it: skillrig add "

// renderShowResult writes a single skill's full record to w. With jsonOut it
// emits one complete JSON object (origin + the whole catalog entry, all fields
// present); otherwise a human-friendly labelled block whose defining feature is
// the COMPLETE, untruncated description — the gap issue #17 closes, since search
// clips it to ~80 chars. The block is a fixed handful of header/field lines plus
// the description body and a footer hint. Data goes to stdout (the caller passes
// cmd.OutOrStdout()).
func renderShowResult(w io.Writer, origin string, e skillcore.CatalogEntry, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)

		return enc.Encode(showResultJSON{Origin: origin, Skill: e})
	}

	var b strings.Builder

	// Catalog text is untrusted (fetched per call): strip terminal control bytes
	// from every field. Single-line fields drop newlines too; the description body
	// keeps newlines so a genuinely multi-line description still renders.
	name := sanitizeTerminal(e.Name, false)

	fmt.Fprintf(&b, "%s  %s  (%s)\n", name, sanitizeTerminal(e.Version, false), sanitizeTerminal(e.Namespace, false))
	fmt.Fprintf(&b, "path:     %s\n", sanitizeTerminal(e.Path, false))

	if len(e.Topics) > 0 {
		fmt.Fprintf(&b, "topics:   %s\n", sanitizeTerminal(strings.Join(e.Topics, ", "), false))
	}

	if len(e.Requires) > 0 {
		fmt.Fprintf(&b, "requires: %s\n", sanitizeTerminal(joinRequires(e.Requires), false))
	}

	// The whole point of show: the complete description, untruncated, set off by a
	// blank line so it reads as the body of the record (control bytes stripped,
	// newlines preserved).
	if desc := strings.TrimSpace(sanitizeTerminal(e.Description, true)); desc != "" {
		fmt.Fprintf(&b, "\n%s\n", desc)
	}

	fmt.Fprintf(&b, "\n%s%s\n", showFooterPrefix, name)

	_, err := io.WriteString(w, b.String())

	return err
}

// joinRequires renders a skill's backing-tool requirements as a compact human
// summary — "tool (version)" joined by commas, or a bare tool when no version
// constraint is recorded — mirroring search's requires summary.
func joinRequires(reqs []skillcore.Require) string {
	parts := make([]string, 0, len(reqs))

	for _, r := range reqs {
		part := r.Tool
		if r.Version != "" {
			part += " (" + r.Version + ")"
		}

		parts = append(parts, part)
	}

	return strings.Join(parts, ", ")
}

// searchEmptyFooter is the next-step hint for an empty search result (still exit 0).
const searchEmptyFooter = "→ broaden the query, or run skillrig search with no filter to list all"

// truncateDesc collapses a description's newlines to spaces and clips it to
// searchDescWidth for compact human output (the complete text is in --json). The
// width budget and the clip are counted in runes, not bytes: byte-slicing could
// split a multibyte rune into invalid UTF-8, and rune counting matches how
// tabwriter measures cell width, so the column budget and the alignment agree.
func truncateDesc(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")

	r := []rune(s)
	if len(r) <= searchDescWidth {
		return s
	}

	return string(r[:searchDescWidth-1]) + "…"
}

// sanitizeTerminal strips control characters from catalog-controlled text before
// it is printed to a human terminal. An origin's index.json is fetched and is NOT
// trusted to be free of ANSI escape sequences or other control bytes that could
// spoof the terminal or inject crafted log lines (PR #19 review). Every control
// rune is dropped — including the ESC that introduces an ANSI/CSI sequence, so
// the sequence is neutralized and only its harmless printable remainder (e.g.
// "[31m") survives — except newlines, which are kept when keepNewlines is set so
// show can still render a genuinely multi-line description body. --json output is
// NEVER routed through this: it must carry the exact bytes for an agent/jq.
func sanitizeTerminal(s string, keepNewlines bool) string {
	return strings.Map(func(r rune) rune {
		if keepNewlines && r == '\n' {
			return r
		}

		if unicode.IsControl(r) {
			return -1
		}

		return r
	}, s)
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

	// Align the failing skills' reason column the same way search aligns its
	// table — the marker+name stays the first (tab-terminated) cell so "✗ <name>"
	// reads as one token, and reasons line up across rows.
	rows := make([][]string, 0, failed)

	for _, v := range r.Verdicts {
		if v.Status == skillcore.StatusOK {
			continue
		}

		rows = append(rows, []string{"  ✗ " + v.Name, verdictLine(v)})
	}

	if err := writeAlignedColumns(&b, rows); err != nil {
		return err
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
