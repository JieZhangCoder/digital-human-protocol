# Build stage
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o dhp-review ./cmd/dhp-review
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o dhp-registry ./cmd/registry

# Runtime stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/dhp-review /usr/local/bin/
COPY --from=builder /app/dhp-registry /usr/local/bin/
COPY config.yaml /app/
RUN mkdir -p /app/halo-data
EXPOSE 8080
CMD ["dhp-registry", "--config", "/app/config.yaml"]
