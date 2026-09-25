#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p data
export LINEJUDGE_DB="${LINEJUDGE_DB:-$ROOT/data/linejudge.db}"
export LINEJUDGE_FIXTURES="${LINEJUDGE_FIXTURES:-$ROOT/fixtures}"

echo "==> api"
go run ./cmd/linejudge &
pid=$!
trap 'kill $pid 2>/dev/null || true' EXIT
for i in $(seq 1 40); do
  curl -sf http://127.0.0.1:8080/healthz && break
  sleep 0.25
done

echo
echo "==> approve llm-assist (mock)"
python3 - <<'PY'
import json, urllib.request
cfg = json.load(urllib.request.urlopen("http://127.0.0.1:8080/v1/config"))
cfg["checks"]["llm-assist"]["approve"] = True
req = urllib.request.Request(
    "http://127.0.0.1:8080/v1/config",
    data=json.dumps(cfg).encode(),
    method="PUT",
    headers={"Content-Type": "application/json"},
)
urllib.request.urlopen(req).read()
print("approved")
PY

echo "==> webhook secret-leak"
curl -sS -X POST http://127.0.0.1:8080/webhooks/github \
  -H 'Content-Type: application/json' \
  -H 'X-GitHub-Event: pull_request' \
  -H "X-GitHub-Delivery: demo-$(date +%s)" \
  --data @fixtures/secret-leak/webhook.json | tee /tmp/lj.json
echo
id=$(python3 -c "import json;print(json.load(open('/tmp/lj.json'))['review']['id'])")
echo "==> wait for $id"
for i in $(seq 1 30); do
  st=$(curl -sf "http://127.0.0.1:8080/v1/reviews/$id" | python3 -c "import json,sys; print(json.load(sys.stdin)['review']['status'])")
  echo "  status=$st"
  [ "$st" = "completed" ] && break
  [ "$st" = "failed" ] && exit 1
  sleep 0.2
done
echo "==> GitHub-shaped comments"
curl -sS "http://127.0.0.1:8080/v1/reviews/$id/comments"
echo
echo "open http://127.0.0.1:3000/reviews/$id"
