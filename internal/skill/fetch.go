package skill

// `sr fetch` is the deterministic bridge Confluence / Google Sheet -> markdown +
// digest, run with ZERO tokens. It fetches a page/sheet by link + token,
// normalizes it to clean markdown (docs/specs/<slug>.md), and prints a compact
// DIGEST — the only thing Claude must read. It is the content-fetching sibling of
// `sr pull` (which bridges OpenAPI -> code). See docs/sr-fetch-design.md.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FetchOptions configures one `sr fetch` run.
type FetchOptions struct {
	From       string // Confluence/Sheet URL, or an alias in config.sources (required)
	Out        string // markdown output path (default docs/specs/<slug>.md)
	Email      string // Confluence Basic-auth email (flag override)
	Token      string // Confluence token passed directly (wins over TokenEnv/env); used by `sr ui`
	TokenEnv   string // env var name holding the token/secret (flag override)
	Range      string // Google: A1 range -> Sheets API v4 values path (e.g. Sheet1!A1:H)
	Gid        string // Google: tab id override (default: #gid= in the URL)
	AllTabs    bool   // Google: fetch every tab via Sheets API v4 (batchGet)
	ProjectDir string // target project (config + cache + output live under it)
	NoCache    bool   // ignore any cached digest and re-fetch
}

// FetchDigest is the compact result Claude reads (target < 2k tokens). It never
// contains the raw page/sheet — only metadata + where the markdown landed.
type FetchDigest struct {
	Source      string        `json:"source"` // "confluence" | "gsheet"
	URL         string        `json:"url"`
	Title       string        `json:"title"`
	File        string        `json:"file"`
	Tables      []DigestTable `json:"tables,omitempty"`
	Sections    []string      `json:"sections,omitempty"`
	ContentHash string        `json:"contentHash"`
	CachedAt    string        `json:"cachedAt"`
}

// DigestTable summarizes one table found in the source (shape, not content).
type DigestTable struct {
	Name    string   `json:"name,omitempty"`
	Headers []string `json:"headers,omitempty"`
	Rows    int      `json:"rows"`
}

// fetchSource is a parsed source URL: which kind, and the ids needed to fetch it.
type fetchSource struct {
	kind      string // "confluence" | "gsheet"
	site      string // confluence host, e.g. acme.atlassian.net
	id        string // page id / spreadsheet id
	gid       string // sheet tab id
	cacheSlug string // stable, content-independent slug for the cache file
}

var (
	reConfluencePage = regexp.MustCompile(`/wiki/(?:spaces/[^/]+/)?pages/(\d+)`)
	reSheetID        = regexp.MustCompile(`/spreadsheets/d/([A-Za-z0-9_-]+)`)
	reGid            = regexp.MustCompile(`[#&?]gid=(\d+)`)
)

// Fetch runs the full bridge and returns the digest plus its pretty-printed JSON
// (the text meant for stdout).
func Fetch(opts FetchOptions) (*FetchDigest, string, error) {
	if strings.TrimSpace(opts.From) == "" {
		return nil, "", fmt.Errorf("fetch requires --from <url|alias>")
	}
	cfg, err := LoadFetchConfig(opts.ProjectDir)
	if err != nil {
		return nil, "", fmt.Errorf("load .skillrunner/fetch.json: %w", err)
	}

	// Resolve an alias to a real URL before detection.
	from := opts.From
	if u, ok := cfg.Sources[from]; ok {
		from = u
	}

	src, err := detectSource(from)
	if err != nil {
		return nil, "", err
	}

	cachePath := filepath.Join(opts.ProjectDir, "docs", "context", "fetch-"+src.cacheSlug+".json")

	// Fetch raw bytes first; the cache key is the hash of the raw payload, so an
	// unchanged page/sheet reuses the cached digest without re-normalizing.
	raw, err := fetchRaw(src, opts, cfg)
	if err != nil {
		return nil, "", err
	}
	contentHash := fmt.Sprintf("sha256:%x", sha256.Sum256(raw.bytes))

	if !opts.NoCache {
		if cached, ok := readCachedFetch(cachePath, contentHash); ok {
			return cached, mustJSON(cached), nil
		}
	}

	var (
		title, markdown string
		tables          []DigestTable
		sections        []string
	)
	switch src.kind {
	case "confluence":
		title, markdown, tables, sections, err = confluenceParse(raw.bytes)
	case "gsheet":
		switch raw.mode {
		case "v4":
			title = raw.title
			if title == "" {
				title = "Sheet " + src.id
			}
			markdown, tables, err = gsheetV4Parse(raw.bytes, title)
		default: // "csv"
			title = "Sheet " + src.id
			if src.gid != "" {
				title += " (gid " + src.gid + ")"
			}
			var t DigestTable
			markdown, t, err = gsheetParse(raw.bytes, title)
			if t.Rows > 0 || len(t.Headers) > 0 {
				tables = []DigestTable{t}
			}
		}
	}
	if err != nil {
		return nil, "", err
	}

	outPath := opts.Out
	if outPath == "" {
		outPath = filepath.Join("docs", "specs", slug(title)+".md")
	}
	absOut := outPath
	if !filepath.IsAbs(absOut) {
		absOut = filepath.Join(opts.ProjectDir, outPath)
	}
	if err := writeGenerated(absOut, markdown); err != nil {
		return nil, "", err
	}

	digest := &FetchDigest{
		Source:      src.kind,
		URL:         from,
		Title:       title,
		File:        filepath.ToSlash(outPath),
		Tables:      tables,
		Sections:    sections,
		ContentHash: contentHash,
		CachedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	writeCachedFetch(cachePath, digest)
	return digest, mustJSON(digest), nil
}

// detectSource classifies a URL and extracts the ids needed to fetch it.
func detectSource(raw string) (fetchSource, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return fetchSource{}, fmt.Errorf("bad --from URL %q: %w", raw, err)
	}
	host := strings.ToLower(u.Host)

	if strings.HasSuffix(host, "atlassian.net") && strings.Contains(u.Path, "/wiki/") {
		m := reConfluencePage.FindStringSubmatch(u.Path)
		if m == nil {
			return fetchSource{}, fmt.Errorf("confluence URL has no page id (expected .../pages/<id>/...): %s", raw)
		}
		return fetchSource{kind: "confluence", site: u.Host, id: m[1], cacheSlug: "confluence-" + m[1]}, nil
	}

	if host == "docs.google.com" && strings.Contains(u.Path, "/spreadsheets/") {
		m := reSheetID.FindStringSubmatch(u.Path)
		if m == nil {
			return fetchSource{}, fmt.Errorf("google sheet URL has no spreadsheet id: %s", raw)
		}
		gid := ""
		if g := reGid.FindStringSubmatch(raw); g != nil {
			gid = g[1]
		}
		cs := "gsheet-" + m[1]
		if gid != "" {
			cs += "-" + gid
		}
		return fetchSource{kind: "gsheet", id: m[1], gid: gid, cacheSlug: cs}, nil
	}

	return fetchSource{}, fmt.Errorf("unsupported source %q — expected a *.atlassian.net/wiki/.../pages/<id> or docs.google.com/spreadsheets/d/<id> URL", raw)
}

// rawResult is one fetched payload plus metadata parse needs but can't derive
// from the bytes alone (the gsheet mode, and the v4 spreadsheet title).
type rawResult struct {
	bytes []byte
	mode  string // gsheet: "csv" | "v4"
	title string // gsheet v4: spreadsheet title
}

// fetchRaw dispatches to the per-source fetcher, resolving secrets flag>env>config.
func fetchRaw(src fetchSource, opts FetchOptions, cfg *FetchConfig) (rawResult, error) {
	switch src.kind {
	case "confluence":
		var cc ConfluenceConfig
		if cfg.Confluence != nil {
			cc = *cfg.Confluence
		}
		email := firstNonEmpty(opts.Email, cc.Email)
		tokenEnv := firstNonEmpty(opts.TokenEnv, cc.TokenEnv, "CONFLUENCE_TOKEN")
		// Precedence: direct token (sr ui) > env var > config.
		token := firstNonEmpty(opts.Token, os.Getenv(tokenEnv))
		b, err := confluenceFetch(src.site, src.id, email, token)
		return rawResult{bytes: b}, err

	case "gsheet":
		// --range or --all-tabs => Sheets API v4 (multi-tab/range); else CSV export.
		if opts.Range != "" || opts.AllTabs {
			bearer, apiKey, err := googleV4Auth(cfg.Google)
			if err != nil {
				return rawResult{}, err
			}
			b, title, err := gsheetV4Fetch(src.id, opts.Range, opts.AllTabs, bearer, apiKey)
			return rawResult{bytes: b, mode: "v4", title: title}, err
		}
		gid := firstNonEmpty(opts.Gid, src.gid)
		token, err := resolveGoogleToken(cfg.Google)
		if err != nil {
			return rawResult{}, err
		}
		b, err := gsheetFetch(src.id, gid, token)
		return rawResult{bytes: b, mode: "csv"}, err
	}
	return rawResult{}, fmt.Errorf("unknown source kind %q", src.kind)
}

// resolveGoogleToken mints an access token when Google auth is configured;
// returns "" (no error) when nothing is configured, allowing link-shared sheets.
func resolveGoogleToken(cfg *GoogleConfig) (string, error) {
	if cfg == nil || (cfg.SAKeyFile == "" && cfg.Refresh == nil) {
		return "", nil // no auth -> attempt anonymous (link-shared) access
	}
	ts, err := googleTokenSource(context.Background(), cfg)
	if err != nil {
		return "", err
	}
	return googleAccessToken(ts)
}

// googleV4Auth resolves credentials for the Sheets API v4 path: a Bearer token
// (service-account/refresh) and/or an API key (public sheets). At least one is
// required — v4 rejects anonymous requests.
func googleV4Auth(cfg *GoogleConfig) (bearer, apiKey string, err error) {
	bearer, err = resolveGoogleToken(cfg)
	if err != nil {
		return "", "", err
	}
	if cfg != nil && cfg.ApiKeyEnv != "" {
		apiKey = os.Getenv(cfg.ApiKeyEnv)
	}
	if bearer == "" && apiKey == "" {
		return "", "", fmt.Errorf("Sheets API v4 (--range/--all-tabs) needs google auth: set google.saKeyFile or google.refresh (or google.apiKeyEnv for a public sheet) in .skillrunner/fetch.json")
	}
	return bearer, apiKey, nil
}

// --- cache (mirrors sr pull) ---

func readCachedFetch(path, contentHash string) (*FetchDigest, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var d FetchDigest
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, false
	}
	if d.ContentHash != contentHash {
		return nil, false
	}
	return &d, true
}

func writeCachedFetch(path string, d *FetchDigest) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return // cache is best-effort; a failure must not abort the run
	}
	_ = os.WriteFile(path, []byte(mustJSON(d)), 0o644)
}

// writeGenerated writes markdown with a tool-owned header, refusing to clobber a
// file that lacks it (same guard as sr pull).
func writeGenerated(path, markdown string) error {
	const header = "<!-- generated by sr fetch — do not edit -->\n\n"
	if existing, err := os.ReadFile(path); err == nil {
		if !strings.Contains(string(existing), "generated by sr fetch") {
			return fmt.Errorf("refusing to overwrite %s (not a sr fetch generated file)", path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("prepare out dir: %w", err)
	}
	return os.WriteFile(path, []byte(header+markdown), 0o644)
}
