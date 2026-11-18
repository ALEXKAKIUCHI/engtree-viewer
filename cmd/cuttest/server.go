package main

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ALEXKAKIUCHI/engtree-viewer/extension"
	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/cutplan"
	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/cutplan/giben"
	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/cutplan/linear"
	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/cutplan/rect"
)

//go:embed page.html
var page []byte

// server runs one plan at a time. The page starts a run, then asks for
// the status until it is done.
type server struct {
	mu     sync.Mutex
	run    int // number of the current run; a newer run replaces an older one
	cancel context.CancelFunc
	status status
}

type status struct {
	Run    int             `json:"run"`
	State  string          `json:"state"` // idle, running, done, error
	Mode   string          `json:"mode,omitempty"`
	Result json.RawMessage `json:"result,omitempty"` // the best plan so far
	// Problems is what the verifier says about the final plan. Empty
	// means the plan passed.
	Problems []string `json:"problems"`
	Verified bool     `json:"verified"`
	Error    string   `json:"error,omitempty"`
}

func newServer() *server { return &server{status: status{State: "idle"}} }

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
	mux.HandleFunc("/api/run", s.handleRun)
	mux.HandleFunc("/api/stop", s.handleStop)
	mux.HandleFunc("/api/giben", handleGiben)
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		st := s.status
		s.mu.Unlock()
		writeJSON(w, st)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string          `json:"mode"`
		Job  json.RawMessage `json:"job"`
	}
	if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "POST {mode, job}", http.StatusBadRequest)
		return
	}
	planner, ok := extension.CutPlannerByID(req.Mode)
	if !ok {
		http.Error(w, "unknown mode "+req.Mode, http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.run++
	run := s.run
	s.cancel = cancel
	s.status = status{Run: run, State: "running", Mode: req.Mode}
	s.mu.Unlock()

	go func() {
		defer cancel()
		// set changes the status unless a newer run has taken over.
		set := func(f func(*status)) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.run == run {
				f(&s.status)
			}
		}
		res, err := planner.Plan(ctx, req.Job, func(better json.RawMessage) {
			set(func(st *status) { st.Result = better })
		})
		if err != nil {
			set(func(st *status) { st.State, st.Error = "error", err.Error() })
			return
		}
		problems := verify(req.Mode, req.Job, res)
		set(func(st *status) {
			st.State, st.Result, st.Problems, st.Verified = "done", res, problems, len(problems) == 0
		})
	}()
	writeJSON(w, map[string]int{"run": run})
}

func (s *server) handleStop(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	writeJSON(w, map[string]bool{"ok": true})
}

// handleGiben turns a rectangular plan into the .AC and .AD files of a
// Giben saw and sends them in a ZIP. Part codes follow the order of the
// parts; labels are their codes.
func handleGiben(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Job       rect.Job    `json:"job"`
		Result    rect.Result `json:"result"`
		Material  string      `json:"material"`
		Thickness float64     `json:"thickness"`
		Project   string      `json:"project"`
	}
	if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&req) != nil {
		http.Error(w, "POST {job, result, material, thickness, project}", http.StatusBadRequest)
		return
	}
	ac, ad, err := giben.Write(req.Job, req.Result, giben.Export{
		Material: req.Material, Thickness: req.Thickness, ProjectCode: req.Project,
		Description: req.Project, Date: time.Now(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	base := fmt.Sprintf("%s_%.1f", strings.Join(strings.Fields(req.Material), "_"), req.Thickness)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct {
		ext  string
		data []byte
	}{{".AC", ac}, {".AD", ad}} {
		fw, err := zw.Create(base + f.ext)
		if err == nil {
			_, err = fw.Write(f.data)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	if err := zw.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="giben_`+base+`.zip"`)
	_, _ = w.Write(buf.Bytes())
}

// verify runs the independent verifier of the mode over the final plan.
func verify(mode string, job, res json.RawMessage) []string {
	var problems []string
	var err error
	switch mode {
	case cutplan.LinearID:
		var j linear.Job
		var r linear.Result
		if err = json.Unmarshal(job, &j); err == nil {
			if err = json.Unmarshal(res, &r); err == nil {
				problems = linear.Verify(j, r)
			}
		}
	case cutplan.RectangularID:
		var j rect.Job
		var r rect.Result
		if err = json.Unmarshal(job, &j); err == nil {
			if err = json.Unmarshal(res, &r); err == nil {
				problems = rect.Verify(j, r)
			}
		}
	default:
		return []string{"no verifier for mode " + mode}
	}
	if err != nil {
		return []string{err.Error()}
	}
	if problems == nil {
		problems = []string{}
	}
	return problems
}

// waitDone is used by the tests: it polls like the page does.
func (s *server) waitDone(timeout time.Duration) status {
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		st := s.status
		s.mu.Unlock()
		if st.State != "running" || time.Now().After(deadline) {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
}
