package skillcore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// supportedConvention is the single origin convention version this binary
// speaks. The gate is EXACT-MATCH (C1): any other value — higher, lower, or an
// absent/zero field — is incompatible. A forward/backward-compat window is a
// deliberate future change, never an accident of a ">"-only check.
const supportedConvention = 1

// originConfigName is the origin's contract file, read by GenerateCatalog for
// the convention version and the skills directory.
const originConfigName = ".skillrig-origin.toml"

// defaultSkillsDir is the origin's skills directory when .skillrig-origin.toml
// does not override it.
const defaultSkillsDir = "skills"

// skillManifestName is the per-skill manifest file walked under the skills dir.
const skillManifestName = "SKILL.md"

// Catalog is the origin's committed, machine-readable list of skills
// (index.json): the single-tip view produced by GenerateCatalog and consumed by
// Search. It is presentation-free — JSON in, JSON out, no human formatting.
type Catalog struct {
	SkillrigConvention int            `json:"skillrigConvention"`
	Origin             string         `json:"origin"`
	Skills             []CatalogEntry `json:"skills"`
}

// CatalogEntry is one skill's discovery record in the catalog: the searchable
// fields (name, description, topics) plus what add needs (version, namespace,
// path, requires). It carries no per-skill commit/treeSha — the catalog is
// discovery-only (D2).
type CatalogEntry struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Namespace   string    `json:"namespace"`
	Description string    `json:"description"`
	Topics      []string  `json:"topics"`
	Path        string    `json:"path"`
	Requires    []Require `json:"requires"`
}

// originConfig is the subset of .skillrig-origin.toml GenerateCatalog reads: the
// convention version (carried into the catalog, never hardcoded — C7) and the
// skills directory to walk.
type originConfig struct {
	ConventionVersion int    `toml:"convention_version"`
	Origin            string `toml:"origin"`
	SkillsDir         string `toml:"skills_dir"`
}

// ParseCatalog decodes index.json bytes into a Catalog. It does not gate the
// convention version — callers run CheckConvention against the decoded
// SkillrigConvention so the parse and the policy stay separable.
func ParseCatalog(data []byte) (Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return Catalog{}, fmt.Errorf("parsing catalog: %w", err)
	}

	return c, nil
}

// CheckConvention enforces the exact-match convention gate (C1): it returns nil
// only when n is exactly supportedConvention, otherwise an
// *IncompatibleConventionError. A higher version, a lower version, and an
// absent/zero field all fail — there is no compatibility window this slice.
func CheckConvention(n int) error {
	if n == supportedConvention {
		return nil
	}

	return &IncompatibleConventionError{Found: n, Supported: supportedConvention}
}

// GenerateCatalog walks the origin checkout at originRoot and produces its
// index.json bytes. It reads the convention version and skills directory from
// the origin's .skillrig-origin.toml (the convention is carried, not
// hardcoded — C7), parses each <skills_dir>/*/SKILL.md via ParseManifest (the
// ONE manifest parser, AP-04), projects them into CatalogEntry, sorts by name
// for determinism (SC-009), and marshals with stable key order plus a trailing
// newline. The returned bytes are byte-identical across runs over an unchanged
// skill set.
func GenerateCatalog(originRoot string) ([]byte, error) {
	cfg, err := readOriginConfig(originRoot)
	if err != nil {
		return nil, err
	}

	skillsDir := cfg.SkillsDir
	if skillsDir == "" {
		skillsDir = defaultSkillsDir
	}

	entries, err := collectEntries(originRoot, skillsDir)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(entries, func(a, b CatalogEntry) int {
		return strings.Compare(a.Name, b.Name)
	})

	catalog := Catalog{
		SkillrigConvention: cfg.ConventionVersion,
		Origin:             cfg.Origin,
		Skills:             entries,
	}

	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encoding catalog: %w", err)
	}

	return append(data, '\n'), nil
}

// readOriginConfig parses the origin's .skillrig-origin.toml at originRoot.
func readOriginConfig(originRoot string) (originConfig, error) {
	path := filepath.Join(originRoot, originConfigName)

	//nolint:gosec // G304: path is originRoot + a fixed file name, not attacker-controlled.
	data, err := os.ReadFile(path)
	if err != nil {
		return originConfig{}, fmt.Errorf("reading origin config %q: %w", path, err)
	}

	var cfg originConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return originConfig{}, fmt.Errorf("parsing origin config %q: %w", path, err)
	}

	return cfg, nil
}

// collectEntries walks <originRoot>/<skillsDir>/*/SKILL.md, parses each via
// ParseManifest, and projects it into a CatalogEntry whose path is the skill
// directory relative to originRoot.
func collectEntries(originRoot, skillsDir string) ([]CatalogEntry, error) {
	root := filepath.Join(originRoot, skillsDir)

	dirEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("reading skills dir %q: %w", root, err)
	}

	entries := make([]CatalogEntry, 0, len(dirEntries))
	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}

		manifestPath := filepath.Join(root, de.Name(), skillManifestName)
		if _, err := os.Stat(manifestPath); err != nil {
			continue // a directory without a SKILL.md is not a skill
		}

		m, err := ParseManifest(manifestPath)
		if err != nil {
			return nil, err
		}

		entries = append(entries, CatalogEntry{
			Name:        m.Name,
			Version:     m.Version,
			Namespace:   m.Namespace,
			Description: m.Description,
			Topics:      m.Topics,
			Path:        filepath.ToSlash(filepath.Join(skillsDir, de.Name())),
			Requires:    m.Requires,
		})
	}

	return entries, nil
}

// relevance bucket scores for search ordering (D8): a higher bucket sorts first,
// ties broken lexicographically by name.
const (
	bucketExactName   = 3 // query (joined) equals the name exactly
	bucketNameHit     = 2 // a query term is a substring of the name
	bucketTopicHit    = 1 // a query term is a substring of a topic
	bucketDescription = 0 // matched on description only
)

// Search filters catalog deterministically (D8, N6) and returns the matching
// entries ordered by relevance bucket then name. A query term matches when it is
// a substring of lower(name+" "+description+" "+join(topics)) (token-AND: every
// term must match); a requested topic must be present (case-insensitive exact
// membership, AND across topics). Empty query and no topics lists everything.
// Stdlib only — no fuzzy/semantic/learned ranking.
func Search(catalog Catalog, query, topics []string) []CatalogEntry {
	lowerQuery := lowerAll(query)
	lowerTopics := lowerAll(topics)

	matched := make([]CatalogEntry, 0, len(catalog.Skills))
	for _, entry := range catalog.Skills {
		if matchesEntry(entry, lowerQuery, lowerTopics) {
			matched = append(matched, entry)
		}
	}

	slices.SortFunc(matched, func(a, b CatalogEntry) int {
		ba := relevance(a, lowerQuery)
		bb := relevance(b, lowerQuery)

		if ba != bb {
			return bb - ba // higher bucket first
		}

		return strings.Compare(a.Name, b.Name)
	})

	return matched
}

// matchesEntry reports whether entry satisfies every lowercased query term
// (token-AND substring over name+description+topics) and carries every requested
// topic (case-insensitive exact membership).
func matchesEntry(entry CatalogEntry, query, topics []string) bool {
	haystack := strings.ToLower(
		entry.Name + " " + entry.Description + " " + strings.Join(entry.Topics, " "),
	)

	for _, term := range query {
		if !strings.Contains(haystack, term) {
			return false
		}
	}

	for _, want := range topics {
		if !hasTopic(entry.Topics, want) {
			return false
		}
	}

	return true
}

// hasTopic reports whether topics contains want, comparing case-insensitively.
func hasTopic(topics []string, want string) bool {
	for _, t := range topics {
		if strings.EqualFold(t, want) {
			return true
		}
	}

	return false
}

// relevance computes entry's ordering bucket for the given lowercased query: an
// exact name match outranks a name substring, which outranks a topic substring,
// which outranks a description-only match.
func relevance(entry CatalogEntry, query []string) int {
	lowerName := strings.ToLower(entry.Name)

	if len(query) > 0 && strings.Join(query, " ") == lowerName {
		return bucketExactName
	}

	for _, term := range query {
		if strings.Contains(lowerName, term) {
			return bucketNameHit
		}
	}

	for _, term := range query {
		for _, topic := range entry.Topics {
			if strings.Contains(strings.ToLower(topic), term) {
				return bucketTopicHit
			}
		}
	}

	return bucketDescription
}

// lowerAll returns a lowercased copy of in.
func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}

	return out
}
