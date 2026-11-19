package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/linear"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/rect"
)

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec
}

func getStatus(t *testing.T, h http.Handler) status {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	var st status
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestPageIsServed(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer().routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != 200 || !bytes.Contains(rec.Body.Bytes(), []byte("<html")) {
		t.Fatalf("page: %d", rec.Code)
	}
}

// only is the single material of a run.
func only(t *testing.T, st status) materialStatus {
	t.Helper()
	if len(st.Materials) != 1 {
		t.Fatalf("%d materials, want 1: %+v", len(st.Materials), st)
	}
	return st.Materials[0]
}

func TestLinearRun(t *testing.T) {
	s := newServer()
	h := s.routes()
	body := `{"mode":"linear","job":{"parts":[{"id":"A","length":2000,"quantity":5}],
	  "stock":[{"id":"bar","length":6000}],"settings":{"kerf":3,"start_loss":20,"end_loss":150}}}`
	if rec := post(t, h, "/api/run", body); rec.Code != 200 {
		t.Fatalf("run: %d %s", rec.Code, rec.Body)
	}
	s.waitDone(10 * time.Second)
	st := getStatus(t, h)
	m := only(t, st)
	if st.State != "done" || !m.Verified || len(m.Problems) != 0 {
		t.Fatalf("status: %+v", st)
	}
	var res linear.Result
	if err := json.Unmarshal(m.Result, &res); err != nil || len(res.Bars) != 3 {
		t.Fatalf("result: %v, %d bars (want 3)", err, len(res.Bars))
	}
}

func TestRectangularRun(t *testing.T) {
	s := newServer()
	h := s.routes()
	body := `{"mode":"rectangular","job":{"parts":[{"id":"A","length":1000,"width":500,"quantity":4}],
	  "stock":[{"id":"sheet","length":2750,"width":1830}],"settings":{"kerf":4,"edge_trim":10}}}`
	if rec := post(t, h, "/api/run", body); rec.Code != 200 {
		t.Fatalf("run: %d %s", rec.Code, rec.Body)
	}
	s.waitDone(10 * time.Second)
	st := getStatus(t, h)
	m := only(t, st)
	if st.State != "done" || !m.Verified {
		t.Fatalf("status: %+v", st)
	}
	var res rect.Result
	if err := json.Unmarshal(m.Result, &res); err != nil || len(res.Sheets) != 1 {
		t.Fatalf("result: %v, %d sheets (want 1)", err, len(res.Sheets))
	}
}

// Three materials among the parts: two have stock, one has none. Each
// material is planned only with its own stock.
func TestMaterialsArePlannedApart(t *testing.T) {
	s := newServer()
	h := s.routes()
	body := `{"mode":"rectangular","job":{"parts":[
	    {"id":"A","length":1000,"width":500,"quantity":2,"material":"MDF15"},
	    {"id":"B","length":600,"width":400,"quantity":3,"material":"mdf6 "},
	    {"id":"C","length":800,"width":300,"quantity":1,"material":"MDF18"},
	    {"id":"D","length":700,"width":450,"quantity":1,"material":"MDF15"}],
	  "stock":[
	    {"id":"chapa 6","length":2750,"width":1830,"material":"MDF6","material_desc":"MDF BRANCO 6MM","thickness":6},
	    {"id":"chapa 15","length":2750,"width":1830,"material":"MDF15","material_desc":"MDF BRANCO 15MM","thickness":15},
	    {"id":"chapa 25","length":2750,"width":1830,"material":"MDF25"}],
	  "settings":{"kerf":4,"edge_trim":10,"time_limit_sec":1}}}`
	if rec := post(t, h, "/api/run", body); rec.Code != 200 {
		t.Fatalf("run: %d %s", rec.Code, rec.Body)
	}
	st := s.waitDone(20 * time.Second)
	if st.State != "done" || len(st.Materials) != 2 {
		t.Fatalf("status: %+v", st)
	}
	want := []struct {
		code, desc, stock string
		parts             []string
	}{{"MDF15", "MDF BRANCO 15MM", "chapa 15", []string{"A", "D"}}, {"MDF6", "MDF BRANCO 6MM", "chapa 6", []string{"B"}}}
	for i, w := range want {
		m := st.Materials[i]
		if m.Code != w.code || m.Desc != w.desc || !m.Verified {
			t.Errorf("material %d: %s %q verified %v %v", i, m.Code, m.Desc, m.Verified, m.Problems)
		}
		var res rect.Result
		_ = json.Unmarshal(m.Result, &res)
		seen := map[string]bool{}
		for _, sh := range res.Sheets {
			if sh.StockID != w.stock {
				t.Errorf("%s cut from %s", w.code, sh.StockID)
			}
			for _, p := range sh.Placements {
				seen[p.PartID] = true
			}
		}
		for _, p := range w.parts {
			if !seen[p] {
				t.Errorf("%s: part %s not placed", w.code, p)
			}
		}
		if len(seen) != len(w.parts) {
			t.Errorf("%s: parts of another material placed: %v", w.code, seen)
		}
	}
	if len(st.NoStock) != 1 || st.NoStock[0].Code != "MDF18" || strings.Join(st.NoStock[0].Parts, ",") != "C" {
		t.Errorf("no stock: %+v", st.NoStock)
	}
}

func TestNoMaterialColumnIsOneMaterial(t *testing.T) {
	jobs, missing, err := splitByMaterial("linear", json.RawMessage(`{"parts":[{"id":"A","length":100,"quantity":1}],
	  "stock":[{"id":"bar","length":6000}],"settings":{"kerf":3}}`))
	if err != nil || len(jobs) != 1 || len(missing) != 0 || jobs[0].Code != "" {
		t.Fatalf("%v %+v %+v", err, jobs, missing)
	}
	// A part with no code does not take a coded sheet.
	_, missing, _ = splitByMaterial("linear", json.RawMessage(`{"parts":[{"id":"A","length":100,"quantity":1}],
	  "stock":[{"id":"bar","length":6000,"material":"TB20"}],"settings":{}}`))
	if len(missing) != 1 || missing[0].Code != "" {
		t.Errorf("missing: %+v", missing)
	}
}

func TestBadRequests(t *testing.T) {
	s := newServer()
	h := s.routes()
	if rec := post(t, h, "/api/run", `{"mode":"spiral","job":{}}`); rec.Code != 400 {
		t.Errorf("unknown mode: %d", rec.Code)
	}
	if rec := post(t, h, "/api/run", `not json`); rec.Code != 400 {
		t.Errorf("broken body: %d", rec.Code)
	}
	// A job the planner refuses ends that material as an error, with the reason.
	post(t, h, "/api/run", `{"mode":"linear","job":{"parts":[{"id":"A","length":0,"quantity":1}],"stock":[{"id":"bar","length":6000}],"settings":{}}}`)
	st := s.waitDone(5 * time.Second)
	if m := only(t, st); m.State != "error" || m.Error == "" {
		t.Errorf("refused job: %+v", st)
	}
}

func TestStopEndsTheRunWithAPlan(t *testing.T) {
	s := newServer()
	h := s.routes()
	// Long enough to still be running when the stop arrives.
	var parts []string
	for i := 0; i < 60; i++ {
		parts = append(parts, `{"id":"P`+string(rune('A'+i%26))+string(rune('a'+i/26))+`","length":`+
			[]string{"731", "1210", "455", "1893", "640"}[i%5]+`,"width":`+[]string{"333", "517", "290"}[i%3]+`,"quantity":9,"can_rotate":true}`)
	}
	body := `{"mode":"rectangular","job":{"parts":[` + strings.Join(parts, ",") + `],
	  "stock":[{"id":"sheet","length":2750,"width":1830}],"settings":{"kerf":4,"edge_trim":10,"time_limit_sec":30}}}`
	post(t, h, "/api/run", body)
	time.Sleep(300 * time.Millisecond)
	began := time.Now()
	post(t, h, "/api/stop", "")
	st := s.waitDone(10 * time.Second)
	if m := only(t, st); st.State != "done" || !m.Verified {
		t.Fatalf("after stop: state %s, verified %v, problems %v", st.State, m.Verified, m.Problems)
	}
	if time.Since(began) > 5*time.Second {
		t.Errorf("stop took %v", time.Since(began))
	}
}

func TestExports(t *testing.T) {
	s := newServer()
	h := s.routes()
	job := `{"parts":[{"id":"Side","length":1705,"width":785,"quantity":3,"can_rotate":true,"material":"MDF15"},
	  {"id":"Shelf","length":968,"width":745,"quantity":4,"can_rotate":true,"material":"MDF15"},
	  {"id":"Back","length":1200,"width":800,"quantity":1,"material":"MDF6"}],
	  "stock":[{"id":"MDF","length":2750,"width":1850,"material":"MDF15","material_desc":"MDF BRANCO 15MM","thickness":15},
	  {"id":"MDF6","length":2750,"width":1850,"material":"MDF6","material_desc":"MDF BRANCO 6MM","thickness":6}],
	  "settings":{"kerf":4.4,"edge_trim":10,"stages":3,"time_limit_sec":1}}`
	if rec := post(t, h, "/api/run", `{"mode":"rectangular","job":`+job+`}`); rec.Code != 200 {
		t.Fatalf("run: %d %s", rec.Code, rec.Body)
	}
	st := s.waitDone(20 * time.Second)
	if len(st.Materials) != 2 || !st.Materials[0].Verified || !st.Materials[1].Verified {
		t.Fatalf("status: %+v", st)
	}
	mats, _ := json.Marshal(st.Materials)
	body := `{"mode":"rectangular","lang":"pt","info":{"product":"P1","product_desc":"MESA"},"materials":` + string(mats) + `,
	  "parts":[{"id":"Side","desc":"LATERAL","project":"OBRA-7"}]}`

	rec := post(t, h, "/api/pdf", body)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(rec.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("pdf: %d %s", rec.Code, rec.Body)
	}

	rec = post(t, h, "/api/production", body)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("production: %d %s", rec.Code, rec.Body)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil || len(zr.File) != 5 {
		t.Fatalf("zip: %v %d files", err, len(zr.File))
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if !strings.HasPrefix(names[0], "Plano de corte - P1 - MESA - ") || !strings.HasSuffix(names[0], ".pdf") ||
		strings.Join(names[1:], ",") != "MDF_BRANCO_15MM_15.0.AC,MDF_BRANCO_15MM_15.0.AD,MDF_BRANCO_6MM_6.0.AC,MDF_BRANCO_6MM_6.0.AD" {
		t.Errorf("files: %q", names)
	}

	// A plan the verifier refuses is not exported.
	bad := strings.Replace(body, string(st.Materials[1].Result), `{"sheets":[{"stock_id":"MDF6","length":2750,"width":1850,"placements":[{"part_id":"Back","instance":1,"x":0,"y":0}]}]}`, 1)
	if rec := post(t, h, "/api/production", bad); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "MDF6") {
		t.Errorf("a wrong plan was exported: %d %s", rec.Code, rec.Body)
	}
	if rec := post(t, h, "/api/pdf", `{}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty request: %d", rec.Code)
	}
}
