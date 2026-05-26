package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeProject writes a project config.toml at dir/.skillrig/config.toml. When
// malformed is true it writes unparseable bytes instead (matrix row 7).
func writeProject(t *testing.T, dir, origin string, malformed bool) string {
	t.Helper()

	path := filepath.Join(dir, configDirName, configFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	data := "origin = '" + origin + "'\n"
	if malformed {
		data = "this is = not ][ valid toml"
	}

	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write project: %v", err)
	}

	return path
}

// writeGlobal writes a global config at home/.config/skillrig/config.toml.
func writeGlobal(t *testing.T, home, origin string) string {
	t.Helper()

	path := filepath.Join(home, ".config", "skillrig", configFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir global: %v", err)
	}

	if err := os.WriteFile(path, []byte("origin = '"+origin+"'\n"), 0o600); err != nil {
		t.Fatalf("write global: %v", err)
	}

	return path
}

// TestResolveOrigin_Precedence is table-driven over the recorded matrix in
// data-model.md (rows 1–7). Each row materializes real files in temp dirs and
// resolves with an injected env — no mocks (Constitution III).
func TestResolveOrigin_Precedence(t *testing.T) {
	t.Parallel()

	type row struct {
		name             string
		envOrigin        string // SKILLRIG_ORIGIN value ("" = unset)
		projectOrigin    string // "" = no project file
		projectMalformed bool
		globalOrigin     string // "" = no global file
		wantOrigin       string
		wantSource       Source
		wantPathIsGlobal bool // ConfigPath should be the global file
		wantPathIsProj   bool // ConfigPath should be the project file
	}

	rows := []row{
		{name: "row1_none", wantSource: SourceNone},
		{name: "row2_project", projectOrigin: "my-org/my-skills", wantOrigin: "my-org/my-skills", wantSource: SourceProject, wantPathIsProj: true},
		{name: "row3_env_beats_project", envOrigin: "ci-org/ci-skills", projectOrigin: "my-org/my-skills", wantOrigin: "ci-org/ci-skills", wantSource: SourceEnv},
		{name: "row4_global", globalOrigin: "personal/skills", wantOrigin: "personal/skills", wantSource: SourceGlobal, wantPathIsGlobal: true},
		{name: "row5_project_beats_global", projectOrigin: "client-a/skills", globalOrigin: "personal/skills", wantOrigin: "client-a/skills", wantSource: SourceProject, wantPathIsProj: true},
		{name: "row6_blank_env_is_unset", envOrigin: "   ", projectOrigin: "my-org/my-skills", wantOrigin: "my-org/my-skills", wantSource: SourceProject, wantPathIsProj: true},
		{name: "row7_malformed_project_skipped", projectOrigin: "ignored", projectMalformed: true, globalOrigin: "personal/skills", wantOrigin: "personal/skills", wantSource: SourceGlobal, wantPathIsGlobal: true},
	}

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cwd := t.TempDir()
			home := t.TempDir()

			var projPath, globalPath string
			if tc.projectOrigin != "" {
				projPath = writeProject(t, cwd, tc.projectOrigin, tc.projectMalformed)
			}

			if tc.globalOrigin != "" {
				globalPath = writeGlobal(t, home, tc.globalOrigin)
			}

			env := mapEnv(map[string]string{
				"SKILLRIG_ORIGIN": tc.envOrigin,
				"HOME":            home,
			})

			got, err := ResolveOrigin(cwd, env)
			if err != nil {
				t.Fatalf("ResolveOrigin: %v", err)
			}

			if got.Origin.String() != originString(tc.wantOrigin) {
				t.Errorf("Origin = %q, want %q", got.Origin.String(), originString(tc.wantOrigin))
			}

			if got.Source != tc.wantSource {
				t.Errorf("Source = %q, want %q", got.Source, tc.wantSource)
			}

			switch {
			case tc.wantPathIsProj:
				if got.ConfigPath != projPath {
					t.Errorf("ConfigPath = %q, want project %q", got.ConfigPath, projPath)
				}
			case tc.wantPathIsGlobal:
				if got.ConfigPath != globalPath {
					t.Errorf("ConfigPath = %q, want global %q", got.ConfigPath, globalPath)
				}
			default:
				if got.ConfigPath != "" {
					t.Errorf("ConfigPath = %q, want empty", got.ConfigPath)
				}
			}
		})
	}
}

// TestResolveOrigin_FromSubdir proves walk-up resolution (US2 / SC-002): a
// project config at the repo root resolves from a nested subdirectory.
func TestResolveOrigin_FromSubdir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	home := t.TempDir()
	projPath := writeProject(t, root, "my-org/my-skills", false)

	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}

	got, err := ResolveOrigin(sub, mapEnv(map[string]string{"HOME": home}))
	if err != nil {
		t.Fatalf("ResolveOrigin: %v", err)
	}

	if got.Source != SourceProject || got.Origin.String() != "my-org/my-skills" {
		t.Errorf("from subdir: got %+v, want project my-org/my-skills", got)
	}

	if got.ConfigPath != projPath {
		t.Errorf("ConfigPath = %q, want %q", got.ConfigPath, projPath)
	}
}

// TestResolveOrigin_MalformedProjectRecordsDiagnostic verifies FR-004: a
// malformed project file is skipped (resolution continues to global) AND its
// cause is recorded as a diagnostic for a --verbose caller — not silently
// swallowed.
func TestResolveOrigin_MalformedProjectRecordsDiagnostic(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	home := t.TempDir()
	projPath := writeProject(t, cwd, "ignored", true) // malformed TOML
	writeGlobal(t, home, "personal/skills")

	got, err := ResolveOrigin(cwd, mapEnv(map[string]string{"HOME": home}))
	if err != nil {
		t.Fatalf("malformed project must not be fatal, got error: %v", err)
	}

	if got.Source != SourceGlobal || got.Origin.String() != "personal/skills" {
		t.Errorf("got %+v, want global personal/skills (project skipped)", got)
	}

	if len(got.Diagnostics) != 1 {
		t.Fatalf("Diagnostics = %v, want exactly 1 (the skipped malformed project)", got.Diagnostics)
	}

	d := got.Diagnostics[0]
	if d.Source != SourceProject || d.Path != projPath || !strings.Contains(d.Reason, "malformed") {
		t.Errorf("diagnostic = %+v, want project %s with a 'malformed' reason", d, projPath)
	}
}

// TestResolveOrigin_InvalidShapeRecordsDiagnostic verifies a parseable file
// whose origin fails OWNER/REPO validation is also skipped with a diagnostic.
func TestResolveOrigin_InvalidShapeRecordsDiagnostic(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	home := t.TempDir()
	projPath := writeProject(t, cwd, "not a valid shape", false) // parses, bad origin
	writeGlobal(t, home, "personal/skills")

	got, err := ResolveOrigin(cwd, mapEnv(map[string]string{"HOME": home}))
	if err != nil {
		t.Fatalf("invalid-shape project must not be fatal, got error: %v", err)
	}

	if got.Source != SourceGlobal {
		t.Errorf("Source = %q, want global (project skipped)", got.Source)
	}

	if len(got.Diagnostics) != 1 || got.Diagnostics[0].Path != projPath {
		t.Errorf("Diagnostics = %+v, want 1 entry for %s", got.Diagnostics, projPath)
	}
}

// TestResolveOrigin_OriginlessNoDiagnostic verifies a parseable file that
// simply lacks an origin key is a quiet fall-through (forward-compat), not a
// diagnostic.
func TestResolveOrigin_OriginlessNoDiagnostic(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	home := t.TempDir()

	path := filepath.Join(cwd, configDirName, configFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, []byte("future_key = \"x\"\n"), 0o600); err != nil {
		t.Fatalf("write origin-less: %v", err)
	}

	writeGlobal(t, home, "personal/skills")

	got, err := ResolveOrigin(cwd, mapEnv(map[string]string{"HOME": home}))
	if err != nil {
		t.Fatalf("origin-less file must not be fatal: %v", err)
	}

	if got.Source != SourceGlobal {
		t.Errorf("Source = %q, want global", got.Source)
	}

	if len(got.Diagnostics) != 0 {
		t.Errorf("Diagnostics = %+v, want none for an origin-less (parseable) file", got.Diagnostics)
	}
}

// TestResolveOrigin_UnreadableFileIsFatal verifies a genuine I/O error (an
// existing but unreadable file) is returned as a fatal error, not skipped
// (contract resolve.md). Skipped when running as root (perms don't restrict).
func TestResolveOrigin_UnreadableFileIsFatal(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("running as root; file permissions do not restrict access")
	}

	cwd := t.TempDir()
	path := writeProject(t, cwd, "my-org/my-skills", false)

	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod 000: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(path, 0o600) }) // let TempDir cleanup remove it

	_, err := ResolveOrigin(cwd, mapEnv(map[string]string{"HOME": t.TempDir()}))
	if err == nil {
		t.Fatal("unreadable project config should be a fatal error, got nil")
	}
}

// TestResolveOrigin_UnreadableProjectIsFatalDespiteGlobal pins the (contract-
// specified, debatable) semantic that an unreadable project config is fatal
// even when a valid global default exists — resolution does not fall through an
// I/O error (contract resolve.md; adversarial finding A3).
func TestResolveOrigin_UnreadableProjectIsFatalDespiteGlobal(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("running as root; file permissions do not restrict access")
	}

	cwd := t.TempDir()
	home := t.TempDir()
	path := writeProject(t, cwd, "my-org/my-skills", false)
	writeGlobal(t, home, "personal/skills") // a usable global that must NOT mask the error

	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod 000: %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })

	if _, err := ResolveOrigin(cwd, mapEnv(map[string]string{"HOME": home})); err == nil {
		t.Fatal("unreadable project config must be fatal even with a valid global, got nil")
	}
}

// TestResolveOrigin_MalformedEnvIsFatal verifies the resolver's one hard-error
// branch: an explicitly set but malformed SKILLRIG_ORIGIN is a deliberate
// override that must be valid — it errors rather than falling through to file
// sources (adversarial finding A1). Distinct from a blank env, which is unset
// (matrix row 6).
func TestResolveOrigin_MalformedEnvIsFatal(t *testing.T) {
	t.Parallel()

	cwd := t.TempDir()
	home := t.TempDir()
	// A valid project origin exists; the malformed env override must NOT fall
	// through to it — it must hard-error.
	writeProject(t, cwd, "my-org/my-skills", false)

	_, err := ResolveOrigin(cwd, mapEnv(map[string]string{
		"SKILLRIG_ORIGIN": "not a valid origin",
		"HOME":            home,
	}))
	if err == nil {
		t.Fatal("malformed SKILLRIG_ORIGIN should be a fatal error, got nil")
	}

	// Error names the offending variable and the underlying cause.
	if !strings.Contains(err.Error(), "SKILLRIG_ORIGIN") {
		t.Errorf("error %q should name SKILLRIG_ORIGIN", err)
	}
}

// originString renders the expected origin: the empty want maps to the zero
// Origin's String ("/"), which we normalize to "" for comparison.
func originString(want string) string {
	if want == "" {
		return (Origin{}).String()
	}

	return want
}
