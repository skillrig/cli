package config

import (
	"errors"
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

// SourceDiagnostic records a config source that was present but unusable — a
// malformed file or one whose origin fails OWNER/REPO validation — and was
// therefore skipped (FR-004). It is the cause a caller surfaces under --verbose
// instead of a raw parser dump; it is never itself a hard error.
type SourceDiagnostic struct {
	// Source is the source the skipped file belonged to (project or global).
	Source Source
	// Path is the skipped file's path.
	Path string
	// Reason is the human-readable cause (parse error or invalid origin).
	Reason string
}

// ResolutionResult is the outcome of ResolveOrigin.
type ResolutionResult struct {
	// Origin is the resolved origin, or the zero Origin when Source is none.
	Origin Origin
	// Source names which input supplied the origin.
	Source Source
	// ConfigPath is the file that supplied it (empty for env and none).
	ConfigPath string
	// Diagnostics lists every source that was present but skipped because it
	// was unusable (malformed / invalid origin), in precedence order. It is
	// populated regardless of the final Source so a caller can explain, under
	// --verbose, why a higher-precedence source did not win (FR-004).
	Diagnostics []SourceDiagnostic
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
	var diags []SourceDiagnostic

	if raw := strings.TrimSpace(env(envOriginKey)); raw != "" {
		origin, err := ParseOrigin(raw)
		if err != nil {
			return ResolutionResult{}, fmt.Errorf("%s: %w", envOriginKey, err)
		}

		return ResolutionResult{Origin: origin, Source: SourceEnv}, nil
	}

	if path, found := FindProjectConfig(cwd); found {
		origin, ok, diag, err := originFromFile(path, SourceProject)
		if err != nil {
			return ResolutionResult{}, err
		}

		if diag != nil {
			diags = append(diags, *diag)
		}

		if ok {
			return ResolutionResult{Origin: origin, Source: SourceProject, ConfigPath: path, Diagnostics: diags}, nil
		}
	}

	globalPath, err := GlobalConfigPath(env)
	if err != nil {
		return ResolutionResult{}, err
	}

	origin, ok, diag, err := originFromFile(globalPath, SourceGlobal)
	if err != nil {
		return ResolutionResult{}, err
	}

	if diag != nil {
		diags = append(diags, *diag)
	}

	if ok {
		return ResolutionResult{Origin: origin, Source: SourceGlobal, ConfigPath: globalPath, Diagnostics: diags}, nil
	}

	return ResolutionResult{Source: SourceNone, Diagnostics: diags}, nil
}

// originFromFile loads one config source and classifies the outcome so the
// caller can apply FR-004 precisely:
//
//   - ok == true                  → a valid origin (no diagnostic, no error)
//   - ok == false, diag != nil    → file present but unusable (malformed, or an
//     origin that fails OWNER/REPO): record the diagnostic and continue
//   - ok == false, diag == nil    → source supplies no origin (absent file, or
//     a parseable file with no origin key): a normal, silent fall-through
//   - err != nil                  → a genuine I/O error (e.g. unreadable file):
//     fatal, returned to the caller (contract resolve.md)
func originFromFile(path string, source Source) (Origin, bool, *SourceDiagnostic, error) {
	cfg, loadErr := Load(path)

	var malformed *MalformedError

	switch {
	case loadErr == nil:
		// File read and parsed; classify its origin below.
	case errors.As(loadErr, &malformed):
		// FR-004: a malformed file is deliberately non-fatal — reported as a
		// skippable diagnostic, not propagated as an error.
		return skip(source, path, loadErr.Error())
	default:
		// A genuine I/O error (e.g. unreadable file) is fatal (contract resolve.md).
		return Origin{}, false, nil, loadErr
	}

	if strings.TrimSpace(cfg.Origin) == "" {
		return Origin{}, false, nil, nil
	}

	origin, parseErr := ParseOrigin(cfg.Origin)
	if parseErr != nil {
		// Present file whose origin fails OWNER/REPO: skippable diagnostic.
		return skip(source, path, parseErr.Error())
	}

	return origin, true, nil, nil
}

// skip builds the "present but unusable, continue down precedence" result: no
// origin, a recorded diagnostic, and no error (the skip is intentional, FR-004).
func skip(source Source, path, reason string) (Origin, bool, *SourceDiagnostic, error) {
	return Origin{}, false, &SourceDiagnostic{Source: source, Path: path, Reason: reason}, nil
}
