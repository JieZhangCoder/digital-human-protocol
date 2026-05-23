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
	if parsed.Slug() == "" || parsed.Version == "" {
		http.Error(w, "spec.store.slug and version are required", http.StatusBadRequest)
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

func (s *Server) serveIndex(name, typeFilter string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		s.mu.RLock()
		entries := append([]indexEntry(nil), s.indexes[typeFilter]...)
		s.mu.RUnlock()
		sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
		writeJSON(w, http.StatusOK, entries)
	}
}

func (s *Server) serveLegacyIndex(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	var merged []indexEntry
	for _, list := range s.indexes {
		merged = append(merged, list...)
	}
	s.mu.RUnlock()
	sort.Slice(merged, func(i, j int) bool { return merged[i].Slug < merged[j].Slug })
	writeJSON(w, http.StatusOK, map[string]any{
		"deprecated": true,
		"notice":     "use digital-humans.json / skills.json / mcps.json directly",
		"entries":    merged,
	})
}

// handleArtifactRoute parses /apps/{slug-or-author}/{id-or-version}/files/...
// and /apps/{author}/{id}/{version}/files/... and forwards to serveFile.
func (s *Server) handleArtifactRoute(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/apps/")
	idx := strings.Index(rest, "/files/")
	if idx < 0 {
		http.NotFound(w, r)
		return
	}
	head := rest[:idx]
	tail := rest[idx+len("/files/"):]
	segments := strings.Split(head, "/")
	var slug, version string
	switch len(segments) {
	case 2:
		slug, version = segments[0], segments[1]
	case 3:
		slug = segments[0] + "/" + segments[1]
		version = segments[2]
	default:
		http.NotFound(w, r)
		return
	}
	s.serveFile(w, r, path.Join("apps", slug, version, "files", tail))
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

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(body)
}
