package config

import (
	"context"
	"errors"
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

// TestSaveLoadRoundTripWithRef verifies an origin carrying an @ref round-trips
// through Save/Load inside the single `origin` key — the ref is stored combined
// (origin = 'my-org/my-skills@main'), so no config-schema change is needed
// (amendment 001-origin-ref-support).
func TestSaveLoadRoundTripWithRef(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, configDirName, configFileName)
	origin := Origin{Owner: "my-org", Repo: "my-skills", Ref: "main"}

	if err := Save(path, origin); err != nil {
		t.Fatalf("Save: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}

	if string(raw) != "origin = 'my-org/my-skills@main'\n" {
		t.Errorf("written config = %q, want origin = 'my-org/my-skills@main'", raw)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Origin != origin.String() {
		t.Errorf("round-trip origin = %q, want %q", got.Origin, origin.String())
	}

	parsed, err := ParseOrigin(got.Origin)
	if err != nil {
		t.Fatalf("ParseOrigin(%q): %v", got.Origin, err)
	}

	if parsed.Ref != "main" {
		t.Errorf("parsed ref = %q, want main", parsed.Ref)
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

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load malformed file should error")
	}

	// FR-004: a malformed file is a *MalformedError so the resolver can skip it
	// (vs. a plain I/O error, which is fatal).
	var malformed *MalformedError
	if !errors.As(err, &malformed) {
		t.Errorf("Load error %T is not a *MalformedError", err)
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

	got, found, err := FindProjectConfig(sub)
	if err != nil {
		t.Fatalf("FindProjectConfig: %v", err)
	}

	if !found {
		t.Fatal("FindProjectConfig from subdir: not found, want found via walk-up")
	}

	if got != cfgPath {
		t.Errorf("FindProjectConfig = %q, want %q", got, cfgPath)
	}
}

func TestFindProjectConfigNoneReturnsFalse(t *testing.T) {
	t.Parallel()

	_, found, err := FindProjectConfig(t.TempDir())
	if err != nil {
		t.Fatalf("FindProjectConfig: %v", err)
	}

	if found {
		t.Error("FindProjectConfig in empty dir should report not found")
	}
}

// TestFindProjectConfigUnstattableIsError pins Qodo #2: a stat failure that is
// NOT fs.ErrNotExist (here, permission denied because an ancestor .skillrig dir
// lacks the execute/search bit) is surfaced as an error, not silently treated
// as "not found". Skipped as root (perms don't restrict).
func TestFindProjectConfigUnstattableIsError(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("running as root; file permissions do not restrict access")
	}

	root := t.TempDir()
	skillDir := filepath.Join(root, configDirName)

	if err := os.MkdirAll(skillDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Remove search permission so stat of <skillDir>/config.toml fails EACCES.
	if err := os.Chmod(skillDir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(skillDir, 0o750) })

	if _, _, err := FindProjectConfig(root); err == nil {
		t.Fatal("FindProjectConfig should surface a permission-denied stat as an error, got nil")
	}
}

// TestProjectWriteTargetCancelledCtxIsFatal pins Qodo #4: an unexpected git
// failure (here a cancelled context) is propagated, NOT masked as a cwd
// fallback — so init never writes config to the wrong directory.
func TestProjectWriteTargetCancelledCtxIsFatal(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the call so git rev-parse fails on the dead context

	if _, err := ProjectWriteTarget(ctx, t.TempDir()); err == nil {
		t.Fatal("cancelled context must yield a fatal error, not a cwd fallback")
	}
}

// TestProjectWriteTargetNonRepoFallsBackToCwd verifies the expected fallback is
// preserved: outside a git repo (clean non-zero git exit), the write target is
// <cwd>/.skillrig/config.toml with no error.
func TestProjectWriteTargetNonRepoFallsBackToCwd(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()

	got, err := ProjectWriteTarget(context.Background(), cwd)
	if err != nil {
		t.Fatalf("non-repo cwd should fall back without error, got: %v", err)
	}

	want := filepath.Join(cwd, configDirName, configFileName)
	if got != want {
		t.Errorf("ProjectWriteTarget = %q, want cwd fallback %q", got, want)
	}
}
