package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// configDirName is the per-repo / per-user config directory.
const configDirName = ".skillrig"

// configFileName is the config file written inside configDirName.
const configFileName = "config.toml"

// Env is an injected environment accessor so resolution is a pure function of
// (cwd, env, filesystem) and tests can set SKILLRIG_ORIGIN / XDG_CONFIG_HOME /
// HOME deterministically without mutating process state.
type Env func(key string) string

// OSEnv reads from the real process environment. Production callers pass this;
// tests pass a map-backed accessor.
func OSEnv(key string) string { return os.Getenv(key) }

// Config is the on-disk shape of both the project and global config.toml. v0
// has a single field; unknown keys are ignored on read for forward
// compatibility (data-model.md).
type Config struct {
	Origin string `toml:"origin"`
}

// MalformedError marks a config file that exists but cannot be parsed. The
// resolver treats it as "no origin from this source" and continues down
// precedence (FR-004), recording a diagnostic instead of failing — whereas a
// genuine read/I/O error (not this type) is fatal. Callers distinguish the two
// with errors.As(&MalformedError{}).
type MalformedError struct {
	Path string
	Err  error
}

func (e *MalformedError) Error() string {
	return fmt.Sprintf("malformed config %s: %v", e.Path, e.Err)
}

func (e *MalformedError) Unwrap() error { return e.Err }

// Load reads and parses the config file at path. A missing file is not an
// error — it yields the zero Config (the source simply supplies no origin). A
// malformed file returns a *MalformedError so the resolver can skip the source
// and surface the cause (FR-004); any other read failure is returned as a plain
// I/O error, which the resolver treats as fatal.
func Load(path string) (Config, error) {
	//nolint:gosec // G304: path is a config location constructed internally
	// (env + walk-up + fixed global path), not attacker-controlled input;
	// reading a designated config file is this function's entire purpose.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, nil
		}

		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}

	var c Config
	if err := toml.Unmarshal(data, &c); err != nil {
		return Config{}, &MalformedError{Path: path, Err: err}
	}

	return c, nil
}

// Save writes origin-only TOML to path atomically: a temp file in the *same*
// directory (so os.Rename stays on one filesystem, research D9) is written then
// renamed over the destination. Parent directories are created as needed.
func Save(path string, o Origin) error {
	c := Config{Origin: o.String()}

	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, configFileName+".tmp-*")
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

// GlobalConfigPath returns the per-user global config path: $XDG_CONFIG_HOME/
// skillrig/config.toml when XDG_CONFIG_HOME is set, else ~/.config/skillrig/
// config.toml (research D2 — git-style, not os.UserConfigDir). The home dir is
// taken from env("HOME") when available (deterministic in tests), falling back
// to os.UserHomeDir for real invocations on platforms where it differs.
func GlobalConfigPath(env Env) (string, error) {
	if xdg := strings.TrimSpace(env("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "skillrig", configFileName), nil
	}

	home := env("HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate home directory: %w", err)
		}

		home = h
	}

	return filepath.Join(home, ".config", "skillrig", configFileName), nil
}

// FindProjectConfig walks up from cwd to the nearest ancestor containing
// .skillrig/config.toml and returns its path. The boolean reports whether one
// was found. This is a pure filesystem walk (no git subprocess) so resolution
// works offline, pre-`git init`, and in sandboxes (research D3).
func FindProjectConfig(cwd string) (string, bool) {
	dir := cwd

	for {
		candidate := filepath.Join(dir, configDirName, configFileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}

		dir = parent
	}
}

// ProjectWriteTarget returns where `skillrig init` should write the project
// config: <git-repo-root>/.skillrig/config.toml when cwd is inside a git work
// tree (located via an offline `git rev-parse --show-toplevel`), else
// <cwd>/.skillrig/config.toml (research D3, FR-010). git is a required
// dependency for the git-root case; absence falls back to cwd.
func ProjectWriteTarget(ctx context.Context, cwd string) string {
	root, ok := gitRoot(ctx, cwd)
	if !ok {
		root = cwd
	}

	return filepath.Join(root, configDirName, configFileName)
}

// gitRoot returns the work-tree root for cwd via `git rev-parse --show-toplevel`
// (offline — reads local .git, no network). The boolean is false when cwd is
// not in a repo or git is unavailable.
func gitRoot(ctx context.Context, cwd string) (string, bool) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd

	out, err := cmd.Output()
	if err != nil {
		return "", false
	}

	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", false
	}

	return root, true
}
