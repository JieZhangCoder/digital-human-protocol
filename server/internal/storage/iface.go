// Package storage abstracts artifact persistence for the registry.
//
// The interface intentionally mirrors a minimal subset of S3 semantics so
// production deployments can swap between local disk, MinIO, and S3 without
// changing handler code.
package storage

import (
	"context"
	"errors"
	"io"
)

// ErrNotFound is returned by Get when the key does not exist.
var ErrNotFound = errors.New("storage: key not found")

// ErrNotImplemented is returned by stub backends (e.g. unconfigured S3).
var ErrNotImplemented = errors.New("storage: backend not implemented")

// Storage is the persistence contract for the registry artifact store.
type Storage interface {
	Put(ctx context.Context, key string, content io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}
