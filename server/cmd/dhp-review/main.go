// Command dhp-review performs an offline review of a DHP spec.yaml bundle.
//
// It is the binary GitHub Actions invokes against PRs: it parses the spec,
// runs every enabled rule, and emits a markdown or JSON report. Exit code
// reflects the worst verdict so the workflow can auto-merge low-risk
// publishes and gate everything else.
//
//	exit 0 → pass (auto-mergeable)
//	exit 1 → warn (needs human review)
//	exit 2 → fail (blocked)
//	exit 3 → runner error (transient — retry)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openkursar/digital-human-protocol/server/internal/ai"
	"github.com/openkursar/digital-human-protocol/server/internal/config"
	"github.com/openkursar/digital-human-protocol/server/internal/rules"
	"github.com/openkursar/digital-human-protocol/server/internal/spec"
)

func main() {
	specPath := flag.String("spec", "", "path to spec.yaml (required)")
	cfgPath := flag.String("config", "", "path to review config yaml (required)")
	format := flag.String("format", "markdown", "report format: markdown|json")
	out := flag.String("out", "", "output file (default: stdout)")
	registryFlag := flag.String("registry-url", "", "override registry URL or local path")
	flag.Parse()

	if *specPath == "" || *cfgPath == "" {
		fmt.Fprintln(os.Stderr, "usage: dhp-review --spec=path --config=path [--format=markdown|json] [--out=path]")
		os.Exit(3)
	}

	exitCode := run(*specPath, *cfgPath, *format, *out, *registryFlag)
	os.Exit(exitCode)
}

func run(specPath, cfgPath, format, out, registryOverride string) int {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		return 3
	}

	raw, err := os.ReadFile(specPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spec read:", err)
		return 3
	}
	s, err := spec.Parse(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spec parse:", err)
		return 2
	}

	// Skill files: anything listed in spec.skill_files relative to spec.yaml.
	skillFiles, err := readSkillFiles(filepath.Dir(specPath), s.SkillFiles)
	if err != nil {
		fmt.Fprintln(os.Stderr, "skill files:", err)
		return 3
	}

	registryBase := registryOverride
	if registryBase == "" {
		registryBase = cfg.RegistryURL
	}
	reg := rules.NewHTTPRegistry(registryBase)

	aiClient := ai.New(cfg.AIJudge.Endpoint, cfg.AIJudge.Model, cfg.AIJudge.APIKey)

	ctx := context.Background()
	opts := &rules.Options{
		Spec:        s,
		SpecBytes:   raw,
		SkillFiles:  skillFiles,
		Config:      cfg,
		AI:          aiClient,
		Registry:    reg,
		RawRegistry: reg.AllEntries(ctx),
	}
	report := rules.Run(ctx, opts)

	var payload []byte
	switch strings.ToLower(format) {
	case "json":
		payload, err = rules.RenderJSON(report)
		if err != nil {
			fmt.Fprintln(os.Stderr, "render json:", err)
			return 3
		}
	case "", "markdown", "md":
		payload = []byte(rules.RenderMarkdown(report))
	default:
		fmt.Fprintln(os.Stderr, "unknown format:", format)
		return 3
	}

	if out == "" {
		if _, err := os.Stdout.Write(payload); err != nil {
			fmt.Fprintln(os.Stderr, "write:", err)
			return 3
		}
		if !strings.HasSuffix(string(payload), "\n") {
			fmt.Println()
		}
	} else {
		if err := os.WriteFile(out, payload, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "write:", err)
			return 3
		}
	}

	return exitFromSeverity(report.Overall)
}

func exitFromSeverity(s rules.Severity) int {
	switch s {
	case rules.SeverityPass:
		return 0
	case rules.SeverityWarn:
		return 1
	case rules.SeverityFail:
		return 2
	default:
		return 3
	}
}

func readSkillFiles(base string, files []string) (map[string][]byte, error) {
	if len(files) == 0 {
		return nil, nil
	}
	out := make(map[string][]byte, len(files))
	for _, f := range files {
		// Reject obvious traversal — bundle layout is flat under spec.yaml.
		if strings.Contains(f, "..") {
			return nil, fmt.Errorf("skill_files: %q escapes bundle root", f)
		}
		path := filepath.Join(base, f)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("skill_files: read %s: %w", f, err)
		}
		out[f] = data
	}
	return out, nil
}
