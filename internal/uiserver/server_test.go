package uiserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(s.Handler()), s.nonce
}

func do(t *testing.T, ts *httptest.Server, nonce, method, path string, body string) (int, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, ts.URL+path, rdr)
	if nonce != "" {
		req.Header.Set("X-SR-UI", nonce)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	var j map[string]any
	if len(b) > 0 {
		json.Unmarshal(b, &j)
	}
	return res.StatusCode, j
}

func TestGuardRejectsWithoutNonce(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()
	code, _ := do(t, ts, "", "GET", "/api/config?dir=/tmp", "")
	if code != http.StatusForbidden {
		t.Errorf("no-nonce request = %d, want 403", code)
	}
}

func TestBrowse(t *testing.T) {
	ts, nonce := newTestServer(t)
	defer ts.Close()
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "myrepo", ".git"), 0o755)
	os.MkdirAll(filepath.Join(root, "plain"), 0o755)

	code, j := do(t, ts, nonce, "GET", "/api/browse?path="+root, "")
	if code != 200 {
		t.Fatalf("browse = %d (%v)", code, j)
	}
	entries, _ := j["entries"].([]any)
	if len(entries) != 2 {
		t.Fatalf("entries = %v", entries)
	}
	var gitMarked bool
	for _, e := range entries {
		m := e.(map[string]any)
		if m["name"] == "myrepo" && m["isGitRepo"] == true {
			gitMarked = true
		}
	}
	if !gitMarked {
		t.Errorf("git repo not marked: %v", entries)
	}
}

func TestTagsCrudRedactsToken(t *testing.T) {
	ts, nonce := newTestServer(t)
	defer ts.Close()
	repo := t.TempDir()

	code, _ := do(t, ts, nonce, "POST", "/api/tags?dir="+repo,
		`{"tag":"X","site":"acme.atlassian.net","email":"me@x.vn","token":"secret"}`)
	if code != 200 {
		t.Fatalf("putTag = %d", code)
	}

	code, j := do(t, ts, nonce, "GET", "/api/config?dir="+repo, "")
	if code != 200 {
		t.Fatal(code)
	}
	tags := j["tags"].(map[string]any)
	x := tags["X"].(map[string]any)
	if x["hasToken"] != true {
		t.Error("hasToken should be true")
	}
	if _, leaked := x["token"]; leaked {
		t.Error("token must not be returned to the client")
	}

	// The raw token is on disk (0600) but never over the wire.
	raw, _ := os.ReadFile(filepath.Join(repo, ".skillrunner", "ui.json"))
	if !strings.Contains(string(raw), "secret") {
		t.Error("token should be persisted on disk")
	}

	code, _ = do(t, ts, nonce, "DELETE", "/api/tags/X?dir="+repo, "")
	if code != 200 {
		t.Fatalf("delTag = %d", code)
	}
	_, j = do(t, ts, nonce, "GET", "/api/config?dir="+repo, "")
	if len(j["tags"].(map[string]any)) != 0 {
		t.Error("tag not deleted")
	}
}

func TestSessionsCrud(t *testing.T) {
	ts, nonce := newTestServer(t)
	defer ts.Close()
	repo := t.TempDir()
	code, _ := do(t, ts, nonce, "POST", "/api/sessions?dir="+repo,
		`{"session":"feat","tag":"X","confluence":["https://c/1"],"sheets":["https://s/1"]}`)
	if code != 200 {
		t.Fatalf("putSession = %d", code)
	}
	_, j := do(t, ts, nonce, "GET", "/api/config?dir="+repo, "")
	sessions := j["sessions"].(map[string]any)
	if _, ok := sessions["feat"]; !ok {
		t.Errorf("session not saved: %v", sessions)
	}
}

func TestRepoParamRequired(t *testing.T) {
	ts, nonce := newTestServer(t)
	defer ts.Close()
	code, _ := do(t, ts, nonce, "GET", "/api/config", "")
	if code != http.StatusBadRequest {
		t.Errorf("missing dir = %d, want 400", code)
	}
}

func TestRunSessionErrors(t *testing.T) {
	repo := t.TempDir()
	// Unknown session.
	if _, err := RunSession(repo, "nope", false, time.Now()); err == nil {
		t.Error("expected error for unknown session")
	}
}
