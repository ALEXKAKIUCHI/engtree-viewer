package app

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/rect"
	registry "github.com/ALEXKAKIUCHI/engtree-viewer/pkg/cutplan"
)

// Eleven parts on one 15 mm sheet of 1840 x 2740 (70.45 % of the sheet),
// and a tube.
func referenceLines() []PlanLine {
	l := func(code, desc string, w, h float64) PlanLine {
		return PlanLine{Code: code, Desc: desc, Material: "100234", Length: h, Width: w, Qty: 1}
	}
	return []PlanLine{
		l("1000-107", "FRENTE 2", 730, 800), l("1000-106", "FRENTE", 670, 690),
		l("1000-121", "PORTA BASCULANTE", 183, 427), l("1000-139", "FECHAMENTO", 170, 230),
		l("1000-150", "LATERAL", 80, 800), l("1000-104", "DIVISORIA", 595, 670),
		l("1000-101", "LATERAL", 580, 670), l("1000-120", "LATERAL", 580, 670),
		l("1000-137", "LATERAL", 580, 660), l("1000-108", "BASE", 580, 660),
		l("1000-138", "LATERAL", 580, 660),
		{Code: "1000-200", Desc: "TUBO 20X20", Material: "TB2020", Length: 1200, Qty: 7},
	}
}

func TestCutReportPDF(t *testing.T) {
	registry.Register()
	mats := testMaterials()
	mats.Materials[0].Description = "MDF BRANCO 2 FACES"
	mats.Materials[0].Length, mats.Materials[0].Width = 2740, 1840
	mc := defaultCutMachine()
	mc.TimeLimitSec, mc.Order, mc.User = 2, "PED-123", "João"
	plans.start(PlanRequest{Product: "PROD-01", ProductDesc: "ARMÁRIO DE EXEMPLO", ProductQty: 1, Lines: referenceLines(), Machine: mc}, mats)
	st := plans.waitDone(30 * time.Second)
	for _, m := range st.Materials {
		if m.State != "done" || len(m.Problems) != 0 {
			t.Fatalf("%s: %s %v", m.Material.Code, m.State, m.Problems)
		}
	}
	b, err := cutReportPDF(st, "pt-BR", `S:\Seccionadora\Plano de corte - PROD-01.pdf`)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatal("not a PDF")
	}
	// One page per different sheet layout, one for the bars.
	pages := len(regexp.MustCompile(`/Type /Page\b`).FindAll(b, -1))
	want := distinctLayouts(mustRect(t, st.Materials[0]).Sheets) + 1
	if pages != want {
		t.Errorf("%d pages, want %d", pages, want)
	}
	if out := os.Getenv("CUT_REPORT_OUT"); out != "" {
		_ = os.WriteFile(out, b, 0644)
	}

	if _, err := cutReportPDF(PlanStatus{}, "pt-BR", ""); err == nil {
		t.Error("a report with no plan should be refused")
	}
}

// distinctLayouts counts the sheet layouts that differ, the way the report
// gives each its page.
func distinctLayouts(sheets []rect.SheetLayout) int {
	seen := map[string]bool{}
	for _, s := range sheets {
		b, _ := json.Marshal(struct {
			ID         string
			L, W       float64
			Placements []rect.Placement
		}{s.StockID, s.Length, s.Width, s.Placements})
		seen[string(b)] = true
	}
	return len(seen)
}

func mustRect(t *testing.T, mp MaterialPlan) (res rect.Result) {
	t.Helper()
	if err := json.Unmarshal(mp.Result, &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSummaryXLSXReadsBack(t *testing.T) {
	st := PlanStatus{Request: PlanRequest{Product: "P"}, Materials: []MaterialPlan{{
		MaterialJob: MaterialJob{
			Material: Material{Code: "100234", Description: "MDF & <cia>", Kind: KindSheet, Thickness: 15},
			Planner:  "rectangular",
			Parts:    []JobPart{{ID: "A", Codes: []string{"A"}, Desc: "FRENTE", Items: []string{"1", "2.1"}, CutLength: 798, CutWidth: 728.5, Length: 800, Width: 730, Qty: 2}},
		},
		Result: json.RawMessage(`{"sheets":[{"length":2740,"width":1840,"placements":[{"part_id":"A"},{"part_id":"A"}],"offcuts":[{"reusable":true}]}],"utilization":0.7045}`),
	}}}
	b, err := writeXLSX("Resumo", cutSummaryRows(st, "pt-BR"))
	if err != nil {
		t.Fatal(err)
	}
	p := t.TempDir() + "/resumo.xlsx"
	if err := os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}
	rows := readXLSX(p)
	if len(rows) < 4 {
		t.Fatalf("rows: %v", rows)
	}
	if got := strings.Join(rows[1], "|"); got != "100234|MDF & <cia>|chapa|15|1|23.06|2|1|0" {
		t.Errorf("material row: %s", got)
	}
	last := rows[len(rows)-1]
	if got := strings.Join(last, "|"); got != "100234|A|FRENTE|1 2.1|798|728.5|800|730|2" {
		t.Errorf("part row: %s", got)
	}
}
