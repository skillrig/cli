# qodo-manage-rules

The single skill for working with your org's **Qodo coding rules** — both *consuming* them
(load the rules relevant to a coding task and apply them while you code) and *administering*
them (modify / scope / deactivate a rule when it's wrong, over-broad, or stale).

See [`SKILL.md`](./SKILL.md) for the full workflow. This README covers **provenance** and
**setup** only.

## Why this exists (and what it replaces)

It replaces the upstream **`qodo-get-rules`** skill, which was vendored from
[`github.com/qodo-ai/qodo-skills`](https://github.com/qodo-ai/qodo-skills) — an
**abandoned** repository. That skill was both incomplete (read-only; no way to manage
rules) and **broken in practice**: it read the API token from `~/.qodo/config.json`'s
`API_KEY` field, which the current Qodo CLI does not write there. Rather than fork a dead
upstream, this skill is self-contained:

- correct auth (reads `~/.qodo/auth.key`, the file the Qodo CLI actually writes),
- adds the rule-management API (list / search / get / modify / deactivate), reverse-engineered
  from the web portal and verified live (see [`references/api-contract.md`](./references/api-contract.md)),
- folds the "load rules for a coding task" job back in, done right (see
  [`references/loading-rules.md`](./references/loading-rules.md)).

**Use this skill instead of `qodo-get-rules`.** The old one has been removed from this repo's
`skills-lock.json`.

## Setup — getting the API token

The skill authenticates with a raw `sk-...` bearer token at **`~/.qodo/auth.key`**
(or the `QODO_API_KEY` env var, which takes precedence).

The fastest way to mint that file is the **Qodo Command** CLI
([docs.qodo.ai/qodo-command](https://docs.qodo.ai/qodo-command)):

```sh
qodo login        # opens a browser, authenticates, and writes ~/.qodo/auth.key
```

> ⚠️ **Qodo Command is itself sunset.** Its *chat* is dead — any chat message just returns a
> "this tool is sunset" notice. But `qodo login` still works and is the easiest way to
> create the API key. After login you don't need the CLI again; this skill talks to the
> rules API directly. (If you already have a token from the Qodo web portal, you can skip
> the CLI entirely and write it to `~/.qodo/auth.key` yourself, or export `QODO_API_KEY`.)

Verify it's in place (the token is 90 bytes, starts `sk-`):

```sh
ls -lah ~/.qodo/auth.key
```

## Configuration

| What | Source (highest precedence first) | Default |
|------|-----------------------------------|---------|
| **Token** | `$QODO_API_KEY` → `~/.qodo/auth.key` | — (required) |
| **API base** | `$QODO_API_URL` (or `QODO_API_URL` in `config.json`) → `$QODO_ENVIRONMENT_NAME` | `https://qodo-platform.qodo.ai/rules/v1` |

`~/.qodo/config.json` holds only Qodo CLI **UI preferences** (theme, etc.) — the token is
**not** there. Don't commit `~/.qodo/auth.key` or any HAR capture; this repo's `.gitignore`
already excludes `*.key` and `*.har`.

## Layout

```
qodo-manage-rules/
├── README.md                         # you are here — provenance + setup
├── SKILL.md                          # the workflow (consume + administer)
├── references/
│   ├── loading-rules.md              # structured queries, two-query strategy, severity
│   ├── managing-derived-rules.md     # modify-vs-deactivate decision tree + PR-triage playbook
│   └── api-contract.md               # endpoints, auth, request/response shapes
└── scripts/
    └── qodo_rules.py                 # stdlib-only client (list/get/search/find/load/set-state/update)
```
