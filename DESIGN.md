# Linejudge — review pipeline for GitHub pull requests

## What this is

A GitHub App-shaped **review pipeline**, not a chat box. Incoming `pull_request` and
`check_suite` events become jobs. Each job parses a unified diff, packs hunk context,
runs static checks, optionally runs a gated LLM stage, merges findings, and publishes
**line-anchored review comments** in the GitHub review-comment JSON shape.

The LLM is one stage. It is fail-closed (off until an operator approves it). A
deterministic mock provider is the default so demo and eval work without keys.

---

## Pipeline stages

```
webhook accept
    → idempotency (delivery_id, then repo+pr+sha+event)
    → enqueue job
    → parse-diff          unified diff → files / hunks / line numbers
    → pack-context        RIGHT-side hunks, per-file char budget
    → static-checks       secrets, complexity, bugs, insecure, js-unsafe
    → llm-assist          skipped | mock | openai  (never fails the review)
    → merge-rank          fingerprint dedupe, severity floor, comment cap
    → publish             GitHub review comments (or local mock publisher)
    → record              findings, comments, stage events for the dashboard
```

Jobs live in SQLite (`jobs` table). The demo binary runs the worker loop in-process
(`LINEJUDGE_ROLE=all`). The same table is the seam for a later split:
`ROLE=api` + `ROLE=worker` replicas on Postgres `FOR UPDATE SKIP LOCKED`.

### Check stages (static)

| id | what it actually does |
|----|------------------------|
| `secrets` | Regex for AWS AKIDs, PEMs, `ghp_` PATs, Slack tokens, high-entropy password/secret assignments on **added** lines |
| `complexity` | Hunk-local cyclomatic heuristic (branch keywords per function). Not `go vet` AST; documented as heuristic |
| `bugs` | `fmt.Sprintf` into SQL, discarded `err` via `_, :=`, `TODO(security)` |
| `insecure` | `InsecureSkipVerify`, MD5/SHA-1, empty `http.Client{}` |
| `js-unsafe` | `innerHTML`, `eval`, `document.write` on JS/TS |

These are **local/static**. They do not call a model. They are the product's
default signal. Optional `go vet` / `eslint` can be added as subprocess stages
behind the same `CheckConfig.enabled` interface; the MVP ships heuristics so the
demo does not require toolchains in the container.

### LLM stage

- Config: `checks.llm-assist.enabled` **and** `checks.llm-assist.approve`.
- Provider: `mock` (default) or `openai` if `OPENAI_API_KEY` is set and
  `llm_provider=openai`.
- Mock is **deterministic rules over packed hunks** (authz gap, N+1, map race,
  dropped context, SSRF). It exists so eval numbers are reproducible. It is not
  a hidden GPT call.
- OpenAI uses JSON-mode chat completions with the packed hunks as input and the
  same `Finding` schema as output. Timeout 45s. On error the stage is `degraded`
  and static findings still publish.
- Honesty: the mock will not find novel bugs. A real model will hallucinate
  line numbers; merge-rank does not currently verify that the line exists in the
  packed RIGHT side (follow-up). Do not sell this as “AI senior engineer.”

---

## Idempotency

1. **Webhook delivery** — `webhook_deliveries.delivery_id` is unique.
   `X-GitHub-Delivery` redelivered → `{"idempotent": true}` and no new job.
2. **Review identity** — unique `(repo_id, pr_number, sha, event_type)`.
   A second event for the same head SHA does not enqueue another review.
   `synchronize` with a new SHA is a new run (demo replay mints a SHA).
3. **Finding fingerprint** — `check_id + path + line + rule` so merge-rank
   does not double-post the same comment.

HMAC: if `GITHUB_WEBHOOK_SECRET` is set, `X-Hub-Signature-256` is required.
If unset, the receiver logs that it is unsigned (demo). Production must set it.

---

## Failure modes

| failure | behavior |
|---------|----------|
| Bad signature | 401, no job |
| Unknown / ignored action (`closed`, etc.) | 202 `ignored` |
| Missing diff (live GitHub without token, no `_fixture`) | job not created; API error. Live fetch of `pulls/{n}.diff` is the next wiring step; fixtures cover the demo. |
| Parse error | review `failed`, job retried up to 5 times with backoff, then poison |
| Static check panic | process crash (treated as worker death); SQLite job stays `running` until process restart — **gap**: no lock timeout reaper in MVP |
| LLM timeout / 5xx | stage `degraded`, review still completes |
| GitHub comment API 4xx | publish stage errors, review `failed`, retry |
| SQLite lock | `busy_timeout=5000`; single writer (`MaxOpenConns=1`) |

Retries: `jobs.attempts`, `run_after`. After 5 failures the review is `failed`
with `jobs.last_error`.

There is **no** at-least-once duplicate comment protection against a crash
after GitHub accepts the review but before we write `comments`. A real App
should store the GitHub review id and skip republish.

---

## Data model (canonical)

`Finding` is the product object: path, line, side=`RIGHT`, severity, check_id,
source=`static|llm`, fingerprint, markdown body.

`ReviewComment` is the GitHub [create a review](https://docs.github.com/en/rest/pulls/reviews) comment element:
`{path, line, side, body}`. The publisher posts those; the dashboard shows both.

---

## Eval

`go run ./cmd/eval` runs the pipeline in-process against `fixtures/*/`.

A **hit** is a finding whose path matches a labeled issue and whose line is
within ±2 of `[line, line_end]`. Metrics: precision, recall (hit-rate), F1,
precision@5, precision@10, clean-PR false positives.

These are measured on the five golden PRs in this repo. They are not a claim
about production GitHub traffic.

---

## What we did not build

- A registered GitHub App with private-key JWT installation tokens (env-shaped;
  `GITHUB_APP_INSTALLATION_TOKEN` / `GITHUB_TOKEN` posts for real if set).
- Clone-and-`go vet` / ESLint subprocess in the worker.
- Multi-tenant auth on the dashboard (it binds to LAN).
- RAG, agents, or chat.
