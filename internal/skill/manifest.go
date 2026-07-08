// Package skill defines the declarative manifest format and the logic that
// turns a skill declaration into a full set of instructions for Claude.
//
// The design principle: this binary is "hands + memory", not a brain. It holds
// shared rules and skill declarations, then composes them into marching orders
// that Claude (the brain) executes. Nothing here reasons — it only reads JSON
// and formats text deterministically.
package skill

import (
	"fmt"
	"os"
	"sort"
)

// Manifest is the root of a skill.json file. It holds the SHARED skills (the
// "verbs") and the base rules. Stack-specific rule content lives in separate
// pack files (packs/<stack>.json) and is merged in at emit time — that is what
// lets one skill definition serve React, Flutter, Go, etc. without duplication.
type Manifest struct {
	Version string           `json:"version"`
	Project string           `json:"project,omitempty"`
	Rules   Rules            `json:"rules"`
	Skills  map[string]Skill `json:"skills"`
	// PackRules lists rule-group names that are supplied by a stack pack rather
	// than by this manifest. Skills may reference them in AppliesRules without
	// tripping validation; they resolve once a pack is merged.
	PackRules []string `json:"packRules,omitempty"`
}

// isPackRule reports whether a group name is declared as pack-provided.
func (m *Manifest) isPackRule(group string) bool {
	for _, g := range m.PackRules {
		if g == group {
			return true
		}
	}
	return false
}

// Merge returns a shallow copy of the manifest with the pack's rule groups
// merged into its rules (pack content is appended after any base content of the
// same group). The original manifest is not modified.
func (m *Manifest) Merge(p *Pack) *Manifest {
	if p == nil {
		return m
	}
	merged := make(Rules, len(m.Rules)+len(p.Rules))
	for g, items := range m.Rules {
		merged[g] = append([]string(nil), items...)
	}
	for g, items := range p.Rules {
		merged[g] = append(merged[g], items...)
	}
	cp := *m
	cp.Rules = merged
	return &cp
}

// Rules are shared constraints applied to skills that opt in via AppliesRules.
// Keys are free-form ("ux", "technical", ...) so a project can add categories.
type Rules map[string][]string

// Skill is one declarative unit of work. It never contains executable code —
// only the intent, the rules it obeys, and the steps Claude should carry out.
type Skill struct {
	Description      string   `json:"description"`
	Goal             string   `json:"goal,omitempty"`
	AppliesRules     []string `json:"appliesRules,omitempty"`
	RequiresApproval bool     `json:"requiresApproval,omitempty"`
	Inputs           []string `json:"inputs,omitempty"`
	Instructions     []string `json:"instructions"`
	Outputs          []string `json:"outputs,omitempty"`
}

// Load reads and parses a manifest file, then validates it.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %q: %w", path, err)
	}
	var m Manifest
	if err := decodeJSONC(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %q: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate checks structural rules that would otherwise produce confusing
// output later. It reports every problem it finds, not just the first.
func (m *Manifest) Validate() error {
	var problems []string
	if m.Version == "" {
		problems = append(problems, "missing \"version\"")
	}
	if len(m.Skills) == 0 {
		problems = append(problems, "no skills defined")
	}
	for name, s := range m.Skills {
		if s.Description == "" {
			problems = append(problems, fmt.Sprintf("skill %q: missing \"description\"", name))
		}
		if len(s.Instructions) == 0 {
			problems = append(problems, fmt.Sprintf("skill %q: needs at least one instruction", name))
		}
		for _, r := range s.AppliesRules {
			if _, ok := m.Rules[r]; ok {
				continue
			}
			if m.isPackRule(r) {
				continue // resolved when a stack pack is merged
			}
			problems = append(problems, fmt.Sprintf("skill %q: appliesRules references unknown rule group %q (add it to rules or packRules)", name, r))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid manifest:\n  - %s", join(problems, "\n  - "))
	}
	return nil
}

// SkillNames returns skill keys in stable, sorted order for deterministic output.
func (m *Manifest) SkillNames() []string {
	names := make([]string, 0, len(m.Skills))
	for n := range m.Skills {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func join(items []string, sep string) string {
	out := ""
	for i, it := range items {
		if i > 0 {
			out += sep
		}
		out += it
	}
	return out
}
