# Multi-stage build: Alpine builder → distroless runtime.
# Produces ~30 MiB images containing only the two static binaries and CA certs.

FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
# CGO disabled so the binaries are fully static and can run on distroless.
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/dhp-review ./cmd/dhp-review
RUN go build -trimpath -ldflags="-s -w" -o /out/registry ./cmd/registry

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/dhp-review /usr/local/bin/dhp-review
COPY --from=builder /out/registry   /usr/local/bin/registry
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/registry"]
CMD ["--config=/etc/dhp/config.yaml"]
