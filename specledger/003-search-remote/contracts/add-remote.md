# Contract: `skillrig add` — remote acquisition + `--pin` (Vendor Mutation)

Extends 002's local-copy `add` with a **remote fetch** path and reproducible pinning. Byte-identical vendoring, idempotent no-op, and force-on-divergence are **unchanged from 002**; this contract adds what's new.

## Synopsis
```
skillrig add <skill> [--pin <ref>] [--dry-run] [--force] [--json] [--verbose]
```

## Flags (new/changed)
| Flag | Meaning |
|---|---|
| `<skill>` | skill directory name (single safe path segment — 002 path-traversal guard applies) |
| `--pin <ref>` | acquire an immutable tag/SHA (D5); distinct from the origin `@ref` branch. **Resolution order (C3, deterministic):** (1) if `<ref>` matches `^v?SEMVER$` (a bare version like `v1.4.0`/`1.4.0`) → expand via `tag_scheme name-vSEMVER` to `<skill>-v<semver>` and resolve that tag; (2) else treat `<ref>` as a **literal git ref** (a full `<skill>-vX.Y.Z` tag or a commit SHA) passed through unchanged. No ambiguity: a bare semver is always tag-expanded, anything else is literal; the two forms for the same release MUST resolve to the same commit/treeSha (asserted by quickstart). |
| `--dry-run`/`--force`/`--json`/`--verbose` | as 002 |

## Behavior
1. Resolve origin; **classify form** (local path vs remote `OWNER/REPO` — D3). Report which form is used.
2. **Remote form:** resolve token (D4: `GH_TOKEN`→`GITHUB_TOKEN`→`gh auth token`); `git clone --sparse` the skill `path` (from the catalog or the conventional `skills/<skill>`) at `--pin` ref else origin `@ref` (D7); inject token via `git -c http.extraHeader`.
   - **Local-path form:** 002 behavior on the explicit path (no fetch).
3. Compute `commit` (`git rev-parse` of the fetched ref) + `treeSha` (`skillcore.TreeSHA` of the subtree — same code `verify` uses, AP-04).
4. Vendor byte-identically into `.agents/skills/<skill>/` (002 copy + symlink guard).
5. Write/refresh the lock entry: `version` = resolved tag (pin) else manifest `version`; `commit`; `treeSha`; `path` (data-model §3).
6. Idempotent: same version+content already present → `unchanged`, **exit 0**, nothing written. Divergent local content → refuse, instruct `--force` (002).

## Output
- **Human:** one-line result (`added <skill> <version> (<short-sha>)` | `unchanged` | dry-run preview) + footer (`run: skillrig verify`). Bounded lines.
- **`--json`:** the written lock entry (`version`/`commit`/`treeSha`/`path`) — structurally complete (Constitution §II).

## Exit codes
| Code | When |
|---|---|
| 0 | vendor completed OR idempotent no-op |
| 1 | no origin; not found; auth; unreachable; convention mismatch; invalid name; symlink; divergence-without-`--force` |

(Exit 2/3 reserved, never emitted here.)

## Errors (distinct, exit 1 — data-model §5)
`NotFoundError` (skill not published — *if private + no token, hint to authenticate*, D4), `AuthError`, `UnreachableError`, `IncompatibleConventionError`, plus 002's `OriginNotFoundError`/`InvalidSkillNameError`/`SymlinkUnsupportedError`/overwrite-divergence. **Pin to a non-existent version → a distinct `NoSuchVersionError`** (C2) — a separate typed error from `NotFoundError` (the skill exists, the version doesn't), so callers branch on the type, not on message text. `--verbose` → raw cause.

## Help
Purpose + ≥2 examples (`skillrig add terraform-plan-review`, `skillrig add terraform-plan-review --pin v1.4.0`). `TestQuickstart_AddHelpExamples` (002, extend for `--pin`).

## Tests
`TestQuickstart_AddRemoteNoLocalCopy` (+ then `verify` passes), `_AddRemoteIdempotent`, `_AddPinnedReproducible` (two clean repos byte-identical), `_AddPinNotFound`, `_AddAuthFailureDistinct`, `_AddUnreachableDistinct`, `_AddPrivateNotFoundHintsAuth`, plus 002's local-path suite stays green (SC-007). Ground-truth: lock `treeSha` == raw `git ls-tree` over the `file://` bare-repo fixture.
