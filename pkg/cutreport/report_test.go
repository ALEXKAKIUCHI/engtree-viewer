package cutreport

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/linear"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/rect"
	"github.com/go-pdf/fpdf"
)

// A small hand-made plan: two sheets cut the same way, one different, and
// a bar.
func samplePlans(t *testing.T) []Plan {
	t.Helper()
	sheet := func(x float64) rect.SheetLayout {
		return rect.SheetLayout{StockID: "M1", Length: 2750, Width: 1840, FirstCut: rect.AlongLength,
			Placements: []rect.Placement{{PartID: "A", X: 10, Y: 10}, {PartID: "B", X: x, Y: 10}},
			Cuts:       []rect.Cut{{Stage: 1, Along: rect.AlongLength, Pos: 810, From: 10, To: 2740}},
			Offcuts:    []rect.Offcut{{X: 10, Y: 814, Length: 2730, Width: 1016, Reusable: true}}}
	}
	res := rect.Result{Sheets: []rect.SheetLayout{sheet(1000), sheet(1000), sheet(1200)}}
	job := rect.Job{Settings: rect.Settings{Kerf: 4, EdgeTrim: 10}}
	bars := linear.Result{Bars: []linear.Bar{{StockID: "T1", Length: 6000, Cuts: []linear.Cut{{PartID: "C", Start: 0, Length: 1200}}, Remainder: 4797, Reusable: true}}}
	mustJSON := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	return []Plan{
		{Material: Material{Code: "M1", Description: "MDF BRANCO", Thickness: 15}, Planner: "rectangular", Job: mustJSON(job), Result: mustJSON(res),
			Parts: []Part{{ID: "A", Code: "1000-1", Desc: "LATERAL", Project: "OBRA-7", CutLength: 800, CutWidth: 600, Qty: 3},
				{ID: "B", Desc: "FUNDO", CutLength: 800, CutWidth: 500, Qty: 3}}},
		{Material: Material{Code: "T1", Description: "TUBO", Length: 6000}, Planner: "linear", Job: mustJSON(linear.Job{}), Result: mustJSON(bars),
			Parts: []Part{{ID: "C", Desc: "PERNA", CutLength: 1200, Qty: 1}}},
	}
}

func TestPDFPages(t *testing.T) {
	b, err := PDF(Header{Product: "P1", ProductDesc: "MESA", Created: time.Now()}, samplePlans(t), "pt-BR", `S:\planos\x.pdf`)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatal("not a PDF")
	}
	// Two different sheet layouts and one bar page.
	if pages := len(regexp.MustCompile(`/Type /Page\b`).FindAll(b, -1)); pages != 3 {
		t.Errorf("%d pages, want 3", pages)
	}
	if _, err := PDF(Header{}, nil, "pt-BR", ""); err == nil {
		t.Error("a report with no plan should be refused")
	}
}

func TestProjectAndCode(t *testing.T) {
	r := &report{h: Header{Product: "P1"}}
	if r.project(Part{Project: "OBRA-7"}) != "OBRA-7" || r.project(Part{}) != "P1" {
		t.Error("project should fall back to the product")
	}
	if firstCode(Part{ID: "B"}) != "B" || firstCode(Part{ID: "B", Code: "1000-1"}) != "1000-1" {
		t.Error("code should fall back to the ID")
	}
}

func TestFooterPathFits(t *testing.T) {
	pdf := fpdf.New("P", "mm", "A4", "")
	r := &report{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor(""), txt: reportText["pt-BR"], h: Header{Product: "P"},
		path: `\\servidor\producao\seccionadora\planos de corte\2026\novembro\Plano de corte - 1000 - ARMARIO DE EXEMPLO COM PORTAS - 2026-11-18 2056.pdf`}
	pdf.AddPage()
	r.footer()
	if pdf.Err() {
		t.Fatal(pdf.Error())
	}
}

func TestFileName(t *testing.T) {
	n := FileName(Header{Product: "P/1", ProductDesc: `MESA "A"`, Created: time.Date(2026, 11, 19, 8, 5, 0, 0, time.UTC)})
	if n != "Plano de corte - P_1 - MESA _A_ - 2026-11-19 0805" || strings.ContainsAny(n, `/"`) {
		t.Errorf("file name: %q", n)
	}
}
