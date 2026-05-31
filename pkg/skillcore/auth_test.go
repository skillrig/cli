package skillcore

import (
	"context"
	"encoding/base64"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TestAuthConfigEnv pins the token-injection transport seam (F2, research D4):
// an empty token yields NO env (an unauthenticated fetch), and a non-empty token
// yields exactly the GIT_CONFIG_* triple that injects an http.extraHeader Basic
// credential via the ENVIRON, never argv. The credential lives only in
// GIT_CONFIG_VALUE_0 as base64("x-access-token:<token>") — out of `ps` reach.
func TestAuthConfigEnv(t *testing.T) {
	t.Parallel()

	t.Run("empty token yields no env", func(t *testing.T) {
		t.Parallel()

		if got := authConfigEnv(""); len(got) != 0 {
			t.Errorf("authConfigEnv(\"\") = %v, want no env", got)
		}
	})

	t.Run("non-empty token yields the exact GIT_CONFIG triple", func(t *testing.T) {
		t.Parallel()

		const token = "TKN"

		wantB64 := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		want := []string{
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.extraHeader",
			"GIT_CONFIG_VALUE_0=Authorization: Basic " + wantB64,
		}

		got := authConfigEnv(token)
		if !slices.Equal(got, want) {
			t.Errorf("authConfigEnv(%q) = %v, want %v", token, got, want)
		}
	})
}

// captureCommandContext returns a commandContext that records each git
// invocation's full argv into *capture AND retains the produced *exec.Cmd into
// *cmds, then routes into the helper-process stub (exit 0) so the gitClient runs
// without a real git. Retaining the *exec.Cmd lets the test inspect cmd.Env AFTER
// run/runEnv has set it — the F2 seam for asserting the token lands in the
// ENVIRON, not argv.
func captureCommandContext(
	capture *[][]string,
	cmds *[]*exec.Cmd,
) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	stub := stubCommandContext(0, "")

	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		*capture = append(*capture, slices.Clone(args))

		cmd := stub(ctx, name, args...)
		*cmds = append(*cmds, cmd)

		return cmd
	}
}

// TestClone_TokenInjectionViaEnv pins the security-relevant invariant (F2): a
// non-empty token is injected via git's GIT_CONFIG_* ENV (git >=2.31), so the
// base64 credential is in the process environ — visible to git but NOT in argv
// (where `ps` would expose a `-c http.extraHeader=...` flag). The token must
// NEVER appear in the clone URL or anywhere as a plain argv value either.
func TestClone_TokenInjectionViaEnv(t *testing.T) {
	t.Parallel()

	const (
		token   = "TKN"
		repoURL = "https://github.com/my-org/my-skills"
		destDir = "/tmp/skillrig-dest"
	)

	var (
		captured [][]string
		cmds     []*exec.Cmd
	)

	c := &gitClient{commandContext: captureCommandContext(&captured, &cmds)}

	if err := c.Clone(context.Background(), repoURL, destDir, token); err != nil {
		t.Fatalf("Clone: unexpected error from stubbed git: %v", err)
	}

	if len(captured) != 1 || len(cmds) != 1 {
		t.Fatalf("Clone issued %d/%d git invocations, want exactly 1", len(captured), len(cmds))
	}

	argv := captured[0]

	wantB64 := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))

	// (a) The base64 credential — and the raw token — must appear in NO argv
	// value, and the token must not leak into the clone URL.
	for i, a := range argv {
		if strings.Contains(a, wantB64) {
			t.Errorf("base64 credential leaked into argv[%d] = %q (must live only in the GIT_CONFIG env)", i, a)
		}

		if strings.Contains(a, token) {
			t.Errorf("raw token leaked into argv[%d] = %q (must live only in the GIT_CONFIG env)", i, a)
		}
	}

	// (b) The token rides in the process environ via the GIT_CONFIG_* triple,
	// which run/runEnv set on the captured *exec.Cmd.
	env := cmds[0].Env

	wantValue := "GIT_CONFIG_VALUE_0=Authorization: Basic " + wantB64

	for _, want := range []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.extraHeader", wantValue} {
		if !slices.Contains(env, want) {
			t.Errorf("cmd.Env is missing %q; the token must be injected via the GIT_CONFIG env, got:\n%v", want, env)
		}
	}
}
