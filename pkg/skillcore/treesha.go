// Package skillcore is the single, presentation-free implementation of
// skillrig's integrity primitives: git tree-SHA, skill.toml parsing,
// skills-lock.json I/O, and the Add/Verify operations. It returns typed
// values and typed errors and never writes to stdout/stderr or formats
// user-facing text — the CLI (or any third-party consumer) owns presentation.
// It never fetches: it operates purely on the local filesystem and local git.
package skillcore

import "strings"

// TreeSHA returns the git tree-object SHA of relPath at ref within the git
// repository rooted at gitDir, by shelling git rev-parse <ref>:<relPath>.
// The value is git-canonical and relocation-invariant: it depends only on the
// subtree contents, so the SHA Add records on the origin equals the SHA Verify
// recomputes on the consumer's committed tree. relPath is repo-relative
// ("skills/foo" on the origin, ".agents/skills/foo" on the consumer) and must
// resolve to a directory (a skill subtree). Returns a *GitError when git fails.
func TreeSHA(gitDir, ref, relPath string) (string, error) {
	sha, err := revParse(gitDir, ref+":"+relPath)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(sha), nil
}
