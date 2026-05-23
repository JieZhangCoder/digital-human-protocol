// Package config loads and validates the registry / review-CLI YAML config.
//
// The schema matches DHP-V2-PLAN.md §10 and is shared between the dhp-review
// CLI (which uses storage/auth-less subsets) and the registry HTTP server.
// Defaults are applied so a minimal config file is enough to boot.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// StorageType enumerates supported backends.
type StorageType string

const (
	StorageLocal StorageType = "local"
	StorageS3    StorageType = "s3"
	StorageMinIO StorageType = "minio" // alias of s3 (path-style)
)

// Config is the top-level configuration.
type Config struct {
	Listen             string        `yaml:"listen"`
	Storage            StorageBlock  `yaml:"storage"`
	AIJudge            AIJudgeBlock  `yaml:"ai_judge"`
	Rules              RulesBlock    `yaml:"rules"`
	Auth               AuthBlock     `yaml:"auth"`
	AutoMergeThreshold string        `yaml:"auto_merge_threshold"`
	RegistryURL        string        `yaml:"registry_url"`
	RequestTimeout     time.Duration `yaml:"request_timeout"`
	MaxUploadBytes     int64         `yaml:"max_upload_bytes"`
}

// StorageBlock configures the artifact backend.
type StorageBlock struct {
	Type   StorageType    `yaml:"type"`
	Config map[string]any `yaml:"config,omitempty"`
}

// AIJudgeBlock configures the AI judge transport.
type AIJudgeBlock struct {
	Endpoint string `yaml:"endpoint,omitempty"`
	Model    string `yaml:"model,omitempty"`
	APIKey   string `yaml:"api_key,omitempty"`
}

// RulesBlock controls which review rules run.
type RulesBlock struct {
	Enabled     []string `yaml:"enabled,omitempty"`
	Disabled    []string `yaml:"disabled,omitempty"`
	CustomRules []string `yaml:"custom_rules,omitempty"`
	Allowlist   struct {
		DangerousPermissions []string `yaml:"dangerous_permissions,omitempty"`
	} `yaml:"allowlist,omitempty"`
	SensitiveKeywords []string `yaml:"sensitive_keywords,omitempty"`
}

// AuthBlock controls registry authentication.
type AuthBlock struct {
	Type  string `yaml:"type"`
	Token string `yaml:"token,omitempty"`
}

// Load reads a YAML file and applies defaults. Missing files return an error
// because we'd rather fail loud than silently boot with no rules.
func Load(path string) (*Config, error) {
	if path == "" {
		return nil, errors.New("config: path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("config: validate %s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.Storage.Type == "" {
		c.Storage.Type = StorageLocal
	}
	if c.AutoMergeThreshold == "" {
		c.AutoMergeThreshold = "low_risk"
	}
	if c.RequestTimeout == 0 {
		c.RequestTimeout = 60 * time.Second
	}
	if c.MaxUploadBytes == 0 {
		c.MaxUploadBytes = 25 << 20 // 25 MiB
	}
	if c.Auth.Type == "" {
		c.Auth.Type = "token"
	}
	if len(c.Rules.Enabled) == 0 {
		c.Rules.Enabled = []string{
			"schema_valid",
			"slug_unique",
			"dangerous_permissions",
			"skill_code_safety",
			"prompt_quality",
			"metadata_compliance",
			"duplicate_detection",
			"sensitive_categories",
		}
	}
	if len(c.Rules.SensitiveKeywords) == 0 {
		c.Rules.SensitiveKeywords = []string{
			"financial advice", "investment", "stock pick", "medical diagnosis",
			"prescription", "election", "voting", "weapons", "self-harm",
			"金融建议", "投资建议", "选股", "医疗诊断", "处方", "选举",
		}
	}
}

func (c *Config) validate() error {
	switch c.Storage.Type {
	case StorageLocal, StorageS3, StorageMinIO:
	default:
		return fmt.Errorf("unknown storage.type %q", c.Storage.Type)
	}
	if c.Auth.Type != "token" {
		return fmt.Errorf("unknown auth.type %q (only token supported)", c.Auth.Type)
	}
	return nil
}

// IsRuleEnabled reports whether the named rule should run.
func (c *Config) IsRuleEnabled(name string) bool {
	for _, d := range c.Rules.Disabled {
		if d == name {
			return false
		}
	}
	for _, e := range c.Rules.Enabled {
		if e == name {
			return true
		}
	}
	return false
}
