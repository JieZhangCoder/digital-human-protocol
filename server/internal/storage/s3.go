package storage

import (
	"context"
	"io"
)

// S3 is a compile-checked stub for S3-compatible storage (AWS S3, MinIO).
//
// TODO: Wire this up to aws-sdk-go-v2 and verify against real credentials
// before launch. The plan calls for a pluggable backend so enterprise
// deployments can drop in their existing object store; for now we keep the
// surface area intact and return ErrNotImplemented so the registry refuses to
// silently lose data.
//
// To implement:
//  1. Add github.com/aws/aws-sdk-go-v2/{config,service/s3} to go.mod.
//  2. Resolve a *s3.Client in NewS3 using shared config or static credentials
//     from S3Config.
//  3. Implement Put/Get/Delete/List using the SDK (mind multipart upload for
//     large files; the registry's max artifact size is configured at the HTTP
//     layer).
//  4. Add an integration test guarded by AWS credentials env vars.
type S3 struct {
	cfg S3Config
}

// S3Config captures the connection parameters. Equally suitable for MinIO via
// the Endpoint override.
type S3Config struct {
	Bucket          string
	Region          string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	UsePathStyle    bool
}

// NewS3 constructs a stub S3 backend. The stub fails every method with
// ErrNotImplemented but compiles cleanly so callers can wire it in advance.
func NewS3(cfg S3Config) (*S3, error) { return &S3{cfg: cfg}, nil }

func (s *S3) Put(context.Context, string, io.Reader, int64) error {
	return ErrNotImplemented
}

func (s *S3) Get(context.Context, string) (io.ReadCloser, int64, error) {
	return nil, 0, ErrNotImplemented
}

func (s *S3) Delete(context.Context, string) error { return ErrNotImplemented }

func (s *S3) List(context.Context, string) ([]string, error) {
	return nil, ErrNotImplemented
}
