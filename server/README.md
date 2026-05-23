# DHP Server (`digital-human-protocol/server`)

Go implementation of the Digital Human Protocol's automated review pipeline
and enterprise artifact registry. One codebase ships two binaries:

| Binary | Purpose |
|---|---|
| `dhp-review` | CLI invoked by GitHub Actions to review a `spec.yaml` and emit a markdown/JSON report. |
| `registry`   | HTTP daemon for enterprise deployments — accepts uploads, runs the same review synchronously, serves the canonical indexes. |

See `/Volumes/s790/Halo/DHP-V2-PLAN.md` §10 for the canonical brief.

## Quickstart

```bash
# Build both binaries.
make build

# Review a spec offline (no AI judge configured).
./bin/dhp-review \
  --spec=../packages/digital-humans/hn-daily/spec.yaml \
  --config=deploy/config.example.yaml \
  --format=markdown

# Run the registry locally.
mkdir -p /tmp/dhp-data && \
  cp deploy/config.example.yaml /tmp/dhp-config.yaml && \
  ./bin/registry --config=/tmp/dhp-config.yaml
```

## Layout

```
cmd/
  dhp-review/   CLI for GitHub Actions (synchronous review, exit code = severity)
  registry/     HTTP server (publish, index, artifact fetch)
internal/
  spec/         spec.yaml parser + validator (subset of TS schema)
  rules/        review rules + runner + report renderer + HTTP registry lookup
  ai/           AI judge HTTP client
  storage/      pluggable artifact backend (local impl + s3 stub)
  auth/         bearer-token middleware (constant-time compare)
  config/       YAML config loader with defaults
deploy/         Dockerfile, docker-compose, k8s manifests, config.example.yaml
```

## Review rules

| Rule | Type | Default | Description |
|---|---|---|---|
| `schema_valid`          | blocking  | on  | Structural / required-field validation. |
| `slug_unique`           | blocking  | on  | Reject downgrades; warn on identical-version republish. |
| `dangerous_permissions` | blocking  | on  | Block flagged perms unless allowlisted. |
| `skill_code_safety`     | AI judge  | on  | Inspect skill files for obvious malice. |
| `prompt_quality`        | AI judge  | on  | Flag jailbreaks / PII / unsafe instructions. |
| `metadata_compliance`   | AI judge  | on  | Detect deceptive metadata. |
| `duplicate_detection`   | AI judge  | on  | Detect near-duplicates / impersonation. |
| `sensitive_categories`  | warn+RH   | on  | Mark sensitive specs for human review. |

AI judges fail OPEN: a network error becomes a `warn`, not a block. This is
deliberate — a flaky judge endpoint should not freeze the entire publish
pipeline. Tighten by configuring CI to treat warnings as failures.

## Exit codes (`dhp-review`)

```
0  pass — auto-mergeable
1  warn — needs human review
2  fail — blocked
3  runner error — transient, retry
```

## Deploying

### Docker

```bash
make docker
docker run --rm -p 8080:8080 \
  -v $PWD/deploy/config.example.yaml:/etc/dhp/config.yaml:ro \
  -v dhp-data:/var/lib/dhp/artifacts \
  ghcr.io/openkursar/dhp-registry:dev
```

`deploy/docker-compose.yml` is the single-node convenience wrapper.

### Kubernetes

```bash
kubectl apply -f deploy/k8s/
```

The provided manifests create a Deployment, Service, ConfigMap, and a 10 GiB
RWO PVC. Override the bearer token via a Secret in production rather than
embedding it in the ConfigMap.

## Auth & token rotation

`registry` enforces `Authorization: Bearer <token>` on `POST /apps`. The token
is configured in `config.yaml` and compared with `crypto/subtle.ConstantTimeCompare`.
**Rotating the token requires restarting the process** — there is no hot
reload. The token can be moved into a Secret + restart on Secret change via
your deployment platform (e.g. Reloader on k8s).

## What's production-ready vs. stubbed

| Component | Status |
|---|---|
| Spec parsing / validation | Production-ready (subset of TS schema). |
| Local storage backend | Production-ready. |
| S3 storage backend | **Stub** — methods return `ErrNotImplemented`. Wire up `aws-sdk-go-v2` and verify with real credentials before launch. See comments in `internal/storage/s3.go`. |
| Auth | Production-ready. Rotation requires restart. |
| Review rules (non-AI) | Production-ready. |
| AI judge client | Production-ready transport; **depends on a real AI endpoint** that follows the `{model, prompt, context}` → `{verdict, reason}` contract. |
| HTTP registry index serving | In-memory. State is rebuilt from `apps/*` on next publish; intentional for v1 — see "Open work" below. |
| Healthcheck, graceful shutdown | Production-ready. |

## Open work / risks

- **Index persistence**: indexes are currently in-memory. Crash → lost
  metadata until next publish. Plan a JSON snapshot under `apps/.indexes/` and
  load it at boot.
- **S3 backend**: needs implementation + integration test with real AWS creds.
- **AI judge endpoint**: caller must provide a stable URL. The contract is
  documented in `internal/ai/types.go` — Anthropic/OpenAI gateway wrappers
  must conform.
- **Container image registry**: the k8s manifest assumes
  `ghcr.io/openkursar/dhp-registry`. Adjust to your private registry.
- **Multi-replica scaling**: file-backed storage requires `Recreate` strategy
  in k8s. For HA, point storage at S3 / MinIO once the S3 backend lands.

## Development

```bash
make test          # race + cover
make vet           # static analysis
make docker        # build container image
make review-spec   # run dhp-review on hn-daily spec
```

Tests live next to the code (`*_test.go`). Coverage targets ≥ 60% for rules
and HTTP handlers; current snapshot is captured in CI output.
