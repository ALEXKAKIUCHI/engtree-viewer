package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/linear"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/rect"
	registry "github.com/ALEXKAKIUCHI/engtree-viewer/pkg/cutplan"
)

func planLines() []PlanLine {
	no := false
	return []PlanLine{
		{Item: "1", Code: "P107", Desc: "FRENTE 2", Material: "100234", Length: 800, Width: 730, Qty: 2},
		{Item: "2.1", Code: "P121", Desc: "LATERAL", Material: "mdf15br", Length: 427, Width: 183, Qty: 4, C1: 1, C2: 1, L1: 2},
		{Item: "3.1", Code: "P121", Desc: "LATERAL", Material: "100234", Length: 427, Width: 183, Qty: 2, C1: 1, C2: 1, L1: 2},
		{Item: "4", Code: "P121", Desc: "LATERAL CURTA", Material: "100234", Length: 300, Width: 183, Qty: 1, CanRotate: &no},
		{Item: "5", Code: "F1", Desc: "FUNDO", Material: "MDF6BR", Length: 500, Width: 400, Qty: 3},
		{Item: "6", Code: "T1", Desc: "TUBO", Material: "TB2020", Length: 1200, Width: 20, Qty: 7},
		{Item: "7", Code: "X1", Desc: "TAMPO", Material: "999999", Length: 900, Width: 600, Qty: 1},
	}
}

func TestBuildJobs(t *testing.T) {
	mats := testMaterials()
	mats.Materials[0].Offcuts = []Offcut{{Length: 1000, Width: 800, Quantity: 1}}
	jobs, problems := buildJobs(planLines(), mats, defaultCutMachine())
	if len(problems) != 1 || problems[0] != (LineProblem{"X1", "999999", "unknown_material"}) {
		t.Errorf("problems: %v", problems)
	}
	if len(jobs) != 3 {
		t.Fatalf("%d jobs, want 3 (one per material)", len(jobs))
	}

	mdf := jobs[0]
	var rj rect.Job
	if err := json.Unmarshal(mdf.Job, &rj); err != nil || mdf.Planner != "rectangular" {
		t.Fatal(mdf.Planner, err)
	}
	// The two P121 lines at the same size become one part of 6, cut at
	// 425 x 181 (banding off); the short one gets its own ID.
	got := map[string]rect.Part{}
	for _, p := range rj.Parts {
		got[p.ID] = p
	}
	if p := got["P121"]; p.Quantity != 6 || p.Length != 425 || p.Width != 181 || !p.CanRotate {
		t.Errorf("P121: %+v", p)
	}
	if p := got["P121 (2)"]; p.Quantity != 1 || p.CanRotate {
		t.Errorf("P121 (2): %+v", p)
	}
	if len(rj.Stock) != 2 || rj.Stock[0].ID != "RET 1" || rj.Stock[1].Quantity != 0 {
		t.Errorf("stock: offcuts first, whole sheet unlimited last: %+v", rj.Stock)
	}
	if rj.Settings.MinOffcutLength != 300 || rj.Settings.Kerf != 4 {
		t.Errorf("settings: %+v", rj.Settings)
	}

	// The 6 mm MDF has grain: its parts never turn.
	var grain rect.Job
	_ = json.Unmarshal(jobs[1].Job, &grain)
	if grain.Parts[0].CanRotate {
		t.Error("a part of a material with grain may turn")
	}

	var lj linear.Job
	if err := json.Unmarshal(jobs[2].Job, &lj); err != nil || jobs[2].Planner != "linear" {
		t.Fatal(err)
	}
	if lj.Parts[0].Length != 1200 || lj.Stock[0].Length != 6000 || lj.Settings.Kerf != 3 {
		t.Errorf("bar job: %+v", lj)
	}
}

func TestPlanRunPassesTheVerifier(t *testing.T) {
	registry.Register()
	mc := defaultCutMachine()
	mc.TimeLimitSec = 1
	st := plans.start(PlanRequest{Product: "49815", Lines: planLines(), Machine: mc}, testMaterials())
	if len(st.Materials) != 3 {
		t.Fatalf("%d materials", len(st.Materials))
	}
	st = plans.waitDone(30 * time.Second)
	if st.State != "done" {
		t.Fatalf("state %s", st.State)
	}
	for _, m := range st.Materials {
		if m.State != "done" || len(m.Problems) != 0 || len(m.Result) == 0 {
			t.Errorf("%s: %s %v %s", m.Material.Code, m.State, m.Problems, m.Error)
		}
	}
	// Only the chosen material is planned.
	st = plans.start(PlanRequest{Lines: planLines(), Machine: mc, Materials: []string{"TB2020"}}, testMaterials())
	if len(st.Materials) != 1 || st.Materials[0].Material.Code != "TB2020" {
		t.Errorf("chosen materials: %+v", st.Materials)
	}
	plans.waitDone(30 * time.Second)
}
