package skillcore

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Manifest is the machine metadata lifted from a skill's SKILL.md YAML
// frontmatter. It is read-only: ParseManifest reads it and Add/Verify consume
// Name/Version, but skillcore never writes it back. Standard agentskills.io
// fields (name, description) are read verbatim; skillrig-specific data lives
// under the standard metadata map as flat, dotted "x-skillrig.*" keys.
type Manifest struct {
	Name        string    // standard frontmatter `name`
	Description string    // standard frontmatter `description`
	Namespace   string    // metadata."x-skillrig.namespace"
	Version     string    // metadata."x-skillrig.version"
	Convention  string    // metadata."x-skillrig.convention-version"
	Topics      []string  // metadata."x-skillrig.topics"
	Requires    []Require // metadata."x-skillrig.requires"
}

// Require is one tool dependency declared under metadata."x-skillrig.requires".
// It is parsed from SKILL.md frontmatter (yaml) and serialized into index.json
// and `search --json` (json). The json tags are lowercase (FIX-5) so the catalog
// emits "tool"/"version"/… as the data-model §2 specifies — without them
// encoding/json would default to PascalCase ("Tool"), breaking every catalog
// consumer. It is deliberately NOT written to the lock — the on-disk manifest
// stays the single source of truth, read later by doctor.
type Require struct {
	Tool    string `yaml:"tool"    json:"tool"`
	Version string `yaml:"version" json:"version"`
	Source  string `yaml:"source"  json:"source"`
	Manager string `yaml:"manager" json:"manager"`
}

// frontmatter mirrors the agentskills.io SKILL.md frontmatter we consume: the
// standard top-level fields plus the free-form metadata map that carries the
// namespaced skillrig extensions. Unknown keys are ignored (forward-compat).
type frontmatter struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Metadata    map[string]any `yaml:"metadata"`
}

// ParseManifest reads the SKILL.md at path, parses its YAML frontmatter, and
// lifts the namespaced metadata."x-skillrig.*" keys into a Manifest. It
// validates that name is present and equals the skill's directory name (the
// directory containing SKILL.md) and that version is present. Unknown keys are
// ignored for forward-compatibility.
func ParseManifest(path string) (Manifest, error) {
	//nolint:gosec // G304: path is a SKILL.md within the resolved origin/repo subtree.
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("reading skill manifest %q: %w", path, err)
	}

	block, err := extractFrontmatter(data)
	if err != nil {
		return Manifest{}, fmt.Errorf("parsing skill manifest %q: %w", path, err)
	}

	var fm frontmatter
	if err := yaml.Unmarshal(block, &fm); err != nil {
		return Manifest{}, fmt.Errorf("parsing skill manifest %q: %w", path, err)
	}

	m := Manifest{
		Name:        fm.Name,
		Description: fm.Description,
		Namespace:   metaString(fm.Metadata, "x-skillrig.namespace"),
		Version:     metaString(fm.Metadata, "x-skillrig.version"),
		Convention:  metaString(fm.Metadata, "x-skillrig.convention-version"),
		Topics:      metaStrings(fm.Metadata, "x-skillrig.topics"),
		Requires:    metaRequires(fm.Metadata, "x-skillrig.requires"),
	}

	dir := filepath.Base(filepath.Dir(path))
	if err := validateManifest(m, dir); err != nil {
		return Manifest{}, fmt.Errorf("invalid skill manifest %q: %w", path, err)
	}

	return m, nil
}

// validateManifest enforces the manifest contract: name is required and MUST
// equal the skill directory name (removing the name/directory drift the old
// skill.toml allowed), and version is required.
func validateManifest(m Manifest, dir string) error {
	if m.Name == "" {
		return fmt.Errorf("frontmatter %q is required", "name")
	}

	if m.Name != dir {
		return fmt.Errorf("name %q must equal the skill directory %q", m.Name, dir)
	}

	if m.Version == "" {
		return fmt.Errorf("metadata %q is required", "x-skillrig.version")
	}

	return nil
}

// extractFrontmatter returns the YAML between the leading and the next "---"
// fences of a SKILL.md. A SKILL.md with no frontmatter block is an error: the
// machine metadata skillrig needs lives there.
func extractFrontmatter(data []byte) ([]byte, error) {
	const fence = "---"

	rest, ok := bytes.CutPrefix(data, []byte(fence+"\n"))
	if !ok {
		// Tolerate a leading BOM/blank lines only via an exact opening fence;
		// anything else means no frontmatter to read.
		return nil, fmt.Errorf("missing YAML frontmatter (no leading %q fence)", fence)
	}

	end := bytes.Index(rest, []byte("\n"+fence))
	if end < 0 {
		return nil, fmt.Errorf("unterminated YAML frontmatter (no closing %q fence)", fence)
	}

	return rest[:end+1], nil
}

// metaString reads a string value at key from the metadata map (absent → "").
func metaString(meta map[string]any, key string) string {
	if v, ok := meta[key].(string); ok {
		return v
	}

	return ""
}

// metaStrings reads a []string value at key, coercing each element via
// fmt.Sprint so a YAML list of scalars (e.g. unquoted topics) is accepted.
func metaStrings(meta map[string]any, key string) []string {
	raw, ok := meta[key].([]any)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, fmt.Sprint(v))
	}

	return out
}

// metaRequires reads the nested x-skillrig.requires list into []Require. Each
// element is a YAML map; missing fields default to the zero string.
func metaRequires(meta map[string]any, key string) []Require {
	raw, ok := meta[key].([]any)
	if !ok {
		return nil
	}

	out := make([]Require, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}

		out = append(out, Require{
			Tool:    mapString(entry, "tool"),
			Version: mapString(entry, "version"),
			Source:  mapString(entry, "source"),
			Manager: mapString(entry, "manager"),
		})
	}

	return out
}

// mapString reads a string value at key from a decoded YAML map (absent → "").
func mapString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}

	return ""
}
