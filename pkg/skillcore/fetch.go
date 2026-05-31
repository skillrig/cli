package skillcore

import (
	"context"
	"errors"
)

// defaultGitHubHost is the host skillrig fetches origins from. It is the seam
// for GitHub Enterprise (research D4): ResolveGitHubToken and the clone URL both
// thread a hostname today fixed to github.com.
const defaultGitHubHost = "github.com"

// FetchRequest names a single skill subtree to fetch from a remote origin. The
// caller (the CLI add path) supplies the already-classified remote coordinates;
// skillcore neither resolves the origin nor reads config — it owns only the git
// transport and the failure classification (AP-06).
type FetchRequest struct {
	// Owner and Repo are the OWNER/REPO halves of a REMOTE origin reference. They
	// are used for error reporting (originRef) and, when RepoURL is empty, to
	// derive the GitHub HTTPS clone URL. They are empty for a LOCAL origin.
	Owner string
	Repo  string
	// RepoURL is the git transport target to clone from when set — the LOCAL
	// origin's file://<path> (FIX-1, the file:// test substrate AND FR-011) or any
	// caller-supplied URL. Empty means "derive https://github.com/Owner/Repo.git".
	// The seam that lets a local-path/file:// origin be fetched without hardcoding
	// github.com (config.Origin.CloneURL produces this value).
	RepoURL string
	// Local marks RepoURL as a file:// (local) target. A local origin needs no
	// GitHub token and its failures are never the remote auth/private-not-found
	// classes, so the token is not resolved for it.
	Local bool
	// Skill is the skill directory name, used in error reporting so the CLI can
	// distinguish a missing skill from a missing origin (errors-as-navigation).
	Skill string
	// SkillPath is the repo-relative path of the skill subtree to sparse-check-out
	// (the catalog's path, or the conventional skills/<skill>).
	SkillPath string
	// Ref is the git ref to check out: the resolved --pin tag/SHA when pinning,
	// else the origin's @ref branch (D7).
	Ref string
	// Pinned marks Ref as a user-supplied --pin. A failed checkout of a pin means
	// the requested version does not exist (NoSuchVersionError, C2), distinct from
	// a branch ref that points at a real, fetchable tip.
	Pinned bool
}

// FetchResult is the outcome of a successful FetchSkill: the local temp dir the
// subtree was checked out into (the caller's to remove) and the resolved upstream
// commit (provenance for the lock entry, D5).
type FetchResult struct {
	Dir    string
	Commit string
}

// originRef renders the origin identity for error reporting — the
// human-meaningful anchor the CLI's what/why/fix prose uses. For a remote origin
// it is OWNER/REPO[@REF]; for a LOCAL origin (no Owner/Repo) it is the RepoURL,
// since that is the only identity the user configured.
func (r FetchRequest) originRef() string {
	if r.Owner == "" && r.Repo == "" {
		return r.RepoURL
	}

	ref := r.Owner + "/" + r.Repo
	if r.Ref != "" {
		ref += "@" + r.Ref
	}

	return ref
}

// FetchSkill fetches one skill subtree from the remote OWNER/REPO origin at
// req.Ref and returns the local temp path plus the resolved upstream commit. It
// resolves a GitHub token via ResolveGitHubToken (skipped silently when none is
// available — a public origin needs none), sparse-checks-out req.SkillPath via
// the git.go FetchSparse helper (one git transport, research D7), then resolves
// HEAD to the exact commit for the lock's provenance field.
//
// All failures are classified, never surfaced raw:
//   - a network/auth/missing-repo git failure → ClassifyGitError's AuthError /
//     UnreachableError / NotFoundError;
//   - a NotFoundError reached with NO resolved token is marked unauthenticated so
//     the CLI can add the "if private, authenticate" hint (research D4);
//   - a failed checkout of a --pin ref → NoSuchVersionError (C2), distinct from a
//     missing skill or origin.
//
// On any failure the temp dir FetchSparse created is already cleaned up by that
// helper, so FetchSkill returns no path to remove.
func FetchSkill(ctx context.Context, req FetchRequest) (FetchResult, error) {
	repoURL := req.cloneURL()

	// A local (file://) origin needs no credential and never produces the remote
	// auth/private-not-found classes, so the token is resolved only for GitHub.
	var (
		token         string
		authenticated bool
	)

	if !req.Local {
		token, authenticated = ResolveGitHubToken(defaultGitHubHost)
	}

	dir, err := FetchSparse(ctx, repoURL, req.SkillPath, req.Ref, token)
	if err != nil {
		return FetchResult{}, classifyFetchError(req, authenticated, err)
	}

	commit, err := revParse(dir, "HEAD")
	if err != nil {
		return FetchResult{}, classifyFetchError(req, authenticated, err)
	}

	return FetchResult{Dir: dir, Commit: commit}, nil
}

// cloneURL derives the git transport target for the request: an explicit
// RepoURL (the LOCAL origin's file://<path>, or any caller-supplied URL) when
// set, else the GitHub HTTPS URL for OWNER/REPO (FIX-1 — the seam that stops
// hardcoding github.com so a file:// origin is fetchable). The token is never
// embedded here — git.go injects it via the GIT_CONFIG http.extraHeader env, kept
// out of argv (research D4) — so the URL is safe to surface in diagnostics.
func (r FetchRequest) cloneURL() string {
	if r.RepoURL != "" {
		return r.RepoURL
	}

	return "https://" + defaultGitHubHost + "/" + r.Owner + "/" + r.Repo + ".git"
}

// classifyFetchError turns a raw fetch failure into the renderable typed error
// the CLI branches on. It runs the shared ClassifyGitError mapping
// (Auth/Unreachable/NotFound), then applies the fetch-specific refinements
// ClassifyGitError cannot know on its own, all anchored on WHICH git phase failed
// (FIX-3 — the *fetchStepError tag):
//
//   - A failed CHECKOUT of a --pin ref is a missing VERSION (NoSuchVersionError,
//     C2): the repo and skill subtree were cloned fine, only the requested
//     tag/SHA does not exist. This is the ONLY path that yields NoSuchVersion — a
//     missing/private/unreachable REPO (a clone-phase failure) never becomes
//     "no such version" even with --pin (FIX-3 fixes that mis-classification).
//   - A genuine clone-phase NotFound gets the origin/skill identity plus the
//     no-token authentication flag (D4).
//   - Auth/Unreachable get the origin identity populated (FIX-7), which
//     ClassifyGitError leaves blank.
//
// A non-*GitError (e.g. an os error from temp-dir creation) is returned
// unchanged.
func classifyFetchError(req FetchRequest, authenticated bool, err error) error {
	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		return err
	}

	// The phase that failed: a checkout-step failure is about ref/version
	// existence; everything else (clone, sparse-cone, object fetch) is about the
	// repo. Absent a step tag (e.g. a rev-parse after the fetch), treat it as a
	// clone-class repo failure — never a missing version.
	var stepErr *fetchStepError

	checkoutFailed := errors.As(err, &stepErr) && stepErr.step == stepCheckout

	classified := ClassifyGitError(gitErr)

	// A pin whose CHECKOUT failed is a missing version, not a missing skill: the
	// skill exists, the requested tag/SHA does not (C2). Gated on the checkout
	// phase so a missing/private repo with --pin stays a NotFound (FIX-3).
	if req.Pinned && checkoutFailed {
		return &NoSuchVersionError{
			Skill: req.Skill,
			Ref:   req.Ref,
			Cause: gitErr,
		}
	}

	// FIX-7: ClassifyGitError builds Auth/Unreachable with a blank Origin; rebuild
	// each with the configured origin identity. NotFound additionally carries the
	// skill name and the no-token flag for the "if private, authenticate" hint
	// (D4): GitHub reports "not found" (not 403) for a private repo reached
	// without a token.
	var (
		authErr  *AuthError
		unreach  *UnreachableError
		notFound *NotFoundError
	)

	switch {
	case errors.As(classified, &authErr):
		return &AuthError{Origin: req.originRef(), Cause: gitErr}
	case errors.As(classified, &unreach):
		return &UnreachableError{Origin: req.originRef(), Cause: gitErr}
	case errors.As(classified, &notFound):
		return &NotFoundError{
			Origin:        req.originRef(),
			Skill:         req.Skill,
			Authenticated: authenticated,
			Cause:         gitErr,
		}
	default:
		return classified
	}
}
