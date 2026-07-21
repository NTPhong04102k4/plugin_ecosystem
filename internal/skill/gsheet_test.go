package skill

import (
	"strings"
	"testing"
)

func TestGsheetParse(t *testing.T) {
	csv := "ID,Bước,Kỳ vọng\nTC1,Mở trang,Thấy form\nTC2,\"Nhập a|b\",OK\n\n"
	md, table, err := gsheetParse([]byte(csv), "Testcase Login")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Testcase Login",
		"| ID | Bước | Kỳ vọng |",
		"| --- | --- | --- |",
		"| TC1 | Mở trang | Thấy form |",
		"| TC2 | Nhập a\\|b | OK |", // pipe escaped
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q\n---\n%s", want, md)
		}
	}
	if table.Rows != 2 || len(table.Headers) != 3 {
		t.Errorf("table = %+v", table)
	}
}

func TestGsheetParseRaggedRows(t *testing.T) {
	// Second data row has fewer columns than the header — must pad, not error.
	csv := "A,B,C\n1,2,3\n4\n"
	md, table, err := gsheetParse([]byte(csv), "S")
	if err != nil {
		t.Fatal(err)
	}
	if table.Rows != 2 {
		t.Errorf("rows = %d", table.Rows)
	}
	if !strings.Contains(md, "| 4 |  |  |") {
		t.Errorf("short row not padded:\n%s", md)
	}
}

func TestGsheetParseEmpty(t *testing.T) {
	md, table, err := gsheetParse([]byte("\n\n"), "S")
	if err != nil {
		t.Fatal(err)
	}
	if table.Rows != 0 || !strings.Contains(md, "sheet trống") {
		t.Errorf("empty handling: rows=%d md=%q", table.Rows, md)
	}
}

func TestGsheetV4Parse(t *testing.T) {
	raw := []byte(`{"valueRanges":[
		{"range":"Khoa!A1:B3","values":[["ID","Tên"],["1","Nội"],["2","Ngoại"]]},
		{"range":"'Cấu hình'!A1:A1","values":[["X"]]}
	]}`)
	md, tables, err := gsheetV4Parse(raw, "Danh mục")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Danh mục",
		"## Khoa",
		"| ID | Tên |",
		"| 1 | Nội |",
		"## Cấu hình", // quoted tab name unwrapped
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q\n---\n%s", want, md)
		}
	}
	if len(tables) != 2 || tables[0].Name != "Khoa" || tables[0].Rows != 2 {
		t.Errorf("tables = %+v", tables)
	}
}

func TestTabName(t *testing.T) {
	for in, want := range map[string]string{
		"Khoa!A1:C3":     "Khoa",
		"'Cấu hình'!A1":  "Cấu hình",
		"Sheet1":         "Sheet1",
	} {
		if got := tabName(in); got != want {
			t.Errorf("tabName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksLikeHTML(t *testing.T) {
	if !looksLikeHTML([]byte("<!DOCTYPE html><html>...")) {
		t.Error("should detect html doctype")
	}
	if looksLikeHTML([]byte("ID,Name\n1,a")) {
		t.Error("csv wrongly flagged as html")
	}
}
