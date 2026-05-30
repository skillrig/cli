package skillcore

import (
	"path/filepath"
	"reflect"
	"testing"
)

// TestParseManifest_RequiresAndUnknownKeys asserts the parse contract: a
// skill.toml with [[requires]] array-of-tables AND unknown keys (both top-level
// and per-require) parses into the expected Manifest, ignoring the unknowns with
// no error (forward-compat — strict mode is deliberately off).
func TestParseManifest_RequiresAndUnknownKeys(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "skill.toml", 0o644, sampleManifest)

	got, err := ParseManifest(filepath.Join(dir, "skill.toml"))
	if err != nil {
		t.Fatalf("ParseManifest: unexpected error: %v", err)
	}

	want := Manifest{
		Name:        "terraform-plan-review",
		Version:     "1.4.0",
		Namespace:   "dev.skillrig.samples",
		Description: "Review a terraform plan",
		Tags:        []string{"terraform", "review"},
		Requires: []Require{
			{
				Tool:    "terraform",
				Version: ">=1.5",
				Source:  "https://releases.hashicorp.com",
				Manager: "asdf",
			},
			{
				Tool:    "tflint",
				Version: "0.50.0",
			},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseManifest mismatch:\n got = %+v\nwant = %+v", got, want)
	}
}

// TestParseManifest_Minimal confirms a manifest with no [[requires]] parses to a
// nil/empty Requires slice (add only needs name + version).
func TestParseManifest_Minimal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, dir, "skill.toml", 0o644, "name = \"solo\"\nversion = \"0.1.0\"\n")

	got, err := ParseManifest(filepath.Join(dir, "skill.toml"))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}

	if got.Name != "solo" || got.Version != "0.1.0" {
		t.Errorf("name/version = %q/%q, want solo/0.1.0", got.Name, got.Version)
	}

	if len(got.Requires) != 0 {
		t.Errorf("Requires = %+v, want empty", got.Requires)
	}
}

// TestParseManifest_Errors covers the failure surface: a missing file and
// malformed TOML must both return an error (the CLI renders it; the SDK only
// returns it).
func TestParseManifest_Errors(t *testing.T) {
	t.Parallel()

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()

		_, err := ParseManifest(filepath.Join(t.TempDir(), "absent.toml"))
		if err == nil {
			t.Fatal("ParseManifest(absent): want error, got nil")
		}
	})

	t.Run("malformed toml", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		writeFile(t, dir, "skill.toml", 0o644, "name = \"x\"\nthis is = = not toml\n")

		_, err := ParseManifest(filepath.Join(dir, "skill.toml"))
		if err == nil {
			t.Fatal("ParseManifest(malformed): want error, got nil")
		}
	})
}
