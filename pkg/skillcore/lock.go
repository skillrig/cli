package skillcore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// lockDirName is the per-repo directory holding the lock file.
const lockDirName = ".skillrig"

// lockFileName is the tool-written lock file inside lockDirName.
const lockFileName = "skills-lock.json"

// LockFile is the parsed .skillrig/skills-lock.json: the committed,
// tool-written record of every vendored skill. It carries no requires data
// (the manifest owns that, D4).
type LockFile struct {
	LockfileVersion int                  `json:"lockfileVersion"`
	Origin          string               `json:"origin"`
	Skills          map[string]LockEntry `json:"skills"`
}

// LockEntry is the locked record for one vendored skill, keyed by skill name.
// Note: no requires field (D4) — the on-disk manifest is the single source of
// truth for dependencies.
type LockEntry struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	TreeSha string `json:"treeSha"`
	Path    string `json:"path"`
}

// lockPath returns the lock file path for a repo root.
func lockPath(repoRoot string) string {
	return filepath.Join(repoRoot, lockDirName, lockFileName)
}

// ReadLock reads the lock at repoRoot's .skillrig/skills-lock.json. An absent
// file is not an error: it returns a zero LockFile and a nil error. A present
// file is JSON-unmarshalled.
func ReadLock(repoRoot string) (LockFile, error) {
	path := lockPath(repoRoot)

	//nolint:gosec // G304: path is built from repoRoot + fixed names, not
	// attacker-controlled; reading the designated lock file is the point.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return LockFile{}, nil
		}

		return LockFile{}, fmt.Errorf("read %s: %w", path, err)
	}

	var lf LockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return LockFile{}, fmt.Errorf("parse %s: %w", path, err)
	}

	return lf, nil
}

// WriteLock writes lf to repoRoot's .skillrig/skills-lock.json with
// deterministic serialization (Go sorts map keys; 2-space indent; trailing
// newline) via an atomic temp-file-plus-rename. The temp file lives in the same
// directory so os.Rename stays on one filesystem, mirroring internal/config.Save.
func WriteLock(repoRoot string, lf LockFile) error {
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return fmt.Errorf("encode lock: %w", err)
	}

	data = append(data, '\n')

	path := lockPath(repoRoot)
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, lockFileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}

	tmpName := tmp.Name()
	// Best-effort cleanup if we bail before the rename.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("write %s: %w", tmpName, err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}

	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("install %s: %w", path, err)
	}

	return nil
}
