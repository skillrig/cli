---
name: create-pr-await-qodo-review
description: Orchestrate the full PR creation workflow — commit, branch, push, create the PR via gh CLI, then optionally wait for the Qodo bot's automated code review and surface its findings. Use this skill whenever the user asks to "create a PR", "open a PR", "submit a PR", "commit and push and create a PR", or any variation of making a pull request. Also trigger when the user says "let's PR this" or "ship it" in a context where there are uncommitted or unpushed changes, or when they want to wait for / poll / check the Qodo review on a PR.
---

# Create PR and Await Qodo Review

This skill walks through the complete pull request workflow for this repository, from staging changes through to waiting for the automated Qodo code review and surfacing its findings.

The reason this skill exists is that PR creation involves several coordinated steps where getting the details right matters — conventional commit format, structured PR bodies, and the opportunity to catch review feedback before context-switching away. Following this workflow means the PR is ready for human review the moment it lands.

## Workflow

### Step 1: Pre-flight

Run these in parallel to understand what you're working with:

```bash
git status                  # untracked + modified files
git diff                    # unstaged changes
git diff --cached           # staged changes
git log --oneline -5        # recent commit style
```

Before proceeding, verify there are actual changes to commit. If the working tree is clean, tell the user and stop.

### Step 2: Commit

Stage the relevant files — prefer naming specific files over `git add -A` to avoid accidentally including sensitive files (`.env`, credentials, local settings).

Write a conventional commit message following the project's release flow (see `docs/guides/release-flow.md`). The format is `type(scope): description` or `type: description`.

**Version-bumping types:** `feat` (minor), `fix` (patch), `perf` (patch), `deps` (patch), `revert` (patch)
**Non-bumping types:** `chore`, `docs`, `style`, `refactor`, `test`, `build`, `ci`

End every commit message with:
```
Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
```

Use a HEREDOC for the commit message to preserve formatting:
```bash
git commit -m "$(cat <<'EOF'
type(scope): short description

Longer explanation of why, not what.

Co-Authored-By: Claude Opus 4.6 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Step 3: Branch and push

If currently on `main`, create a feature branch first — never commit directly to main. Branch naming convention: `type/short-description` (e.g., `fix/yaml-parsing`, `ci/harden-workflows`).

```bash
git checkout -b <branch-name>   # only if on main
git push -u origin <branch-name>
```

### Step 4: Create the PR

Use `gh pr create` with a structured body. PR titles follow the same conventional commit format (they become the squash-merge commit message on main).

```bash
gh pr create --title "type(scope): description" --body "$(cat <<'EOF'
## Summary
<1-3 bullet points explaining what and why>

## Test plan
[Describe how changes were verified]

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Capture the returned PR URL — `gh pr create` prints it to stdout.

### Step 5: Wait for the Qodo review (always)

Every PR in this repo gets an automated Qodo code review, and the whole point of this skill is to catch that feedback before you context-switch away — so **always** wait for it. Don't ask the user whether to wait; just tell them you're watching for it and that they're free to keep working in the meantime.

> "PR created at {url}. I'll watch for the Qodo code review and surface its findings as soon as it lands."

Drive the wait with the **Monitor** tool, not a foreground command. The poller streams progress to stderr (which goes only to the monitor's output file) and prints exactly one terminal event to stdout — the full review body when it lands, or a `TIMEOUT:` line if it never does — so each notification is something you'd actually act on, with no log noise in between.

Call the `Monitor` tool with:

- **command**: `python scripts/review_poll.py <pr-url> --interval 30 --timeout 900`
- **description**: something specific like `"Qodo review on PR #123"` (it appears in every notification)
- **persistent**: `true` — the poller ends itself when the review lands or it hits its own `--timeout`, so let it run for the session rather than racing a separate monitor deadline

Then keep working. When the monitor fires, react to what it emitted:

- **Review body** (markdown) → parse out findings. Look for sections like `Bugs`, `Rule violations`, `Action required`, or `Review recommended`. Summarize them for the user, highlighting bugs and actionable items, and offer to fix anything real.
- **`TIMEOUT: ...` line** (or a non-zero exit) → the review didn't post in time. Tell the user, share the PR URL, and offer to re-arm the monitor with a longer `--timeout` or check the PR manually.

A 30s poll interval keeps you clear of GitHub API rate limits, and 900s (15 min) comfortably covers how long Qodo typically takes. Bump `--timeout` for unusually large PRs.

## Script reference

**`scripts/review_poll.py`** — Polls a GitHub PR for the Qodo bot's code review comment.

```
python scripts/review_poll.py <pr-url> [--interval 15] [--timeout 300]
```

- Accepts `https://github.com/owner/repo/pull/123` or `owner/repo#123`
- Shells out to `gh api` (Python standard library only, no third-party dependencies)
- Detects the Qodo bot comment by author login (contains "qodo")
- Distinguishes placeholder ("Looking for bugs? Check back in a few minutes") from the final review (contains `Bugs`, `Rule violations`, `Action required`, etc.)
- Exit 0 = review found (body on stdout), exit 1 = timeout (a `TIMEOUT:` line on stdout + message on stderr), exit 2 = bad args
- Designed for the Monitor tool: only terminal events (the review body, or the `TIMEOUT:` line) go to stdout; per-poll progress goes to stderr so it never spams notifications

## Decision patterns

### When to create a new branch

- On `main` → always create a branch
- On an existing feature branch → stay on it (the user likely has in-progress work)
- Branch already tracks remote and is up to date → just push new commits

### Commit granularity

- One logical change = one commit. Don't bundle unrelated fixes.
- If the user explicitly asks for a single commit covering multiple changes, that's fine.
- If a pre-commit hook fails, fix the issue and create a NEW commit (don't amend — the failed commit didn't happen).

### PR body depth

- Small changes (typos, config tweaks) → brief summary, minimal test plan
- Feature work → explain the why, list affected areas, thorough test plan
- Security/CI changes → explain the threat model or reasoning, link to relevant incidents or docs
