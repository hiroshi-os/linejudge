# linejudge

Production-ready **code review pipeline** for GitHub pull requests.

Not a paste-diff chatbot. Incoming App-shaped webhooks become jobs: parse the
diff, pack hunks, run static checks, optionally a gated LLM stage, publish
line-anchored review comments, show the run on a dashboard, and measure the
pipeline on labeled fixtures.

```
webhook → job → parse → pack → static checks → [llm] → merge → GitHub comments
                                                      ↘ dashboard / eval
```

## 60-second happy path (Loom this)

Needs Go 1.22+ and Node 20+. No API keys.

```bash
# 1. API + worker (SQLite in ./data)
go run ./cmd/linejudge

# 2. Dashboard (another terminal)
cd web && npm install && npm run dev

# 3. Fire a pull_request webhook for the secret-leak fixture
curl -sS -X POST http://127.0.0.1:8080/webhooks/github \
  -H 'Content-Type: application/json' \
  -H 'X-GitHub-Event: pull_request' \
  -H 'X-GitHub-Delivery: loom-1' \
  --data @fixtures/secret-leak/webhook.json | tee /tmp/lj.json

# 4. Wait for the worker (~300ms) then print GitHub-shaped comments
id=$(python3 -c "import json;print(json.load(open('/tmp/lj.json'))['review']['id'])")
sleep 1
curl -sS "http://127.0.0.1:8080/v1/reviews/$id/comments"
```

Open [http://127.0.0.1:3000](http://127.0.0.1:3000) → **Reviews** → the PR.
You should see yellow-gutter findings on `config/prod.yaml` (AWS key) and
`internal/config/load.go` (GitHub PAT).

Or one shot: `make demo` (starts the API, posts the webhook, prints comments).

### docker compose

```bash
docker compose up --build
# dashboard :3000   api :8080
curl -sS -X POST http://127.0.0.1:8080/v1/demo/replay/secret-leak
```

Replay also exists in the dashboard (Reviews page, fixture chips).

## Eval (real numbers, this repo)

```bash
go run ./cmd/eval
```

Measured on 5 golden PRs / 15 labeled issues (mock LLM approved, no network):

| metric | value |
|--------|------:|
| precision | 0.789 |
| recall / hit-rate | 1.000 |
| F1 | 0.882 |
| precision@5 | 0.824 |
| precision@10 | 0.789 |
| clean-PR false positives | 0 |

Re-run after you change a check. The harness is `internal/evalx`. Labels live
next to each fixture in `fixtures/*/labels.json`. Hit = same path, line ±2.

## Config

Dashboard **Checks**: enable/disable each static check, severity, complexity
threshold, **approve LLM stage**, provider `mock` | `openai`.

LLM is fail-closed. Approve it only if you want that stage. Mock needs no key.
OpenAI:

```
export OPENAI_API_KEY=sk-...          # never commit
export OPENAI_MODEL=gpt-4o-mini       # optional
```

Then set provider to `openai` in Checks. Secrets are env-only; see `.env.example`.

GitHub App (optional, live comments):

```
GITHUB_WEBHOOK_SECRET=...             # HMAC on /webhooks/github
GITHUB_TOKEN=...                      # or GITHUB_APP_INSTALLATION_TOKEN
# payloads should include installation + pull_request; without a token
# comments are stored locally in the GitHub review-comment shape
```

## Layout

```
cmd/linejudge     API + in-process worker
cmd/eval          fixture harness
internal/pipeline stages
internal/checks   static analyzers
internal/llm      mock + OpenAI
internal/githubx  webhook parse, HMAC, publisher
web/              Next.js dashboard
fixtures/         golden PRs (webhook + diff + labels)
DESIGN.md         stages, idempotency, failure modes
```

## Tests

```bash
go test ./...
```

## DRAFT resume bullets

Billy will polish before resume use.

- **DRAFT** Built a GitHub App-shaped PR review pipeline (webhook → job →
  diff parse → static checks → optional LLM → line comments) with a Next.js
  ops dashboard, not a chat wrapper.
- **DRAFT** Implemented unified-diff hunk packing, fingerprint merge/rank, and
  HMAC + delivery-id idempotency so redelivered GitHub events do not double-post.
- **DRAFT** Shipped an eval harness on 5 labeled golden PRs; measured
  precision 0.79 / recall 1.00 / P@5 0.82 on the fixture set (re-run `go run ./cmd/eval`).
- **DRAFT** Gated LLM assist behind an explicit approve flag with a
  deterministic mock provider so demo and CI run without API keys.

See [DESIGN.md](DESIGN.md) for what the LLM does not do.
