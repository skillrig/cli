package skillcore

import (
	"errors"
	"testing"
)

// TestResolvePin pins the deterministic --pin resolution rule (C3, data-model
// §3): a bare semver (optionally v-prefixed) expands via the name-vSEMVER tag
// scheme; a full tag or commit SHA is taken literally; an empty pin falls back
// to the origin branch ref and records no explicit version. resolvePin is the one
// implementation add dispatches to, so the rule is asserted directly on it rather
// than only end-to-end.
func TestResolvePin(t *testing.T) {
	t.Parallel()

	const (
		skill    = "terraform-plan-review"
		fallback = "main"
		// sha is a 40-hex commit id — neither a bare semver nor a name-vX tag.
		sha = "0123456789abcdef0123456789abcdef01234567"
	)

	tests := []struct {
		name        string
		pin         string
		wantRef     string
		wantVersion string
		wantPinned  bool
	}{
		{
			name:        "empty pin falls back to the origin branch ref, unpinned",
			pin:         "",
			wantRef:     fallback,
			wantVersion: "",
			wantPinned:  false,
		},
		{
			name:        "bare v-prefixed semver expands to the name-vSEMVER tag",
			pin:         "v1.4.0",
			wantRef:     skill + "-v1.4.0",
			wantVersion: skill + "-v1.4.0",
			wantPinned:  true,
		},
		{
			name:        "bare semver without v expands to the same name-vSEMVER tag",
			pin:         "1.4.0",
			wantRef:     skill + "-v1.4.0",
			wantVersion: skill + "-v1.4.0",
			wantPinned:  true,
		},
		{
			name:        "full tag is taken literally (no re-expansion)",
			pin:         skill + "-v1.4.0",
			wantRef:     skill + "-v1.4.0",
			wantVersion: skill + "-v1.4.0",
			wantPinned:  true,
		},
		{
			name:        "commit SHA is a literal ref",
			pin:         sha,
			wantRef:     sha,
			wantVersion: sha,
			wantPinned:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ref, version, pinned := resolvePin(skill, tt.pin, fallback)

			if ref != tt.wantRef {
				t.Errorf("resolvePin(%q) ref = %q, want %q", tt.pin, ref, tt.wantRef)
			}

			if version != tt.wantVersion {
				t.Errorf("resolvePin(%q) version = %q, want %q", tt.pin, version, tt.wantVersion)
			}

			if pinned != tt.wantPinned {
				t.Errorf("resolvePin(%q) pinned = %v, want %v", tt.pin, pinned, tt.wantPinned)
			}
		})
	}
}

// TestResolvePin_BareSemverEqualsFullTag is the SC-004 invariant at the resolver
// level: `--pin v1.4.0` (bare-semver expansion) and `--pin <skill>-v1.4.0` (the
// full-tag literal) MUST resolve to the SAME git ref, so they fetch the same
// commit and thus the same treeSha. The end-to-end equivalence is also asserted
// by TestQuickstart_AddPinTagFormEquivalent over a real file:// origin; this pins
// the rule that makes that possible.
func TestResolvePin_BareSemverEqualsFullTag(t *testing.T) {
	t.Parallel()

	const skill = "terraform-plan-review"

	bareRef, _, barePinned := resolvePin(skill, "v1.4.0", "main")
	tagRef, _, tagPinned := resolvePin(skill, skill+"-v1.4.0", "main")

	if bareRef != tagRef {
		t.Errorf("bare-semver ref %q != full-tag ref %q (SC-004: both forms must resolve to one tag)", bareRef, tagRef)
	}

	if !barePinned || !tagPinned {
		t.Errorf("both forms must be pinned, got bare=%v tag=%v", barePinned, tagPinned)
	}
}

// classifyClass names the typed fetch-error class classifyFetchError can produce,
// so the table asserts the matched class declaratively.
type classifyClass int

const (
	classNoVersion classifyClass = iota
	classFetchAuth
	classFetchUnreachable
	classFetchNotFound
	classFetchRaw
)

func (c classifyClass) String() string {
	switch c {
	case classNoVersion:
		return "NoSuchVersionError"
	case classFetchAuth:
		return "AuthError"
	case classFetchUnreachable:
		return "UnreachableError"
	case classFetchNotFound:
		return "NotFoundError"
	case classFetchRaw:
		return "raw *GitError"
	default:
		return "unknown"
	}
}

// classifyClassOf reports which class classifyFetchError mapped err into.
func classifyClassOf(err error) classifyClass {
	var (
		nv *NoSuchVersionError
		a  *AuthError
		u  *UnreachableError
		nf *NotFoundError
	)

	switch {
	case errors.As(err, &nv):
		return classNoVersion
	case errors.As(err, &a):
		return classFetchAuth
	case errors.As(err, &u):
		return classFetchUnreachable
	case errors.As(err, &nf):
		return classFetchNotFound
	default:
		return classFetchRaw
	}
}

// TestClassifyFetchError pins the repo-vs-skill-vs-version distinction (FIX-3 /
// C2/C3): a missing/private/unreachable REPO is a CLONE-phase failure and must
// NEVER become NoSuchVersionError, even with --pin; only a CHECKOUT-phase failure
// of a pinned ref is a missing VERSION (the repo and skill cloned fine, the
// tag/SHA does not exist). It also pins FIX-7 — Auth/Unreachable/NotFound carry
// the configured origin identity that ClassifyGitError leaves blank.
func TestClassifyFetchError(t *testing.T) {
	t.Parallel()

	const (
		owner = "my-org"
		repo  = "my-skills"
		skill = "terraform-plan-review"
		ref   = skill + "-v9.9.9"
	)

	// step wraps a *GitError in a fetchStepError for the named phase, mirroring
	// what fetchSparseInto produces. A nil step means "no step tag" (e.g. a
	// post-fetch rev-parse failure) — classifyFetchError must treat that as a
	// clone-class repo failure, never a missing version.
	cloneErr := func(stderr string) error {
		return &fetchStepError{step: stepClone, err: &GitError{ExitCode: 128, Stderr: stderr}}
	}
	checkoutErr := func(stderr string) error {
		return &fetchStepError{step: stepCheckout, err: &GitError{ExitCode: 128, Stderr: stderr}}
	}

	const (
		notFoundStderr = "fatal: repository 'https://github.com/my-org/my-skills/' not found"
		authStderr     = "remote: Authentication failed for 'https://github.com/my-org/my-skills/'"
		unreachStderr  = "fatal: unable to access '...': Could not resolve host: github.com"
		refStderr      = "error: pathspec 'terraform-plan-review-v9.9.9' did not match any file(s) known to git"
	)

	tests := []struct {
		name   string
		req    FetchRequest
		authed bool
		err    error
		want   classifyClass
	}{
		{
			name: "pinned CHECKOUT failure is a missing version (the repo cloned, the tag does not exist)",
			req:  FetchRequest{Owner: owner, Repo: repo, Skill: skill, Ref: ref, Pinned: true},
			err:  checkoutErr(refStderr),
			want: classNoVersion,
		},
		{
			name: "pinned CLONE not-found is a missing/private REPO, NOT a missing version (FIX-3)",
			req:  FetchRequest{Owner: owner, Repo: repo, Skill: skill, Ref: ref, Pinned: true},
			err:  cloneErr(notFoundStderr),
			want: classFetchNotFound,
		},
		{
			name: "unpinned checkout failure is not a version error (no pin to promote)",
			req:  FetchRequest{Owner: owner, Repo: repo, Skill: skill, Ref: "main"},
			err:  checkoutErr(refStderr),
			want: classFetchRaw,
		},
		{
			name: "clone auth failure classifies as AuthError",
			req:  FetchRequest{Owner: owner, Repo: repo, Skill: skill},
			err:  cloneErr(authStderr),
			want: classFetchAuth,
		},
		{
			name: "clone unreachable classifies as UnreachableError",
			req:  FetchRequest{Owner: owner, Repo: repo, Skill: skill},
			err:  cloneErr(unreachStderr),
			want: classFetchUnreachable,
		},
		{
			name: "pinned clone auth failure stays AuthError (a pin never overrides a clone-phase class)",
			req:  FetchRequest{Owner: owner, Repo: repo, Skill: skill, Ref: ref, Pinned: true},
			err:  cloneErr(authStderr),
			want: classFetchAuth,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := classifyFetchError(tt.req, tt.authed, tt.err)

			if got := classifyClassOf(out); got != tt.want {
				t.Fatalf("classifyFetchError class = %v, want %v (err: %v)", got, tt.want, out)
			}

			// Every classified error must still unwrap to the raw *GitError so
			// --verbose can reach the original stderr (errors-as-navigation).
			var ge *GitError
			if !errors.As(out, &ge) {
				t.Fatalf("classified error %v does not unwrap to *GitError", out)
			}
		})
	}
}

// TestClassifyFetchError_PopulatesOrigin pins FIX-7: ClassifyGitError builds
// Auth/Unreachable/NotFound with a blank Origin; classifyFetchError must rebuild
// each with the configured origin identity so the CLI's what/why/fix names the
// real origin. NotFound additionally carries the skill name and the no-token flag
// for the "if private, authenticate" hint (D4).
func TestClassifyFetchError_PopulatesOrigin(t *testing.T) {
	t.Parallel()

	const (
		owner    = "my-org"
		repo     = "my-skills"
		skill    = "terraform-plan-review"
		wantOrig = "my-org/my-skills"
	)

	base := FetchRequest{Owner: owner, Repo: repo, Skill: skill}

	clone := func(stderr string) error {
		return &fetchStepError{step: stepClone, err: &GitError{ExitCode: 128, Stderr: stderr}}
	}

	t.Run("AuthError carries the origin", func(t *testing.T) {
		t.Parallel()

		out := classifyFetchError(base, true, clone("remote: Authentication failed for 'x'"))

		var ae *AuthError
		if !errors.As(out, &ae) {
			t.Fatalf("error = %T, want *AuthError", out)
		}

		if ae.Origin != wantOrig {
			t.Errorf("AuthError.Origin = %q, want %q (FIX-7)", ae.Origin, wantOrig)
		}
	})

	t.Run("UnreachableError carries the origin", func(t *testing.T) {
		t.Parallel()

		out := classifyFetchError(base, false, clone("fatal: Could not resolve host: github.com"))

		var ue *UnreachableError
		if !errors.As(out, &ue) {
			t.Fatalf("error = %T, want *UnreachableError", out)
		}

		if ue.Origin != wantOrig {
			t.Errorf("UnreachableError.Origin = %q, want %q (FIX-7)", ue.Origin, wantOrig)
		}
	})

	t.Run("NotFound without a token records the no-token flag for the auth hint", func(t *testing.T) {
		t.Parallel()

		// authenticated=false → the CLI adds the "if private, authenticate" hint.
		out := classifyFetchError(base, false, clone("fatal: repository 'x' not found"))

		var nf *NotFoundError
		if !errors.As(out, &nf) {
			t.Fatalf("error = %T, want *NotFoundError", out)
		}

		if nf.Origin != wantOrig {
			t.Errorf("NotFoundError.Origin = %q, want %q", nf.Origin, wantOrig)
		}

		if nf.Skill != skill {
			t.Errorf("NotFoundError.Skill = %q, want %q", nf.Skill, skill)
		}

		if nf.Authenticated {
			t.Error("NotFoundError.Authenticated = true, want false (no token resolved → auth hint)")
		}
	})
}
