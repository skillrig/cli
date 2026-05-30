package skillcore

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// Manifest is a parsed skill.toml. It is read-only: ParseManifest reads it and
// Add/Verify consume Name/Version, but skillcore never writes it back.
type Manifest struct {
	Name        string    `toml:"name"`
	Version     string    `toml:"version"`
	Namespace   string    `toml:"namespace"`
	Description string    `toml:"description"`
	Tags        []string  `toml:"tags"`
	Requires    []Require `toml:"requires"`
}

// Require is one tool dependency declared in a skill.toml. It is parsed but is
// deliberately NOT written to the lock — the on-disk manifest stays the single
// source of truth, read later by doctor.
type Require struct {
	Tool    string `toml:"tool"`
	Version string `toml:"version"`
	Source  string `toml:"source"`
	Manager string `toml:"manager"`
}

// ParseManifest parses the skill.toml at path. Unknown keys are ignored for
// forward-compatibility (default Unmarshal — strict mode is deliberately off).
func ParseManifest(path string) (Manifest, error) {
	//nolint:gosec // G304: path is a skill.toml within the resolved origin/repo subtree.
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("reading skill manifest %q: %w", path, err)
	}

	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parsing skill manifest %q: %w", path, err)
	}

	return m, nil
}
