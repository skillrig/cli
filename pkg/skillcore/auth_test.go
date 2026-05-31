package skillcore

import (
	"context"
	"encoding/base64"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TestAuthConfigArgs pins the token-injection transport seam (F2, research D4):
// an empty token yields NO args (an unauthenticated fetch), and a non-empty token
// yields exactly the global `-c http.extraHeader=Authorization: Basic <b64>` pair,
// where <b64> is base64("x-access-token:<token>"). The credential lives only in a
// git config value, never in a URL or argv positional.
func TestAuthConfigArgs(t *testing.T) {
	t.Parallel()

	t.Run("empty token yields no args", func(t *testing.T) {
		t.Parallel()

		if got := authConfigArgs(""); len(got) != 0 {
			t.Errorf("authConfigArgs(\"\") = %v, want no args", got)
		}
	})

	t.Run("non-empty token yields the exact extraHeader pair", func(t *testing.T) {
		t.Parallel()

		const token = "TKN"

		wantB64 := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		want := []string{"-c", "http.extraHeader=Authorization: Basic " + wantB64}

		got := authConfigArgs(token)
		if !slices.Equal(got, want) {
			t.Errorf("authConfigArgs(%q) = %v, want %v", token, got, want)
		}
	})
}

// captureCommandContext returns a commandContext that records the full argv of
// each git invocation into *capture and then routes into the helper-process stub
// (exit 0), so the gitClient runs without a real git. It is the F2 seam for
// asserting WHERE the token-injection flags land in the argv.
func captureCommandContext(capture *[][]string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	stub := stubCommandContext(0, "")

	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		*capture = append(*capture, slices.Clone(args))

		return stub(ctx, name, args...)
	}
}

// TestClone_TokenInjectionArgvOrder pins the security-relevant invariant (F2): a
// non-empty token must inject the `-c http.extraHeader=...` pair as GLOBAL flags
// BEFORE the `clone` subcommand (git only honors -c before the subcommand), and
// the token must NEVER appear in the clone URL or anywhere as a plain argv value.
func TestClone_TokenInjectionArgvOrder(t *testing.T) {
	t.Parallel()

	const (
		token   = "TKN"
		repoURL = "https://github.com/my-org/my-skills"
		destDir = "/tmp/skillrig-dest"
	)

	var captured [][]string

	c := &gitClient{commandContext: captureCommandContext(&captured)}

	if err := c.Clone(context.Background(), repoURL, destDir, token); err != nil {
		t.Fatalf("Clone: unexpected error from stubbed git: %v", err)
	}

	if len(captured) != 1 {
		t.Fatalf("Clone issued %d git invocations, want exactly 1", len(captured))
	}

	argv := captured[0]

	cIdx := slices.Index(argv, "-c")
	cloneIdx := slices.Index(argv, "clone")

	if cIdx < 0 {
		t.Fatalf("argv %v carries no -c global flag for the token header", argv)
	}

	if cloneIdx < 0 {
		t.Fatalf("argv %v carries no clone subcommand", argv)
	}

	if cIdx >= cloneIdx {
		t.Errorf("-c (at %d) must precede the clone subcommand (at %d); a global flag after the subcommand is ignored: %v", cIdx, cloneIdx, argv)
	}

	// The header value immediately follows -c and carries the base64 credential.
	wantHeader := "http.extraHeader=Authorization: Basic " +
		base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token))
	if argv[cIdx+1] != wantHeader {
		t.Errorf("argv[%d] = %q, want the token header %q", cIdx+1, argv[cIdx+1], wantHeader)
	}

	// The raw token must never leak into the URL or any other plain argv value;
	// it lives only inside the (base64-encoded) config header.
	for i, a := range argv {
		if i == cIdx+1 {
			continue // the header carries the base64 form, not the raw token
		}

		if strings.Contains(a, token) {
			t.Errorf("raw token leaked into argv[%d] = %q (must live only in the -c header)", i, a)
		}
	}
}
