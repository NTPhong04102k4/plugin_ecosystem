package skill

// Confluence source for `sr fetch`: given a page URL + Basic auth (email + API
// token), fetch the page via the v2 REST API in ADF (atlas_doc_format) and
// normalize it to clean markdown. ADF is JSON, so table extraction is structural
// and deterministic — no HTML parsing. See docs/sr-fetch-design.md §6.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// confluenceFetch GETs the page body (ADF) and returns the raw response bytes.
// Kept separate from parsing so the caller can hash raw bytes for the cache.
func confluenceFetch(site, id, email, token string) ([]byte, error) {
	if email == "" || token == "" {
		return nil, fmt.Errorf("confluence needs an email and API token (set --email and the token env; see .skillrunner/fetch.json)")
	}
	url := "https://" + site + "/wiki/api/v2/pages/" + id + "?body-format=atlas_doc_format"
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.SetBasicAuth(email, token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch confluence page: %w", err)
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("confluence returned HTTP %d — token/email wrong or no permission on page %s", res.StatusCode, id)
	case http.StatusNotFound:
		return nil, fmt.Errorf("confluence page %s not found (check the URL/id)", id)
	default:
		return nil, fmt.Errorf("confluence returned HTTP %d for page %s", res.StatusCode, id)
	}
	return io.ReadAll(res.Body)
}

// confluencePage is the shallow shape of a v2 page response.
type confluencePage struct {
	Title string `json:"title"`
	Body  struct {
		ADF struct {
			Value string `json:"value"` // JSON-encoded ADF document as a string
		} `json:"atlas_doc_format"`
	} `json:"body"`
}

// confluenceParse turns a raw v2 response into (title, markdown, tables, sections).
func confluenceParse(raw []byte) (title, markdown string, tables []DigestTable, sections []string, err error) {
	var page confluencePage
	if err = json.Unmarshal(raw, &page); err != nil {
		return "", "", nil, nil, fmt.Errorf("parse confluence response: %w", err)
	}
	title = page.Title
	if page.Body.ADF.Value == "" {
		return title, "# " + title + "\n\n_(trang trống)_\n", nil, nil, nil
	}
	var doc adfNode
	if err = json.Unmarshal([]byte(page.Body.ADF.Value), &doc); err != nil {
		return "", "", nil, nil, fmt.Errorf("parse ADF body: %w", err)
	}
	r := &adfRenderer{}
	r.blocks(doc.Content, "")
	body := strings.TrimRight(r.b.String(), "\n")
	markdown = "# " + title + "\n\n" + body + "\n"
	return title, markdown, r.tables, r.sections, nil
}

// --- ADF (Atlassian Document Format) model + renderer ---

type adfNode struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	Attrs   map[string]any  `json:"attrs,omitempty"`
	Marks   []adfMark       `json:"marks,omitempty"`
	Content []adfNode       `json:"content,omitempty"`
}

type adfMark struct {
	Type string `json:"type"`
}

// adfRenderer accumulates markdown while collecting table + heading metadata.
type adfRenderer struct {
	b        strings.Builder
	tables   []DigestTable
	sections []string
}

// blocks renders a slice of block-level nodes. prefix is prepended to each line
// (used to indent list items).
func (r *adfRenderer) blocks(nodes []adfNode, prefix string) {
	for _, n := range nodes {
		r.block(n, prefix)
	}
}

func (r *adfRenderer) block(n adfNode, prefix string) {
	switch n.Type {
	case "paragraph":
		if txt := r.inline(n.Content); strings.TrimSpace(txt) != "" {
			r.line(prefix + txt)
			r.b.WriteByte('\n')
		}
	case "heading":
		level := 2
		if lv, ok := attrInt(n.Attrs, "level"); ok {
			level = lv
		}
		txt := r.inline(n.Content)
		r.line(strings.Repeat("#", level) + " " + txt)
		r.b.WriteByte('\n')
		if s := strings.TrimSpace(txt); s != "" {
			r.sections = append(r.sections, s)
		}
	case "bulletList":
		r.list(n.Content, prefix, "- ")
	case "orderedList":
		r.list(n.Content, prefix, "1. ")
	case "codeBlock":
		r.line("```")
		r.line(r.inline(n.Content))
		r.line("```")
		r.b.WriteByte('\n')
	case "blockquote":
		sub := &adfRenderer{}
		sub.blocks(n.Content, "")
		for _, ln := range strings.Split(strings.TrimRight(sub.b.String(), "\n"), "\n") {
			r.line("> " + ln)
		}
		r.b.WriteByte('\n')
	case "rule":
		r.line("---")
		r.b.WriteByte('\n')
	case "table":
		r.table(n)
	default:
		// Unknown block: recurse into its children so content is never dropped.
		if len(n.Content) > 0 {
			r.blocks(n.Content, prefix)
		}
	}
}

func (r *adfRenderer) list(items []adfNode, prefix, bullet string) {
	for _, item := range items {
		if item.Type != "listItem" {
			continue
		}
		// A listItem's first paragraph is the bullet text; nested lists indent.
		first := true
		for _, child := range item.Content {
			if child.Type == "paragraph" {
				txt := r.inline(child.Content)
				if first {
					r.line(prefix + bullet + txt)
					first = false
				} else {
					r.line(prefix + "  " + txt)
				}
			} else {
				r.block(child, prefix+"  ")
			}
		}
	}
	r.b.WriteByte('\n')
}

// table renders an ADF table as a markdown table (first row = header) and records
// a DigestTable (headers + data-row count).
func (r *adfRenderer) table(n adfNode) {
	var rows [][]string
	for _, row := range n.Content {
		if row.Type != "tableRow" {
			continue
		}
		var cells []string
		for _, cell := range row.Content {
			if cell.Type != "tableCell" && cell.Type != "tableHeader" {
				continue
			}
			cells = append(cells, cellText(cell))
		}
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	}
	if len(rows) == 0 {
		return
	}
	headers := rows[0]
	r.line("| " + strings.Join(headers, " | ") + " |")
	sep := make([]string, len(headers))
	for i := range sep {
		sep[i] = "---"
	}
	r.line("| " + strings.Join(sep, " | ") + " |")
	for _, row := range rows[1:] {
		r.line("| " + strings.Join(padRow(row, len(headers)), " | ") + " |")
	}
	r.b.WriteByte('\n')
	r.tables = append(r.tables, DigestTable{Headers: headers, Rows: len(rows) - 1})
}

// inline renders inline nodes (text with bold/italic/code marks) to a string.
func (r *adfRenderer) inline(nodes []adfNode) string {
	var b strings.Builder
	for _, n := range nodes {
		switch n.Type {
		case "text":
			b.WriteString(applyMarks(n.Text, n.Marks))
		case "hardBreak":
			b.WriteString("<br>")
		case "inlineCard":
			if href := attrString(n.Attrs, "url"); href != "" {
				b.WriteString(href)
			}
		case "mention":
			if txt := attrString(n.Attrs, "text"); txt != "" {
				b.WriteString(txt)
			}
		default:
			if len(n.Content) > 0 {
				b.WriteString(r.inline(n.Content))
			}
		}
	}
	return b.String()
}

func (r *adfRenderer) line(s string) {
	r.b.WriteString(s)
	r.b.WriteByte('\n')
}

// cellText flattens a table cell's block content into one pipe-safe line.
func cellText(cell adfNode) string {
	sub := &adfRenderer{}
	txt := strings.TrimSpace(sub.inline(flattenInline(cell.Content)))
	txt = strings.ReplaceAll(txt, "|", "\\|")
	txt = strings.ReplaceAll(txt, "\n", " ")
	return txt
}

// flattenInline collects inline nodes from a cell's paragraphs into one slice,
// joining paragraphs with a break so multi-line cells stay on one row.
func flattenInline(nodes []adfNode) []adfNode {
	var out []adfNode
	for i, n := range nodes {
		if n.Type == "paragraph" {
			if i > 0 {
				out = append(out, adfNode{Type: "hardBreak"})
			}
			out = append(out, n.Content...)
		} else {
			out = append(out, n)
		}
	}
	return out
}

func applyMarks(text string, marks []adfMark) string {
	for _, m := range marks {
		switch m.Type {
		case "strong":
			text = "**" + text + "**"
		case "em":
			text = "_" + text + "_"
		case "code":
			text = "`" + text + "`"
		case "strike":
			text = "~~" + text + "~~"
		}
	}
	return text
}

func padRow(row []string, n int) []string {
	for len(row) < n {
		row = append(row, "")
	}
	return row[:n]
}

func attrInt(attrs map[string]any, key string) (int, bool) {
	if attrs == nil {
		return 0, false
	}
	if v, ok := attrs[key]; ok {
		if f, ok := v.(float64); ok {
			return int(f), true
		}
	}
	return 0, false
}

func attrString(attrs map[string]any, key string) string {
	if attrs == nil {
		return ""
	}
	if v, ok := attrs[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
