package cutplan

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPlannersAreListed(t *testing.T) {
	for _, id := range []string{LinearID, RectangularID} {
		p, ok := ByID(id)
		if !ok || p.Label == "" || p.Plan == nil || p.Verify == nil {
			t.Errorf("planner %s: %+v", id, p)
		}
	}
	if _, ok := ByID("nope"); ok {
		t.Error("unknown ID found")
	}
}

func TestPlanAndVerifyThroughJSON(t *testing.T) {
	jobs := map[string]string{
		LinearID: `{"parts":[{"id":"A","length":2000,"quantity":3}],
		            "stock":[{"id":"bar","length":6000}],
		            "settings":{"kerf":3,"start_loss":20,"end_loss":150}}`,
		RectangularID: `{"parts":[{"id":"A","length":1000,"width":500,"quantity":4}],
		                 "stock":[{"id":"sheet","length":2750,"width":1830}],
		                 "settings":{"kerf":4,"edge_trim":10}}`,
	}
	for id, job := range jobs {
		p, _ := ByID(id)
		updates := 0
		out, err := p.Plan(context.Background(), json.RawMessage(job), func(json.RawMessage) { updates++ })
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if bad := p.Verify(json.RawMessage(job), out); bad == nil || len(bad) != 0 {
			t.Errorf("%s: verifier says %v", id, bad)
		}
		if updates == 0 {
			t.Errorf("%s: no progress updates", id)
		}
	}
}

func TestBadJSON(t *testing.T) {
	p, _ := ByID(LinearID)
	if _, err := p.Plan(context.Background(), json.RawMessage(`{"parts":`), nil); err == nil {
		t.Error("broken job accepted")
	}
	if bad := p.Verify(json.RawMessage(`{}`), json.RawMessage(`{"bars":`)); len(bad) == 0 {
		t.Error("broken result passed the verifier")
	}
}
