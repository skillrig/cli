package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/skillrig/cli/internal/config"
	"github.com/skillrig/cli/pkg/skillcore"
)

// TestExitCodeFor pins the load-bearing typed switch: nil → 0, a verification
// failure → 2 (even wrapped), everything else (incl. *UsageError) → 1.
func TestExitCodeFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil is ok", nil, ExitOK},
		{"usage error", &UsageError{Msg: "bad"}, ExitUsage},
		{"plain error", errors.New("boom"), ExitUsage},
		{"verify failure", &skillcore.VerifyFailure{}, ExitVerification},
		{"wrapped verify failure", fmt.Errorf("ctx: %w", &skillcore.VerifyFailure{}), ExitVerification},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := exitCodeFor(tt.err); got != tt.want {
				t.Errorf("exitCodeFor(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

// TestMapAddError asserts each skillcore add error maps to a navigational
// *UsageError with the distinguishing what/why/fix, and preserves the raw cause.
func TestMapAddError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		wantParts []string
	}{
		{
			"origin checkout missing",
			&skillcore.OriginNotFoundError{OriginDir: "/repo/my-org/my-skills"},
			[]string{"origin checkout not found", "/repo/my-org/my-skills", "check out the origin", "init --origin"},
		},
		{
			"skill not found",
			&skillcore.SkillNotFoundError{Skill: "x"},
			[]string{"not found in origin", "check the skill name"},
		},
		{
			"overwrite refused",
			&skillcore.OverwriteError{Skill: "x", Path: ".agents/skills/x"},
			[]string{"refusing to overwrite", "--force"},
		},
		{
			"git error",
			&skillcore.GitError{ExitCode: 128, Stderr: "fatal"},
			[]string{"git error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := mapAddError("x", "my-org/my-skills", tt.err)

			var ue *UsageError
			if !errors.As(got, &ue) {
				t.Fatalf("mapAddError = %T, want *UsageError", got)
			}

			for _, part := range tt.wantParts {
				if !strings.Contains(ue.Msg, part) {
					t.Errorf("message %q missing %q", ue.Msg, part)
				}
			}

			if !errors.Is(got, tt.err) {
				t.Errorf("mapAddError dropped the raw cause %v", tt.err)
			}
		})
	}
}

// TestOriginDirRef maps a resolved origin to (relative owner/repo dir, ref) and
// defaults an empty ref to HEAD.
func TestOriginDirRef(t *testing.T) {
	t.Parallel()

	dir, ref := originDirRef(config.Origin{Owner: "my-org", Repo: "my-skills"})
	if dir != "my-org/my-skills" || ref != "HEAD" {
		t.Errorf("originDirRef = (%q,%q), want (my-org/my-skills, HEAD)", dir, ref)
	}

	_, ref = originDirRef(config.Origin{Owner: "my-org", Repo: "my-skills", Ref: "staging"})
	if ref != "staging" {
		t.Errorf("ref = %q, want staging", ref)
	}
}

// TestAddArgsValidator: misinvocation is navigational (a *UsageError), one arg ok.
func TestAddArgsValidator(t *testing.T) {
	t.Parallel()

	cmd := newAddCmd(&globalOpts{})

	for _, args := range [][]string{nil, {"a", "b"}} {
		err := cmd.Args(cmd, args)

		var ue *UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("Args(%v) = %T, want *UsageError", args, err)
		}

		if !strings.Contains(ue.Msg, "skillrig add <skill>") {
			t.Errorf("Args(%v) message missing the fix example: %q", args, ue.Msg)
		}
	}

	if err := cmd.Args(cmd, []string{"terraform-plan-review"}); err != nil {
		t.Errorf("Args(one arg) = %v, want nil", err)
	}
}

// TestVerifyArgsValidator: an extra positional is navigational, no args ok.
func TestVerifyArgsValidator(t *testing.T) {
	t.Parallel()

	cmd := newVerifyCmd(&globalOpts{})

	err := cmd.Args(cmd, []string{"extra"})

	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("Args(extra) = %T, want *UsageError", err)
	}

	if !strings.Contains(ue.Msg, "verify takes no arguments") {
		t.Errorf("message missing the what: %q", ue.Msg)
	}

	if err := cmd.Args(cmd, nil); err != nil {
		t.Errorf("Args(no args) = %v, want nil", err)
	}
}

// TestRenderAddResult_Human asserts the compact human shapes per Action.
func TestRenderAddResult_Human(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		res  skillcore.AddResult
		want string
	}{
		{"vendored", skillcore.AddResult{Name: "tf", Version: "1.4.0", Path: ".agents/skills/tf", TreeSha: "abc1234def", Action: skillcore.ActionVendored}, "vendored tf@1.4.0"},
		{"unchanged", skillcore.AddResult{Name: "tf", Version: "1.4.0", Action: skillcore.ActionUnchanged}, "already vendored (no change)"},
		{"dry-run", skillcore.AddResult{Name: "tf", Version: "1.4.0", Action: skillcore.ActionVendored, DryRun: true}, "would vendor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var b bytes.Buffer
			if err := renderAddResult(&b, tt.res, false); err != nil {
				t.Fatalf("renderAddResult: %v", err)
			}

			out := b.String()
			if !strings.Contains(out, tt.want) {
				t.Errorf("human output %q missing %q", out, tt.want)
			}

			if lines := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1; lines > 2 {
				t.Errorf("human output has %d lines, want <= 2:\n%s", lines, out)
			}

			if !strings.Contains(out, "skillrig verify") {
				t.Errorf("missing next-step footer: %q", out)
			}
		})
	}
}

// TestRenderAddResult_JSON asserts the complete --json view (all keys, lowercased action).
func TestRenderAddResult_JSON(t *testing.T) {
	t.Parallel()

	res := skillcore.AddResult{Name: "tf", Version: "1.4.0", Path: ".agents/skills/tf", Commit: "c0ffee", TreeSha: "deadbeef", Action: skillcore.ActionVendored}

	var b bytes.Buffer

	if err := renderAddResult(&b, res, true); err != nil {
		t.Fatalf("renderAddResult json: %v", err)
	}

	var obj map[string]any
	if err := json.Unmarshal(b.Bytes(), &obj); err != nil {
		t.Fatalf("json.Unmarshal: %v\n%s", err, b.String())
	}

	for _, k := range []string{"ok", "name", "version", "path", "commit", "treeSha", "action", "dryRun"} {
		if _, present := obj[k]; !present {
			t.Errorf("json missing key %q: %v", k, obj)
		}
	}

	if obj["action"] != "vendored" || obj["ok"] != true {
		t.Errorf("json action/ok = %v/%v, want vendored/true", obj["action"], obj["ok"])
	}
}

// TestRenderVerifyReport_JSONComplete asserts the report JSON is structurally
// complete for both empty and populated reports (counts has all five keys;
// verdicts is [] not null when empty; each verdict carries all six fields).
func TestRenderVerifyReport_JSONComplete(t *testing.T) {
	t.Parallel()

	reports := map[string]skillcore.Report{
		"empty": {OK: true},
		"populated": {
			OK:     false,
			Counts: skillcore.Counts{Verified: 1, Mismatch: 1},
			Verdicts: []skillcore.Verdict{
				{Name: "ok-skill", Path: ".agents/skills/ok-skill", Status: skillcore.StatusOK, ExpectedTreeSha: "a", ActualTreeSha: "a"},
				{Name: "bad", Path: ".agents/skills/bad", Status: skillcore.StatusMismatch, ExpectedTreeSha: "a", ActualTreeSha: "b", Reason: "content does not match"},
			},
		},
	}

	for name, rep := range reports {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var b bytes.Buffer
			if err := renderVerifyReport(&b, rep, true); err != nil {
				t.Fatalf("renderVerifyReport: %v", err)
			}

			// Decode into the concrete shape so missing keys / null verdicts fail.
			var got verifyReportJSON
			if err := json.Unmarshal(b.Bytes(), &got); err != nil {
				t.Fatalf("json.Unmarshal: %v\n%s", err, b.String())
			}

			if got.Verdicts == nil {
				t.Errorf("verdicts is null, want [] (empty repo must serialize as [])")
			}

			if !strings.Contains(b.String(), `"counts":{"verified"`) {
				t.Errorf("counts not fully present: %s", b.String())
			}

			if len(got.Verdicts) != len(rep.Verdicts) {
				t.Errorf("verdicts = %d, want %d (every checked skill must appear)", len(got.Verdicts), len(rep.Verdicts))
			}
		})
	}
}

// TestRenderVerifyReport_Human asserts the bounded human shapes for pass and fail.
func TestRenderVerifyReport_Human(t *testing.T) {
	t.Parallel()

	var pass bytes.Buffer
	if err := renderVerifyReport(&pass, skillcore.Report{OK: true, Counts: skillcore.Counts{Verified: 3}}, false); err != nil {
		t.Fatalf("render pass: %v", err)
	}

	if lines := nonEmptyCount(pass.String()); lines != 2 {
		t.Errorf("pass output = %d lines, want exactly 2:\n%s", lines, pass.String())
	}

	if !strings.Contains(pass.String(), "verified 3 skills") {
		t.Errorf("pass output missing count: %s", pass.String())
	}

	failReport := skillcore.Report{
		OK:     false,
		Counts: skillcore.Counts{Verified: 1, Mismatch: 1},
		Verdicts: []skillcore.Verdict{
			{Name: "ok-skill", Status: skillcore.StatusOK},
			{Name: "bad", Status: skillcore.StatusMismatch, ExpectedTreeSha: "aaaaaaaa", ActualTreeSha: "bbbbbbbb"},
		},
	}

	var fail bytes.Buffer
	if err := renderVerifyReport(&fail, failReport, false); err != nil {
		t.Fatalf("render fail: %v", err)
	}

	out := fail.String()
	if !strings.Contains(out, "verify FAILED: 1 of 2 skills") || !strings.Contains(out, "✗ bad") {
		t.Errorf("fail output wrong shape:\n%s", out)
	}
	// Bounded: header + one line per FAILING verdict + footer (passing ones summarized).
	if lines := nonEmptyCount(out); lines > failReport.Counts.Mismatch+2 {
		t.Errorf("fail output = %d lines, want <= findings+2:\n%s", lines, out)
	}
}

// nonEmptyCount counts the non-blank lines in s.
func nonEmptyCount(s string) int {
	n := 0

	for line := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}

	return n
}
