package skill

import (
	"encoding/json"
	"testing"
)

const sampleSpec = `{
  "openapi": "3.0.1",
  "servers": [{ "url": "/env/dev/api" }],
  "components": {
    "parameters": {
      "Keyword": { "name": "keyword", "in": "query" }
    }
  },
  "paths": {
    "/khoa": {
      "get": {
        "operationId": "getKhoaList",
        "tags": ["Khoa"],
        "parameters": [{ "$ref": "#/components/parameters/Keyword" }]
      },
      "post": {
        "operationId": "createKhoa",
        "tags": ["Khoa"],
        "requestBody": { "content": { "application/json": {} } }
      }
    },
    "/khoa/{id}": {
      "get": { "operationId": "getKhoaById", "tags": ["Khoa"] },
      "delete": { "operationId": "deleteKhoa", "tags": ["Khoa"] }
    },
    "/khoa/stats": {
      "get": { "tags": ["Khoa"] }
    },
    "/nhanvien": {
      "get": { "operationId": "getNhanVien", "tags": ["NhanVien"] }
    }
  }
}`

func mustSpec(t *testing.T) *oaSpec {
	t.Helper()
	var s oaSpec
	if err := json.Unmarshal([]byte(sampleSpec), &s); err != nil {
		t.Fatalf("unmarshal sample spec: %v", err)
	}
	return &s
}

func TestBuildCodegenFiltersAndMaps(t *testing.T) {
	data, deps, err := buildCodegen(mustSpec(t), "Khoa", "/env/dev/api")
	if err != nil {
		t.Fatalf("buildCodegen: %v", err)
	}

	// Only Khoa ops (5), not NhanVien.
	if len(data.Endpoints) != 5 {
		t.Fatalf("want 5 Khoa endpoints, got %d: %+v", len(data.Endpoints), deps)
	}
	if len(deps) != len(data.Endpoints) {
		t.Fatalf("digest endpoints (%d) != codegen endpoints (%d)", len(deps), len(data.Endpoints))
	}

	by := map[string]CodegenEndpoint{}
	for _, e := range data.Endpoints {
		by[e.Op] = e
	}

	list := by["getKhoaList"]
	if !list.IsQuery || !list.HasQuery || !list.HasOp || list.HookName != "useGetKhoaList" {
		t.Errorf("getKhoaList wrong: %+v", list)
	}
	if list.PathExpr != "'/khoa'" {
		t.Errorf("getKhoaList PathExpr = %q", list.PathExpr)
	}

	byID := by["getKhoaById"]
	if len(byID.PathParams) != 1 || byID.PathParams[0] != "id" || !byID.NeedsVars {
		t.Errorf("getKhoaById path params wrong: %+v", byID)
	}
	if byID.PathExpr != "`/khoa/${vars.id}`" {
		t.Errorf("getKhoaById PathExpr = %q", byID.PathExpr)
	}

	create := by["createKhoa"]
	if create.IsQuery || !create.HasBody || !create.HasOp {
		t.Errorf("createKhoa wrong: %+v", create)
	}

	del := by["deleteKhoa"]
	if del.IsQuery || del.HasBody || len(del.PathParams) != 1 {
		t.Errorf("deleteKhoa wrong: %+v", del)
	}

	// /khoa/stats has no operationId -> synthesized op, HasOp false.
	found := false
	for _, e := range data.Endpoints {
		if e.Path == "/khoa/stats" {
			found = true
			if e.HasOp {
				t.Errorf("stats endpoint should have HasOp=false: %+v", e)
			}
		}
	}
	if !found {
		t.Error("stats endpoint (no operationId) was dropped")
	}

	if !data.HasQueries || !data.HasMutations || !data.UsesOperations {
		t.Errorf("data flags wrong: q=%v m=%v ops=%v", data.HasQueries, data.HasMutations, data.UsesOperations)
	}
}

func TestBuildCodegenUnknownTag(t *testing.T) {
	_, _, err := buildCodegen(mustSpec(t), "KhongCo", "/x")
	if err == nil {
		t.Fatal("expected error for unknown tag")
	}
}

func TestPullHelpers(t *testing.T) {
	cases := []struct{ in, want string }{
		{"getKhoaList", "GetKhoaList"},
		{"get_khoa_stats", "GetKhoaStats"},
		{"delete-khoa", "DeleteKhoa"},
	}
	for _, c := range cases {
		if got := pascal(c.in); got != c.want {
			t.Errorf("pascal(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := slug("HR API"); got != "hr-api" {
		t.Errorf("slug(HR API) = %q", got)
	}
	if got := pathExpr("/khoa/{id}/con/{cid}", []string{"id", "cid"}); got != "`/khoa/${vars.id}/con/${vars.cid}`" {
		t.Errorf("pathExpr nested = %q", got)
	}
}
