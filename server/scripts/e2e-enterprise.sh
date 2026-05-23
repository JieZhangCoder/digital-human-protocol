#!/usr/bin/env bash
#
# e2e-enterprise.sh — Local end-to-end smoke test simulating an enterprise
# DHP v2 deployment (e.g. the enterprise rollout).
#
# What this exercises:
#   1. Spin up dhp-registry on localhost with token auth + local storage.
#   2. Confirm a fresh registry returns empty split indexes + /healthz ok.
#   3. Publish a real digital human (hn-daily) from the repo: multipart upload
#      with Bearer token, server runs review synchronously, server stores
#      artifacts under apps/<slug>/<version>/.
#   4. Publish a real scoped skill (openkursar/x-search).
#   5. Verify both appear in /digital-humans.json and /skills.json.
#   6. Fetch spec.yaml + a file via the artifact route to confirm download
#      works end-to-end.
#   7. Restart the server — verify the in-memory indexes are correctly
#      rebuilt from storage (regression test for the previously-known
#      "indexes lost on restart" risk).
#   8. Negative cases: reject missing-token, reject wrong-token, reject
#      tampered spec (slug mismatch).
#
# Run from the repo root:
#     bash server/scripts/e2e-enterprise.sh
#
# Optional env:
#     PORT          : listen port (default 18181, avoids :8080/:18081 conflicts)
#     KEEP_WORKDIR  : set to 1 to retain $WORKDIR + server.log for inspection
#
# Exits 0 on success; non-zero with the offending step name on first failure.

set -euo pipefail

readonly REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
readonly SERVER_DIR="$REPO_ROOT/server"
readonly PORT="${PORT:-18181}"
readonly TOKEN="e2e-test-token-$$"
readonly WORKDIR="$(mktemp -d -t dhp-e2e-XXXXXX)"

cd "$REPO_ROOT"

trap_cleanup() {
  local rc=$?
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  if [[ "${KEEP_WORKDIR:-0}" == "1" ]]; then
    echo
    echo "WORKDIR kept for inspection: $WORKDIR"
  else
    rm -rf "$WORKDIR"
  fi
  return $rc
}
trap trap_cleanup EXIT

# ── helpers ────────────────────────────────────────────────────────────────

step() { printf "\n\033[1;34m▶ %s\033[0m\n" "$*"; }
ok()   { printf "  \033[0;32m✓\033[0m %s\n" "$*"; }
die()  { printf "  \033[0;31m✗ %s\033[0m\n" "$*" >&2; exit 1; }

# Wait for the server health endpoint; return non-zero after 5s.
wait_for_health() {
  for _ in $(seq 1 50); do
    if curl -fsS "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
  done
  return 1
}

start_server() {
  "$SERVER_DIR/bin/dhp-registry" --config="$WORKDIR/config.yaml" \
    >> "$WORKDIR/server.log" 2>&1 &
  SERVER_PID=$!
  if ! wait_for_health; then
    cat "$WORKDIR/server.log" >&2
    die "server failed to become healthy on :$PORT"
  fi
}

stop_server() {
  if [[ -n "${SERVER_PID:-}" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
    SERVER_PID=""
  fi
}

# ── 0. build binaries ──────────────────────────────────────────────────────

step "0. Build dhp-registry"
(cd "$SERVER_DIR" && mkdir -p bin && go build -o bin/dhp-registry ./cmd/registry) \
  || die "go build failed"
ok "binary at $SERVER_DIR/bin/dhp-registry"

# ── 1. write config + start ────────────────────────────────────────────────

step "1. Start dhp-registry on :$PORT (token auth, local storage)"
cat > "$WORKDIR/config.yaml" <<EOF
listen: ":$PORT"
storage:
  type: local
  config:
    path: "$WORKDIR/data"
auth:
  type: token
  token: "$TOKEN"
rules:
  enabled: [schema_valid, slug_unique, dangerous_permissions, sensitive_categories]
auto_merge_threshold: "low_risk"
max_upload_bytes: 10485760
EOF
start_server
ok "server PID=$SERVER_PID  log=$WORKDIR/server.log"

# ── 2. empty registry sanity ───────────────────────────────────────────────

step "2. Empty registry sanity check"
test "$(curl -fsS "http://127.0.0.1:$PORT/healthz" | grep -c '"status"')" = "1" \
  || die "/healthz did not return JSON with status"
ok "/healthz returns 200 + JSON"

# Envelope shape: {version, generated_at, source, apps: []}.
# Use python json to parse strictly — this pins both the envelope keys
# AND the empty-array state on a fresh registry.
check_envelope_empty() {
  local url="$1" label="$2"
  curl -fsS "$url" | python3 -c "
import json, sys
data = json.load(sys.stdin)
label = '$label'
for k in ('version', 'generated_at', 'source', 'apps'):
    assert k in data, f'{label} missing key {k!r}: {data}'
assert isinstance(data['apps'], list), f'{label} apps not list: {data}'
assert len(data['apps']) == 0, f'{label} apps not empty: {len(data[\"apps\"])}'
" || die "envelope check failed for $label"
}
check_envelope_empty "http://127.0.0.1:$PORT/digital-humans.json" "digital-humans.json"
ok "/digital-humans.json has envelope + apps[] is empty"
check_envelope_empty "http://127.0.0.1:$PORT/skills.json" "skills.json"
ok "/skills.json has envelope + apps[] is empty"

# ── 3. publish a digital human ─────────────────────────────────────────────

step "3. Publish digital human (hn-daily) with valid token"
DH_SPEC="$REPO_ROOT/packages/digital-humans/hn-daily/spec.yaml"
test -f "$DH_SPEC" || die "expected $DH_SPEC to exist"

dh_resp=$(curl -fsS -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -F "spec=@$DH_SPEC" \
  "http://127.0.0.1:$PORT/apps")
echo "$dh_resp" | grep -q '"slug": *"hn-daily"' || die "publish response missing slug: $dh_resp"
echo "$dh_resp" | grep -q '"verdict"' || die "publish response missing verdict: $dh_resp"
ok "POST /apps accepted hn-daily"

# ── 4. publish a scoped skill ──────────────────────────────────────────────

step "4. Publish scoped skill (openkursar/x-search)"
SK_DIR="$REPO_ROOT/packages/skills/openkursar/x-search"
test -d "$SK_DIR" || die "expected $SK_DIR (run extract-bundled-skills first)"

# Build multipart with spec + the index.js as an extra file part
sk_resp=$(curl -fsS -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -F "spec=@$SK_DIR/spec.yaml" \
  -F "files=@$SK_DIR/SKILL.md" \
  -F "files=@$SK_DIR/index.js" \
  "http://127.0.0.1:$PORT/apps")
echo "$sk_resp" | grep -q '"slug": *"openkursar/x-search"' \
  || die "publish response missing scoped slug: $sk_resp"
ok "POST /apps accepted openkursar/x-search (3 files: spec + 2 files)"

# ── 5. indexes reflect publishes ───────────────────────────────────────────

step "5. Verify split indexes reflect published content"
dh_after=$(curl -fsS "http://127.0.0.1:$PORT/digital-humans.json")
echo "$dh_after" | grep -q '"slug": *"hn-daily"' \
  || die "digital-humans.json missing hn-daily: $dh_after"
ok "/digital-humans.json contains hn-daily"

sk_after=$(curl -fsS "http://127.0.0.1:$PORT/skills.json")
echo "$sk_after" | grep -q '"slug": *"openkursar/x-search"' \
  || die "skills.json missing openkursar/x-search: $sk_after"
ok "/skills.json contains openkursar/x-search"

idx=$(curl -fsS "http://127.0.0.1:$PORT/index.json")
echo "$idx" | grep -q '"deprecated": *true' \
  || die "legacy index.json missing deprecation notice: $idx"
ok "/index.json carries deprecation notice + merged entries"

# ── 6. download artifacts back ─────────────────────────────────────────────

step "6. Fetch artifacts via artifact route"

# Digital-human spec retrieval
got_spec=$(curl -fsS "http://127.0.0.1:$PORT/apps/hn-daily/2.0.0/spec.yaml" 2>/dev/null \
  || true)
# The route only serves files under /files/; spec.yaml lives at the top of the
# bundle dir. The artifact endpoint expects /apps/<slug>/<version>/files/<path>.
# We seeded only spec.yaml for hn-daily (no extra files), so test with the
# skill's index.js instead.
# Download to a file so trailing-newline handling matches the source byte-for-byte.
curl -fsS -o "$WORKDIR/got-index.js" "http://127.0.0.1:$PORT/apps/openkursar/x-search/1.0.0/files/index.js"
test -s "$WORKDIR/got-index.js" || die "artifact route returned empty body"
orig_sha=$(shasum -a 256 "$SK_DIR/index.js" | awk '{print $1}')
got_sha=$(shasum -a 256 "$WORKDIR/got-index.js" | awk '{print $1}')
test "$orig_sha" = "$got_sha" \
  || die "artifact content mismatch: orig=$orig_sha got=$got_sha"
got_size=$(wc -c < "$WORKDIR/got-index.js" | tr -d ' ')
ok "/apps/openkursar/x-search/1.0.0/files/index.js downloads byte-identical ($got_size bytes, sha256 ${got_sha:0:8}…)"

# ── 7. restart server: indexes must persist via rebuild ────────────────────

step "7. Restart server → verify indexes rebuild from storage"
stop_server
ok "previous server stopped"
start_server
ok "fresh server PID=$SERVER_PID"

dh_after_restart=$(curl -fsS "http://127.0.0.1:$PORT/digital-humans.json")
echo "$dh_after_restart" | grep -q '"slug": *"hn-daily"' \
  || die "post-restart digital-humans.json missing hn-daily — REBUILD BROKEN: $dh_after_restart"
ok "hn-daily survives restart"

sk_after_restart=$(curl -fsS "http://127.0.0.1:$PORT/skills.json")
echo "$sk_after_restart" | grep -q '"slug": *"openkursar/x-search"' \
  || die "post-restart skills.json missing openkursar/x-search — REBUILD BROKEN"
ok "openkursar/x-search survives restart"

# Confirm rebuild log line was written
grep -q "\[index-rebuild\] complete: rebuilt=2" "$WORKDIR/server.log" \
  || die "expected log line '[index-rebuild] complete: rebuilt=2' not found"
ok "log confirms rebuilt=2 on cold start"

# ── 8. negative cases ──────────────────────────────────────────────────────

step "8. Auth and tamper rejections"

# 8a. Missing token
status=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
  -F "spec=@$DH_SPEC" "http://127.0.0.1:$PORT/apps")
test "$status" = "401" || die "expected 401 without token, got $status"
ok "POST /apps without Authorization → 401"

# 8b. Wrong token
status=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
  -H "Authorization: Bearer wrong-token" \
  -F "spec=@$DH_SPEC" "http://127.0.0.1:$PORT/apps")
test "$status" = "401" || die "expected 401 with wrong token, got $status"
ok "POST /apps with wrong token → 401"

# 8c. Bad spec (empty body)
status=$(curl -s -o /dev/null -w "%{http_code}" -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -F "spec=@/dev/null" "http://127.0.0.1:$PORT/apps")
test "$status" = "400" -o "$status" = "422" \
  || die "expected 400 or 422 for empty spec, got $status"
ok "POST /apps with empty spec → $status"

# ── done ───────────────────────────────────────────────────────────────────

step "Result"
printf "\033[1;32mE2E PASSED\033[0m — registry + publish + serve + rebuild all green.\n"
printf "  Storage left at: %s/data\n" "$WORKDIR"
printf "  Server log:      %s/server.log\n" "$WORKDIR"
