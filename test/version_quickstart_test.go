package quickstart

import (
	"strings"
	"testing"
)

// TestQuickstart_Version asserts the `--version` contract that GoReleaser's
// -ldflags injection feeds: a single human line carrying the binary name plus
// the reproducibility coordinates (commit + build date). The test binary is
// built without -ldflags, so the embedded defaults ("dev"/"none"/"unknown")
// stand in for a release build — we assert the SHAPE, not the values.
func TestQuickstart_Version(t *testing.T) {
	t.Parallel()

	res := run(t, runOpts{args: []string{"--version"}})

	if res.exit != 0 {
		t.Fatalf("--version exit = %d, want 0 (stderr: %q)", res.exit, res.stderr)
	}

	lines := nonEmptyLines(res.stdout)
	if len(lines) != 1 {
		t.Fatalf("--version stdout = %d lines, want 1 (compact single line): %q", len(lines), res.stdout)
	}

	for _, want := range []string{"skillrig", "dev", "commit", "built"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("--version output %q missing %q", res.stdout, want)
		}
	}
}
