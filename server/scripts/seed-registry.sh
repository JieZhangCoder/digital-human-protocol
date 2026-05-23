#!/usr/bin/env bash
#
# seed-registry.sh — Bulk-publish a set of bundles to a DHP registry.
#
# Use cases:
#   - Local smoke test:  pre-fill an empty test registry so a tester
#                        sees realistic content on first browse.
#   - Enterprise day-1:  same script seeds the production registry
#                        with a curated set of in-house defaults.
#
# Usage:
#   bash seed-registry.sh <REGISTRY_URL> <TOKEN> [bundle-path ...]
#
# Examples:
#   # Local: default seeds (hn-daily + openkursar/x-search)
#   bash server/scripts/seed-registry.sh \
#        http://127.0.0.1:18181 enterprise-local-test
#
#   # Enterprise: seed with a curated list
#   bash server/scripts/seed-registry.sh \
#        http://10.107.118.2:18081 "$WEBANK_TOKEN" \
#        packages/digital-humans/{hn-daily,ai-daily-news} \
#        packages/skills/openkursar/{x-search,x-post}
#
# Behavior:
#   - For each bundle dir, publishes spec.yaml + every other file in the
#     bundle as multipart "files" parts.
#   - Logs response verdict per bundle; continues on per-bundle failure.
#   - Exits 1 if any publish failed; 0 if all succeeded.

set -uo pipefail

if [[ $# -lt 2 ]]; then
  echo "usage: $0 <REGISTRY_URL> <TOKEN> [bundle-path ...]" >&2
  exit 2
fi

readonly REGISTRY_URL="${1%/}"
readonly TOKEN="$2"
shift 2

readonly REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

# Default seed set when no bundles are passed: one digital human + one skill,
# enough to verify both index types are populated.
DEFAULT_SEEDS=(
  "packages/digital-humans/hn-daily"
  "packages/skills/openkursar/x-search"
)

if [[ $# -eq 0 ]]; then
  set -- "${DEFAULT_SEEDS[@]}"
fi

step() { printf "\n\033[1;34m▶ %s\033[0m\n" "$*"; }
ok()   { printf "  \033[0;32m✓\033[0m %s\n" "$*"; }
warn() { printf "  \033[0;33m! %s\033[0m\n" "$*"; }
err()  { printf "  \033[0;31m✗ %s\033[0m\n" "$*" >&2; }

step "Sanity: registry healthy?"
if ! curl -fsS "$REGISTRY_URL/healthz" >/dev/null; then
  err "registry not reachable at $REGISTRY_URL"
  exit 1
fi
ok "$REGISTRY_URL/healthz returns 200"

failed=0
published=0

for bundle in "$@"; do
  step "Publish $bundle"

  if [[ ! -d "$bundle" ]]; then
    err "$bundle is not a directory — skipped"
    failed=$((failed + 1))
    continue
  fi

  if [[ ! -f "$bundle/spec.yaml" ]]; then
    err "$bundle has no spec.yaml — skipped"
    failed=$((failed + 1))
    continue
  fi

  # Build curl arg array: -F spec=@... + one -F files=@... per non-spec file.
  curl_args=(-fsS -X POST -H "Authorization: Bearer $TOKEN")
  curl_args+=(-F "spec=@$bundle/spec.yaml")

  # Walk other files in the bundle (top-level only; nested dirs like
  # skills/ are intentionally skipped because they no longer exist after
  # extract-bundled-skills.mjs --apply).
  while IFS= read -r -d '' f; do
    rel="${f#"$bundle/"}"
    [[ "$rel" == "spec.yaml" ]] && continue
    curl_args+=(-F "files=@$f")
  done < <(find "$bundle" -maxdepth 1 -type f -print0)

  response=$(curl "${curl_args[@]}" "$REGISTRY_URL/apps" 2>&1)
  rc=$?

  if [[ $rc -ne 0 ]]; then
    # curl -f returns 22 on HTTP >= 400; fetch the body without -f for context
    body=$(curl -sS -X POST \
      -H "Authorization: Bearer $TOKEN" \
      -F "spec=@$bundle/spec.yaml" \
      "$REGISTRY_URL/apps" 2>&1 || true)
    err "publish failed (curl rc=$rc): $body"
    failed=$((failed + 1))
    continue
  fi

  verdict=$(echo "$response" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('verdict','?'))" 2>/dev/null || echo "?")
  slug=$(echo "$response" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('slug','?'))" 2>/dev/null || echo "?")
  ok "$slug → verdict=$verdict"
  published=$((published + 1))
done

step "Result"
echo "  published=$published  failed=$failed"

if [[ $failed -gt 0 ]]; then
  exit 1
fi

# Print what the registry now exposes — useful as test confirmation.
step "Registry now exposes"
echo "  digital-humans.json:"
curl -fsS "$REGISTRY_URL/digital-humans.json" | python3 -m json.tool 2>/dev/null | \
  grep -E '"slug"' | sed 's/^/    /' || echo "    (empty)"
echo "  skills.json:"
curl -fsS "$REGISTRY_URL/skills.json" | python3 -m json.tool 2>/dev/null | \
  grep -E '"slug"' | sed 's/^/    /' || echo "    (empty)"
