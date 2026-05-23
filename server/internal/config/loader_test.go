package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := `
storage:
  type: local
  config: { path: ./data }
auth:
  type: token
  token: hello
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Listen != ":8080" {
		t.Errorf("default listen: %s", cfg.Listen)
	}
	if !cfg.IsRuleEnabled("schema_valid") {
		t.Errorf("schema_valid should default to enabled")
	}
	if cfg.IsRuleEnabled("nonexistent") {
		t.Errorf("unknown rule should not be enabled")
	}
}

func TestLoadRejectsUnknownStorage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := `
storage:
  type: nfs
auth:
  type: token
`
	_ = os.WriteFile(path, []byte(body), 0o644)
	if _, err := Load(path); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	// Missing config files boot with built-in defaults (logged to stderr)
	// so first-time installs succeed without manual config.
	cfg, err := Load("/no/such/path")
	if err != nil {
		t.Fatalf("missing file should fall back to defaults, got error: %v", err)
	}
	if cfg.Listen != ":8080" {
		t.Errorf("default listen: %s", cfg.Listen)
	}
	if !cfg.IsRuleEnabled("schema_valid") {
		t.Errorf("schema_valid should be enabled in defaults")
	}
}

func TestLoadEmptyPathFails(t *testing.T) {
	if _, err := Load(""); err == nil {
		t.Fatalf("expected error for empty path")
	}
}
