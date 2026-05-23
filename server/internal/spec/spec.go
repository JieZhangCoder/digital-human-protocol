// Package spec parses and validates DHP App Spec (spec.yaml) documents.
//
// Validation here is intentionally a subset of the canonical TypeScript schema:
// it focuses on the fields the Go review pipeline needs to make blocking
// decisions (type, identity, prompt, permissions, dependencies). Display-only
// fields are tolerated permissively to avoid coupling the Go server to every
// schema iteration.
package spec

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Type enumerates supported app types.
type Type string

const (
	TypeAutomation Type = "automation"
	TypeSkill      Type = "skill"
	TypeMCP        Type = "mcp"
	TypeExtension  Type = "extension"
)

// Spec is the parsed and normalized form of a DHP app spec.yaml.
type Spec struct {
	SpecVersion      string                 `yaml:"spec_version"`
	Name             string                 `yaml:"name"`
	Version          string                 `yaml:"version"`
	Author           string                 `yaml:"author"`
	Description      string                 `yaml:"description"`
	Type             Type                   `yaml:"type"`
	Icon             string                 `yaml:"icon,omitempty"`
	SystemPrompt     string                 `yaml:"system_prompt,omitempty"`
	Subscriptions    []Subscription         `yaml:"subscriptions,omitempty"`
	ConfigSchema     []ConfigField          `yaml:"config_schema,omitempty"`
	Requires         *Requires              `yaml:"requires,omitempty"`
	Filters          []FilterRule           `yaml:"filters,omitempty"`
	MemorySchema     map[string]MemoryField `yaml:"memory_schema,omitempty"`
	Output           *OutputConfig          `yaml:"output,omitempty"`
	Escalation       *EscalationConfig      `yaml:"escalation,omitempty"`
	MCPServer        *MCPServerConfig       `yaml:"mcp_server,omitempty"`
	Permissions      []string               `yaml:"permissions,omitempty"`
	RecommendedModel string                 `yaml:"recommended_model,omitempty"`
	Store            *StoreMetadata         `yaml:"store,omitempty"`
	SkillFiles       []string               `yaml:"skill_files,omitempty"`

	// Raw retains any additional fields not modeled above. This keeps rules
	// like duplicate_detection capable of inspecting custom keys without
	// forcing a full schema port.
	Raw map[string]any `yaml:"-"`
}

// Subscription mirrors SubscriptionDef from the spec.
type Subscription struct {
	ID        string             `yaml:"id,omitempty"`
	Source    *SubscriptionSrc   `yaml:"source,omitempty"`
	Type      string             `yaml:"type,omitempty"`   // shorthand
	Config    map[string]any     `yaml:"config,omitempty"` // shorthand
	Frequency *FrequencyDef      `yaml:"frequency,omitempty"`
	ConfigKey string             `yaml:"config_key,omitempty"`
	Input     string             `yaml:"input,omitempty"` // legacy alias
}

// SubscriptionSrc is the canonical source block.
type SubscriptionSrc struct {
	Type   string         `yaml:"type"`
	Config map[string]any `yaml:"config,omitempty"`
}

// FrequencyDef declares user-adjustable frequencies.
type FrequencyDef struct {
	Default string `yaml:"default"`
	Min     string `yaml:"min,omitempty"`
	Max     string `yaml:"max,omitempty"`
}

// ConfigField describes a user-supplied input field.
type ConfigField struct {
	Key         string         `yaml:"key"`
	Label       string         `yaml:"label"`
	Type        string         `yaml:"type"`
	Description string         `yaml:"description,omitempty"`
	Required    bool           `yaml:"required,omitempty"`
	Default     any            `yaml:"default,omitempty"`
	Placeholder string         `yaml:"placeholder,omitempty"`
	Options     []SelectOption `yaml:"options,omitempty"`
}

// SelectOption is a dropdown option for select inputs.
type SelectOption struct {
	Label string `yaml:"label"`
	Value any    `yaml:"value"`
}

// Requires describes external MCP and skill dependencies.
type Requires struct {
	MCPs   []McpDependency   `yaml:"mcps,omitempty"`
	Skills []SkillDependency `yaml:"skills,omitempty"`
}

// McpDependency describes a required MCP server.
type McpDependency struct {
	ID      string `yaml:"id"`
	Reason  string `yaml:"reason,omitempty"`
	Bundled bool   `yaml:"bundled,omitempty"`
}

// SkillDependency describes a required skill. May be parsed from string shorthand.
type SkillDependency struct {
	ID      string   `yaml:"id"`
	Version string   `yaml:"version,omitempty"`
	Reason  string   `yaml:"reason,omitempty"`
	Bundled bool     `yaml:"bundled,omitempty"`
	Files   []string `yaml:"files,omitempty"`
}

// UnmarshalYAML supports both string and object forms for SkillDependency.
func (s *SkillDependency) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		s.ID = node.Value
		return nil
	}
	type raw SkillDependency
	var r raw
	if err := node.Decode(&r); err != nil {
		return err
	}
	*s = SkillDependency(r)
	return nil
}

// FilterRule is a single AND-combined pre-run filter rule.
type FilterRule struct {
	Field string `yaml:"field"`
	Op    string `yaml:"op"`
	Value any    `yaml:"value"`
}

// MemoryField describes a single persistent memory entry.
type MemoryField struct {
	Type        string `yaml:"type"`
	Description string `yaml:"description,omitempty"`
}

// OutputConfig controls post-run notifications.
type OutputConfig struct {
	Notify *NotifyConfig `yaml:"notify,omitempty"`
	Format string        `yaml:"format,omitempty"`
}

// NotifyConfig is the notification target block.
type NotifyConfig struct {
	System   bool     `yaml:"system,omitempty"`
	Channels []string `yaml:"channels,omitempty"`
}

// EscalationConfig describes human-in-the-loop escalation policy.
type EscalationConfig struct {
	Enabled      bool `yaml:"enabled"`
	TimeoutHours int  `yaml:"timeout_hours,omitempty"`
}

// MCPServerConfig describes the spawn command for type:mcp apps.
type MCPServerConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Cwd     string            `yaml:"cwd,omitempty"`
}

// StoreMetadata is registry / discovery metadata.
type StoreMetadata struct {
	Slug          string         `yaml:"slug,omitempty"`
	Category      string         `yaml:"category,omitempty"`
	Tags          []string       `yaml:"tags,omitempty"`
	Locale        string         `yaml:"locale,omitempty"`
	MinAppVersion string         `yaml:"min_app_version,omitempty"`
	License       string         `yaml:"license,omitempty"`
	Homepage      string         `yaml:"homepage,omitempty"`
	Repository    string         `yaml:"repository,omitempty"`
	RegistryID    string         `yaml:"registry_id,omitempty"`
	Meta          map[string]any `yaml:"meta,omitempty"`
}

// scopedSlugRe matches "author/id" or plain "id" (legacy).
var scopedSlugRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:/[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)?$`)

// semverLooseRe matches loose versions: "1.0", "1.0.0", "0.1-beta", etc.
var semverLooseRe = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+){0,2}(?:[-+][0-9A-Za-z.\-]+)?$`)

// Parse decodes raw YAML into a Spec value. It does not perform validation —
// call Validate separately so callers (review CLI, registry, tests) can decide
// how to surface errors.
func Parse(data []byte) (*Spec, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("yaml decode: %w", err)
	}
	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("yaml decode (typed): %w", err)
	}
	s.Raw = raw
	normalize(&s)
	return &s, nil
}

// normalize applies a few backward-compat rewrites mirrored from the TS parser.
func normalize(s *Spec) {
	if s.SpecVersion == "" {
		s.SpecVersion = "1"
	}
	// Shorthand subscriptions: type/config at the entry level.
	for i := range s.Subscriptions {
		sub := &s.Subscriptions[i]
		if sub.Source == nil && sub.Type != "" {
			sub.Source = &SubscriptionSrc{Type: sub.Type, Config: sub.Config}
			sub.Type = ""
			sub.Config = nil
		}
		if sub.ConfigKey == "" && sub.Input != "" {
			sub.ConfigKey = sub.Input
			sub.Input = ""
		}
	}
}

// ValidationError aggregates one or more validation failures.
type ValidationError struct {
	Issues []string
}

func (v *ValidationError) Error() string {
	return "spec validation failed: " + strings.Join(v.Issues, "; ")
}

// Validate performs structural and semantic checks. Returns nil on success or
// a *ValidationError when issues are found. The returned error wraps the issue
// list so callers can introspect with errors.As.
func Validate(s *Spec) error {
	if s == nil {
		return errors.New("spec is nil")
	}
	v := &ValidationError{}
	if strings.TrimSpace(s.Name) == "" {
		v.add("name is required and non-empty")
	}
	if strings.TrimSpace(s.Version) == "" {
		v.add("version is required")
	} else if !semverLooseRe.MatchString(s.Version) {
		v.add(fmt.Sprintf("version %q does not match loose semver", s.Version))
	}
	if strings.TrimSpace(s.Author) == "" {
		v.add("author is required")
	}
	if strings.TrimSpace(s.Description) == "" {
		v.add("description is required")
	}
	switch s.Type {
	case TypeAutomation, TypeSkill, TypeMCP, TypeExtension:
	case "":
		v.add("type is required")
	default:
		v.add(fmt.Sprintf("unknown type %q", s.Type))
	}

	switch s.Type {
	case TypeAutomation:
		if strings.TrimSpace(s.SystemPrompt) == "" {
			v.add("system_prompt is required for type:automation")
		}
		if len(s.Subscriptions) == 0 {
			v.add("at least one subscription is required for type:automation")
		}
	case TypeSkill:
		if strings.TrimSpace(s.SystemPrompt) == "" {
			v.add("system_prompt is required for type:skill")
		}
		if len(s.Subscriptions) > 0 {
			v.add("type:skill must not declare subscriptions")
		}
	case TypeMCP:
		if s.MCPServer == nil || strings.TrimSpace(s.MCPServer.Command) == "" {
			v.add("mcp_server.command is required for type:mcp")
		}
		if len(s.Subscriptions) > 0 {
			v.add("type:mcp must not declare subscriptions")
		}
	}

	if s.Store != nil && s.Store.Slug != "" {
		if !scopedSlugRe.MatchString(s.Store.Slug) {
			v.add(fmt.Sprintf("store.slug %q must match [a-z0-9-]+ optionally scoped author/id", s.Store.Slug))
		}
	}

	for i, sub := range s.Subscriptions {
		if sub.Source == nil || strings.TrimSpace(sub.Source.Type) == "" {
			v.add(fmt.Sprintf("subscriptions[%d].source.type is required", i))
		}
	}

	if len(v.Issues) > 0 {
		return v
	}
	return nil
}

func (v *ValidationError) add(msg string) { v.Issues = append(v.Issues, msg) }

// Slug returns the registry slug, falling back to a best-effort identifier.
func (s *Spec) Slug() string {
	if s.Store != nil && s.Store.Slug != "" {
		return s.Store.Slug
	}
	return ""
}
