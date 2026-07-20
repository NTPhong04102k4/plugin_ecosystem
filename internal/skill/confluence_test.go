package skill

import (
	"encoding/json"
	"strings"
	"testing"
)

// buildConfluenceResponse wraps an ADF document in a v2 page response, with the
// ADF encoded as a JSON string in body.atlas_doc_format.value (as the API does).
func buildConfluenceResponse(t *testing.T, title string, adf map[string]any) []byte {
	t.Helper()
	adfStr, err := json.Marshal(adf)
	if err != nil {
		t.Fatal(err)
	}
	resp := map[string]any{
		"title": title,
		"body": map[string]any{
			"atlas_doc_format": map[string]any{"value": string(adfStr)},
		},
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func node(typ string, content ...map[string]any) map[string]any {
	m := map[string]any{"type": typ}
	if len(content) > 0 {
		m["content"] = content
	}
	return m
}

func text(s string, marks ...string) map[string]any {
	m := map[string]any{"type": "text", "text": s}
	if len(marks) > 0 {
		var ms []map[string]any
		for _, mk := range marks {
			ms = append(ms, map[string]any{"type": mk})
		}
		m["marks"] = ms
	}
	return m
}

func TestConfluenceParse(t *testing.T) {
	heading := node("heading", text("Mục tiêu"))
	heading["attrs"] = map[string]any{"level": 2}

	para := node("paragraph", text("Xin chào "), text("đậm", "strong"))

	list := node("bulletList",
		node("listItem", node("paragraph", text("item 1"))),
		node("listItem", node("paragraph", text("item 2"))),
	)

	headerRow := node("tableRow",
		node("tableHeader", node("paragraph", text("ID"))),
		node("tableHeader", node("paragraph", text("Bước"))),
	)
	dataRow := node("tableRow",
		node("tableCell", node("paragraph", text("TC1"))),
		node("tableCell", node("paragraph", text("Mở | trang"))),
	)
	table := node("table", headerRow, dataRow)

	adf := map[string]any{
		"version": 1, "type": "doc",
		"content": []map[string]any{heading, para, list, table},
	}
	raw := buildConfluenceResponse(t, "Spec đăng nhập", adf)

	title, md, tables, sections, err := confluenceParse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Spec đăng nhập" {
		t.Errorf("title = %q", title)
	}
	if !strings.HasPrefix(md, "# Spec đăng nhập\n") {
		t.Errorf("markdown should start with H1 title, got:\n%s", md)
	}
	for _, want := range []string{
		"## Mục tiêu",
		"Xin chào **đậm**",
		"- item 1",
		"- item 2",
		"| ID | Bước |",
		"| --- | --- |",
		"| TC1 | Mở \\| trang |", // pipe inside a cell is escaped
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q\n---\n%s", want, md)
		}
	}
	if len(tables) != 1 || tables[0].Rows != 1 || len(tables[0].Headers) != 2 {
		t.Errorf("tables = %+v", tables)
	}
	if len(sections) != 1 || sections[0] != "Mục tiêu" {
		t.Errorf("sections = %v", sections)
	}
}

func TestConfluenceParseEmptyBody(t *testing.T) {
	raw := []byte(`{"title":"Trống","body":{"atlas_doc_format":{"value":""}}}`)
	title, md, tables, _, err := confluenceParse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Trống" || !strings.Contains(md, "trang trống") || tables != nil {
		t.Errorf("empty body handling: title=%q md=%q tables=%v", title, md, tables)
	}
}
