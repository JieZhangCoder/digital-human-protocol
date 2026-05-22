package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// StorageConfig holds backend settings.
type StorageConfig struct {
	Type   string                 `yaml:"type"`
	Config map[string]interface{} `yaml:"config"`
}

// AIJudgeConfig holds the LLM endpoint settings.
type AIJudgeConfig struct {
	Endpoint string `yaml:"endpoint"`
	Model    string `yaml:"model"`
	APIKey   string `yaml:"api_key"`
}

// RulesConfig controls which rules are active.
type RulesConfig struct {
	Enabled     []string `yaml:"enabled"`
	CustomRules []string `yaml:"custom_rules"`
}

// AuthConfig holds simple token auth.
type AuthConfig struct {
	Type  string `yaml:"type"`
	Token string `yaml:"token"`
}

// Config is the top-level server configuration.
type Config struct {
	Storage            StorageConfig `yaml:"storage"`
	AIJudge            AIJudgeConfig `yaml:"ai_judge"`
	Rules              RulesConfig   `yaml:"rules"`
	Auth               AuthConfig    `yaml:"auth"`
	AutoMergeThreshold string        `yaml:"auto_merge_threshold"`
	Listen             string        `yaml:"listen"`
}

// Load reads a YAML file into Config, falling back to defaults on error.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultConfig(), nil
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	applyDefaults(&cfg)
	return &cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Storage: StorageConfig{
			Type:   "local",
			Config: map[string]interface{}{"path": "./halo-data"},
		},
		AutoMergeThreshold: "low_risk",
		Listen:             ":8080",
		Rules: RulesConfig{
			Enabled: []string{
				"schema_valid",
				"slug_unique",
				"dangerous_permissions",
				"skill_code_safety",
				"prompt_quality",
				"metadata_compliance",
				"duplicate_detection",
				"sensitive_categories",
			},
		},
		Auth: AuthConfig{Type: "token"},
	}
}

func applyDefaults(cfg *Config) {
	if cfg.Listen == "" {
		cfg.Listen = ":8080"
	}
	if cfg.AutoMergeThreshold == "" {
		cfg.AutoMergeThreshold = "low_risk"
	}
	if cfg.Storage.Type == "" {
		cfg.Storage.Type = "local"
	}
	if len(cfg.Rules.Enabled) == 0 {
		cfg.Rules.Enabled = []string{
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
}
