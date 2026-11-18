package cutplan

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/linear"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/rect"
	"github.com/ALEXKAKIUCHI/engtree-viewer/extension"
)

func TestBuiltInPlannersAreRegistered(t *testing.T) {
	Register()
	ids := map[string]bool{}
	for _, p := range extension.CutPlanners() {
		ids[p.ID()] = true
		if p.Label() == "" {
			t.Errorf("planner %s has no label", p.ID())
		}
	}
	if !ids[LinearID] || !ids[RectangularID] {
		t.Fatalf("registered planners: %v", ids)
	}
}

func TestLinearPlansThroughJSON(t *testing.T) {
	Register()
	p, _ := extension.CutPlannerByID(LinearID)
	job := `{"parts":[{"id":"A","length":2000,"quantity":3}],
	         "stock":[{"id":"bar","length":6000}],
	         "settings":{"kerf":3,"start_loss":20,"end_loss":150,"min_offcut":300}}`
	updates := 0
	out, err := p.Plan(context.Background(), json.RawMessage(job), func(json.RawMessage) { updates++ })
	if err != nil {
		t.Fatal(err)
	}
	var j linear.Job
	var res linear.Result
	if err := json.Unmarshal([]byte(job), &j); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatal(err)
	}
	if bad := linear.Verify(j, res); len(bad) != 0 {
		t.Fatalf("plan refused: %v", bad)
	}
	if len(res.Bars) != 2 || updates == 0 {
		t.Errorf("%d bars (want 2), %d progress updates", len(res.Bars), updates)
	}
}

func TestRectangularPlansThroughJSON(t *testing.T) {
	Register()
	p, _ := extension.CutPlannerByID(RectangularID)
	job := `{"parts":[{"id":"A","length":1000,"width":500,"quantity":4},
	                  {"id":"B","length":400,"width":700,"quantity":2,"can_rotate":true}],
	         "stock":[{"id":"sheet","length":2750,"width":1830}],
	         "settings":{"kerf":4,"edge_trim":10,"min_offcut_length":300,"min_offcut_width":300}}`
	out, err := p.Plan(context.Background(), json.RawMessage(job), nil)
	if err != nil {
		t.Fatal(err)
	}
	var j rect.Job
	var res rect.Result
	if err := json.Unmarshal([]byte(job), &j); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatal(err)
	}
	if bad := rect.Verify(j, res); len(bad) != 0 {
		t.Fatalf("plan refused: %v", bad)
	}
	if len(res.Sheets) != 1 {
		t.Errorf("%d sheets, want 1", len(res.Sheets))
	}
}

func TestBadJSONIsAnError(t *testing.T) {
	Register()
	p, _ := extension.CutPlannerByID(LinearID)
	if _, err := p.Plan(context.Background(), json.RawMessage(`{"parts":`), nil); err == nil {
		t.Error("broken JSON accepted")
	}
}
