package uiserver

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ghmsoft/skillrunner/internal/skill"
	"github.com/ghmsoft/skillrunner/internal/uistore"
)

//go:embed assets/index.html
var assets embed.FS

// Server serves the embedded SPA + JSON API. It is stateless per request except
// for a startup nonce that the SPA must echo (X-SR-UI) to block cross-site calls.
type Server struct {
	nonce string
	html  []byte
	now   func() time.Time // injectable for tests
}

// New loads the SPA, injects a fresh nonce, and returns a Server.
func New() (*Server, error) {
	raw, err := assets.ReadFile("assets/index.html")
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	nonce := hex.EncodeToString(buf)
	return &Server{
		nonce: nonce,
		html:  []byte(strings.Replace(string(raw), "__NONCE__", nonce, 1)),
		now:   time.Now,
	}, nil
}

// Handler builds the router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.index)
	mux.HandleFunc("GET /api/browse", s.guard(s.browse))
	mux.HandleFunc("GET /api/config", s.guard(s.getConfig))
	mux.HandleFunc("POST /api/tags", s.guard(s.putTag))
	mux.HandleFunc("DELETE /api/tags/{tag}", s.guard(s.delTag))
	mux.HandleFunc("POST /api/tags/{tag}/check", s.guard(s.checkTag))
	mux.HandleFunc("POST /api/sessions", s.guard(s.putSession))
	mux.HandleFunc("DELETE /api/sessions/{name}", s.guard(s.delSession))
	mux.HandleFunc("POST /api/run", s.guard(s.runHandler(false)))
	mux.HandleFunc("POST /api/refresh", s.guard(s.runHandler(true)))
	return mux
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(s.html)
}

// guard enforces the nonce header and a localhost-only Origin (CSRF defense).
func (s *Server) guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-SR-UI") != s.nonce {
			fail(w, http.StatusForbidden, fmt.Errorf("missing/invalid UI token"))
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !isLocalOrigin(origin) {
			fail(w, http.StatusForbidden, fmt.Errorf("cross-origin blocked"))
			return
		}
		h(w, r)
	}
}

func isLocalOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

// --- handlers ---

func (s *Server) browse(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		if home, err := os.UserHomeDir(); err == nil {
			path = home
		} else {
			path = "/"
		}
	}
	path = filepath.Clean(path)
	ents, err := os.ReadDir(path)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	type entry struct {
		Name      string `json:"name"`
		IsDir     bool   `json:"isDir"`
		IsGitRepo bool   `json:"isGitRepo"`
	}
	var out []entry
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") && e.Name() != ".skillrunner" {
			continue // dirs only; hide dotfolders (keep visible clutter low)
		}
		_, gitErr := os.Stat(filepath.Join(path, e.Name(), ".git"))
		out = append(out, entry{Name: e.Name(), IsDir: true, IsGitRepo: gitErr == nil})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, map[string]any{"path": path, "parent": filepath.Dir(path), "entries": out})
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	repo, ok := repoParam(w, r)
	if !ok {
		return
	}
	cfg, err := uistore.Load(repo)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	// Redact tokens: expose only hasToken.
	tags := map[string]any{}
	for name, t := range cfg.Tags {
		tags[name] = map[string]any{"site": t.Site, "email": t.Email, "hasToken": t.Token != ""}
	}
	writeJSON(w, map[string]any{"tags": tags, "sessions": cfg.Sessions, "history": cfg.History})
}

func (s *Server) putTag(w http.ResponseWriter, r *http.Request) {
	repo, ok := repoParam(w, r)
	if !ok {
		return
	}
	var body struct{ Tag, Site, Email, Token string }
	if !decode(w, r, &body) {
		return
	}
	if body.Tag == "" || body.Site == "" || body.Email == "" {
		fail(w, http.StatusBadRequest, fmt.Errorf("tag, site, email are required"))
		return
	}
	if err := uistore.PutTag(repo, body.Tag, uistore.Tag{Site: body.Site, Email: body.Email, Token: body.Token}); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) delTag(w http.ResponseWriter, r *http.Request) {
	repo, ok := repoParam(w, r)
	if !ok {
		return
	}
	if err := uistore.DeleteTag(repo, r.PathValue("tag")); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) checkTag(w http.ResponseWriter, r *http.Request) {
	repo, ok := repoParam(w, r)
	if !ok {
		return
	}
	cfg, err := uistore.Load(repo)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	t := cfg.Tags[r.PathValue("tag")]
	if t == nil {
		fail(w, http.StatusNotFound, fmt.Errorf("tag not found"))
		return
	}
	acct, err := skill.ConfluenceCheck(t.Site, t.Email, t.Token)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]any{"valid": true, "accountEmail": acct})
}

func (s *Server) putSession(w http.ResponseWriter, r *http.Request) {
	repo, ok := repoParam(w, r)
	if !ok {
		return
	}
	var body struct {
		Session    string   `json:"session"`
		Tag        string   `json:"tag"`
		Confluence []string `json:"confluence"`
		Sheets     []string `json:"sheets"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Session == "" {
		fail(w, http.StatusBadRequest, fmt.Errorf("session name required"))
		return
	}
	if err := uistore.PutSession(repo, body.Session, uistore.Session{Tag: body.Tag, Confluence: body.Confluence, Sheets: body.Sheets}); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) delSession(w http.ResponseWriter, r *http.Request) {
	repo, ok := repoParam(w, r)
	if !ok {
		return
	}
	if err := uistore.DeleteSession(repo, r.PathValue("name")); err != nil {
		fail(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) runHandler(noCache bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, ok := repoParam(w, r)
		if !ok {
			return
		}
		var body struct{ Session string }
		if !decode(w, r, &body) {
			return
		}
		results, err := RunSession(repo, body.Session, noCache, s.now())
		if err != nil {
			fail(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, map[string]any{"results": results})
	}
}

// --- helpers ---

func repoParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	dir := r.URL.Query().Get("dir")
	if dir == "" {
		fail(w, http.StatusBadRequest, fmt.Errorf("missing ?dir= (pick a repo first)"))
		return "", false
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		fail(w, http.StatusBadRequest, fmt.Errorf("repo dir not found: %s", dir))
		return "", false
	}
	return dir, true
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("bad JSON body: %w", err))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
