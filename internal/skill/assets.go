package skill

import (
	"fmt"
	"os"
	"path/filepath"
)

// Apply status values, reported per asset by ApplyBase.
const (
	ApplyWrote     = "wrote"   // file was written (created or overwritten with --force)
	ApplySkipped   = "skipped" // destination already exists and --force was not set
	ApplyMissing   = "missing" // source asset file not found in the pack
	ApplyOverwrote = "overwrote"
)

// ApplyResult records what ApplyBase did for a single asset.
type ApplyResult struct {
	To     string // destination path within the project
	Status string // one of the Apply* constants above
	Detail string // human-readable note (source path on miss, description on write)
}

// ApplyBase copies the pack's base-config assets into projectDir. It never
// overwrites an existing file unless force is set — matching the manifest rule
// "never overwrite a file the tool did not generate without explicit approval".
// Each asset is reported independently; a missing source or a pre-existing
// destination is skipped, not fatal, so one bad asset can't abort the rest.
// packDir is the directory holding packs/ (i.e. the manifest's dir).
func (p *Pack) ApplyBase(packDir, projectDir string, force bool) ([]ApplyResult, error) {
	if len(p.Assets) == 0 {
		return nil, fmt.Errorf("stack %q ships no base assets (add an \"assets\" list to packs/%s.json)", p.Stack, p.Stack)
	}
	results := make([]ApplyResult, 0, len(p.Assets))
	for _, a := range p.Assets {
		src := filepath.Join(packDir, "packs", "assets", a.From)
		dst := filepath.Join(projectDir, a.To)

		data, err := os.ReadFile(src)
		if err != nil {
			results = append(results, ApplyResult{a.To, ApplyMissing, "source not found: " + src})
			continue
		}

		existed := false
		if _, err := os.Stat(dst); err == nil {
			existed = true
			if !force {
				results = append(results, ApplyResult{a.To, ApplySkipped, "already exists (use --force to overwrite)"})
				continue
			}
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return results, fmt.Errorf("prepare dir for %s: %w", a.To, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return results, fmt.Errorf("write %s: %w", a.To, err)
		}
		status := ApplyWrote
		if existed {
			status = ApplyOverwrote
		}
		results = append(results, ApplyResult{a.To, status, a.Desc})
	}
	return results, nil
}
