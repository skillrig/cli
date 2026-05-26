package config

import (
	"fmt"
	"strings"
)

// envOriginKey is the environment variable that overrides all file sources.
const envOriginKey = "SKILLRIG_ORIGIN"

// Source identifies which input supplied the resolved origin.
type Source string

const (
	// SourceEnv is the SKILLRIG_ORIGIN environment variable.
	SourceEnv Source = "env"
	// SourceProject is a project .skillrig/config.toml found via walk-up.
	SourceProject Source = "project"
	// SourceGlobal is the per-user global config.
	SourceGlobal Source = "global"
	// SourceNone means no origin was configured in any source — a normal,
	// first-class outcome the caller turns into the US3 "no origin" error.
	SourceNone Source = "none"
)

// ResolutionResult is the outcome of ResolveOrigin.
type ResolutionResult struct {
	// Origin is the resolved origin, or the zero Origin when Source is none.
	Origin Origin
	// Source names which input supplied the origin.
	Source Source
	// ConfigPath is the file that supplied it (empty for env and none).
	ConfigPath string
}

// ResolveOrigin determines the active origin for cwd and env, applying
// precedence SKILLRIG_ORIGIN > project config (nearest ancestor) > global
// config (FR-002). It is the single resolver every command uses (AP-06): a pure
// function of (cwd, env, filesystem) with no network, time, or global state.
//
// A blank SKILLRIG_ORIGIN is treated as unset. A malformed or origin-less file
// source yields no origin and resolution continues down precedence (FR-004); it
// is not an error. Source==none with the zero Origin is a normal return. An
// explicitly set but malformed SKILLRIG_ORIGIN is the one hard error: it is a
// deliberate override that must be valid.
func ResolveOrigin(cwd string, env Env) (ResolutionResult, error) {
	if raw := strings.TrimSpace(env(envOriginKey)); raw != "" {
		origin, err := ParseOrigin(raw)
		if err != nil {
			return ResolutionResult{}, fmt.Errorf("%s: %w", envOriginKey, err)
		}

		return ResolutionResult{Origin: origin, Source: SourceEnv}, nil
	}

	if path, found := FindProjectConfig(cwd); found {
		if origin, ok := usableOrigin(path); ok {
			return ResolutionResult{Origin: origin, Source: SourceProject, ConfigPath: path}, nil
		}
	}

	globalPath, err := GlobalConfigPath(env)
	if err != nil {
		return ResolutionResult{}, err
	}

	if origin, ok := usableOrigin(globalPath); ok {
		return ResolutionResult{Origin: origin, Source: SourceGlobal, ConfigPath: globalPath}, nil
	}

	return ResolutionResult{Source: SourceNone}, nil
}

// usableOrigin loads the config at path and returns a valid origin from it. Any
// failure mode — missing file, unreadable, malformed TOML, origin-less, or an
// origin that fails OWNER/REPO validation — collapses to (zero, false) so the
// caller skips the source and continues down precedence (FR-004).
func usableOrigin(path string) (Origin, bool) {
	cfg, err := Load(path)
	if err != nil || strings.TrimSpace(cfg.Origin) == "" {
		return Origin{}, false
	}

	origin, err := ParseOrigin(cfg.Origin)
	if err != nil {
		return Origin{}, false
	}

	return origin, true
}
