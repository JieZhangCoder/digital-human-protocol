package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"

	"github.com/openkursar/digital-human-protocol/server/internal/ai"
	"github.com/openkursar/digital-human-protocol/server/internal/config"
	"github.com/openkursar/digital-human-protocol/server/internal/rules"
	"github.com/openkursar/digital-human-protocol/server/internal/storage"
)

var (
	cfg    *config.Config
	store  *storage.LocalProvider
	engine *rules.Engine
)

func main() {
	var configPath string
	flag := os.Args
	for i, arg := range flag {
		if arg == "--config" && i+1 < len(flag) {
			configPath = flag[i+1]
		}
	}
	if configPath == "" {
		configPath = "config.yaml"
	}

	var err error
	cfg, err = config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	basePath := "./halo-data"
	if p, ok := cfg.Storage.Config["path"].(string); ok {
		basePath = p
	}
	store = storage.NewLocalProvider(basePath)
	judge := ai.NewJudge(cfg.AIJudge.Endpoint, cfg.AIJudge.Model, cfg.AIJudge.APIKey)
	engine = rules.NewEngine(cfg.Rules.Enabled, judge)

	r := mux.NewRouter()
	r.HandleFunc("/apps", requireToken(uploadApp)).Methods("POST")
	r.HandleFunc("/digital-humans.json", serveIndex).Methods("GET")
	r.HandleFunc("/skills.json", serveIndex).Methods("GET")
	r.HandleFunc("/mcps.json", serveIndex).Methods("GET")
	r.HandleFunc("/apps/{slug:.*}/{version}/files/{file_path:.*}", serveFile).Methods("GET")

	port := strings.TrimPrefix(cfg.Listen, ":")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port
	fmt.Printf("DHP Registry listening on %s\n", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth.Type == "token" && cfg.Auth.Token != "" {
			header := r.Header.Get("Authorization")
			token := strings.TrimPrefix(header, "Bearer ")
			if token != cfg.Auth.Token {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func uploadApp(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	slug := r.FormValue("slug")
	version := r.FormValue("version")
	if slug == "" || version == "" {
		http.Error(w, `{"error":"slug and version required"}`, http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("dhpkg")
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}
	defer file.Close()

	raw, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid dhpkg: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	var spec map[string]interface{}
	files := make(map[string][]byte)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		if f.Name == "spec.yaml" {
			_ = yaml.Unmarshal(data, &spec)
		} else {
			files[f.Name] = data
		}
	}
	if spec == nil {
		http.Error(w, `{"error":"spec.yaml not found in dhpkg"}`, http.StatusBadRequest)
		return
	}

	existing, _ := store.List()
	inp := &rules.Input{
		Slug:     slug,
		Version:  version,
		Spec:     spec,
		Files:    files,
		Existing: existing,
	}
	results := engine.Run(inp)
	v := rules.Verdict(results, cfg.AutoMergeThreshold)

	if v == "approved" {
		for name, body := range files {
			_, _ = store.Save(slug, version, name, body)
		}
		specYAML, _ := yaml.Marshal(spec)
		_, _ = store.Save(slug, version, "spec.yaml", specYAML)
	}

	w.Header().Set("Content-Type", "application/json")
	out := map[string]interface{}{
		"slug":    slug,
		"version": version,
		"verdict": v,
		"comment": rules.CommentFromResults(results),
		"results": results,
	}
	_ = json.NewEncoder(w).Encode(out)
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	appType := strings.TrimSuffix(path, ".json")

	// URL slug -> spec type mapping
	switch appType {
	case "digital-humans":
		appType = "digital-human"
	case "skills":
		appType = "skill"
	case "mcps":
		appType = "mcp"
	}

	entries, _ := store.List()
	apps := []map[string]interface{}{}
	for _, ref := range entries {
		specPath := filepath.Join(ref.Path, "spec.yaml")
		data, err := os.ReadFile(specPath)
		if err != nil {
			continue
		}
		var spec map[string]interface{}
		if err := yaml.Unmarshal(data, &spec); err != nil {
			continue
		}
		if spec == nil {
			continue
		}
		if appType != "index" && spec["type"] != appType {
			continue
		}

		// Required fields per Halo client RegistryEntrySchema:
		//   slug, name, version, author, description, type, format, path, category, tags
		// `path` is the directory (relative to base URL) where spec.yaml lives;
		// it must match the static file route so client can fetch `{path}/spec.yaml`.
		entry := map[string]interface{}{
			"slug":        ref.Slug,
			"version":     ref.Version,
			"name":        spec["name"],
			"type":        spec["type"],
			"author":      spec["author"],
			"description": spec["description"],
			"format":      "bundle",
			"path":        fmt.Sprintf("apps/%s/%s/files", ref.Slug, ref.Version),
			"category":    stringOr(spec["category"], "other"),
			"tags":        stringSliceOr(spec["tags"]),
		}
		// Optional pass-through fields (only set when present in spec).
		copyIfPresent(entry, spec, "icon")
		copyIfPresent(entry, spec, "locale")
		copyIfPresent(entry, spec, "i18n")
		copyIfPresent(entry, spec, "created_at")
		copyIfPresent(entry, spec, "updated_at")
		apps = append(apps, entry)
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}
	source := fmt.Sprintf("%s://%s", scheme, r.Host)

	out := map[string]interface{}{
		"version":      1,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"source":       source,
		"apps":         apps,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// stringOr returns v as string when non-empty, otherwise fallback.
func stringOr(v interface{}, fallback string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return fallback
}

// stringSliceOr coerces a YAML-decoded list (`[]interface{}`) into `[]string`.
// Non-string elements are skipped. Returns an empty (non-nil) slice on miss
// so JSON encodes `[]` instead of `null`.
func stringSliceOr(v interface{}) []string {
	out := []string{}
	if list, ok := v.([]interface{}); ok {
		for _, item := range list {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// copyIfPresent copies spec[key] into entry[key] when the source value is not nil.
func copyIfPresent(entry, spec map[string]interface{}, key string) {
	if v, ok := spec[key]; ok && v != nil {
		entry[key] = v
	}
}

func serveFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slug := vars["slug"]
	version := vars["version"]
	filePath := vars["file_path"]

	fullPath := store.FilePath(slug, version, filePath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, fullPath)
}
