package config

import (
	"os"
	"path/filepath"
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

// originString renders the expected origin: the empty want maps to the zero
// Origin's String ("/"), which we normalize to "" for comparison.
func originString(want string) string {
	if want == "" {
		return (Origin{}).String()
	}

	return want
}
