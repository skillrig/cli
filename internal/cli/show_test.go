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

	// Full description, both lines, untruncated.
	for _, want := range []string{
		"Review a terraform plan for risk before apply.",
		"It is read-only and never mutates the plan.",
		"terraform-plan-review  1.4.0  (my-org)",
		"skills/terraform-plan-review",
		"platform-team, terraform, aws",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human output missing %q:\n%s", want, out)
		}
	}

	// requires: a version constraint is parenthesised; a bare tool stands alone.
	if !strings.Contains(out, "requires: oxid (>=0.4.0), terraform") {
		t.Errorf("requires summary wrong, want 'oxid (>=0.4.0), terraform':\n%s", out)
	}

	// Footer hint is the runnable add command.
	if !strings.Contains(out, "→ vendor it: skillrig add terraform-plan-review") {
		t.Errorf("missing runnable footer hint:\n%s", out)
	}
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
