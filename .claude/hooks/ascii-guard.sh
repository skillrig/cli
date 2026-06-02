#!/usr/bin/env bash
# Agent-loop guard, paired with the internal/sourceguard CI test: after a .go
# file is edited, run the non-ASCII source guard and block the edit (exit 2,
# stderr fed back to the agent) when it introduces an unsanctioned non-ASCII
# rune. Wired from .claude/settings.json as a PostToolUse hook on
# Edit|Write|MultiEdit. Dependency-free (no jq); works on macOS and Linux.
set -u

# The PostToolUse payload arrives as JSON on stdin. Trigger only when it
# references a .go file, so edits to other file types are no-ops.
input=$(cat)
case "$input" in
*'.go"'*) ;;
*) exit 0 ;;
esac

cd "${CLAUDE_PROJECT_DIR:-.}" || exit 0

if ! out=$(go test ./internal/sourceguard/ -count=1 2>&1); then
	printf '%s\n' "$out" >&2
	exit 2
fi
