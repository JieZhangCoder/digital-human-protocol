package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Local persists artifacts under a single root directory.
//
// Keys are interpreted as relative POSIX paths. Path traversal (".." segments,
// absolute keys) is rejected so the registry cannot be tricked into writing
// outside its root.
type Local struct {
	root string
}

// NewLocal creates a directory-backed Storage. The root directory is created
// if absent.
func NewLocal(root string) (*Local, error) {
	if root == "" {
		return nil, errors.New("local storage: root path is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("local storage: resolve root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("local storage: mkdir root: %w", err)
	}
	return &Local{root: abs}, nil
}

// Root returns the absolute root directory backing this storage.
func (l *Local) Root() string { return l.root }

func (l *Local) resolve(key string) (string, error) {
	if key == "" {
		return "", errors.New("storage: key is empty")
	}
	if strings.Contains(key, "\x00") {
		return "", errors.New("storage: key contains NUL")
	}
	// Reject any segment that is ".." — filepath.Clean would silently collapse it.
	for _, seg := range strings.Split(filepath.ToSlash(key), "/") {
		if seg == ".." {
			return "", fmt.Errorf("storage: key %q escapes root", key)
		}
	}
	clean := filepath.ToSlash(filepath.Clean("/" + key))
	rel := strings.TrimPrefix(clean, "/")
	abs := filepath.Join(l.root, filepath.FromSlash(rel))
	// Final safety net: ensure the resolved path is within root.
	if !strings.HasPrefix(abs, l.root) {
		return "", fmt.Errorf("storage: key %q escapes root", key)
	}
	return abs, nil
}

// Put writes content to key, creating intermediate directories. size is
// advisory — Local trusts the reader instead of enforcing the length.
func (l *Local) Put(_ context.Context, key string, content io.Reader, _ int64) error {
	dest, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("local storage: mkdir: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(dest), ".tmp-*")
	if err != nil {
		return fmt.Errorf("local storage: tempfile: %w", err)
	}
	tmp := f.Name()
	if _, err := io.Copy(f, content); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("local storage: write: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("local storage: close: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("local storage: rename: %w", err)
	}
	return nil
}

// Get returns a reader for key. The caller must close the returned reader.
func (l *Local) Get(_ context.Context, key string) (io.ReadCloser, int64, error) {
	dest, err := l.resolve(key)
	if err != nil {
		return nil, 0, err
	}
	f, err := os.Open(dest)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, fmt.Errorf("local storage: open: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, fmt.Errorf("local storage: stat: %w", err)
	}
	return f, st.Size(), nil
}

// Delete removes the file at key. Missing keys are a no-op (returns nil).
func (l *Local) Delete(_ context.Context, key string) error {
	dest, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(dest); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("local storage: delete: %w", err)
	}
	return nil
}

// List returns all keys under prefix (POSIX-style).
func (l *Local) List(_ context.Context, prefix string) ([]string, error) {
	base, err := l.resolve(prefix)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(base)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("local storage: stat prefix: %w", err)
	}
	var out []string
	if !info.IsDir() {
		rel, _ := filepath.Rel(l.root, base)
		out = append(out, filepath.ToSlash(rel))
		return out, nil
	}
	err = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(l.root, path)
		if relErr != nil {
			return relErr
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("local storage: walk: %w", err)
	}
	return out, nil
}
