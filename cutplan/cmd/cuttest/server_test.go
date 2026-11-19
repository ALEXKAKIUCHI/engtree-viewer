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
	if st.State != "done" || !st.Verified || len(st.Problems) != 0 {
		t.Fatalf("status: %+v", st)
	}
	var res linear.Result
	if err := json.Unmarshal(st.Result, &res); err != nil || len(res.Bars) != 3 {
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
	if st.State != "done" || !st.Verified {
		t.Fatalf("status: %+v", st)
	}
	var res rect.Result
	if err := json.Unmarshal(st.Result, &res); err != nil || len(res.Sheets) != 1 {
		t.Fatalf("result: %v, %d sheets (want 1)", err, len(res.Sheets))
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
	// A job the planner refuses ends as an error, with the reason.
	post(t, h, "/api/run", `{"mode":"linear","job":{"parts":[],"stock":[]}}`)
	if st := s.waitDone(5 * time.Second); st.State != "error" || st.Error == "" {
		t.Errorf("empty job: %+v", st)
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
	if st.State != "done" || !st.Verified {
		t.Fatalf("after stop: state %s, verified %v, problems %v", st.State, st.Verified, st.Problems)
	}
	if time.Since(began) > 5*time.Second {
		t.Errorf("stop took %v", time.Since(began))
	}
}

func TestGibenExport(t *testing.T) {
	s := newServer()
	h := s.routes()
	job := `{"parts":[{"id":"Side","length":1705,"width":785,"quantity":3,"can_rotate":true},
	  {"id":"Shelf","length":968,"width":745,"quantity":4,"can_rotate":true}],
	  "stock":[{"id":"MDF","length":2750,"width":1850}],"settings":{"kerf":4.4,"edge_trim":10,"stages":3,"time_limit_sec":1}}`
	if rec := post(t, h, "/api/run", `{"mode":"rectangular","job":`+job+`}`); rec.Code != 200 {
		t.Fatalf("run: %d %s", rec.Code, rec.Body)
	}
	s.waitDone(10 * time.Second)
	st := getStatus(t, h)
	if !st.Verified {
		t.Fatalf("status: %+v", st)
	}
	rec := post(t, h, "/api/giben", `{"job":`+job+`,"result":`+string(st.Result)+`,"material":"MDF BRANCO 15MM","thickness":15,"project":"P1"}`)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("giben: %d %s", rec.Code, rec.Body)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil || len(zr.File) != 2 || zr.File[0].Name != "MDF_BRANCO_15MM_15.0.AC" || zr.File[1].Name != "MDF_BRANCO_15MM_15.0.AD" {
		t.Fatalf("zip: %v %v", err, zr)
	}
}
