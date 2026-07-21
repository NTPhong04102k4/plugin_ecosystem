package skill

// Sheets API v4 path for `sr fetch`: used when --range or --all-tabs is set, to
// pull multiple tabs / a specific range (the default CSV-export path only reads
// one tab). It reads spreadsheet metadata (title + tab names) then values via
// values:batchGet. Requires a Bearer token (service-account/refresh) or, for a
// public sheet, an API key. See docs/sr-fetch-design.md §6.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// gsheetV4Base is the Sheets API host. A package var so integration tests can
// point it at a local httptest server; production uses the real API.
var gsheetV4Base = "https://sheets.googleapis.com"

// sheetMeta is the shallow metadata response (title + tabs).
type sheetMeta struct {
	Properties struct {
		Title string `json:"title"`
	} `json:"properties"`
	Sheets []struct {
		Properties struct {
			SheetID int    `json:"sheetId"`
			Title   string `json:"title"`
		} `json:"properties"`
	} `json:"sheets"`
}

// batchValues is the values:batchGet response.
type batchValues struct {
	ValueRanges []struct {
		Range  string     `json:"range"`
		Values [][]any    `json:"values"`
	} `json:"valueRanges"`
}

// gsheetV4Fetch resolves the spreadsheet title + the ranges to read (all tabs, or
// a single explicit range), fetches their values, and returns the raw values
// payload (for hashing) plus the spreadsheet title.
func gsheetV4Fetch(id, rng string, allTabs bool, bearer, apiKey string) (raw []byte, title string, err error) {
	metaRaw, err := gsheetV4Get(fmt.Sprintf("/v4/spreadsheets/%s", id),
		url.Values{"fields": {"properties.title,sheets.properties(title,sheetId)"}}, bearer, apiKey)
	if err != nil {
		return nil, "", err
	}
	var meta sheetMeta
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return nil, "", fmt.Errorf("parse sheet metadata: %w", err)
	}
	title = meta.Properties.Title

	var ranges []string
	if rng != "" {
		ranges = []string{rng}
	} else { // allTabs
		for _, s := range meta.Sheets {
			ranges = append(ranges, s.Properties.Title)
		}
	}
	if len(ranges) == 0 {
		return nil, "", fmt.Errorf("spreadsheet %s has no tabs", id)
	}

	q := url.Values{"valueRenderOption": {"FORMATTED_VALUE"}}
	for _, r := range ranges {
		q.Add("ranges", r)
	}
	raw, err = gsheetV4Get(fmt.Sprintf("/v4/spreadsheets/%s/values:batchGet", id), q, bearer, apiKey)
	if err != nil {
		return nil, "", err
	}
	return raw, title, nil
}

// gsheetV4Get performs one authenticated GET against the Sheets API.
func gsheetV4Get(path string, q url.Values, bearer, apiKey string) ([]byte, error) {
	if apiKey != "" {
		q.Set("key", apiKey)
	}
	u := gsheetV4Base + path + "?" + q.Encode()
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sheets API v4 request: %w", err)
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("sheets API v4 returned HTTP %d — share the sheet with the service-account, or check the api key", res.StatusCode)
	case http.StatusNotFound:
		return nil, fmt.Errorf("sheets API v4: spreadsheet not found")
	default:
		return nil, fmt.Errorf("sheets API v4 returned HTTP %d", res.StatusCode)
	}
	return io.ReadAll(res.Body)
}

// gsheetV4Parse renders a batchGet payload to markdown (one "## <tab>" section +
// table per range) and one DigestTable per range.
func gsheetV4Parse(raw []byte, title string) (markdown string, tables []DigestTable, err error) {
	var bv batchValues
	if err := json.Unmarshal(raw, &bv); err != nil {
		return "", nil, fmt.Errorf("parse batchGet values: %w", err)
	}
	var b strings.Builder
	b.WriteString("# " + title + "\n\n")
	if len(bv.ValueRanges) == 0 {
		b.WriteString("_(không có dữ liệu)_\n")
		return b.String(), nil, nil
	}
	for _, vr := range bv.ValueRanges {
		tab := tabName(vr.Range)
		b.WriteString("## " + tab + "\n\n")
		rows := stringifyRows(vr.Values)
		rows = trimEmptyRows(rows)
		if len(rows) == 0 {
			b.WriteString("_(tab trống)_\n\n")
			tables = append(tables, DigestTable{Name: tab, Rows: 0})
			continue
		}
		width := 0
		for _, r := range rows {
			if len(r) > width {
				width = len(r)
			}
		}
		headers := padRow(append([]string{}, rows[0]...), width)
		b.WriteString("| " + strings.Join(escapeCells(headers), " | ") + " |\n")
		sep := make([]string, width)
		for i := range sep {
			sep[i] = "---"
		}
		b.WriteString("| " + strings.Join(sep, " | ") + " |\n")
		for _, row := range rows[1:] {
			b.WriteString("| " + strings.Join(escapeCells(padRow(append([]string{}, row...), width)), " | ") + " |\n")
		}
		b.WriteString("\n")
		tables = append(tables, DigestTable{Name: tab, Headers: headers, Rows: len(rows) - 1})
	}
	return strings.TrimRight(b.String(), "\n") + "\n", tables, nil
}

// tabName extracts the sheet name from an A1 range like "Tab1!A1:C3".
func tabName(r string) string {
	if i := strings.LastIndex(r, "!"); i >= 0 {
		return strings.Trim(r[:i], "'")
	}
	return strings.Trim(r, "'")
}

// stringifyRows converts JSON cell values (strings/numbers/bools) to strings.
func stringifyRows(rows [][]any) [][]string {
	out := make([][]string, len(rows))
	for i, row := range rows {
		cells := make([]string, len(row))
		for j, c := range row {
			cells[j] = fmt.Sprint(c)
		}
		out[i] = cells
	}
	return out
}
