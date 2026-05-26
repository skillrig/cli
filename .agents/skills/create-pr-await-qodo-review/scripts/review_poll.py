#!/usr/bin/env python3
"""
review_poll.py — Poll a GitHub PR for the Qodo code review comment.

Qodo posts a placeholder comment immediately after PR creation, then edits it
with the full review once analysis is complete. This script polls until the
final review appears and prints it to stdout.

Usage:
    python scripts/review_poll.py <pr-url> [--interval 15] [--timeout 300]

Accepts:
    https://github.com/owner/repo/pull/123
    owner/repo#123

Exit codes:
    0 — review found (body printed to stdout)
    1 — timeout
    2 — argument/usage error

Standard library only — shells out to `gh api`, no third-party dependencies.
"""

import argparse
import json
import re
import subprocess
import sys
import time

PLACEHOLDER_MARKERS = (
    "Looking for bugs?",
    "Check back in a few minutes",
)

REVIEW_READY_MARKERS = (
    "Bugs",
    "Rule violations",
    "Action required",
    "Review recommended",
    "Requirement gaps",
    "UX Issues",
)

VALID_SLUG = re.compile(r"^[A-Za-z0-9_.-]+$")


def parse_pr(value):
    """Parse a PR reference into (owner, repo, number) or raise ValueError."""
    url_match = re.search(r"github\.com/([^/]+)/([^/]+)/pull/(\d+)", value)
    if url_match:
        owner, repo, number = url_match.groups()
    else:
        short_match = re.match(r"^([^/]+)/([^#]+)#(\d+)$", value)
        if not short_match:
            raise ValueError(f"could not parse PR reference: {value}")
        owner, repo, number = short_match.groups()

    if not VALID_SLUG.match(owner) or not VALID_SLUG.match(repo):
        raise ValueError(f"invalid owner/repo characters: {owner}/{repo}")
    if not number.isdigit():
        raise ValueError(f"invalid PR number: {number}")

    return owner, repo, number


def positive_int(value):
    ivalue = int(value)
    if ivalue < 1:
        raise argparse.ArgumentTypeError(f"must be a positive integer (got: {value})")
    return ivalue


def parse_args(argv):
    parser = argparse.ArgumentParser(
        prog="review_poll.py",
        description="Poll a GitHub PR for the Qodo code review comment.",
    )
    parser.add_argument("pr", metavar="pr-url", help="GitHub PR URL or owner/repo#number")
    parser.add_argument(
        "--interval", type=positive_int, default=15, help="seconds between polls (default: 15)"
    )
    parser.add_argument(
        "--timeout", type=positive_int, default=300, help="max seconds to wait (default: 300)"
    )
    args = parser.parse_args(argv)

    try:
        owner, repo, number = parse_pr(args.pr)
    except ValueError as err:
        parser.error(str(err))

    return owner, repo, number, args.interval, args.timeout


def fetch_comments(owner, repo, number):
    """Return a list of {id, author, body} dicts for the PR's issue comments."""
    result = subprocess.run(
        [
            "gh",
            "api",
            f"repos/{owner}/{repo}/issues/{number}/comments",
            "--paginate",
            "--jq",
            ".[] | {id, author: .user.login, body}",
        ],
        capture_output=True,
        text=True,
        check=True,
    )
    # --paginate with --jq outputs one JSON object per line (NDJSON).
    return [json.loads(line) for line in result.stdout.splitlines() if line.strip()]


def is_placeholder(body):
    return any(marker in body for marker in PLACEHOLDER_MARKERS)


def is_review_ready(body):
    return not is_placeholder(body) and any(marker in body for marker in REVIEW_READY_MARKERS)


def find_qodo_review_comment(comments):
    """Pick the Qodo "Code Review" comment, falling back to placeholder/last."""
    qodo = [c for c in comments if c.get("author") and "qodo" in c["author"].lower()]

    code_review = next((c for c in qodo if "Code Review" in c["body"]), None)
    if code_review:
        return code_review

    placeholder = next((c for c in qodo if is_placeholder(c["body"])), None)
    if placeholder:
        return placeholder

    return qodo[-1] if qodo else None


def main():
    owner, repo, number, interval, timeout = parse_args(sys.argv[1:])
    pr_ref = f"{owner}/{repo}#{number}"

    print(f"Polling {pr_ref} every {interval}s (timeout: {timeout}s)...", file=sys.stderr)

    start = time.monotonic()
    while time.monotonic() - start < timeout:
        elapsed = round(time.monotonic() - start)
        try:
            comments = fetch_comments(owner, repo, number)
            qodo = find_qodo_review_comment(comments)

            if qodo:
                if is_review_ready(qodo["body"]):
                    print(f"Review ready for {pr_ref}", file=sys.stderr)
                    print(qodo["body"])
                    return 0
                print(
                    f"Found Qodo comment but still placeholder... ({elapsed}s elapsed)",
                    file=sys.stderr,
                )
            else:
                print(f"No Qodo comment yet... ({elapsed}s elapsed)", file=sys.stderr)
        except subprocess.CalledProcessError as err:
            print(f"API error: {err.stderr.strip()}, retrying...", file=sys.stderr)

        time.sleep(interval)

    # Emit the timeout on stdout too, not just stderr: when run under the
    # Monitor tool only stdout lines become notifications, and a silent exit
    # would be indistinguishable from "still polling".
    print(f"TIMEOUT: no Qodo review for {pr_ref} after {timeout}s")
    print(f"Timeout: no Qodo review found after {timeout}s", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
