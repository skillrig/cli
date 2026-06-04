//go:build e2e

// True-authentication end-to-end suite (run with `make test-e2e`). See doc.go for
// why this tier exists. It stands up a real `git http-backend` behind an
// Authorization-header gate and drives the real skillrig binary at it over the
// real HTTPS token path (redirected to the local server with git's url.insteadOf),
// so a valid token genuinely authenticates and a missing/wrong one genuinely 401s.
package e2e

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
)

const (
	// sampleSkill is the one skill the sample-origin fixture ships.
	sampleSkill = "terraform-plan-review"
	// originRepo is the REMOTE OWNER/REPO origin form, so skillrig's CloneURL is
	// https://github.com/my-org/my-skills.git — redirected to the local server.
	originRepo = "my-org/my-skills"
	// validToken is the credential the server requires. skillrig injects it as
	// `Authorization: Basic base64("x-access-token:<token>")` via http.extraHeader;
	// any other value (or none) gets a real 401.
	validToken = "s3cr3t-e2e-token"
)

// binPath is the built skillrig binary, shared across scenarios.
var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "skillrig-e2e-bin-*")
	if err != nil {
		panic(err)
	}

	binPath = filepath.Join(dir, "skillrig")

	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = filepath.Join("..", "..") // module root, relative to test/e2e/

	if out, err := build.CombinedOutput(); err != nil {
		_, _ = os.Stderr.WriteString("e2e build failed: " + err.Error() + "\n" + string(out))
		os.Exit(1)
	}

	code := m.Run()

	_ = os.RemoveAll(dir)

	os.Exit(code)
}

// ---------------------------------------------------------------------------
// The auth-gated git server (the real substrate).
// ---------------------------------------------------------------------------

// reqRecord is one HTTP request the server saw — used to prove the credential
// rode the Authorization header (not the URL) and that a real round-trip happened.
type reqRecord struct {
	method string
	uri    string
	auth   string
}

// gitAuthServer is an in-process git remote: an Authorization-header gate fronting
// `git http-backend` (the real CGI binary) over a bare repo. It records every
// request so a test can assert the auth round-trip and the no-leak invariant.
type gitAuthServer struct {
	*httptest.Server

	token string

	mu      sync.Mutex
	records []reqRecord
}

// newGitAuthServer serves projectRoot's bare repos over smart HTTP, accepting only
// the exact `Authorization: Basic base64("x-access-token:<token>")` header (else a
// real 401). git-http-backend is re-instantiated per request so the client's
// Git-Protocol (v2) header is threaded into the CGI env.
func newGitAuthServer(t *testing.T, projectRoot, token string) *gitAuthServer {
	t.Helper()

	backend := gitHTTPBackendPath(t)
	s := &gitAuthServer{token: token}

	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")

		s.mu.Lock()
		s.records = append(s.records, reqRecord{method: r.Method, uri: r.URL.RequestURI(), auth: auth})
		s.mu.Unlock()

		if auth != expectedAuthHeader(token) {
			w.Header().Set("WWW-Authenticate", `Basic realm="skillrig-e2e"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)

			return
		}

		env := []string{
			"GIT_PROJECT_ROOT=" + projectRoot,
			"GIT_HTTP_EXPORT_ALL=1",
		}
		// Thread protocol v2 (the client advertises it via the Git-Protocol header;
		// Go's cgi exposes it as HTTP_GIT_PROTOCOL, but git-http-backend reads
		// GIT_PROTOCOL) so partial-clone negotiation matches skillrig's real client.
		if gp := r.Header.Get("Git-Protocol"); gp != "" {
			env = append(env, "GIT_PROTOCOL="+gp)
		}

		(&cgi.Handler{Path: backend, Env: env}).ServeHTTP(w, r)
	}))

	t.Cleanup(s.Server.Close)

	return s
}

// assertAuthenticatedAndNoTokenInURL proves the credential reached the server in
// the Authorization HEADER and that neither the token nor its base64 form ever
// appeared in a request URL (the e2e face of the http.extraHeader-via-env design;
// the argv-level no-leak is pinned by skillcore's TestClone_TokenInjectionViaEnv).
func (s *gitAuthServer) assertAuthenticatedAndNoTokenInURL(t *testing.T, token string) {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	want := expectedAuthHeader(token)
	b64 := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	sawAuthHeader := false

	for _, r := range s.records {
		if r.auth == want {
			sawAuthHeader = true
		}

		if strings.Contains(r.uri, token) {
			t.Errorf("token leaked into request URL %q (must ride the Authorization header, not the URL)", r.uri)
		}

		if strings.Contains(r.uri, b64) {
			t.Errorf("base64 credential leaked into request URL %q", r.uri)
		}
	}

	if !sawAuthHeader {
		t.Errorf("server never received the expected Authorization header (auth never reached the wire); records=%+v", s.records)
	}
}

// assertSawRequestWithAuth asserts the server saw at least one request carrying
// exactly authValue — proving skillrig actually transmitted that header value
// (empty for the no-credential case, the wrong base64 for the wrong-token case).
func (s *gitAuthServer) assertSawRequestWithAuth(t *testing.T, authValue string) {
	t.Helper()

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.records) == 0 {
		t.Fatalf("server saw no requests at all — skillrig never reached the real server")
	}

	for _, r := range s.records {
		if r.auth == authValue {
			return
		}
	}

	t.Errorf("server never saw a request with Authorization=%q; records=%+v", authValue, s.records)
}

// expectedAuthHeader is the exact header skillrig injects via http.extraHeader for
// token (base64 of x-access-token:<token>, the gh-cli convention).
func expectedAuthHeader(token string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token))
}

// gitHTTPBackendPath resolves the real git-http-backend CGI binary shipped with
// git, skipping the scenario cleanly if git (or the backend) is unavailable.
func gitHTTPBackendPath(t *testing.T) string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "git", "--exec-path").Output()
	if err != nil {
		t.Skipf("git --exec-path failed (%v); skipping e2e", err)
	}

	p := filepath.Join(strings.TrimSpace(string(out)), "git-http-backend")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("git-http-backend not found at %s (%v); skipping e2e", p, err)
	}

	return p
}

// ---------------------------------------------------------------------------
// Origin bare-repo fixture + skillrig wiring.
// ---------------------------------------------------------------------------

// newOriginBareRepo builds <projectRoot>/my-org/my-skills.git from the committed
// sample-origin fixture (a real bare repo the server serves over smart HTTP) and
// returns projectRoot. Partial-clone-over-HTTP is enabled so skillrig's
// --filter=blob:none clone + on-demand blob fetch work against it.
func newOriginBareRepo(t *testing.T) string {
	t.Helper()

	projectRoot := t.TempDir()

	work := t.TempDir()
	copyTree(t, sampleOriginDir(t), work)
	runGit(t, work, "init", "-q", "-b", "main")
	runGit(t, work, "add", "-A")
	runGit(t, work, "commit", "-q", "-m", "origin fixture")

	bare := filepath.Join(projectRoot, "my-org", "my-skills.git")
	if err := os.MkdirAll(filepath.Dir(bare), 0o755); err != nil {
		t.Fatalf("mkdir origin parent: %v", err)
	}

	runGit(t, "", "clone", "-q", "--bare", work, bare)
	runGit(t, bare, "config", "uploadpack.allowFilter", "true")
	runGit(t, bare, "config", "uploadpack.allowAnySHA1InWant", "true")

	return projectRoot
}

// originEnv wires skillrig at the local server: url.insteadOf rewrites
// https://github.com/ → the server, a fake `gh` (exit 1) on PATH keeps the gh tier
// from yielding any real token, and ghToken (when non-empty) sets GH_TOKEN. System
// + global git config are neutralized so only the insteadOf + skillrig's injected
// http.extraHeader apply (hermetic).
func originEnv(t *testing.T, srvURL, ghToken string) map[string]string {
	t.Helper()

	binDir := t.TempDir()
	writeExec(t, filepath.Join(binDir, "gh"), "#!/bin/sh\nexit 1\n")

	cfgDir := t.TempDir()
	gitconfig := filepath.Join(cfgDir, "insteadof.gitconfig")
	writeFile(t, gitconfig, "[url \""+srvURL+"/\"]\n\tinsteadOf = https://github.com/\n")

	env := map[string]string{
		"PATH":              binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GIT_CONFIG_GLOBAL": gitconfig,
		"GIT_CONFIG_SYSTEM": os.DevNull,
		"SKILLRIG_ORIGIN":   originRepo,
	}

	if ghToken != "" {
		env["GH_TOKEN"] = ghToken
	}

	return env
}

// runResult is the observable contract of a skillrig invocation.
type runResult struct {
	stdout string
	stderr string
	exit   int
}

// runSkillrig execs the built binary with an isolated HOME and the given extra env
// (which may override PATH/HOME). stdin is a pipe (never a TTY), so the binary is
// non-interactive — exactly the CI shape issue #25 is about.
func runSkillrig(t *testing.T, args []string, cwd string, extraEnv map[string]string) runResult {
	t.Helper()

	home := t.TempDir()

	cmd := exec.CommandContext(t.Context(), binPath, args...)
	cmd.Dir = cwd

	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
	}
	for k, v := range extraEnv {
		env = append(env, k+"="+v)
	}

	cmd.Env = env
	cmd.Stdin = strings.NewReader("")

	var stdout, stderr strings.Builder

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	exit := 0

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exit = exitErr.ExitCode()
		} else {
			t.Fatalf("exec skillrig %v: %v", args, err)
		}
	}

	return runResult{stdout: stdout.String(), stderr: stderr.String(), exit: exit}
}

// ---------------------------------------------------------------------------
// Scenarios.
// ---------------------------------------------------------------------------

// TestE2E_ValidToken_SearchLists (scenario 1, search — the issue's primary
// command) — with a valid GH_TOKEN, the real header authenticates against the real
// server and search lists the origin's skill. Also covers scenario 4 (no-leak).
func TestE2E_ValidToken_SearchLists(t *testing.T) {
	projectRoot := newOriginBareRepo(t)
	srv := newGitAuthServer(t, projectRoot, validToken)

	res := runSkillrig(t, []string{"search", "--json"}, t.TempDir(), originEnv(t, srv.URL, validToken))
	if res.exit != 0 {
		t.Fatalf("authenticated search exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	if !strings.Contains(res.stdout, sampleSkill) {
		t.Errorf("authenticated search did not list %q:\n%s", sampleSkill, res.stdout)
	}

	srv.assertAuthenticatedAndNoTokenInURL(t, validToken)
}

// TestE2E_ValidToken_AddVendors (scenario 1, add) — the full fetch+vendor path
// authenticates and writes the skill byte-identical to the origin, with a lock.
// This is the strongest "the token really works" proof: nothing is vendored unless
// the real 401 gate let the real clone through.
func TestE2E_ValidToken_AddVendors(t *testing.T) {
	projectRoot := newOriginBareRepo(t)
	srv := newGitAuthServer(t, projectRoot, validToken)

	consumer := t.TempDir()
	runGit(t, consumer, "init", "-q", "-b", "main")

	res := runSkillrig(t, []string{"add", sampleSkill}, consumer, originEnv(t, srv.URL, validToken))
	if res.exit != 0 {
		t.Fatalf("authenticated add exit = %d, want 0 (stderr: %s)", res.exit, res.stderr)
	}

	got := readFile(t, filepath.Join(consumer, ".agents", "skills", sampleSkill, "SKILL.md"))
	want := readFile(t, filepath.Join(sampleOriginDir(t), "skills", sampleSkill, "SKILL.md"))

	if got != want {
		t.Errorf("vendored SKILL.md differs from the origin fixture")
	}

	if _, err := os.Stat(filepath.Join(consumer, ".skillrig", "skills-lock.json")); err != nil {
		t.Errorf("lock not written after authenticated add: %v", err)
	}

	srv.assertAuthenticatedAndNoTokenInURL(t, validToken)
}

// TestE2E_NoToken_FailsFastAsAuth (scenario 2) — no credential anywhere
// (GH_TOKEN/GITHUB_TOKEN unset, gh yields nothing): the real server 401s, git —
// forced non-interactive — fails fast, and skillrig renders an AuthError pointing
// at authentication. No hang, no prompt, exit 1.
func TestE2E_NoToken_FailsFastAsAuth(t *testing.T) {
	projectRoot := newOriginBareRepo(t)
	srv := newGitAuthServer(t, projectRoot, validToken)

	res := runSkillrig(t, []string{"search"}, t.TempDir(), originEnv(t, srv.URL, ""))
	if res.exit != 1 {
		t.Fatalf("no-credential search exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	if !strings.Contains(res.stderr, "authentication failed") {
		t.Errorf("no-credential failure should render an AuthError, got:\n%s", res.stderr)
	}

	if !strings.Contains(res.stderr, "gh auth login") &&
		!strings.Contains(res.stderr, "GH_TOKEN") &&
		!strings.Contains(res.stderr, "GITHUB_TOKEN") {
		t.Errorf("auth fix should point at gh auth login / GH_TOKEN / GITHUB_TOKEN, got:\n%s", res.stderr)
	}

	// A real round-trip happened with NO credential header (proving it 401'd for
	// real, not via a stub).
	srv.assertSawRequestWithAuth(t, "")
}

// TestE2E_WrongToken_FailsAsAuth (scenario 3) — a wrong GH_TOKEN is injected as the
// header, the server rejects it (401), and skillrig renders an AuthError. The
// server confirms it actually received the (wrong) credential, distinguishing this
// from the no-credential case at the wire.
func TestE2E_WrongToken_FailsAsAuth(t *testing.T) {
	projectRoot := newOriginBareRepo(t)
	srv := newGitAuthServer(t, projectRoot, validToken)

	const wrong = "not-the-right-token"

	res := runSkillrig(t, []string{"search"}, t.TempDir(), originEnv(t, srv.URL, wrong))
	if res.exit != 1 {
		t.Fatalf("wrong-token search exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if res.stdout != "" {
		t.Errorf("error path must keep stdout empty, got: %q", res.stdout)
	}

	if !strings.Contains(res.stderr, "authentication failed") {
		t.Errorf("wrong-token failure should render an AuthError, got:\n%s", res.stderr)
	}

	// The wrong credential really hit the wire as the injected header.
	srv.assertSawRequestWithAuth(t, expectedAuthHeader(wrong))
}

// TestE2E_NoToken_DoesNotInvokeAskpass (issue #25 — the VS Code footgun) — a GUI
// shell (VS Code's integrated terminal) exports GIT_ASKPASS pointing at its own
// interactive credential dialog. git consults askpass BEFORE the terminal, so
// GIT_TERMINAL_PROMPT=0 alone does NOT stop the prompt — without neutralizing
// GIT_ASKPASS, an unauthenticated fetch pops a real dialog (and hangs) instead of
// failing. This stands up that exact environment (a fake askpass that records
// every call) and asserts skillrig (a) fails fast as an AuthError and (b) NEVER
// invokes the askpass program — no dialog, no hang.
func TestE2E_NoToken_DoesNotInvokeAskpass(t *testing.T) {
	projectRoot := newOriginBareRepo(t)
	srv := newGitAuthServer(t, projectRoot, validToken)

	probeDir := t.TempDir()
	marker := filepath.Join(probeDir, "askpass-invoked")
	askpass := filepath.Join(probeDir, "askpass.sh")
	// A stand-in for VS Code's askpass: record the call (so the test can prove it
	// fired) and emit a dummy answer (so a non-neutralized git would proceed).
	writeExec(t, askpass, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+shellSingleQuote(marker)+"\necho dummy\n")

	env := originEnv(t, srv.URL, "") // no credential available
	env["GIT_ASKPASS"] = askpass     // ...but the GUI shell exports an askpass dialog

	res := runSkillrig(t, []string{"search"}, t.TempDir(), env)
	if res.exit != 1 {
		t.Fatalf("no-token (askpass-exported) search exit = %d, want 1 (stderr: %s)", res.exit, res.stderr)
	}

	if !strings.Contains(res.stderr, "authentication failed") {
		t.Errorf("should render an AuthError, got:\n%s", res.stderr)
	}

	// THE regression: the askpass program must never have run. If the marker exists,
	// git invoked it — i.e. a real GUI helper would have popped a dialog and hung.
	if data, err := os.ReadFile(marker); err == nil {
		t.Errorf("git invoked GIT_ASKPASS despite skillrig forcing non-interactive — a GUI helper "+
			"(e.g. VS Code's) would have prompted/hung (issue #25). askpass call log:\n%s", data)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat askpass marker: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Full init → search → add workflow (multi-skill catalog, branch + auth).
// ---------------------------------------------------------------------------

// writeOriginSkill writes a minimal but valid skill at skills/<name>/SKILL.md in
// the origin work tree (valid x-skillrig frontmatter incl. the version `index`
// requires), so `skillrig index` lists it and `add` can ParseManifest + vendor it.
func writeOriginSkill(t *testing.T, work, name, desc string, topics []string) {
	t.Helper()

	fm := "---\n" +
		"name: " + name + "\n" +
		"description: " + desc + "\n" +
		"metadata:\n" +
		"  x-skillrig.namespace: my-org\n" +
		"  x-skillrig.version: 1.0.0\n" +
		"  x-skillrig.convention-version: \"1\"\n" +
		"  x-skillrig.topics: [" + strings.Join(topics, ", ") + "]\n" +
		"---\n\n# " + name + "\n\nSkill body for " + name + ".\n"

	writeFile(t, filepath.Join(work, "skills", name, "SKILL.md"), fm)
}

// newMultiSkillOrigin builds a bare origin whose DEFAULT branch (main) publishes
// TWO skills and a NON-DEFAULT branch (staging) publishes THREE (a staging-only
// extra), each catalog generated by the REAL `skillrig index` from the skill
// frontmatter. It returns the projectRoot the auth server serves. The branch
// difference lets the workflow test prove search/add actually read the @ref'd
// branch (not main) — the issue this fix is about.
func newMultiSkillOrigin(t *testing.T) string {
	t.Helper()

	projectRoot := t.TempDir()

	work := t.TempDir()
	copyTree(t, sampleOriginDir(t), work) // skills/terraform-plan-review + .skillrig-origin.toml
	runGit(t, work, "init", "-q", "-b", "main")

	// main: add a second skill, then regenerate the catalog with `skillrig index`.
	writeOriginSkill(t, work, "aws-iam-audit", "Audit an AWS IAM policy set for drift.", []string{"aws", "security"})
	indexOrigin(t, work)
	runGit(t, work, "add", "-A")
	runGit(t, work, "commit", "-q", "-m", "main: two skills")

	// staging: a third, branch-only skill + a regenerated catalog.
	runGit(t, work, "checkout", "-q", "-b", "staging")
	writeOriginSkill(t, work, "staging-only", "A skill published only on the staging branch.", []string{"staging"})
	indexOrigin(t, work)
	runGit(t, work, "add", "-A")
	runGit(t, work, "commit", "-q", "-m", "staging: three skills")
	runGit(t, work, "checkout", "-q", "main")

	bare := filepath.Join(projectRoot, "my-org", "my-skills.git")
	if err := os.MkdirAll(filepath.Dir(bare), 0o755); err != nil {
		t.Fatalf("mkdir origin parent: %v", err)
	}

	runGit(t, work, "clone", "-q", "--bare", ".", bare)
	runGit(t, bare, "config", "uploadpack.allowFilter", "true")
	runGit(t, bare, "config", "uploadpack.allowAnySHA1InWant", "true")

	return projectRoot
}

// indexOrigin runs the real `skillrig index` inside the origin work tree to
// regenerate index.json from the skill frontmatter (it needs a git toplevel +
// .skillrig-origin.toml, both present). It is local + offline — no origin, token,
// or insteadOf wiring needed.
func indexOrigin(t *testing.T, work string) {
	t.Helper()

	if r := runSkillrig(t, []string{"index"}, work, map[string]string{"GIT_CONFIG_SYSTEM": os.DevNull}); r.exit != 0 {
		t.Fatalf("skillrig index in origin exit = %d (stderr: %s)", r.exit, r.stderr)
	}
}

// workflowEnv is originEnv WITHOUT SKILLRIG_ORIGIN, so the active origin is
// resolved from the project config that `skillrig init` writes (exercising the
// real init → resolve → search → add chain, not an env shortcut).
func workflowEnv(t *testing.T, srvURL, token string) map[string]string {
	t.Helper()

	env := originEnv(t, srvURL, token)
	delete(env, "SKILLRIG_ORIGIN")

	return env
}

// searchSkillNames decodes `search --json` stdout into the sorted list of skill
// names (order-independent membership for the workflow assertions).
func searchSkillNames(t *testing.T, stdout string) []string {
	t.Helper()

	var payload struct {
		Skills []struct {
			Name string `json:"name"`
		} `json:"skills"`
	}

	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("search --json is not parseable: %v\n%s", err, stdout)
	}

	names := make([]string, len(payload.Skills))
	for i, s := range payload.Skills {
		names[i] = s.Name
	}

	sort.Strings(names)

	return names
}

// TestE2E_FullWorkflow drives the complete consumer loop against the real
// auth-gated server: `skillrig init --origin` (BOTH the default-branch and the
// @staging forms, with auth), then `search` (asserting >1 result, exactly the
// branch's catalog), then `add` of the first and second skill (asserting each is
// vendored and locked). The @staging case is the end-to-end proof of the
// non-default-branch catalog fix — its search returns the staging-only skill that
// only exists on that branch, which a pre-fix build could not fetch at all.
func TestE2E_FullWorkflow(t *testing.T) {
	projectRoot := newMultiSkillOrigin(t)
	srv := newGitAuthServer(t, projectRoot, validToken)

	cases := []struct {
		name       string
		originArg  string
		wantSkills []string // the branch's full catalog (sorted)
		add        []string // first, then second
	}{
		{
			name:       "default branch (no @ref)",
			originArg:  "my-org/my-skills",
			wantSkills: []string{"aws-iam-audit", "terraform-plan-review"},
			add:        []string{"terraform-plan-review", "aws-iam-audit"},
		},
		{
			name:       "non-default branch (@staging)",
			originArg:  "my-org/my-skills@staging",
			wantSkills: []string{"aws-iam-audit", "staging-only", "terraform-plan-review"},
			add:        []string{"staging-only", "terraform-plan-review"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := workflowEnv(t, srv.URL, validToken)

			consumer := t.TempDir()
			runGit(t, consumer, "init", "-q", "-b", "main")

			// init --origin (with/without @branch) — writes the project config.
			if r := runSkillrig(t, []string{"init", "--origin", tc.originArg}, consumer, env); r.exit != 0 {
				t.Fatalf("init --origin %s exit = %d (stderr: %s)", tc.originArg, r.exit, r.stderr)
			}

			// Explicitly confirm init wrote the resolved origin (incl. any @branch)
			// to the project config — the source the search/add below resolve from.
			cfg := readFile(t, filepath.Join(consumer, ".skillrig", "config.toml"))
			if !strings.Contains(cfg, tc.originArg) {
				t.Errorf("init did not write origin %q to .skillrig/config.toml:\n%s", tc.originArg, cfg)
			}

			// search (authenticated) — resolves the just-written origin, fetches the
			// branch's catalog, returns more than one result.
			sr := runSkillrig(t, []string{"search", "--json"}, consumer, env)
			if sr.exit != 0 {
				t.Fatalf("search exit = %d (stderr: %s)", sr.exit, sr.stderr)
			}

			got := searchSkillNames(t, sr.stdout)
			if len(got) < 2 {
				t.Fatalf("search returned %d skills, want > 1: %v", len(got), got)
			}

			if !slices.Equal(got, tc.wantSkills) {
				t.Errorf("search names = %v, want %v (wrong branch's catalog)", got, tc.wantSkills)
			}

			// add the first and second skill — each vendored byte-present + locked.
			for i, skill := range tc.add {
				if r := runSkillrig(t, []string{"add", skill}, consumer, env); r.exit != 0 {
					t.Fatalf("add #%d (%s) exit = %d (stderr: %s)", i+1, skill, r.exit, r.stderr)
				}

				if _, err := os.Stat(filepath.Join(consumer, ".agents", "skills", skill, "SKILL.md")); err != nil {
					t.Errorf("add %s did not vendor SKILL.md: %v", skill, err)
				}
			}

			lock := readFile(t, filepath.Join(consumer, ".skillrig", "skills-lock.json"))
			for _, skill := range tc.add {
				if !strings.Contains(lock, skill) {
					t.Errorf("lock missing added skill %q:\n%s", skill, lock)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Local helpers (self-contained — this package does not share the quickstart
// suite's helpers).
// ---------------------------------------------------------------------------

// sampleOriginDir resolves the committed sample-origin fixture (../testdata/...).
func sampleOriginDir(t *testing.T) string {
	t.Helper()

	abs, err := filepath.Abs(filepath.Join("..", "testdata", "sample-origin"))
	if err != nil {
		t.Fatalf("resolve sample-origin: %v", err)
	}

	if _, err := os.Stat(filepath.Join(abs, ".skillrig-origin.toml")); err != nil {
		t.Fatalf("sample-origin fixture missing at %s: %v", abs, err)
	}

	return abs
}

// runGit runs a real git command with a pinned identity and neutralized ambient
// config (hermetic fixture), failing the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=skillrig", "GIT_AUTHOR_EMAIL=ci@skillrig.dev", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_NAME=skillrig", "GIT_COMMITTER_EMAIL=ci@skillrig.dev", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
		"GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %q: %v\n%s", args, dir, err, out)
	}

	return strings.TrimSpace(string(out))
}

// copyTree recursively copies src into dst, preserving file modes (the exec bit is
// part of git's content identity).
func copyTree(t *testing.T, src, dst string) {
	t.Helper()

	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(target, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copy fixture %s -> %s: %v", src, dst, err)
	}
}

// writeFile writes content to path (parents created), failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// writeExec writes an executable script to path, failing the test on error.
func writeExec(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write exec %s: %v", path, err)
	}
}

// shellSingleQuote single-quotes s for safe embedding in a POSIX sh script.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// readFile reads a file, failing the test on error.
func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(data)
}
