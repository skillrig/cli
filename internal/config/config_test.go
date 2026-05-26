package config

import (
	"os"
	"path/filepath"
	"testing"
)

// mapEnv builds an injected Env accessor from a map (parallel-safe; no process
// env mutation).
func mapEnv(m map[string]string) Env {
	return func(k string) string { return m[k] }
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, configDirName, configFileName)
	origin := Origin{Owner: "my-org", Repo: "my-skills"}

	if err := Save(path, origin); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Origin != origin.String() {
		t.Errorf("round-trip origin = %q, want %q", got.Origin, origin.String())
	}
}

// TestSaveMatchesFixture anchors Save's byte-for-byte output to the committed
// ground-truth fixture (Constitution III). go-toml/v2 emits TOML literal
// strings (single-quoted) for values needing no escaping; the fixture is the
// real output, regenerated from Save (review finding G1).
func TestSaveMatchesFixture(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, configFileName)

	if err := Save(path, Origin{Owner: "my-org", Repo: "my-skills"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}

	want, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "config.toml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	if string(got) != string(want) {
		t.Errorf("Save output = %q, want fixture %q", got, want)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	t.Parallel()

	got, err := Load(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if err != nil {
		t.Fatalf("Load missing file should not error, got: %v", err)
	}

	if got.Origin != "" {
		t.Errorf("missing file origin = %q, want empty", got.Origin)
	}
}

func TestLoadMalformedErrors(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)
	if err := os.WriteFile(path, []byte("this is = not valid = toml ]["), 0o644); err != nil {
		t.Fatalf("write malformed: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Error("Load malformed file should error")
	}
}

func TestLoadIgnoresUnknownKeys(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), configFileName)
	contents := "origin = \"my-org/my-skills\"\nfuture_key = \"ignored\"\n[clients]\ntarget = \"x\"\n"

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load with unknown keys should not error: %v", err)
	}

	if got.Origin != "my-org/my-skills" {
		t.Errorf("origin = %q, want my-org/my-skills", got.Origin)
	}
}

func TestGlobalConfigPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "XDG_CONFIG_HOME set",
			env:  map[string]string{"XDG_CONFIG_HOME": "/cfg"},
			want: filepath.Join("/cfg", "skillrig", "config.toml"),
		},
		{
			name: "falls back to HOME/.config",
			env:  map[string]string{"HOME": "/home/u"},
			want: filepath.Join("/home/u", ".config", "skillrig", "config.toml"),
		},
		{
			name: "blank XDG falls through to HOME",
			env:  map[string]string{"XDG_CONFIG_HOME": "   ", "HOME": "/home/u"},
			want: filepath.Join("/home/u", ".config", "skillrig", "config.toml"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := GlobalConfigPath(mapEnv(tc.env))
			if err != nil {
				t.Fatalf("GlobalConfigPath: %v", err)
			}

			if got != tc.want {
				t.Errorf("GlobalConfigPath = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFindProjectConfigWalkUp(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfgPath := filepath.Join(root, configDirName, configFileName)

	if err := Save(cfgPath, Origin{Owner: "my-org", Repo: "my-skills"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}

	got, found := FindProjectConfig(sub)
	if !found {
		t.Fatal("FindProjectConfig from subdir: not found, want found via walk-up")
	}

	if got != cfgPath {
		t.Errorf("FindProjectConfig = %q, want %q", got, cfgPath)
	}
}

func TestFindProjectConfigNoneReturnsFalse(t *testing.T) {
	t.Parallel()

	if _, found := FindProjectConfig(t.TempDir()); found {
		t.Error("FindProjectConfig in empty dir should report not found")
	}
}
