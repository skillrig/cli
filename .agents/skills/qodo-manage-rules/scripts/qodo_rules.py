#!/usr/bin/env python3
"""Manage Qodo coding rules (list / search / get / modify / deactivate).

Read operations are unrestricted. WRITE operations (set-state, update) default to a
DRY-RUN that prints the before/after — pass --apply to actually send the PUT. This
matters because Qodo rules are org-wide: a careless edit changes what every teammate's
PR gets graded against. Deactivating (state=inactive) is the reversible alternative to
deleting; this tool deliberately offers no hard DELETE.

Auth: bearer token from $QODO_API_KEY, else ~/.qodo/auth.key (a raw `sk-...` token).
NOTE: the token is NOT in ~/.qodo/config.json — that file is only UI prefs.

The Qodo API is a FULL-DOCUMENT PUT: to change one field you must fetch the whole rule,
mutate it, and send the whole thing back. This script does that read-modify-write for
you so callers never hand-build a partial body (which would blank out other fields).
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import uuid
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

# Fields the API accepts on a PUT. Everything else (ruleId, createdAt, updatedAt,
# source, sourceType, sourceUris, insights, similaritiesCount, url, suggestionType)
# is server-managed and must be dropped from the body.
MUTABLE_FIELDS = (
    "name", "category", "severity", "content",
    "goodExamples", "badExamples", "sourceUri", "scopes", "state",
)


def resolve_token() -> str:
    env = os.environ.get("QODO_API_KEY", "").strip()
    if env:
        return env
    path = os.path.expanduser("~/.qodo/auth.key")
    try:
        with open(path) as fh:
            tok = fh.read().strip()
    except OSError:
        sys.exit(
            "ERROR: no Qodo token. Set $QODO_API_KEY or create ~/.qodo/auth.key "
            "(a raw `sk-...` token). The token is NOT read from config.json."
        )
    if not tok:
        sys.exit(f"ERROR: {path} is empty.")
    return tok


def resolve_base() -> str:
    """{QODO_API_URL}/rules/v1, else ENVIRONMENT_NAME-based, else production."""
    api_url = os.environ.get("QODO_API_URL", "").strip()
    cfg = os.path.expanduser("~/.qodo/config.json")
    if not api_url and os.path.exists(cfg):
        try:
            with open(cfg) as fh:
                api_url = (json.load(fh).get("QODO_API_URL") or "").strip()
        except (OSError, json.JSONDecodeError):
            api_url = ""
    if api_url:
        return f"{api_url.rstrip('/')}/rules/v1"
    env = os.environ.get("QODO_ENVIRONMENT_NAME", "").strip()
    if env:
        return f"https://qodo-platform.{env}.qodo.ai/rules/v1"
    return "https://qodo-platform.qodo.ai/rules/v1"


def _request(method: str, path: str, body: dict | None = None) -> dict:
    base = resolve_base()
    url = f"{base}{path}"
    headers = {
        "Authorization": f"Bearer {resolve_token()}",
        "Accept": "application/json",
        "request-id": str(uuid.uuid4()),
        "qodo-client-type": "skill-qodo-manage-rules",
    }
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    req = Request(url, data=data, headers=headers, method=method)
    try:
        with urlopen(req, timeout=30) as resp:
            raw = resp.read()
    except HTTPError as e:
        detail = e.read().decode(errors="replace")[:500]
        sys.exit(f"ERROR: {method} {path} -> HTTP {e.code}\n{detail}")
    except URLError as e:
        sys.exit(f"ERROR: network failure reaching Qodo ({e.reason}).")
    return json.loads(raw) if raw else {}


# --- read ops ---------------------------------------------------------------

def cmd_list(args) -> None:
    rules, page, total = [], 1, None
    while True:
        d = _request("GET", f"/rules?page={page}")
        rules += d.get("rules", [])
        total = d.get("totalCount", len(rules))
        if not args.all or len(rules) >= total or not d.get("rules"):
            break
        page += 1
    _emit_rules(rules, total, args)


def cmd_get(args) -> None:
    r = _request("GET", f"/rule/{args.rule_id}")
    print(json.dumps(r, indent=2) if args.json else _fmt_rule(r))


def cmd_search(args) -> None:
    # NOTE: /rules/search returns a SPARSE shape — {id, name, content} only,
    # no severity/state/source, and the id key is `id` not `ruleId`. _rid()
    # normalizes it; use `get <id>` for the full record.
    body = {"query": args.query, "top_k": args.top_k}
    if args.scope:
        body["scopes"] = [args.scope]
    rules = _request("POST", "/rules/search", body).get("rules", [])
    _emit_rules(rules, len(rules), args)


def cmd_find(args) -> None:
    """Resolve a ruleId from free text (e.g. a Qodo PR-review comment).

    Combines semantic search with a literal substring scan over the full catalog,
    so a half-remembered rule name still maps to its ruleId. Each candidate id is
    then fetched via GET so the output carries severity/state/source uniformly
    (search results alone are too sparse to act on).
    """
    needle = args.text.lower()
    ids: set[int] = set()
    for r in _request("POST", "/rules/search",
                      {"query": args.text, "top_k": args.top_k}).get("rules", []):
        ids.add(_rid(r))
    page, total = 1, 0
    while True:
        d = _request("GET", f"/rules?page={page}")
        for r in d.get("rules", []):
            blob = (r.get("name", "") + " " + (r.get("content") or "")).lower()
            if needle in blob:
                ids.add(_rid(r))
        total = d.get("totalCount", 0)
        if page * 50 >= total or not d.get("rules"):
            break
        page += 1
    full = [_request("GET", f"/rule/{rid}") for rid in ids if rid]
    full.sort(key=lambda r: (r.get("state") != "active", r.get("name") or ""))
    _emit_rules(full, len(full), args)


def cmd_load(args) -> None:
    """Load the rules relevant to the CURRENT coding task, to apply while coding.

    This is the 'consume' path (what the deprecated qodo-get-rules skill did, fixed):
    run one or two structured queries, merge with the first taking priority, dedupe,
    enrich each hit via GET (search is sparse — no severity/state), keep only ACTIVE
    rules, and print them grouped by severity. See references/loading-rules.md for how
    to write the queries.
    """
    order: list[int] = []
    for q in args.query:
        body = {"query": q, "top_k": args.top_k}
        if args.scope:
            body["scopes"] = [args.scope]
        for r in _request("POST", "/rules/search", body).get("rules", []):
            rid = _rid(r)
            if rid and rid not in order:
                order.append(rid)
    rules = [_request("GET", f"/rule/{rid}") for rid in order]
    rules = [r for r in rules if r.get("state") == "active"]
    if args.json:
        print(json.dumps(rules, indent=2))
        return
    print("# 📋 Qodo Rules Loaded\n")
    if not rules:
        print("No active rules matched this task. Proceeding without rule constraints.\n\n---")
        return
    print(f"{len(rules)} active rule(s), most relevant first. "
          f"Apply by severity (error=must, warning=should, recommendation=consider):\n")
    rank = {"error": 0, "warning": 1, "recommendation": 2}
    for r in sorted(rules, key=lambda x: rank.get(x.get("severity"), 9)):
        print(f"- **{r.get('name')}** [{(r.get('severity') or '?').upper()}] "
              f"(rule {_rid(r)})\n  {(r.get('content') or '').strip()}\n")
    print("---")


# --- write ops (default dry-run) -------------------------------------------

def _put_with_preview(rule_id: int, current: dict, changes: dict, apply: bool) -> None:
    body = {k: current.get(k) for k in MUTABLE_FIELDS if k in current}
    print(f"\nRule {rule_id}: {current.get('name')}")
    for k, new in changes.items():
        old = body.get(k)
        if k == "content":
            print(f"  {k}: (content change — {len(str(old or ''))} -> {len(str(new))} chars)")
        else:
            print(f"  {k}: {old!r} -> {new!r}")
        body[k] = new
    if not apply:
        print("\nDRY-RUN. Re-run with --apply to send the PUT. "
              "(Org-wide rule — this changes grading for every teammate.)")
        return
    res = _request("PUT", f"/rule/{rule_id}", body)
    print(f"\nAPPLIED. state={res.get('state')} severity={res.get('severity')} "
          f"updatedAt={res.get('updatedAt')}")


def cmd_set_state(args) -> None:
    if args.state not in ("active", "inactive"):
        sys.exit("ERROR: state must be 'active' or 'inactive'.")
    current = _request("GET", f"/rule/{args.rule_id}")
    if current.get("state") == args.state:
        print(f"Rule {args.rule_id} already {args.state} — no-op.")
        return
    _put_with_preview(args.rule_id, current, {"state": args.state}, args.apply)


def cmd_update(args) -> None:
    current = _request("GET", f"/rule/{args.rule_id}")
    changes: dict = {}
    if args.severity:
        changes["severity"] = args.severity
    if args.content_file:
        with open(args.content_file) as fh:
            changes["content"] = fh.read()
    if args.append_content:
        changes["content"] = (current.get("content") or "") + "\n" + args.append_content
    if args.scope_add:
        scopes = list(current.get("scopes") or [])
        if args.scope_add not in scopes:
            scopes.append(args.scope_add)
        changes["scopes"] = scopes
    if not changes:
        sys.exit("ERROR: nothing to change. Pass --severity / --content-file / "
                 "--append-content / --scope-add.")
    _put_with_preview(args.rule_id, current, changes, args.apply)


# --- formatting -------------------------------------------------------------

def _rid(r: dict) -> int | None:
    """Normalize the id key: list/get use `ruleId`, search uses `id`."""
    return r.get("ruleId") or r.get("id")


def _fmt_rule(r: dict) -> str:
    sev = r.get("severity") or "?"
    state = r.get("state") or "?"
    src = r.get("source") or r.get("sourceUri") or "(search result — run `get` for source)"
    return f"[{_rid(r)}] ({sev}/{state}) {r.get('name')}\n    source: {src}"


def _emit_rules(rules, total, args) -> None:
    if args.json:
        print(json.dumps(rules, indent=2))
        return
    if not rules:
        print("No matching rules.")
        return
    for r in rules[: args.limit]:
        print(_fmt_rule(r))
    shown = min(len(rules), args.limit)
    print(f"\n{shown} shown · {len(rules)} matched · {total} total in catalog")
    if shown < len(rules):
        print("(use --limit N or --json for more)")


def main() -> int:
    p = argparse.ArgumentParser(description="Manage Qodo coding rules.")
    p.add_argument("--json", action="store_true", help="raw JSON output")
    p.add_argument("--limit", type=int, default=30, help="max rows in human output")
    sub = p.add_subparsers(dest="cmd", required=True)

    lp = sub.add_parser("list", help="list rules (paginated)")
    lp.add_argument("--all", action="store_true", help="fetch every page")
    lp.set_defaults(func=cmd_list)

    gp = sub.add_parser("get", help="get one rule by id")
    gp.add_argument("rule_id", type=int)
    gp.set_defaults(func=cmd_get)

    sp = sub.add_parser("search", help="semantic search")
    sp.add_argument("query")
    sp.add_argument("--scope", help="e.g. /skillrig/cli/")
    sp.add_argument("--top-k", type=int, default=20)
    sp.set_defaults(func=cmd_search)

    fp = sub.add_parser("find", help="resolve ruleId from free text (semantic + substring)")
    fp.add_argument("text")
    fp.add_argument("--top-k", type=int, default=20)
    fp.set_defaults(func=cmd_find)

    ld = sub.add_parser("load", help="load rules relevant to the current coding task (apply while coding)")
    ld.add_argument("--query", action="append", required=True,
                    help="structured Name/Category/Content query; pass twice (topic + cross-cutting)")
    ld.add_argument("--scope", help="e.g. /skillrig/cli/ (omit for org-wide)")
    ld.add_argument("--top-k", type=int, default=20)
    ld.set_defaults(func=cmd_load)

    stp = sub.add_parser("set-state", help="activate / deactivate a rule (reversible)")
    stp.add_argument("rule_id", type=int)
    stp.add_argument("state", help="active | inactive")
    stp.add_argument("--apply", action="store_true", help="send the PUT (default: dry-run)")
    stp.set_defaults(func=cmd_set_state)

    up = sub.add_parser("update", help="modify rule fields (read-modify-write PUT)")
    up.add_argument("rule_id", type=int)
    up.add_argument("--severity", help="error | warning | recommendation")
    up.add_argument("--content-file", help="replace content with file contents")
    up.add_argument("--append-content", help="append a line to content (e.g. a carve-out)")
    up.add_argument("--scope-add", help="add a scope path")
    up.add_argument("--apply", action="store_true", help="send the PUT (default: dry-run)")
    up.set_defaults(func=cmd_update)

    args = p.parse_args()
    args.func(args)
    return 0


if __name__ == "__main__":
    sys.exit(main())
