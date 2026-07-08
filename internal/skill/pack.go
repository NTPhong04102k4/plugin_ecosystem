package skill

import (
	"fmt"
	"os"
	"path/filepath"
)

// Pack is a stack-specific bundle of rule content (architecture, conventions,
// lint, templates, library-docs...). Packs supply the CONTENT for the generic
// rule-group names that shared skills reference. One pack per stack.
type Pack struct {
	Stack string `json:"stack"`
	Rules Rules  `json:"rules"`
}

// LoadPack reads packs/<stack>.json relative to dir (the manifest's directory).
func LoadPack(dir, stack string) (*Pack, error) {
	path := filepath.Join(dir, "packs", stack+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pack %q: %w", path, err)
	}
	var p Pack
	if err := decodeJSONC(data, &p); err != nil {
		return nil, fmt.Errorf("parse pack %q: %w", path, err)
	}
	if p.Stack == "" {
		p.Stack = stack
	}
	return &p, nil
}

// AvailablePacks lists stack names for which a packs/<stack>.json exists.
func AvailablePacks(dir string) []string {
	entries, err := os.ReadDir(filepath.Join(dir, "packs"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) == ".json" {
			out = append(out, name[:len(name)-len(".json")])
		}
	}
	return out
}
