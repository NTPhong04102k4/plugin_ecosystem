package skill

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// Codegen points a stack pack at real code templates that `sr pull` fills from an
// OpenAPI spec. Paths are relative to packs/assets/ (same root as Asset.From), so
// the template files ship centrally with the pack. These produce ACTUAL code
// deterministically — unlike Rules["templates"], which is prose for Claude.
type Codegen struct {
	Hooks string `json:"hooks,omitempty"` // template for the per-resource hooks file
	Index string `json:"index,omitempty"` // template for the barrel re-export (optional)
}

// CodegenData is the context passed to a pack's codegen templates. `sr pull`
// (step B) builds it from the filtered OpenAPI operations; the template shape and
// these fields are a contract — keep them in sync with the .tmpl files.
type CodegenData struct {
	Tag            string            // resource tag, e.g. "Khoa"
	ProxyBase      string            // baseUrl the generated calls hit, e.g. "/env/dev/api"
	HooksModule    string            // module specifier for the hooks file, e.g. "./khoa.hooks"
	HasQueries     bool              // any GET endpoint present (drives conditional imports)
	HasMutations   bool              // any non-GET endpoint present
	UsesOperations bool              // any endpoint has an operationId (drives the operations import + OpRes)
	Endpoints      []CodegenEndpoint // one per operation, in stable order
}

// CodegenEndpoint is one operation rendered into a hook skeleton.
type CodegenEndpoint struct {
	Op         string   // operationId, e.g. "getKhoaList" — must match a key in `operations`
	HookName   string   // e.g. "useKhoaList"
	Method     string   // upper-case HTTP verb: GET, POST, ...
	Path       string   // raw OpenAPI path, e.g. "/khoa/{id}"
	PathExpr   string   // TS expression for the path: "'/khoa'" or "`/khoa/${vars.id}`"
	IsQuery    bool     // true for GET (useQuery), false otherwise (useMutation)
	PathParams []string // path parameter names, e.g. ["id"]
	HasQuery   bool     // has query parameters
	HasBody    bool     // has a request body
	NeedsVars  bool     // vars arg is required (true when path params exist)
	HasOp      bool     // operationId exists in the spec (reference `operations`; else fall back to unknown)
}

// RenderCodegen executes the pack's codegen templates against data and returns a
// map keyed by "hooks"/"index" with the rendered file contents. packDir is the
// directory holding packs/ (the manifest's dir).
func (p *Pack) RenderCodegen(packDir string, data CodegenData) (map[string][]byte, error) {
	if p.Codegen == nil || p.Codegen.Hooks == "" {
		return nil, fmt.Errorf("stack %q ships no codegen templates (add \"codegen\" to packs/%s.json)", p.Stack, p.Stack)
	}
	out := make(map[string][]byte, 2)

	hooks, err := renderPackTemplate(packDir, p.Codegen.Hooks, data)
	if err != nil {
		return nil, err
	}
	out["hooks"] = hooks

	if p.Codegen.Index != "" {
		idx, err := renderPackTemplate(packDir, p.Codegen.Index, data)
		if err != nil {
			return nil, err
		}
		out["index"] = idx
	}
	return out, nil
}

// renderPackTemplate loads a template file (rel to packs/assets/) and executes it
// against data. Deterministic: same template + same data => same bytes.
func renderPackTemplate(packDir, rel string, data any) ([]byte, error) {
	path := filepath.Join(packDir, "packs", "assets", rel)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read codegen template %q: %w", path, err)
	}
	t, err := template.New(filepath.Base(rel)).Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse codegen template %q: %w", path, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render codegen template %q: %w", path, err)
	}
	return buf.Bytes(), nil
}
