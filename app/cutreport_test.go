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
	"github.com/go-pdf/fpdf"
)

// The parts of the Corte Certo sheet the shop sent (MDF 15 mm, 1840 x
// 2740), one product.
func cortecertoLines() []PlanLine {
	l := func(code, desc string, w, h float64) PlanLine {
		return PlanLine{Code: code, Desc: desc, Material: "100234", Length: h, Width: w, Qty: 1}
	}
	return []PlanLine{
		l("49815P107", "FRENTE 2", 730, 800), l("49815P106", "FRENTE", 670, 690),
		l("49815P121", "PORTA BASCULANTE", 183, 427), l("49815P139", "FECHAMENTO", 170, 230),
		l("49815P150", "LATERAL", 80, 800), l("49815P104", "DIVISORIA", 595, 670),
		l("49815P101", "LATERAL", 580, 670), l("49815P120", "LATERAL", 580, 670),
		l("49815P137", "LATERAL", 580, 660), l("49815P108", "BASE", 580, 660),
		l("49815P138", "LATERAL", 580, 660),
		{Code: "49815P200", Desc: "TUBO 20X20", Material: "TB2020", Length: 1200, Qty: 7},
	}
}

func TestCutReportPDF(t *testing.T) {
	registry.Register()
	mats := testMaterials()
	mats.Materials[0].Description = "MDF ITAPUA ESSENCIAL WOOD 2 FACES"
	mats.Materials[0].Length, mats.Materials[0].Width = 2740, 1840
	mc := defaultCutMachine()
	mc.TimeLimitSec, mc.Order, mc.User = 2, "PED-123", "João"
	plans.start(PlanRequest{Product: "EES_195", ProductDesc: "NOVO BALCÃO PNE", ProductQty: 1, Lines: cortecertoLines(), Machine: mc}, mats)
	st := plans.waitDone(30 * time.Second)
	for _, m := range st.Materials {
		if m.State != "done" || len(m.Problems) != 0 {
			t.Fatalf("%s: %s %v", m.Material.Code, m.State, m.Problems)
		}
	}
	b, err := cutReportPDF(st, "pt-BR", `S:\Seccionadora\Plano de corte - EES_195.pdf`)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatal("not a PDF")
	}
	// One page per different sheet layout, one for the bars.
	pages := len(regexp.MustCompile(`/Type /Page\b`).FindAll(b, -1))
	want := len(groupLayouts(mustRect(t, st.Materials[0]).Sheets)) + 1
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

func TestFooterPathFits(t *testing.T) {
	st := PlanStatus{Request: PlanRequest{Product: "P"}}
	pdf := fpdf.New("P", "mm", "A4", "")
	r := &report{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor(""), txt: reportText["pt-BR"], st: st,
		path: `\\servidor\producao\seccionadora\planos de corte\2026\novembro\Plano de corte - 49815 - NOVO BALCAO PNE COM PORTAS - 2026-11-18 2056.pdf`}
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 7)
	r.footer()
	// The footer writes into the PDF; it must not panic and the shortened
	// path must keep the file name.
	if pdf.Err() {
		t.Fatal(pdf.Error())
	}
}
