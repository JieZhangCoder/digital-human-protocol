package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/openkursar/digital-human-protocol/server/internal/ai"
	"github.com/openkursar/digital-human-protocol/server/internal/auth"
	"github.com/openkursar/digital-human-protocol/server/internal/config"
	"github.com/openkursar/digital-human-protocol/server/internal/rules"
	"github.com/openkursar/digital-human-protocol/server/internal/spec"
	"github.com/openkursar/digital-human-protocol/server/internal/storage"
)

// Server is the registry HTTP server. It is intentionally split from main()
// so tests can construct one with an httptest.Server.
type Server struct {
	cfg     *config.Config
	store   storage.Storage
	auth    *auth.TokenAuth
	ai      *ai.Client
	mu      sync.RWMutex // guards in-memory index
	indexes map[string][]indexEntry
}

type indexEntry struct {
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Type        string    `json:"type"`
	// Format is the packaging format of the published artifact. DHP v2 only
	// defines "bundle" (spec.yaml + auxiliary files). Halo's adapter requires
	// this field — keep it stable even though we currently have a single value.
	Format      string    `json:"format"`
	Author      string    `json:"author"`
	Description string    `json:"description"`
	Path        string    `json:"path"`
	SizeBytes   int64     `json:"size_bytes"`
	Checksum    string    `json:"checksum"`
	Tags        []string  `json:"tags,omitempty"`
	Locale      string    `json:"locale,omitempty"`
	License     string    `json:"license,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NewServer builds a Server with the supplied dependencies.
func NewServer(cfg *config.Config, store storage.Storage) *Server {
	return &Server{
		cfg:     cfg,
		store:   store,
		auth:    auth.NewTokenAuth(cfg.Auth.Token),
		ai:      ai.New(cfg.AIJudge.Endpoint, cfg.AIJudge.Model, cfg.AIJudge.APIKey),
		indexes: map[string][]indexEntry{},
	}
}

// Handler wires the routes and returns the root http.Handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)

	mux.Handle("POST /apps", withAuth(s.auth, http.HandlerFunc(s.handlePublish)))

	mux.HandleFunc("GET /digital-humans.json", s.serveIndex("digital-humans.json", "automation"))
	mux.HandleFunc("GET /skills.json", s.serveIndex("skills.json", "skill"))
	mux.HandleFunc("GET /mcps.json", s.serveIndex("mcps.json", "mcp"))
	mux.HandleFunc("GET /index.json", s.serveLegacyIndex)

	// Artifact files. Scoped slugs (author/id) are differentiated from the
	// flat slug form at request time. Using two ServeMux patterns is
	// ambiguous in Go 1.22's stdlib mux, so we mount a single handler at
	// /apps/ and parse the remainder ourselves.
	mux.HandleFunc("GET /apps/", s.handleArtifactRoute)

	mux.Handle("POST /internal/review", loopbackOnly(http.HandlerFunc(s.handleInternalReview)))

	return logMiddleware(mux)
}

// logMiddleware emits a one-line summary per request — production-ready
// log volume without pulling in a logging dependency.
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		fmt.Printf("%s %s %d %s %s\n",
			r.Method, r.URL.Path, rw.status, time.Since(start), r.RemoteAddr)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(c int) { r.status = c; r.ResponseWriter.WriteHeader(c) }

// ---- handlers --------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := r.ParseMultipartForm(s.cfg.MaxUploadBytes); err != nil {
		http.Error(w, "parse multipart: "+err.Error(), http.StatusBadRequest)
		return
	}
	specFile, _, err := r.FormFile("spec")
	if err != nil {
		http.Error(w, "missing 'spec' file part", http.StatusBadRequest)
		return
	}
	defer specFile.Close()
	specBytes, err := io.ReadAll(io.LimitReader(specFile, s.cfg.MaxUploadBytes))
	if err != nil {
		http.Error(w, "read spec: "+err.Error(), http.StatusBadRequest)
		return
	}
	parsed, err := spec.Parse(specBytes)
	if err != nil {
		http.Error(w, "parse spec: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := spec.Validate(parsed); err != nil {
		http.Error(w, "validate spec: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Enforce scoped slug ("author/app-id") and author-prefix consistency.
	if err := spec.ValidatePublish(parsed); err != nil {
		http.Error(w, "publish validation: "+err.Error(), http.StatusBadRequest)
		return
	}

	skillFiles := map[string][]byte{}
	if r.MultipartForm != nil {
		for name, files := range r.MultipartForm.File {
			if name == "spec" {
				continue
			}
			for _, fh := range files {
				f, err := fh.Open()
				if err != nil {
					http.Error(w, "open part "+name+": "+err.Error(), http.StatusBadRequest)
					return
				}
				data, err := io.ReadAll(io.LimitReader(f, s.cfg.MaxUploadBytes))
				f.Close()
				if err != nil {
					http.Error(w, "read part "+name+": "+err.Error(), http.StatusBadRequest)
					return
				}
				skillFiles[fh.Filename] = data
			}
		}
	}

	// Run synchronous review.
	report := s.runReview(ctx, parsed, specBytes, skillFiles)
	if report.Overall >= rules.SeverityFail {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"slug": parsed.Slug(), "version": parsed.Version,
			"verdict": report.OverallText, "report": report,
		})
		return
	}

	// Store artifacts under apps/<slug>/<version>/...
	keyBase := path.Join("apps", parsed.Slug(), parsed.Version)
	if err := s.store.Put(ctx, path.Join(keyBase, "spec.yaml"), bytes.NewReader(specBytes), int64(len(specBytes))); err != nil {
		http.Error(w, "store spec: "+err.Error(), http.StatusInternalServerError)
		return
	}
	var totalSize int64 = int64(len(specBytes))
	hasher := sha256.New()
	hasher.Write(specBytes)
	for name, data := range skillFiles {
		key := path.Join(keyBase, "files", name)
		if err := s.store.Put(ctx, key, bytes.NewReader(data), int64(len(data))); err != nil {
			http.Error(w, "store file: "+err.Error(), http.StatusInternalServerError)
			return
		}
		totalSize += int64(len(data))
		hasher.Write(data)
	}
	checksum := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	s.upsertIndex(parsed, keyBase, totalSize, checksum)

	writeJSON(w, http.StatusOK, map[string]any{
		"slug": parsed.Slug(), "version": parsed.Version,
		"verdict": report.OverallText, "report": report,
		"path": keyBase, "checksum": checksum, "size_bytes": totalSize,
	})
}

func (s *Server) runReview(ctx context.Context, parsed *spec.Spec, raw []byte, skillFiles map[string][]byte) *rules.Report {
	registry := rules.NewHTTPRegistry(s.cfg.RegistryURL)
	return rules.Run(ctx, &rules.Options{
		Spec:        parsed,
		SpecBytes:   raw,
		SkillFiles:  skillFiles,
		Config:      s.cfg,
		AI:          s.ai,
		Registry:    registry,
		RawRegistry: registry.AllEntries(ctx),
	})
}

func (s *Server) handleInternalReview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := io.ReadAll(io.LimitReader(r.Body, s.cfg.MaxUploadBytes))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	parsed, err := spec.Parse(body)
	if err != nil {
		http.Error(w, "parse spec: "+err.Error(), http.StatusBadRequest)
		return
	}
	report := s.runReview(ctx, parsed, body, nil)
	writeJSON(w, http.StatusOK, report)
}

// envelope is the canonical registry index envelope shape the Halo client
// (RegistryIndexSchema in halo.adapter.ts) and the static build-index.mjs
// pipeline both produce. Keeping the registry HTTP server in sync with this
// shape is what lets clients consume hosted indexes interchangeably whether
// the source is a GitHub Pages mirror or the enterprise registry daemon.
type envelope struct {
	Version     int          `json:"version"`
	GeneratedAt string       `json:"generated_at"`
	Source      string       `json:"source"`
	Apps        []indexEntry `json:"apps"`
}

// requestSource derives the index "source" URL from the incoming request so
// clients can build artifact URLs without needing the registry's external
// hostname configured anywhere. Honours X-Forwarded-Proto when behind a TLS
// terminator.
func requestSource(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}

func (s *Server) serveIndex(name, typeFilter string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.RLock()
		entries := append([]indexEntry{}, s.indexes[typeFilter]...)
		s.mu.RUnlock()
		sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
		writeJSON(w, http.StatusOK, envelope{
			Version:     1,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Source:      requestSource(r),
			Apps:        entries,
		})
	}
}

func (s *Server) serveLegacyIndex(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	merged := []indexEntry{}
	for _, list := range s.indexes {
		merged = append(merged, list...)
	}
	s.mu.RUnlock()
	sort.Slice(merged, func(i, j int) bool { return merged[i].Slug < merged[j].Slug })

	// Legacy index uses the same envelope as the split files, with a
	// `deprecated` flag tacked on so clients can warn / migrate. The
	// canonical fields stay where clients expect them.
	type legacyEnvelope struct {
		envelope
		Deprecated bool   `json:"deprecated"`
		Notice     string `json:"notice"`
	}
	writeJSON(w, http.StatusOK, legacyEnvelope{
		envelope: envelope{
			Version:     1,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Source:      requestSource(r),
			Apps:        merged,
		},
		Deprecated: true,
		Notice:     "use digital-humans.json / skills.json / mcps.json directly",
	})
}

// handleArtifactRoute serves every file inside a published bundle directory.
//
// Accepted URL shapes:
//
//	/apps/<slug>/<version>/spec.yaml                  (flat slug)
//	/apps/<slug>/<version>/files/<path>               (flat slug)
//	/apps/<author>/<id>/<version>/spec.yaml           (scoped slug)
//	/apps/<author>/<id>/<version>/files/<path>        (scoped slug)
//
// The path tail after <head> = <slug>/<version> (flat) or
// <author>/<id>/<version> (scoped) maps verbatim into the bundle directory
// at apps/<head>/<tail>. The whole bundle layout (spec.yaml at root, files/
// for auxiliary uploads) is exposed as static files; we don't expose any
// additional metadata routes.
//
// Halo's client adapter requests both spec.yaml and files/ files via this
// route, so any change here must be mirrored by the route tests in
// server_test.go (TestArtifactRouteShapes) to avoid silent 404s breaking the
// store detail view and install flow.
func (s *Server) handleArtifactRoute(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/apps/")
	head, tail, ok := splitArtifactPath(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.serveFile(w, r, path.Join("apps", head, tail))
}

// splitArtifactPath separates "<head>/<tail>" where head is either
// "<slug>/<version>" or "<author>/<id>/<version>" and tail is everything
// inside the published bundle (spec.yaml, files/SKILL.md, files/x/y.md, …).
//
// We anchor on the well-known tail prefixes ("spec.yaml" or "files/") so
// scoped slugs (which contain a single extra '/') are unambiguous.
func splitArtifactPath(rest string) (head, tail string, ok bool) {
	switch {
	case strings.HasSuffix(rest, "/spec.yaml"):
		head = strings.TrimSuffix(rest, "/spec.yaml")
		tail = "spec.yaml"
	default:
		idx := strings.Index(rest, "/files/")
		if idx < 0 {
			return "", "", false
		}
		head = rest[:idx]
		tail = "files/" + rest[idx+len("/files/"):]
	}
	// head must be exactly <slug>/<version> (2 segments) or
	// <author>/<id>/<version> (3 segments).
	switch strings.Count(head, "/") {
	case 1, 2:
		return head, tail, true
	default:
		return "", "", false
	}
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, key string) {
	// Guard against traversal.
	if strings.Contains(key, "..") {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	rc, size, err := s.store.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "fetch: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	if size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	}
	_, _ = io.Copy(w, rc)
}

// ---- helpers ---------------------------------------------------------------

func (s *Server) upsertIndex(parsed *spec.Spec, keyBase string, size int64, checksum string) {
	entry := indexEntry{
		Slug:        parsed.Slug(),
		Name:        parsed.Name,
		Version:     parsed.Version,
		Type:        string(parsed.Type),
		Format:      "bundle",
		Author:      parsed.Author,
		Description: parsed.Description,
		Path:        keyBase,
		SizeBytes:   size,
		Checksum:    checksum,
		UpdatedAt:   time.Now().UTC(),
	}
	if parsed.Store != nil {
		entry.Tags = parsed.Store.Tags
		entry.Locale = parsed.Store.Locale
		entry.License = parsed.Store.License
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket := entry.Type
	list := s.indexes[bucket]
	replaced := false
	for i, e := range list {
		if e.Slug == entry.Slug {
			list[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, entry)
	}
	s.indexes[bucket] = list
}

// RebuildIndexes scans the storage backend on boot and reconstructs the
// in-memory indexes from the persisted `apps/<slug>/<version>/spec.yaml`
// files. Without this, a process restart leaves the registry serving empty
// index responses until the next successful publish.
//
// The function is intentionally tolerant: a single corrupted spec is logged
// and skipped rather than aborting startup. Total cost is O(N) where N is
// the number of published versions — fine for thousands of apps, deferred
// to a snapshot file only if that ceiling becomes a problem.
func (s *Server) RebuildIndexes(ctx context.Context) error {
	keys, err := s.store.List(ctx, "apps")
	if err != nil {
		return fmt.Errorf("list storage: %w", err)
	}

	// Group keys by their owning <slug>/<version> base so we can compute a
	// per-version checksum + size in one pass over the file set. A slug may
	// be flat ("hn-daily") or scoped ("author/id") — we recover this from
	// the storage key by treating everything between "apps/" and "/<version>"
	// as the slug.
	type bundleKey struct {
		slug, version string
	}
	bundles := map[bundleKey]struct {
		specKey string
		files   []string
	}{}

	for _, k := range keys {
		// Expected key shapes:
		//   apps/<slug>/<version>/spec.yaml
		//   apps/<slug>/<version>/files/<name>      (slug may contain '/')
		rest := strings.TrimPrefix(k, "apps/")
		if rest == k {
			continue
		}
		// Find the boundary between <version> and the trailing path.
		// We anchor on either "/spec.yaml" at the end or "/files/" mid-string.
		var slug, version, tail string
		switch {
		case strings.HasSuffix(rest, "/spec.yaml"):
			head := strings.TrimSuffix(rest, "/spec.yaml")
			i := strings.LastIndex(head, "/")
			if i <= 0 {
				continue
			}
			slug, version = head[:i], head[i+1:]
			tail = "spec.yaml"
		default:
			i := strings.Index(rest, "/files/")
			if i <= 0 {
				continue
			}
			head := rest[:i]
			j := strings.LastIndex(head, "/")
			if j <= 0 {
				continue
			}
			slug, version = head[:j], head[j+1:]
			tail = rest[i+1:] // "files/<name>"
		}

		bk := bundleKey{slug, version}
		b := bundles[bk]
		if tail == "spec.yaml" {
			b.specKey = k
		} else {
			b.files = append(b.files, k)
		}
		bundles[bk] = b
	}

	rebuilt := 0
	skipped := 0
	for bk, b := range bundles {
		if b.specKey == "" {
			log.Printf("[index-rebuild] skip %s@%s: no spec.yaml", bk.slug, bk.version)
			skipped++
			continue
		}

		rc, _, err := s.store.Get(ctx, b.specKey)
		if err != nil {
			log.Printf("[index-rebuild] skip %s@%s: read spec failed: %v", bk.slug, bk.version, err)
			skipped++
			continue
		}
		specBytes, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			log.Printf("[index-rebuild] skip %s@%s: read spec body failed: %v", bk.slug, bk.version, err)
			skipped++
			continue
		}

		parsed, err := spec.Parse(specBytes)
		if err != nil {
			log.Printf("[index-rebuild] skip %s@%s: parse failed: %v", bk.slug, bk.version, err)
			skipped++
			continue
		}

		// Verify the stored slug matches what the directory says — defensive
		// guard against tampered filesystems.
		if got := parsed.Slug(); got != bk.slug {
			log.Printf("[index-rebuild] skip %s@%s: slug mismatch (spec says %q)", bk.slug, bk.version, got)
			skipped++
			continue
		}

		// Recompute size + checksum across spec + files (same order as publish).
		hasher := sha256.New()
		var totalSize int64
		hasher.Write(specBytes)
		totalSize += int64(len(specBytes))

		// Sort files for stable hash ordering — different list ordering must
		// not produce different checksums.
		sort.Strings(b.files)
		for _, fk := range b.files {
			frc, fsize, err := s.store.Get(ctx, fk)
			if err != nil {
				log.Printf("[index-rebuild] skip %s@%s: read file %s failed: %v", bk.slug, bk.version, fk, err)
				skipped++
				goto nextBundle
			}
			data, err := io.ReadAll(frc)
			frc.Close()
			if err != nil {
				log.Printf("[index-rebuild] skip %s@%s: read file body %s failed: %v", bk.slug, bk.version, fk, err)
				skipped++
				goto nextBundle
			}
			hasher.Write(data)
			if fsize > 0 {
				totalSize += fsize
			} else {
				totalSize += int64(len(data))
			}
		}

		s.upsertIndex(parsed, path.Join("apps", bk.slug, bk.version), totalSize, "sha256:"+hex.EncodeToString(hasher.Sum(nil)))
		rebuilt++
	nextBundle:
	}

	log.Printf("[index-rebuild] complete: rebuilt=%d skipped=%d (digital-humans=%d skills=%d mcps=%d)",
		rebuilt, skipped,
		len(s.indexes["automation"]), len(s.indexes["skill"]), len(s.indexes["mcp"]))
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(body)
}
