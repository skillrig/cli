# Research: Auth / Token Resolution for Private GitHub Origins

**Date**: 2026-05-31
**Context**: Spike S3 from `spec-tech.md §8b` — how does `skillrig` obtain a GitHub token to fetch a PRIVATE origin? Direction was already decided: `os.exec` of `gh`/`git`, NOT vendoring `gh` auth as a library. This spike validates the exact mechanism, precedence order, and how to detect and distinguish the three failure classes (FR-017: auth, FR-018: unreachable, and "not found").
**Time-box**: ~45 minutes

---

## Question

What is the correct token-resolution order for skillrig to use when fetching a private GitHub origin, and how should it detect/distinguish auth failure (FR-017) from "not found" (404) and "unreachable" (FR-018) when shelling git/gh?

---

## Findings

### Finding 1: gh-cli token-resolution internal chain (confidence: HIGH)

Source: `/Users/vincentdesmet/specledger/skillrig/gh-cli` — specifically:
- `internal/config/config.go` — `AuthConfig.ActiveToken(hostname)`
- `internal/go-gh/v2/pkg/auth/auth.go` (vendored via go.mod `github.com/cli/go-gh/v2 v2.13.0`) — `TokenForHost` / `TokenFromEnvOrConfig`

The `gh` CLI resolves a token via this internal chain (for `github.com`):

1. **`GH_TOKEN` env var** — checked first, always wins for github.com
2. **`GITHUB_TOKEN` env var** — second for github.com
3. **`hosts.yml` `oauth_token`** (plain-text config at `~/.config/gh/hosts.yml` or `$GH_CONFIG_DIR/hosts.yml`)
4. **System keyring** — `gh auth token --secure-storage` shells to the OS keyring (macOS Keychain, Linux secret service, Windows Credential Manager)

The function `go-gh/v2/pkg/auth.TokenFromEnvOrConfig` handles steps 1–3; step 4 is the keyring, accessed via `gh auth token --secure-storage --hostname <host>` (that's what `go-gh`'s `TokenForHost` does when the env/config lookup returns empty — it shells to `gh auth token --secure-storage`).

The `gh auth token` CLI command (`pkg/cmd/auth/token/token.go`) calls `authCfg.ActiveToken(hostname)`, which delegates to `ghauth.TokenFromEnvOrConfig` then falls back to the keyring. It prints the token to **stdout**, exits **0** on success, exits **1** and prints `"no oauth token found for <host>"` to **stderr** on failure.

**Verified behavior** (live test):
```
# With valid session (keyring-stored):
gh auth token          → prints token to stdout, exit 0

# With empty config dir, no env vars, no keyring:
GH_CONFIG_DIR=/tmp/empty HOME=/tmp gh auth token
→ "no oauth token found for github.com" on stderr, exit 1
```

**Key point**: `GH_TOKEN` takes priority over `GITHUB_TOKEN` inside `gh`. For `skillrig`'s own resolution before calling `gh`, it must check env vars in the same order to avoid surprising double-exec.

---

### Finding 2: mise token-resolution chain (confidence: HIGH)

Source: `/tmp/mise-spike/src/github.rs` (`resolve_token` function) and `/tmp/mise-spike/src/tokens.rs`.

mise's full precedence for `github.com` (from doc comments in `resolve_token`):

```
1. MISE_GITHUB_ENTERPRISE_TOKEN  (non-github.com only — skipped for github.com)
2. MISE_GITHUB_TOKEN             (mise-specific env var)
   GITHUB_API_TOKEN              (GitHub Actions alt)
   GITHUB_TOKEN                  (standard GitHub Actions)
3. credential_command            (mise config: settings.github.credential_command — user-defined shell cmd)
4. GitHub OAuth device-flow      (mise's own native OAuth cache — mise-specific)
5. github_tokens.toml            ($MISE_CONFIG_DIR/github_tokens.toml — per-host TOML file)
6. gh CLI hosts.yml              (reads $GH_CONFIG_DIR/hosts.yml or ~/.config/gh/hosts.yml directly as YAML)
7. git credential fill           (shells `git credential fill` with protocol=https + host)
```

**Architecture claim vs. actual source** (spec-tech.md §8b.2 claims):
```
credential_command > MISE_GITHUB_TOKEN > github_tokens.toml > gh hosts.yml > git credential
```
The **actual** order from source is:
```
MISE_GITHUB_TOKEN/GITHUB_API_TOKEN/GITHUB_TOKEN > credential_command > OAuth > github_tokens.toml > gh hosts.yml > git credential
```
The spec-tech.md claim had `credential_command` before env vars — **this is incorrect**. Env vars win in mise too.

**Important nuance for skillrig**: mise reads `gh`'s `hosts.yml` directly as a YAML file (parsing `oauth_token` field per host) rather than shelling to `gh auth token`. This is a deliberate choice in mise to avoid spawning a subprocess — but it **misses tokens stored only in the keyring** (since `hosts.yml` only holds plaintext `oauth_token` when `--insecure-storage` was used). If the user logged in with `gh auth login` using secure storage (the default), the `oauth_token` field in `hosts.yml` is absent, and mise's step 6 finds nothing.

**Implication for skillrig**: shelling to `gh auth token` (no `--secure-storage`) is the correct approach — it handles both plaintext-config and keyring tokens via one exec, making it strictly more capable than mise's direct-file approach.

The `git credential fill` subprocess in mise (`tokens.rs:get_git_credential_token`):
```
input to stdin: "protocol=https\nhost=github.com\n\n"
parses stdout for: "password=<token>" line
exit code: non-zero → no token (silent)
```

---

### Finding 3: `gh auth token` as the clean os.exec mechanism (confidence: HIGH)

`gh auth token` is the right primitive for skillrig:

- **Signature**: `gh auth token [--hostname <host>]` (default host = `github.com`)
- **stdout**: raw token string + newline (nothing else)
- **stderr**: error message only on failure
- **exit 0**: token available (token on stdout)
- **exit 1**: no token found (`"no oauth token found for <host>"` on stderr)
- **exit non-zero + empty stdout**: treat as "no gh session, skip"

The go-gh library (`TokenForHost`) uses `gh auth token --secure-storage --hostname <host>` internally to reach the keyring. skillrig does NOT need `--secure-storage` because `gh auth token` (without the flag) calls `ActiveToken` which already tries env, plaintext config, then keyring — it's the full chain.

**What `gh auth token` does NOT do**: it does NOT make a network call to validate the token. It just returns whatever token gh has stored. The token may be expired or revoked; skillrig will discover this when the actual git/gh API call returns 401/403.

---

### Finding 4: Distinguishing the three error classes from git/gh stderr (confidence: HIGH)

All three failure classes result in `git ls-remote` (and `git clone`) exiting with **128** and writing to **stderr**. The exit code alone does not distinguish them. The stderr message pattern does:

| Class | stderr contains | FR |
|---|---|---|
| **Auth failure** | `"fatal: Authentication failed for 'URL'"` | FR-017 |
| **Repo not found** | `"fatal: repository 'URL' not found"` | (not-found) |
| **Network unreachable** | `"fatal: unable to access 'URL': Could not resolve host: HOST"` | FR-018 |
| **Network timeout/refused** | `"fatal: unable to access 'URL': Failed to connect to HOST"` | FR-018 |

**Verified live**:
```bash
# Auth failure (bad token):
git ls-remote https://x-access-token:badtoken@github.com/skillrig/origin-template
→ "remote: Invalid username or token."
   "fatal: Authentication failed for 'https://github.com/skillrig/origin-template/'"
   exit: 128

# Not found (nonexistent public repo):
git ls-remote https://github.com/skillrig/this-repo-does-not-exist-xyzabc
→ "remote: Repository not found."
   "fatal: repository 'https://github.com/skillrig/this-repo-does-not-exist-xyzabc/' not found"
   exit: 128

# Unreachable (DNS failure):
git ls-remote https://github.nonexistentdomain.invalid/owner/repo
→ "fatal: unable to access 'https://github.nonexistentdomain.invalid/owner/repo/': Could not resolve host: github.nonexistentdomain.invalid"
   exit: 128
```

**Note for private repos**: a private repo with no/invalid token returns a "not found" message, not "authentication failed". GitHub deliberately obscures private repos by returning 404/not-found when unauthenticated rather than 403/forbidden. This means:

- "Repository not found" + no token → likely auth/visibility problem (warn user)
- "Repository not found" + valid token → origin typo or repo deleted
- "Authentication failed" → token exists but is wrong/revoked

skillrig should special-case the "not found" + no-token path with a hint: `"origin not found — if this is a private repo, ensure you are authenticated (run 'gh auth login' or set GITHUB_TOKEN)"`

---

### Finding 5: `git credential fill` as last resort (confidence: MEDIUM)

mise uses `git credential fill` as its step 7 fallback. For skillrig this is a reasonable last resort but adds complexity. The call:

```bash
printf 'protocol=https\nhost=github.com\n\n' | git credential fill
# Parses "password=<token>" from stdout
```

This reaches `git`'s configured credential helper (macOS Keychain via `osxkeychain`, Windows Credential Manager via `manager`, `gh`'s credential helper if configured as `git config --global credential.helper 'gh auth git-credential'`). Since `gh auth login` sets `git config --global credential.helper gh` on newer versions, this path may already be covered by the `gh auth token` exec. Avoid adding `git credential fill` as a separate step — it would only add value for users who have git credentials configured but have not run `gh auth login`. That is a very narrow case; defer to a later iteration.

---

### Finding 6: GitHub Enterprise (GHES/GHE.com) — DEFERRED

Per the spike instructions. Note for backlog:
- `GH_ENTERPRISE_TOKEN` / `GITHUB_ENTERPRISE_TOKEN` env vars (from go-gh source)
- `gh auth token --hostname <custom-host>` supports non-github.com hosts
- The origin `OWNER/REPO` parser already has room for a custom host prefix
- Design note: keep the token-resolution code path host-parameterized (pass hostname, default `"github.com"`) so GHE can be wired in without restructuring

---

## Decisions

- **D-auth-1: Token resolution order for skillrig (3 steps, in precedence order):**
  1. **`GITHUB_TOKEN` env var** (check first — matches CI/CD standard; also check `GH_TOKEN` as alias, with `GH_TOKEN` winning per gh-cli behavior)
  2. **`gh auth token` exec** — shells to `gh auth token --hostname github.com`; stdout is the token if exit 0, skip if exit non-zero
  3. **No token** → surface a typed `ErrNoAuth` immediately with an actionable message ("set GITHUB_TOKEN or run 'gh auth login'") rather than attempting the fetch unauthenticated and getting a confusing "not found"

  The `git credential fill` path (mise's step 7) is **deferred** — it only adds value for a narrow case already covered by step 2 in most developer environments.

- **D-auth-2: When to resolve the token**: resolve lazily, at fetch time (not at `config.ResolveOrigin`). The origin may be public; don't require auth just because the config is set. Attempt unauthenticated first; if the origin is private and returns "not found", retry with the token (or prompt the user). Actually simpler: resolve the token eagerly at fetch entry, pass it to the transport — git accepts `--config http.extraheader` or the `GITHUB_TOKEN` env is picked up by `git` automatically.

- **D-auth-3: Inject token into git via environment** — pass `GITHUB_TOKEN=<token>` into the `git clone`/`git ls-remote` subprocess environment. Git's credential system reads it if configured via `credential.helper` with `gh auth git-credential`, but more reliably: construct the clone URL as `https://x-access-token:<token>@github.com/OWNER/REPO` OR set the `Authorization` header via `-c http.extraheader="Authorization: Basic $(printf 'x:%s' $TOKEN | base64)"`. Using the URL with token embedded is simpler and avoids leaking to process list — prefer `-c http.extraheader` or `git credential approve` + `git clone` pattern.

  **Actually simplest**: `git clone --config "http.https://github.com.extraheader=Authorization: Basic $(printf 'x-access-token:%s' "$token" | base64)"` avoids embedding in URL.

  Even simpler for skillrig's fetch: if `gh` is available and the token came from `gh auth token`, **shell the entire fetch via `gh`**: `gh api /repos/OWNER/REPO/contents/path` or `gh api /repos/OWNER/REPO/tarball/REF` — `gh` handles auth injection automatically. Reserve raw `git` transport for when `gh` is unavailable.

- **D-auth-4: Error classification** — Parse `stderr` from `git ls-remote` / `git clone` to distinguish:
  - Contains `"Authentication failed"` or `"Invalid username or token"` → `ErrAuth` (FR-017)
  - Contains `"not found"` → `ErrNotFound`; if no token was resolved, annotate: likely private origin needing auth
  - Contains `"unable to access"` + `"Could not resolve host"` or `"Failed to connect"` → `ErrUnreachable` (FR-018)
  - All others → wrap as `ErrUnknown` with raw stderr in `--verbose`

- **D-auth-5: `gh auth token` output contract** — `exit 0` + non-empty stdout = token. `exit non-zero` = no session (skip, not fatal at resolution time). `gh` not found in PATH = no `gh` session available; fall back to env-only. Never treat a missing `gh` binary as an error.

- **D-auth-6: GitHub Enterprise — deferred** to a follow-up; design the token-resolution function to accept a `hostname string` parameter (default `"github.com"`) so GHE is a one-line extension.

---

## Recommendations

1. **Implement `pkg/skillcore.ResolveGitHubToken(hostname string) (token string, source string, err error)`** with this body:
   ```go
   // Step 1: env vars (GH_TOKEN wins over GITHUB_TOKEN, matching gh's own precedence)
   if t := os.Getenv("GH_TOKEN"); t != "" {
       return t, "GH_TOKEN", nil
   }
   if t := os.Getenv("GITHUB_TOKEN"); t != "" {
       return t, "GITHUB_TOKEN", nil
   }
   // Step 2: gh auth token exec
   out, err := exec.Command("gh", "auth", "token", "--hostname", hostname).Output()
   if err == nil && len(strings.TrimSpace(string(out))) > 0 {
       return strings.TrimSpace(string(out)), "gh", nil
   }
   // Step 3: no token found
   return "", "", nil  // not an error; caller decides if token is required
   ```
   Return `("", "", nil)` — callers that need a token check for empty string and surface `ErrNoAuth`.

2. **Implement `pkg/skillcore.ClassifyGitError(stderr string, exitCode int) error`** using the patterns from Finding 4. Use the typed errors `ErrAuth`, `ErrNotFound`, `ErrUnreachable` so `internal/cli` can render distinct messages per FR-016–018.

3. **When "not found" + no token**: render as: `"origin not found: github.com/OWNER/REPO — this may be a private origin; set GITHUB_TOKEN or run 'gh auth login'"` — not just "not found".

4. **Auth failure wording** (FR-017): `"authentication failed for github.com/OWNER/REPO — your token may be invalid or expired; run 'gh auth login' or check GITHUB_TOKEN"`. Always include the fix.

5. **Inject token into git subprocess** via `-c http.extraheader` flag rather than URL-embedding (avoids tokens in shell history and process table). Pattern:
   ```go
   headerVal := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token))
   cmd := exec.Command("git", "-c", "http.extraheader="+headerVal, "ls-remote", repoURL)
   ```

6. **Do not read `~/.config/gh/hosts.yml` directly** (like mise does). Shelling to `gh auth token` is the correct interface — it handles keyring-stored tokens that plaintext YAML misses, and it is the stable public API surface for the `gh` binary.

7. **GitHub Enterprise**: defer. Note in `docs/ARCHITECTURE-v0.md` as backlog: `gh auth token --hostname <ghe-host>` and `GITHUB_ENTERPRISE_TOKEN` env var.

---

## References

- `gh-cli` token resolution (checked out at `/Users/vincentdesmet/specledger/skillrig/gh-cli`):
  - `internal/config/config.go` — `AuthConfig.ActiveToken` (env → config → keyring chain)
  - `internal/config/auth_config_test.go` — confirms `GH_TOKEN` > `GITHUB_TOKEN` precedence (see `TestTokenStoredInEnv`)
  - `pkg/cmd/auth/token/token.go` — `gh auth token` command: stdout=token, exit 1 + stderr on failure
  - Go module `github.com/cli/go-gh/v2@v2.13.0` at `/Users/vincentdesmet/go/pkg/mod/github.com/cli/go-gh/v2@v2.13.0/pkg/auth/auth.go` — `TokenFromEnvOrConfig` + `tokenFromGh` (the `--secure-storage` exec path)

- `mise` token resolution (cloned at `/tmp/mise-spike`):
  - `src/github.rs` — `resolve_token` function (lines 479–565): full 7-step precedence with doc comments
  - `src/tokens.rs` — `get_credential_command_token` and `get_git_credential_token` subprocess implementations
  - `src/cli/token/github.rs` — `mise token github` command (debug/diagnostics)

- Live test observations (2026-05-31):
  - `gh auth token` with empty config + no env → exit 1, `"no oauth token found for github.com"` to stderr
  - `git ls-remote` with bad token → exit 128, `"fatal: Authentication failed for 'URL'"` to stderr
  - `git ls-remote` on nonexistent repo → exit 128, `"fatal: repository 'URL' not found"` to stderr
  - `git ls-remote` with bad hostname → exit 128, `"fatal: unable to access '...': Could not resolve host: ..."` to stderr
