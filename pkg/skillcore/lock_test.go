package skillcore

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// sampleLock is the data-model's ground-truth lock example: a single skill with
// a 40-hex commit + treeSha and a repo-relative path under .agents/skills.
func sampleLock() LockFile {
	return LockFile{
		LockfileVersion: 1,
		Origin:          "my-org/my-skills",
		Skills: map[string]LockEntry{
			"terraform-plan-review": {
				Version: "1.4.0",
				Commit:  "9f1a052e596d5d28f13838061a1ab93207ef6fc3",
				TreeSha: "c967789527370d2e0fba03a92e70dffef6f3bf31",
				Path:    ".agents/skills/terraform-plan-review",
			},
		},
	}
}

// TestLock_RoundTrip writes a lock, reads it back, and asserts equality — the
// committed, tool-written record must survive serialization losslessly.
func TestLock_RoundTrip(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	want := sampleLock()

	if err := WriteLock(repoRoot, want); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}

	got, err := ReadLock(repoRoot)
	if err != nil {
		t.Fatalf("ReadLock: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("round-trip mismatch:\n got = %+v\nwant = %+v", got, want)
	}
}

// TestLock_OnDiskShape pins the serialization contract (data-model §LockFile):
// 2-space indentation, a trailing newline, and crucially NO "requires" key — the
// manifest is the single source of truth for dependencies (D4).
func TestLock_OnDiskShape(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := WriteLock(repoRoot, sampleLock()); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}

	path := filepath.Join(repoRoot, ".skillrig", "skills-lock.json")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}

	text := string(raw)

	if !strings.HasSuffix(text, "\n") {
		t.Error("lock file does not end with a trailing newline")
	}

	if strings.Contains(text, "\t") {
		t.Error("lock file contains a tab; indentation must be 2 spaces")
	}

	// The first nested object key must be indented by exactly two spaces.
	if !strings.Contains(text, "\n  \"origin\"") {
		t.Errorf("lock file is not 2-space indented:\n%s", text)
	}

	if strings.Contains(strings.ToLower(text), "requires") {
		t.Errorf("lock file leaks a 'requires' key (D4: manifest owns deps):\n%s", text)
	}
}

// TestReadLock_Absent codifies the contract that a missing lock is not an error:
// ReadLock returns a zero LockFile and a nil error so first-add flows just work.
func TestReadLock_Absent(t *testing.T) {
	t.Parallel()

	got, err := ReadLock(t.TempDir())
	if err != nil {
		t.Fatalf("ReadLock(absent): unexpected error: %v", err)
	}

	if !reflect.DeepEqual(got, LockFile{}) {
		t.Errorf("ReadLock(absent) = %+v, want zero LockFile", got)
	}
}

// TestWriteLock_NoPartialTempFile asserts the atomic-write discipline leaves no
// .tmp-* sibling behind after a successful WriteLock (temp file + rename).
func TestWriteLock_NoPartialTempFile(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	if err := WriteLock(repoRoot, sampleLock()); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}

	dir := filepath.Join(repoRoot, ".skillrig")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read .skillrig: %v", err)
	}

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file after WriteLock: %q", e.Name())
		}
	}

	if len(names) != 1 || names[0] != "skills-lock.json" {
		t.Errorf(".skillrig contents = %v, want exactly [skills-lock.json]", names)
	}
}
