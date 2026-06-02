package sourceguard

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// modRoot is the module root discovered by moduleRoot, reused to render paths
// relative to it. Set once before the parallel scan reads it.
var modRoot string

// moduleRoot walks up from the test's working directory (the package dir) until
// it finds go.mod, and returns that directory — the module root to scan.
func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			modRoot = dir

			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found walking up from working directory")
		}

		dir = parent
	}
}

// skipDir reports whether a directory should be skipped: VCS/tooling/build
// directories that either are not our source or hold generated/vendored trees.
func skipDir(name string) bool {
	switch name {
	case ".git", ".claude", "vendor", "node_modules", "dist", "testdata":
		return true
	default:
		return false
	}
}

// relOrPath renders path relative to the module root, falling back to the
// absolute path if that fails — purely for readable violation messages.
func relOrPath(path string) string {
	if rel, err := filepath.Rel(modRoot, path); err == nil {
		return rel
	}

	return path
}

// formatViolation renders one offending rune as "rel:line:col: U+XXXX 'r'".
func formatViolation(rel string, line, col int, r rune) string {
	return fmt.Sprintf("  %s:%d:%d: %U %q", rel, line, col, r, string(r))
}
