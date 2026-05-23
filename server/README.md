# DHP Server

Go service for the Digital Human Protocol v2 — two binaries built from
one codebase:

| Binary | Purpose | Invoked by |
|---|---|---|
| `dhp-review` | Offline review of a single `spec.yaml` bundle | GitHub Actions (`.github/workflows/review.yml`) |
| `dhp-registry` | HTTP daemon that hosts the registry + accepts publish | Enterprise deployments |

Both share the same 8 review rules, AI judge client, and configuration
schema.

> **Deployment**: see [`DEPLOY.md`](./DEPLOY.md) for the complete guide
> covering systemd, Docker, and Kubernetes paths (Chinese).

## Quickstart

```bash
# Build
make build

# Review a spec locally
./bin/dhp-review --spec=../packages/digital-humans/hn-daily/spec.yaml \
                 --config=deploy/config.example.yaml --format=markdown

# Run the registry (defaults to :8080)
cp deploy/config.example.yaml config.yaml      # edit auth.token first
./bin/dhp-registry --config=config.yaml
curl http://localhost:8080/healthz
```

## Module Layout

```
cmd/
  dhp-review/    review CLI (GitHub Actions entry)
  registry/      HTTP daemon (enterprise entry)
internal/
  ai/            OpenAI-compat AI judge client (SSE + non-SSE)
  auth/          Bearer-token auth (constant-time compare)
  config/        YAML config loader + defaults
  rules/         8 review rules + runner (severity ladder)
  spec/          spec.yaml parser + validator
  storage/       file storage iface (local + s3 stub)
deploy/
  Dockerfile, docker-compose.yml, config.example.yaml
  k8s/           Kubernetes manifests
  systemd/       systemd unit (bare-metal)
  scripts/       install.sh (bare-metal one-shot)
```

## Review Rules

| Rule | Type | Blocking |
|---|---|---|
| `schema_valid` | structural | yes |
| `slug_unique` | registry lookup | yes (semver-aware) |
| `dangerous_permissions` | pattern | yes (allow-listable) |
| `sensitive_categories` | keyword | flags for human |
| `skill_code_safety` | AI judge | yes on `fail` |
| `prompt_quality` | AI judge | warn-only |
| `metadata_compliance` | AI judge | warn-only |
| `duplicate_detection` | AI judge | warn-only |

Exit code: `0` pass / `1` warn / `2` fail / `3` runner error. The
GitHub Actions workflow uses these to drive label assignment and the
auto-merge gate.

## Configuration

See [`deploy/config.example.yaml`](./deploy/config.example.yaml) for
the full schema with comments, or [`DEPLOY.md` §七](./DEPLOY.md) for
field-by-field documentation.

## Testing

```bash
make test         # full unit + integration suite
make vet          # static checks
```

Coverage is around 60% across rules / ai / auth / config / storage /
registry handlers. The S3 storage backend is a compile-checked stub —
see `internal/storage/s3.go` for the wire-up TODO.
