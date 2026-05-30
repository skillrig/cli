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
	// Owner and Repo are the OWNER/REPO halves of the remote origin reference.
	Owner string
	Repo  string
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

// originRef renders the OWNER/REPO[@REF] reference for error reporting. It is the
// human-meaningful origin identity the CLI's what/why/fix prose anchors on.
func (r FetchRequest) originRef() string {
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
	token, authenticated := ResolveGitHubToken(defaultGitHubHost)

	repoURL := cloneURL(req.Owner, req.Repo)

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

// cloneURL builds the HTTPS clone URL for OWNER/REPO. The token is never embedded
// here — git.go injects it via http.extraHeader (research D4) — so the URL is
// safe to surface in diagnostics.
func cloneURL(owner, repo string) string {
	return "https://" + defaultGitHubHost + "/" + owner + "/" + repo + ".git"
}

// classifyFetchError turns a raw fetch failure into the renderable typed error
// the CLI branches on. It runs the shared ClassifyGitError mapping
// (Auth/Unreachable/NotFound), then applies the two fetch-specific refinements
// ClassifyGitError cannot know on its own: a missing-repo failure on a --pin ref
// is really a missing VERSION (NoSuchVersionError, C2), and a genuine NotFound
// gets the origin/skill identity plus the no-token authentication flag (D4). A
// non-*GitError (e.g. an os error from temp-dir creation) is returned unchanged.
func classifyFetchError(req FetchRequest, authenticated bool, err error) error {
	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		return err
	}

	classified := ClassifyGitError(gitErr)

	var notFound *NotFoundError
	if !errors.As(classified, &notFound) {
		return classified
	}

	// A pin that resolves to no ref is a missing version, not a missing skill:
	// the skill exists, the requested tag/SHA does not (C2). Callers/CI branch on
	// the type, never on prose.
	if req.Pinned {
		return &NoSuchVersionError{
			Skill: req.Skill,
			Ref:   req.Ref,
			Cause: gitErr,
		}
	}

	// Enrich the bare NotFound with the origin/skill identity and whether a token
	// was presented. GitHub reports "not found" (not 403) for a private repo
	// reached without a token, so the unauthenticated flag is what lets the CLI
	// add the "if private, authenticate" hint (D4).
	return &NotFoundError{
		Origin:        req.originRef(),
		Skill:         req.Skill,
		Authenticated: authenticated,
		Cause:         gitErr,
	}
}
