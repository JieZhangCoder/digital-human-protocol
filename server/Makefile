# DHP server Makefile
#
# Targets:
#   build       compile dhp-review and registry to ./bin
#   test        run unit + integration tests with coverage
#   vet         go vet ./...
#   lint        same as vet (golangci-lint optional; not vendored)
#   docker      build the container image
#   clean       remove ./bin and coverage artifacts
#   review-spec run dhp-review against a sample spec (SPEC=path)

GO         ?= go
PKGS       := ./...
BIN_DIR    := bin
IMAGE      ?= ghcr.io/openkursar/dhp-registry:dev
SPEC       ?= ../packages/digital-humans/hn-daily/spec.yaml
CONFIG     ?= deploy/config.example.yaml

.PHONY: build test vet lint docker clean review-spec

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/dhp-review ./cmd/dhp-review
	$(GO) build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/registry   ./cmd/registry

test:
	$(GO) test -race -cover $(PKGS)

vet:
	$(GO) vet $(PKGS)

lint: vet

docker:
	docker build -t $(IMAGE) -f deploy/Dockerfile .

clean:
	rm -rf $(BIN_DIR) coverage.out

review-spec: build
	./$(BIN_DIR)/dhp-review --spec=$(SPEC) --config=$(CONFIG) --format=markdown
