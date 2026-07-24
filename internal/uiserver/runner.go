// Package uiserver hosts the local web UI for `sr ui` plus the session runner
// that drives `sr fetch` from stored tags/sessions. See docs/sr-ui-design.md.
package uiserver

import (
	"fmt"
	"time"

	"github.com/ghmsoft/skillrunner/internal/skill"
	"github.com/ghmsoft/skillrunner/internal/uistore"
)

// RunResult is one fetched link's outcome, returned to the UI.
type RunResult struct {
	URL   string             `json:"url"`
	OK    bool               `json:"ok"`
	Error string             `json:"error,omitempty"`
	Item  *skill.FetchDigest `json:"item,omitempty"`
}

// RunSession fetches every Confluence + Sheet link of a session into <repo>, using
// the session's tag credentials (Confluence). Sheets are treated as link-shared
// (no auth). noCache forces a refresh. It appends a history entry and returns the
// per-link results. now is injected so the caller stamps time deterministically.
func RunSession(repo, session string, noCache bool, now time.Time) ([]RunResult, error) {
	cfg, err := uistore.Load(repo)
	if err != nil {
		return nil, err
	}
	s := cfg.Sessions[session]
	if s == nil {
		return nil, fmt.Errorf("session %q not found", session)
	}
	tag := cfg.Tags[s.Tag]
	if tag == nil {
		return nil, fmt.Errorf("session %q references unknown tag %q", session, s.Tag)
	}

	var results []RunResult
	var files []string
	allOK := true

	for _, url := range s.Confluence {
		d, _, err := skill.Fetch(skill.FetchOptions{
			From:       url,
			Email:      tag.Email,
			Token:      tag.Token,
			ProjectDir: repo,
			NoCache:    noCache,
		})
		results = append(results, toResult(url, d, err))
		if err != nil {
			allOK = false
		} else {
			files = append(files, d.File)
		}
	}

	for _, url := range s.Sheets {
		// Sheets are link-shared — no credentials.
		d, _, err := skill.Fetch(skill.FetchOptions{
			From:       url,
			ProjectDir: repo,
			NoCache:    noCache,
		})
		results = append(results, toResult(url, d, err))
		if err != nil {
			allOK = false
		} else {
			files = append(files, d.File)
		}
	}

	_ = uistore.AppendHistory(repo, uistore.HistoryEntry{
		Session: session,
		At:      now.UTC().Format(time.RFC3339),
		Files:   files,
		OK:      allOK,
	})
	return results, nil
}

func toResult(url string, d *skill.FetchDigest, err error) RunResult {
	if err != nil {
		return RunResult{URL: url, OK: false, Error: err.Error()}
	}
	return RunResult{URL: url, OK: true, Item: d}
}
