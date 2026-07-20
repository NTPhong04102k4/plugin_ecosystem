package skill

// `sr pull` is the deterministic bridge authswagger -> data layer + digest, run
// with ZERO tokens. It fetches an OpenAPI spec, filters it to one tag, generates
// TypeScript types (via openapi-typescript) and a react-query hook skeleton (via
// the pack's codegen templates), then prints a compact DIGEST — the only thing
// Claude reads. See docs/sr-pull-design.md.
//
// It parses the spec with the standard library only (shallow metadata:
// operationId/method/path/params/body) so the binary keeps its no-dependency
// property; the heavy schema->type work is delegated to openapi-typescript.

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PullOptions configures one `sr pull` run.
type PullOptions struct {
	From       string // spec URL (http...) or a local file path
	Env        string // authswagger environment, added as ?env=
	Spec       string // authswagger spec group, added as ?spec=
	Tag        string // OpenAPI tag to filter operations by (required)
	Out        string // output dir for generated files (relative to ProjectDir or absolute)
	Base       string // override proxyBase; default: spec servers[0].url
	Codegen    string // type generator; only "ts" (openapi-typescript) supported
	ProjectDir string // target project (cache + default out live under it)
	PackDir    string // directory holding packs/ (where codegen templates live)
	NoCache    bool   // ignore any cached digest and regenerate
}

// Digest is the compact result Claude reads (target < 2k tokens). It never
// contains raw schema — only endpoint metadata + where files landed.
type Digest struct {
	Tag       string           `json:"tag"`
	ProxyBase string           `json:"proxyBase"`
	Endpoints []DigestEndpoint `json:"endpoints"`
	Files     []string         `json:"files"`
	Rules     []string         `json:"rules"`
	SpecHash  string           `json:"specHash"`
	CachedAt  string           `json:"cachedAt"`
}

// DigestEndpoint is one operation, as seen by Claude.
type DigestEndpoint struct {
	Op     string `json:"op"`
	Method string `json:"method"`
	Path   string `json:"path"`
	Hook   string `json:"hook"`
}

// --- shallow OpenAPI model (only what pull needs) ---

type oaSpec struct {
	Servers []struct {
		URL string `json:"url"`
	} `json:"servers"`
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Parameters map[string]oaParam `json:"parameters"`
	} `json:"components"`
}

type oaOp struct {
	OperationID string          `json:"operationId"`
	Tags        []string        `json:"tags"`
	Parameters  []oaParam       `json:"parameters"`
	RequestBody json.RawMessage `json:"requestBody"`
}

type oaParam struct {
	Ref  string `json:"$ref"`
	Name string `json:"name"`
	In   string `json:"in"`
}

// httpMethods is the fixed order pull walks per path item — fixed so output is
// deterministic and non-method keys (parameters, summary) are skipped.
var httpMethods = []string{"get", "post", "put", "patch", "delete"}

// Pull runs the full bridge and returns the digest plus its pretty-printed JSON
// (the text meant for stdout). pack supplies the codegen templates and rules.
func Pull(opts PullOptions, pack *Pack) (*Digest, string, error) {
	if opts.Tag == "" {
		return nil, "", fmt.Errorf("pull requires --tag")
	}
	if opts.Codegen != "" && opts.Codegen != "ts" {
		return nil, "", fmt.Errorf("unsupported --codegen %q (only \"ts\")", opts.Codegen)
	}
	if pack == nil || pack.Codegen == nil {
		return nil, "", fmt.Errorf("stack pack has no codegen templates; run in a project whose stack pack ships \"codegen\"")
	}

	specBytes, err := fetchSpec(opts)
	if err != nil {
		return nil, "", err
	}
	specHash := fmt.Sprintf("sha256:%x", sha256.Sum256(specBytes))

	// Cache: same spec + tag => reuse the digest, skip fetch/parse/codegen.
	cachePath := filepath.Join(opts.ProjectDir, "docs", "context", "pull-"+slug(opts.Tag)+".json")
	if !opts.NoCache {
		if cached, ok := readCachedDigest(cachePath, specHash); ok {
			return cached, mustJSON(cached), nil
		}
	}

	var spec oaSpec
	if err := json.Unmarshal(specBytes, &spec); err != nil {
		return nil, "", fmt.Errorf("parse OpenAPI spec: %w", err)
	}

	proxyBase := opts.Base
	if proxyBase == "" && len(spec.Servers) > 0 {
		proxyBase = spec.Servers[0].URL
	}
	if proxyBase == "" && opts.Env != "" {
		proxyBase = "/env/" + opts.Env + "/api"
	}

	data, deps, err := buildCodegen(&spec, opts.Tag, proxyBase)
	if err != nil {
		return nil, "", err
	}

	// Resolve output dir + file names.
	outDir := opts.Out
	if outDir == "" {
		outDir = filepath.Join("src", "api", slug(opts.Tag))
	}
	absOut := outDir
	if !filepath.IsAbs(absOut) {
		absOut = filepath.Join(opts.ProjectDir, outDir)
	}
	if err := os.MkdirAll(absOut, 0o755); err != nil {
		return nil, "", fmt.Errorf("prepare out dir: %w", err)
	}
	hooksFile := slug(opts.Tag) + ".hooks.ts"
	data.HooksModule = "./" + slug(opts.Tag) + ".hooks"

	// 1) types.ts via openapi-typescript (delegated schema work). Feed it a spec
	// reduced to this tag's operations so it never emits `operations` entries for
	// OTHER tags — real specs reuse operationIds across tags (e.g. GenerateCode),
	// which would make the whole-spec output fail to compile with duplicates.
	typesPath := filepath.Join(absOut, "types.ts")
	filtered, err := filterSpecByTag(specBytes, opts.Tag)
	if err != nil {
		return nil, "", err
	}
	if err := genTypes(filtered, typesPath); err != nil {
		return nil, "", err
	}

	// 2) hook skeleton + barrel via the pack templates.
	rendered, err := pack.RenderCodegen(opts.PackDir, data)
	if err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(filepath.Join(absOut, hooksFile), rendered["hooks"], 0o644); err != nil {
		return nil, "", fmt.Errorf("write hooks: %w", err)
	}
	if idx, ok := rendered["index"]; ok {
		if err := os.WriteFile(filepath.Join(absOut, "index.ts"), idx, 0o644); err != nil {
			return nil, "", fmt.Errorf("write index: %w", err)
		}
	}

	digest := &Digest{
		Tag:       opts.Tag,
		ProxyBase: proxyBase,
		Endpoints: deps,
		Files: []string{
			filepath.ToSlash(filepath.Join(outDir, "types.ts")),
			filepath.ToSlash(filepath.Join(outDir, hooksFile)),
			filepath.ToSlash(filepath.Join(outDir, "index.ts")),
		},
		Rules:    digestRules(pack),
		SpecHash: specHash,
		CachedAt: time.Now().UTC().Format(time.RFC3339),
	}
	writeCachedDigest(cachePath, digest)
	return digest, mustJSON(digest), nil
}

// buildCodegen turns the filtered spec into template data + digest endpoints.
// Pure and deterministic (sorted paths, fixed method order) — unit-testable
// without network or Node.
func buildCodegen(spec *oaSpec, tag, proxyBase string) (CodegenData, []DigestEndpoint, error) {
	data := CodegenData{Tag: tag, ProxyBase: proxyBase}
	var deps []DigestEndpoint
	seenTags := map[string]bool{}

	for _, p := range sortedKeys(spec.Paths) {
		item := spec.Paths[p]
		for _, m := range httpMethods {
			raw, ok := item[m]
			if !ok {
				continue
			}
			var op oaOp
			if err := json.Unmarshal(raw, &op); err != nil {
				continue
			}
			for _, t := range op.Tags {
				seenTags[t] = true
			}
			if !hasTag(op.Tags, tag) {
				continue
			}

			pathParams := pathParamNames(p)
			hasQuery := false
			for _, pr := range op.Parameters {
				rp := resolveParam(pr, spec)
				if strings.EqualFold(rp.In, "query") {
					hasQuery = true
				}
			}
			isQuery := m == "get"
			opID := op.OperationID
			hasOp := opID != ""
			if opID == "" {
				opID = m + "_" + slug(p)
			}

			ep := CodegenEndpoint{
				Op:         opID,
				HookName:   "use" + pascal(opID),
				Method:     strings.ToUpper(m),
				Path:       p,
				PathExpr:   pathExpr(p, pathParams),
				IsQuery:    isQuery,
				PathParams: pathParams,
				HasQuery:   hasQuery,
				HasBody:    !isQuery && len(op.RequestBody) > 0,
				NeedsVars:  len(pathParams) > 0,
				HasOp:      hasOp,
			}
			data.Endpoints = append(data.Endpoints, ep)
			deps = append(deps, DigestEndpoint{Op: ep.Op, Method: ep.Method, Path: ep.Path, Hook: ep.HookName})
			if isQuery {
				data.HasQueries = true
			} else {
				data.HasMutations = true
			}
			if hasOp {
				data.UsesOperations = true
			}
		}
	}

	if len(data.Endpoints) == 0 {
		tags := make([]string, 0, len(seenTags))
		for t := range seenTags {
			tags = append(tags, t)
		}
		sort.Strings(tags)
		return data, nil, fmt.Errorf("no operations tagged %q; available tags: %v", tag, tags)
	}
	return data, deps, nil
}

// filterSpecByTag returns a reduced spec keeping only operations tagged tag (all
// components preserved untouched so $refs still resolve). This scopes the
// openapi-typescript output to one tag, avoiding cross-tag duplicate-operationId
// collisions and shrinking the generated types.
func filterSpecByTag(specBytes []byte, tag string) ([]byte, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(specBytes, &top); err != nil {
		return nil, fmt.Errorf("parse spec top-level: %w", err)
	}
	rawPaths, ok := top["paths"]
	if !ok {
		return specBytes, nil
	}
	var paths map[string]map[string]json.RawMessage
	if err := json.Unmarshal(rawPaths, &paths); err != nil {
		return nil, fmt.Errorf("parse spec paths: %w", err)
	}
	filtered := map[string]map[string]json.RawMessage{}
	for p, item := range paths {
		kept := map[string]json.RawMessage{}
		for m, raw := range item {
			if !isHTTPMethod(m) {
				continue
			}
			var op oaOp
			if err := json.Unmarshal(raw, &op); err != nil {
				continue
			}
			if hasTag(op.Tags, tag) {
				kept[m] = raw
			}
		}
		if len(kept) > 0 {
			filtered[p] = kept
		}
	}
	pf, err := json.Marshal(filtered)
	if err != nil {
		return nil, err
	}
	top["paths"] = pf
	return json.Marshal(top)
}

func isHTTPMethod(m string) bool {
	for _, x := range httpMethods {
		if x == m {
			return true
		}
	}
	return false
}

// --- spec fetching ---

// fetchSpec returns the raw spec bytes. From may be an http(s) URL (with a
// healthz preflight per the design: never auto-start authswagger) or a local
// file path (for tests/offline).
func fetchSpec(opts PullOptions) ([]byte, error) {
	if !strings.HasPrefix(opts.From, "http://") && !strings.HasPrefix(opts.From, "https://") {
		data, err := os.ReadFile(opts.From)
		if err != nil {
			return nil, fmt.Errorf("read spec file %q: %w", opts.From, err)
		}
		return data, nil
	}

	u, err := url.Parse(opts.From)
	if err != nil {
		return nil, fmt.Errorf("bad --from URL: %w", err)
	}
	q := u.Query()
	if opts.Env != "" {
		q.Set("env", opts.Env)
	}
	if opts.Spec != "" {
		q.Set("spec", opts.Spec)
	}
	u.RawQuery = q.Encode()

	if err := preflight(u); err != nil {
		return nil, err
	}

	req, _ := http.NewRequest(http.MethodGet, u.String(), nil)
	if user := os.Getenv("AUTHSWAGGER_USER"); user != "" {
		req.SetBasicAuth(user, os.Getenv("AUTHSWAGGER_PASS"))
	}
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch spec: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("spec fetch got 401 — set AUTHSWAGGER_USER/AUTHSWAGGER_PASS if authswagger has Basic Auth on")
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("spec fetch got HTTP %d from %s", res.StatusCode, u.String())
	}
	buf, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read spec body: %w", err)
	}
	return buf, nil
}

// preflight checks authswagger is up. Per design decision #1 it never starts it —
// it tells the user the command to run.
func preflight(specURL *url.URL) error {
	health := specURL.Scheme + "://" + specURL.Host + "/healthz"
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Get(health)
	if err == nil {
		res.Body.Close()
		if res.StatusCode == http.StatusOK {
			return nil
		}
	}
	return fmt.Errorf("authswagger not reachable at %s://%s — start it yourself:\n  cd <WorkFlowAutomation> && go run .",
		specURL.Scheme, specURL.Host)
}

// genTypes writes a temp spec file and runs openapi-typescript into outPath, then
// prepends the generated-file header.
func genTypes(specBytes []byte, outPath string) error {
	tmp, err := os.CreateTemp("", "sr-pull-spec-*.json")
	if err != nil {
		return fmt.Errorf("temp spec: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(specBytes); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp spec: %w", err)
	}
	tmp.Close()

	cmd := exec.Command("npx", "-y", "openapi-typescript", tmp.Name(), "-o", outPath)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("openapi-typescript failed (%v): %s\nHint: needs Node/npx on PATH", err, strings.TrimSpace(stderr.String()))
	}
	// Prepend the generated header so the file is recognizably tool-owned.
	body, err := os.ReadFile(outPath)
	if err != nil {
		return fmt.Errorf("read generated types: %w", err)
	}
	return os.WriteFile(outPath, append([]byte("// generated by sr pull — do not edit\n"), body...), 0o644)
}

// --- cache ---

func readCachedDigest(path, specHash string) (*Digest, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var d Digest
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, false
	}
	if d.SpecHash != specHash {
		return nil, false
	}
	return &d, true
}

func writeCachedDigest(path string, d *Digest) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return // cache is best-effort; a failure must not abort the run
	}
	_ = os.WriteFile(path, []byte(mustJSON(d)), 0o644)
}

// --- helpers ---

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}

// resolveParam dereferences a $ref parameter against components.parameters;
// inline params pass through unchanged.
func resolveParam(p oaParam, spec *oaSpec) oaParam {
	if p.Ref == "" {
		return p
	}
	name := p.Ref[strings.LastIndex(p.Ref, "/")+1:]
	if rp, ok := spec.Components.Parameters[name]; ok {
		return rp
	}
	return p
}

// pathParamNames extracts {name} segments from an OpenAPI path, in order.
func pathParamNames(p string) []string {
	var out []string
	for {
		i := strings.IndexByte(p, '{')
		if i < 0 {
			break
		}
		j := strings.IndexByte(p[i:], '}')
		if j < 0 {
			break
		}
		out = append(out, p[i+1:i+j])
		p = p[i+j+1:]
	}
	return out
}

// pathExpr renders the TS expression for a path: a single-quoted literal when it
// has no params, or a template literal with ${vars.<name>} substitutions.
func pathExpr(p string, params []string) string {
	if len(params) == 0 {
		return "'" + p + "'"
	}
	ts := p
	for _, name := range params {
		ts = strings.ReplaceAll(ts, "{"+name+"}", "${vars."+name+"}")
	}
	return "`" + ts + "`"
}

// pascal upper-cases the first rune of each token (split on _, -, space) while
// preserving existing internal camelCase, so getKhoaList -> GetKhoaList.
func pascal(s string) string {
	var b strings.Builder
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' || r == ' ' }) {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(part[1:])
	}
	return b.String()
}

// slug lower-cases and replaces non-alphanumeric runs with '-' for file names.
func slug(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// digestRules gives Claude the wiring contract for the data layer: the pack's
// architecture + conventions rule lines (bounded, high-value).
func digestRules(pack *Pack) []string {
	var out []string
	out = append(out, pack.Rules["architecture"]...)
	out = append(out, pack.Rules["conventions"]...)
	return out
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}
