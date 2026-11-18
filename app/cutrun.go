package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan"
	"github.com/ALEXKAKIUCHI/engtree-viewer/extension"
)

// PlanRequest starts a cut plan: the lines from the conference screen,
// the machine values and the materials chosen (empty = all).
type PlanRequest struct {
	Product     string     `json:"product"`
	ProductDesc string     `json:"product_desc"`
	ProductQty  int        `json:"product_qty"`
	Lines       []PlanLine `json:"lines"`
	Machine     CutMachine `json:"machine"`
	Materials   []string   `json:"materials"`
}

// MaterialPlan is the job of one material and how far it got.
type MaterialPlan struct {
	MaterialJob
	State    string          `json:"state"` // waiting, running, done, error, stopped
	Result   json.RawMessage `json:"result,omitempty"`
	Problems []string        `json:"problems"` // what the verifier says; empty = passed
	Error    string          `json:"error,omitempty"`
}

// PlanStatus is the plan as the screen and the report see it.
type PlanStatus struct {
	Run       int            `json:"run"`
	State     string         `json:"state"` // idle, running, done
	Request   PlanRequest    `json:"request"`
	Created   time.Time      `json:"created"`
	Materials []MaterialPlan `json:"materials"`
	Problems  []string       `json:"problems"` // lines left out, with the reason
}

// planRunner runs one plan at a time. A new plan stops the old one.
type planRunner struct {
	mu     sync.Mutex
	run    int
	cancel context.CancelFunc
	status PlanStatus
}

var plans = &planRunner{status: PlanStatus{State: "idle", Materials: []MaterialPlan{}, Problems: []string{}}}

func (p *planRunner) get() PlanStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.status
	st.Materials = append([]MaterialPlan(nil), p.status.Materials...)
	return st
}

func (p *planRunner) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
	}
}

// start builds the jobs and runs them one after another in the
// background. It returns the status as it is right after the jobs are
// built.
func (p *planRunner) start(req PlanRequest, mats MaterialList) PlanStatus {
	want := map[string]bool{}
	for _, m := range req.Materials {
		if mm, ok := mats.Find(m); ok {
			want[materialKey(mm.Code)] = true
		}
	}
	var lines []PlanLine
	for _, l := range req.Lines {
		m, ok := mats.Find(l.Material)
		if len(want) == 0 || (ok && want[materialKey(m.Code)]) || !ok {
			lines = append(lines, l)
		}
	}
	jobs, problems := buildJobs(lines, mats, req.Machine)
	if problems == nil {
		problems = []string{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	p.run++
	run := p.run
	p.cancel = cancel
	st := PlanStatus{Run: run, State: "running", Request: req, Created: time.Now(), Problems: problems}
	for _, j := range jobs {
		st.Materials = append(st.Materials, MaterialPlan{MaterialJob: j, State: "waiting", Problems: []string{}})
	}
	if len(jobs) == 0 {
		st.State = "done"
		st.Materials = []MaterialPlan{}
	}
	p.status = st
	p.mu.Unlock()

	// set changes material i unless a newer plan has taken over.
	set := func(i int, f func(*MaterialPlan)) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.run == run && i < len(p.status.Materials) {
			f(&p.status.Materials[i])
		}
	}
	go func() {
		defer cancel()
		for i, j := range jobs {
			if ctx.Err() != nil {
				set(i, func(m *MaterialPlan) { m.State = "stopped" })
				continue
			}
			set(i, func(m *MaterialPlan) { m.State = "running" })
			planner, ok := extension.CutPlannerByID(j.Planner)
			if !ok {
				set(i, func(m *MaterialPlan) { m.State, m.Error = "error", "no planner "+j.Planner })
				continue
			}
			res, err := planner.Plan(ctx, j.Job, func(better json.RawMessage) {
				set(i, func(m *MaterialPlan) { m.Result = better })
			})
			if err != nil {
				set(i, func(m *MaterialPlan) { m.State, m.Error = "error", err.Error() })
				continue
			}
			var problems []string
			if v, ok := cutplan.ByID(j.Planner); ok {
				problems = v.Verify(j.Job, res)
			}
			set(i, func(m *MaterialPlan) { m.State, m.Result, m.Problems = "done", res, problems })
		}
		p.mu.Lock()
		if p.run == run {
			p.status.State = "done"
		}
		p.mu.Unlock()
	}()
	return p.get()
}

// waitDone is used by the tests.
func (p *planRunner) waitDone(timeout time.Duration) PlanStatus {
	deadline := time.Now().Add(timeout)
	for {
		st := p.get()
		if st.State != "running" || time.Now().After(deadline) {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// rememberMachine keeps the machine values and report header for next time.
func rememberMachine(mc CutMachine) {
	mc.Order, mc.User = strings.TrimSpace(mc.Order), strings.TrimSpace(mc.User)
	setLocal(func(c *Config) { c.CutMachine = &mc })
}

func currentMachine() CutMachine {
	if c := getConfig(); c.CutMachine != nil {
		return *c.CutMachine
	}
	return defaultCutMachine()
}
