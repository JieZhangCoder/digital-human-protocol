// Command registry runs the DHP enterprise registry HTTP server.
//
// Routes (Go 1.22 method+path patterns):
//
//	POST /apps                              -- multipart upload + sync review
//	GET  /digital-humans.json
//	GET  /skills.json
//	GET  /mcps.json
//	GET  /index.json                        -- legacy consolidated
//	GET  /apps/{slug...}/files/{path...}    -- artifact files (supports scoped slugs)
//	POST /internal/review                   -- loopback-only sync review
//	GET  /healthz
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/openkursar/digital-human-protocol/server/internal/auth"
	"github.com/openkursar/digital-human-protocol/server/internal/config"
	"github.com/openkursar/digital-human-protocol/server/internal/storage"
)

func main() {
	cfgPath := flag.String("config", "", "path to registry config yaml (required)")
	flag.Parse()
	if *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "usage: registry --config=path")
		os.Exit(2)
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	store, err := buildStorage(cfg)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	srv := NewServer(cfg, store)
	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      5 * time.Minute,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go func() {
		log.Printf("dhp-registry listening on %s (storage=%s, auth=%t)",
			cfg.Listen, cfg.Storage.Type, cfg.Auth.Token != "")
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http: %v", err)
		}
	}()
	<-ctx.Done()
	log.Println("shutting down…")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

func buildStorage(cfg *config.Config) (storage.Storage, error) {
	switch cfg.Storage.Type {
	case config.StorageLocal:
		root, _ := cfg.Storage.Config["path"].(string)
		if root == "" {
			root = "./data"
		}
		return storage.NewLocal(root)
	case config.StorageS3, config.StorageMinIO:
		sc := storage.S3Config{}
		if b, ok := cfg.Storage.Config["bucket"].(string); ok {
			sc.Bucket = b
		}
		if r, ok := cfg.Storage.Config["region"].(string); ok {
			sc.Region = r
		}
		if e, ok := cfg.Storage.Config["endpoint"].(string); ok {
			sc.Endpoint = e
		}
		if k, ok := cfg.Storage.Config["access_key_id"].(string); ok {
			sc.AccessKeyID = k
		}
		if s, ok := cfg.Storage.Config["secret_access_key"].(string); ok {
			sc.SecretAccessKey = s
		}
		if p, ok := cfg.Storage.Config["use_path_style"].(bool); ok {
			sc.UsePathStyle = p
		}
		return storage.NewS3(sc)
	}
	return nil, fmt.Errorf("unsupported storage type %q", cfg.Storage.Type)
}

// loopbackOnly wraps a handler to refuse non-loopback callers. Used for
// /internal/review so the registry can self-call without exposing the
// unauthenticated endpoint externally.
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			http.Error(w, "internal endpoint", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withAuth applies bearer-token auth, unless auth is disabled (empty token).
func withAuth(a *auth.TokenAuth, next http.Handler) http.Handler {
	if !a.Enabled() {
		return next
	}
	return a.Middleware(next)
}
