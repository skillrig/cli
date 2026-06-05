package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/skillrig/cli/pkg/skillcore"
)

// sampleShowEntry is a catalog entry with a long, multi-line description, topics,
// and a mix of version-constrained and bare requires — the full surface
// renderShowResult must project.
func sampleShowEntry() skillcore.CatalogEntry {
	return skillcore.CatalogEntry{
		Name:        "terraform-plan-review",
		Version:     "1.4.0",
		Namespace:   "my-org",
		Description: "Review a terraform plan for risk before apply.\n\nIt is read-only and never mutates the plan.",
		Topics:      []string{"platform-team", "terraform", "aws"},
		Path:        "skills/terraform-plan-review",
		Requires: []skillcore.Require{
			{Tool: "oxid", Version: ">=0.4.0"},
			{Tool: "terraform"},
		},
	}
}

// TestRenderShowResult_Human asserts the human block carries the full
// (untruncated, multi-line) description — the issue #17 contract — plus the
// labelled fields, a "tool (version)"/"tool" requires summary, and a runnable
// footer hint.
func TestRenderShowResult_Human(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	e := sampleShowEntry()
	if err := renderShowResult(&buf, "my-org/my-skills", e, false); err != nil {
		t.Fatalf("renderShowResult: %v", err)
	}

	out := buf.String()

	// Output SHAPE (not just substrings): a fixed header block — header line, then
	// the path/topics/requires field lines in order — a blank-line-separated
	// description body, and the footer as the final line. Asserting the exact lines
	// and their order catches a layout regression a Contains check would miss.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	wantHead := []string{
		"terraform-plan-review  1.4.0  (my-org)",
		"path:     skills/terraform-plan-review",
		"topics:   platform-team, terraform, aws",
		"requires: oxid (>=0.4.0), terraform", // version parenthesised; bare tool stands alone
	}
	for i, want := range wantHead {
		if i >= len(lines) || lines[i] != want {
			t.Fatalf("header line %d = %q, want %q\nfull output:\n%s", i, lineAt(lines, i), want, out)
		}
	}

	// The footer is the final line and is the runnable next-step command.
	if got := lines[len(lines)-1]; got != "→ vendor it: skillrig add terraform-plan-review" {
		t.Errorf("final line = %q, want the add footer hint\nfull output:\n%s", got, out)
	}

	// Exactly one blank line precedes the footer (footer is set off from the body).
	if n := len(lines); n < 2 || lines[n-2] != "" {
		t.Errorf("footer is not preceded by a blank line:\n%s", out)
	}

	// The full description body appears verbatim, both paragraphs, untruncated —
	// the issue #17 contract.
	for _, want := range []string{
		"Review a terraform plan for risk before apply.",
		"It is read-only and never mutates the plan.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing description line %q:\n%s", want, out)
		}
	}
}

// TestSanitizeTerminal asserts the untrusted-catalog guard: control bytes and
// the ESC that introduces an ANSI sequence are dropped (neutralizing the escape,
// leaving only its harmless printable remainder), while newlines are kept only
// when requested. --json is never routed through this, so it is not covered here.
func TestSanitizeTerminal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		in           string
		keepNewlines bool
		want         string
	}{
		{name: "plain text unchanged", in: "terraform-plan-review", want: "terraform-plan-review"},
		{name: "em dash and ellipsis kept (not control)", in: "review — a plan…", want: "review — a plan…"},
		{
			name: "ANSI escape neutralized (ESC dropped, remainder harmless)",
			in:   "\x1b[31mred\x1b[0m",
			want: "[31mred[0m",
		},
		{name: "carriage return and bell dropped", in: "a\rb\x07c", want: "abc"},
		{name: "newline dropped for single-line field", in: "a\nb", keepNewlines: false, want: "ab"},
		{name: "newline kept for body", in: "a\nb", keepNewlines: true, want: "a\nb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := sanitizeTerminal(tt.in, tt.keepNewlines); got != tt.want {
				t.Errorf("sanitizeTerminal(%q, %v) = %q, want %q", tt.in, tt.keepNewlines, got, tt.want)
			}
		})
	}
}

// TestRenderShowResult_StripsControlBytes proves a malicious catalog entry cannot
// inject terminal control sequences through show's human output: an ESC-laden
// name/description is rendered with the ESC bytes removed.
func TestRenderShowResult_StripsControlBytes(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	e := skillcore.CatalogEntry{
		Name:        "evil\x1b[2J",
		Version:     "1.0.0",
		Namespace:   "x",
		Description: "line1\x1b[31m\nline2",
		Path:        "skills/evil",
	}
	if err := renderShowResult(&buf, "o/r", e, false); err != nil {
		t.Fatalf("renderShowResult: %v", err)
	}

	if strings.ContainsRune(buf.String(), '\x1b') {
		t.Errorf("human output contains a raw ESC byte (terminal injection):\n%q", buf.String())
	}
}

// lineAt returns lines[i] or "<none>" when i is out of range, for clearer
// shape-assertion failure messages.
func lineAt(lines []string, i int) string {
	if i < 0 || i >= len(lines) {
		return "<none>"
	}

	return lines[i]
}

// TestRenderShowResult_JSONComplete asserts --json is parseable and structurally
// complete: origin + a skill object with every field, the description untruncated.
func TestRenderShowResult_JSONComplete(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	e := sampleShowEntry()
	if err := renderShowResult(&buf, "my-org/my-skills", e, true); err != nil {
		t.Fatalf("renderShowResult: %v", err)
	}

	var payload struct {
		Origin string                 `json:"origin"`
		Skill  skillcore.CatalogEntry `json:"skill"`
	}

	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("show --json not parseable: %v\n%s", err, buf.String())
	}

	if payload.Origin != "my-org/my-skills" {
		t.Errorf("origin = %q, want my-org/my-skills", payload.Origin)
	}

	if payload.Skill.Name != e.Name || payload.Skill.Version != e.Version || payload.Skill.Path != e.Path {
		t.Errorf("skill round-trip mismatch: %+v", payload.Skill)
	}

	if payload.Skill.Description != e.Description {
		t.Errorf("--json description truncated/altered:\ngot:  %q\nwant: %q", payload.Skill.Description, e.Description)
	}

	if len(payload.Skill.Topics) != 3 || len(payload.Skill.Requires) != 2 {
		t.Errorf("topics/requires not complete: topics=%v requires=%v", payload.Skill.Topics, payload.Skill.Requires)
	}
}

// TestJoinRequires covers the requires summary forms in isolation.
func TestJoinRequires(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []skillcore.Require
		want string
	}{
		{name: "empty", in: nil, want: ""},
		{name: "bare tool only", in: []skillcore.Require{{Tool: "terraform"}}, want: "terraform"},
		{
			name: "version constrained and bare mixed",
			in:   []skillcore.Require{{Tool: "oxid", Version: ">=0.4.0"}, {Tool: "terraform"}},
			want: "oxid (>=0.4.0), terraform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := joinRequires(tt.in); got != tt.want {
				t.Errorf("joinRequires(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
