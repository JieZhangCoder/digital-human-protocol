package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openkursar/digital-human-protocol/server/internal/storage"
)

// HTTPRegistry implements RegistryLookup against a remote registry index.
//
// It fetches one of the canonical indexes — digital-humans.json, skills.json,
// mcps.json, or the legacy consolidated index.json — depending on the spec
// type. The base URL may be an http(s) URL or a local filesystem path; the
// latter is convenient for GitHub Actions where the workflow has the registry
// checked out in the workspace.
type HTTPRegistry struct {
	base string
	http *http.Client
}

// NewHTTPRegistry creates a lookup against an HTTP or file:// base. Examples:
//
//	NewHTTPRegistry("https://openkursar.github.io/digital-human-protocol")
//	NewHTTPRegistry("/workspace/digital-human-protocol")
func NewHTTPRegistry(base string) *HTTPRegistry {
	return &HTTPRegistry{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 30 * time.Second}}
}

// indexEntry is the partial schema we need from the canonical indexes.
type indexEntry struct {
	Slug    string `json:"slug"`
	Version string `json:"version"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Author  string `json:"author"`
}

// LookupSlug returns the registry entry for slug, or storage.ErrNotFound.
func (h *HTTPRegistry) LookupSlug(ctx context.Context, slug string) (*RegistryEntry, error) {
	if h == nil || h.base == "" {
		return nil, storage.ErrNotFound
	}
	for _, name := range []string{"digital-humans.json", "skills.json", "mcps.json", "index.json"} {
		entries, err := h.fetchIndex(ctx, name)
		if err != nil {
			// A missing file in the legacy migration window is not fatal;
			// only the consolidated index is guaranteed to exist.
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.Slug == slug {
				return &RegistryEntry{
					Slug:    e.Slug,
					Version: e.Version,
					Author:  e.Author,
					Name:    e.Name,
				}, nil
			}
		}
	}
	return nil, storage.ErrNotFound
}

// AllEntries returns every entry across all indexes — used by duplicate_detection.
func (h *HTTPRegistry) AllEntries(ctx context.Context) []map[string]any {
	if h == nil || h.base == "" {
		return nil
	}
	var out []map[string]any
	for _, name := range []string{"digital-humans.json", "skills.json", "mcps.json"} {
		entries, err := h.fetchIndex(ctx, name)
		if err != nil {
			continue
		}
		for _, e := range entries {
			out = append(out, map[string]any{
				"slug": e.Slug, "version": e.Version, "type": e.Type,
				"name": e.Name, "author": e.Author,
			})
		}
	}
	return out
}

func (h *HTTPRegistry) fetchIndex(ctx context.Context, name string) ([]indexEntry, error) {
	body, err := h.fetch(ctx, name)
	if err != nil {
		return nil, err
	}
	// Indexes may be a top-level array OR an object wrapping entries.
	var asArray []indexEntry
	if jsonErr := json.Unmarshal(body, &asArray); jsonErr == nil && asArray != nil {
		return asArray, nil
	}
	var asObject struct {
		Entries        []indexEntry `json:"entries"`
		DigitalHumans  []indexEntry `json:"digital_humans"`
		Skills         []indexEntry `json:"skills"`
		MCPs           []indexEntry `json:"mcps"`
		Apps           []indexEntry `json:"apps"`
	}
	if err := json.Unmarshal(body, &asObject); err != nil {
		return nil, fmt.Errorf("registry: parse %s: %w", name, err)
	}
	merged := asObject.Entries
	merged = append(merged, asObject.DigitalHumans...)
	merged = append(merged, asObject.Skills...)
	merged = append(merged, asObject.MCPs...)
	merged = append(merged, asObject.Apps...)
	return merged, nil
}

func (h *HTTPRegistry) fetch(ctx context.Context, name string) ([]byte, error) {
	// Local filesystem path?
	if !strings.Contains(h.base, "://") {
		path := filepath.Join(h.base, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, storage.ErrNotFound
			}
			return nil, fmt.Errorf("registry: read %s: %w", path, err)
		}
		return data, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.base+"/"+name, nil)
	if err != nil {
		return nil, fmt.Errorf("registry: build request: %w", err)
	}
	resp, err := h.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registry: GET %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, storage.ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("registry: status %d for %s", resp.StatusCode, name)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}
