package skill

// Google Sheet source for `sr fetch`. Uses the CSV export endpoint with a Bearer
// access token (minted in gauth.go) — this reads private sheets shared with the
// service account, and link-shared sheets without a token. CSV -> markdown table
// keeps testcase headers intact. See docs/sr-fetch-design.md §6.

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// gsheetFetch GETs the CSV export of one tab and returns the raw CSV bytes. token
// may be empty for link-shared sheets; when set it is sent as a Bearer header.
func gsheetFetch(id, gid, token string) ([]byte, error) {
	url := "https://docs.google.com/spreadsheets/d/" + id + "/export?format=csv"
	if gid != "" {
		url += "&gid=" + gid
	}
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch google sheet: %w", err)
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("google sheet returned HTTP %d — share the sheet with the service-account email, or make it link-shared", res.StatusCode)
	case http.StatusNotFound:
		return nil, fmt.Errorf("google sheet %s (gid %s) not found", id, gid)
	default:
		return nil, fmt.Errorf("google sheet returned HTTP %d for %s", res.StatusCode, id)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	// A CSV export of a private sheet without auth silently returns an HTML login
	// page with 200 — detect that so we fail loudly instead of writing garbage.
	if looksLikeHTML(body) {
		return nil, fmt.Errorf("google sheet %s is not accessible with the given credentials (got an HTML page, not CSV)", id)
	}
	return body, nil
}

// gsheetParse turns raw CSV into (markdown, DigestTable). title is supplied by the
// caller (CSV export carries no sheet title).
func gsheetParse(raw []byte, title string) (markdown string, table DigestTable, err error) {
	rd := csv.NewReader(strings.NewReader(string(raw)))
	rd.FieldsPerRecord = -1 // tolerate ragged rows
	records, err := rd.ReadAll()
	if err != nil {
		return "", DigestTable{}, fmt.Errorf("parse CSV: %w", err)
	}
	records = trimEmptyRows(records)
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")
	if len(records) == 0 {
		b.WriteString("_(sheet trống)_\n")
		return b.String(), DigestTable{Name: title, Rows: 0}, nil
	}
	width := 0
	for _, r := range records {
		if len(r) > width {
			width = len(r)
		}
	}
	headers := padRow(append([]string{}, records[0]...), width)
	b.WriteString("| " + strings.Join(escapeCells(headers), " | ") + " |\n")
	sep := make([]string, width)
	for i := range sep {
		sep[i] = "---"
	}
	b.WriteString("| " + strings.Join(sep, " | ") + " |\n")
	for _, row := range records[1:] {
		b.WriteString("| " + strings.Join(escapeCells(padRow(append([]string{}, row...), width)), " | ") + " |\n")
	}
	return b.String(), DigestTable{Name: title, Headers: headers, Rows: len(records) - 1}, nil
}

func escapeCells(cells []string) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		c = strings.ReplaceAll(c, "|", "\\|")
		c = strings.ReplaceAll(c, "\n", " ")
		out[i] = strings.TrimSpace(c)
	}
	return out
}

// trimEmptyRows drops trailing all-empty rows (common in CSV exports).
func trimEmptyRows(records [][]string) [][]string {
	end := len(records)
	for end > 0 {
		empty := true
		for _, c := range records[end-1] {
			if strings.TrimSpace(c) != "" {
				empty = false
				break
			}
		}
		if !empty {
			break
		}
		end--
	}
	return records[:end]
}

func looksLikeHTML(b []byte) bool {
	head := strings.ToLower(strings.TrimSpace(string(b[:min(len(b), 200)])))
	return strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<html")
}
