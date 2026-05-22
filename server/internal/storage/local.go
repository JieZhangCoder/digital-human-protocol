package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LocalProvider stores files on disk.
type LocalProvider struct {
	BasePath string
}

// NewLocalProvider creates the base directory if needed.
func NewLocalProvider(basePath string) *LocalProvider {
	_ = os.MkdirAll(basePath, 0755)
	return &LocalProvider{BasePath: basePath}
}

// Save writes data to {base}/{slug}@{version}/{filename}.
func (p *LocalProvider) Save(slug, version, filename string, data []byte) (string, error) {
	dir := filepath.Join(p.BasePath, fmt.Sprintf("%s@%s", slug, version))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, filename)
	return path, os.WriteFile(path, data, 0644)
}

// Open reads a file under {base}/{slug}@{version}/{filename}.
func (p *LocalProvider) Open(slug, version, filename string) ([]byte, error) {
	return os.ReadFile(p.FilePath(slug, version, filename))
}

// List scans the base directory recursively for {slug}@{version} folders.
func (p *LocalProvider) List() ([]AppRef, error) {
	var refs []AppRef
	err := filepath.Walk(p.BasePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		name := info.Name()
		if !strings.Contains(name, "@") {
			return nil
		}
		rel, _ := filepath.Rel(p.BasePath, path)
		rel = strings.ReplaceAll(rel, "\\", "/")
		at := strings.LastIndex(rel, "@")
		if at < 0 {
			return nil
		}
		refs = append(refs, AppRef{
			Slug:    rel[:at],
			Version: rel[at+1:],
			Path:    path,
		})
		return nil
	})
	return refs, err
}

// FilePath returns the absolute on-disk path for a stored file.
func (p *LocalProvider) FilePath(slug, version, filename string) string {
	return filepath.Join(p.BasePath, fmt.Sprintf("%s@%s", slug, version), filename)
}
