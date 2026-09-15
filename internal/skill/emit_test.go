package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// taxonomyRel is the pool-relative path the fixture manifest points at.
//
// It is deliberately a plain string rather than a filepath.Join: skills spell
// pool paths with "/" on every platform, and resolveHome only substitutes the
// home prefix — so the emitted text is home + "/docs/..." verbatim, mixed
// separators and all. Building the expectation with filepath.Join would assert
// backslashes that emit never produces on Windows.
const taxonomyRel = "/docs/ui-ux-conflicts.md"

// writeManifest drops a minimal one-skill manifest in a temp dir and returns the
// dir plus the manifest path.
func writeManifest(t *testing.T, instruction string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.json")
	body := `{
  "version": "1",
  "skills": {
    "demo": {
      "description": "Demo skill.",
      "inputs": ["Taxonomy at ` + homePlaceholder + taxonomyRel + `"],
      "instructions": ["` + instruction + `"]
    }
  }
}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

// The $SKILLRUNNER_HOME placeholder must be resolved by emit itself. The `sr`
// shell wrapper exports a variable of that name, but a subprocess — the MCP
// server, cron, a direct binary call — never inherits it, so a skill pointing at
// a file inside the pool would otherwise hand Claude an unresolvable path.
func TestEmitResolvesHomePlaceholder(t *testing.T) {
	dir, path := writeManifest(t, "Read "+homePlaceholder+taxonomyRel+" before judging.")

	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	out, err := m.Emit("demo")
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(out, homePlaceholder) {
		t.Errorf("emit left the placeholder unresolved:\n%s", out)
	}
	want := dir + taxonomyRel
	if !strings.Contains(out, want) {
		t.Errorf("emit did not expand to %q:\n%s", want, out)
	}
	// Inputs are emitted through the same path, so they must expand too.
	if strings.Count(out, want) < 2 {
		t.Errorf("expected the path in both inputs and steps, got %d occurrence(s)", strings.Count(out, want))
	}
}

// EmitAll goes through Emit, so the catalog dump must expand it as well.
func TestEmitAllResolvesHomePlaceholder(t *testing.T) {
	_, path := writeManifest(t, "Read "+homePlaceholder+taxonomyRel+".")

	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	out, err := m.EmitAll()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, homePlaceholder) {
		t.Errorf("emit all left the placeholder unresolved:\n%s", out)
	}
}

// Merging a stack pack copies the manifest; the resolved home must survive that
// copy, otherwise every emit that goes through a detected pack regresses.
func TestMergeKeepsHome(t *testing.T) {
	dir, path := writeManifest(t, "Read "+homePlaceholder+taxonomyRel+".")

	m, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	merged := m.Merge(&Pack{Stack: "go", Rules: Rules{"conventions": {"Some go rule."}}})
	out, err := merged.Emit("demo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, dir+"/docs") {
		t.Errorf("home lost across Merge:\n%s", out)
	}
}
