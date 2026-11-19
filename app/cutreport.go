package app

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/linear"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/rect"
	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/cutreport"
)

// The cut plan report is written by pkg/cutreport; this file hands it the
// plan of the Cut plan tab, and writes the summary spreadsheet.

// cutReportPDF writes the report of a plan. path only appears in the
// footer; the caller saves the bytes where it wants.
func cutReportPDF(st PlanStatus, language, path string) ([]byte, error) {
	return cutreport.PDF(reportHeader(st), reportPlans(st), language, path)
}

func reportHeader(st PlanStatus) cutreport.Header {
	req := st.Request
	return cutreport.Header{Product: req.Product, ProductDesc: req.ProductDesc, ProductQty: req.ProductQty,
		Order: req.Machine.Order, User: req.Machine.User, Created: st.Created}
}

// reportPlans keeps the materials that have a result to print.
func reportPlans(st PlanStatus) []cutreport.Plan {
	var out []cutreport.Plan
	for _, mp := range st.Materials {
		if mp.State != "done" && mp.State != "stopped" || len(mp.Result) == 0 {
			continue
		}
		m := mp.Material
		out = append(out, cutreport.Plan{
			Material: cutreport.Material{Code: m.Code, Description: m.Description, Thickness: m.Thickness, Length: m.Length},
			Planner:  mp.Planner, Job: mp.Job, Result: mp.Result, Parts: reportParts(mp.Parts),
		})
	}
	return out
}

func reportParts(parts []JobPart) []cutreport.Part {
	out := make([]cutreport.Part, len(parts))
	for i, p := range parts {
		out[i] = cutreport.Part{ID: p.ID, Code: firstCode(p), Desc: p.Desc, CutLength: p.CutLength, CutWidth: p.CutWidth, Qty: p.Qty}
	}
	return out
}

func firstCode(p JobPart) string {
	if len(p.Codes) > 0 && p.Codes[0] != "" {
		return p.Codes[0]
	}
	return p.ID
}

// reportFileName is the name of a saved report.
func reportFileName(st PlanStatus) string { return cutreport.FileName(reportHeader(st)) + ".pdf" }

// fmtMM writes a size the way the shop reads it: no decimals when whole,
// a comma otherwise.
func fmtMM(v float64) string {
	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", v), "0"), ".")
	return strings.Replace(s, ".", ",", 1)
}

// cutSummaryRows is the summary spreadsheet: one row per material, then
// the parts of each material.
func cutSummaryRows(st PlanStatus, language string) [][]xlsxCell {
	h := []xlsxCell{"Material", "Descrição", "Tipo", "Espessura (mm)", "Chapas/barras", "Aproveitamento (%)", "Peças", "Retalhos", "Não couberam"}
	h2 := []xlsxCell{"Material", "Código", "Descrição", "Itens", "Comp. corte (mm)", "Larg. corte (mm)", "Comp. final (mm)", "Larg. final (mm)", "Qtd"}
	kind := map[string]string{KindSheet: "chapa", KindBar: "barra"}
	if language == "en" {
		h = []xlsxCell{"Material", "Description", "Type", "Thickness (mm)", "Sheets/bars", "Use (%)", "Parts", "Offcuts", "Not placed"}
		h2 = []xlsxCell{"Material", "Code", "Description", "Items", "Cut length (mm)", "Cut width (mm)", "Final length (mm)", "Final width (mm)", "Qty"}
		kind = map[string]string{KindSheet: "sheet", KindBar: "bar"}
	}
	rows := [][]xlsxCell{h}
	for _, mp := range st.Materials {
		m := mp.Material
		var count, parts, offcuts, unplaced int
		var use float64
		switch mp.Planner {
		case cutplan.RectangularID:
			var res rect.Result
			if json.Unmarshal(mp.Result, &res) != nil {
				continue
			}
			count, use = len(res.Sheets), cutreport.SheetsUse(res.Sheets, reportParts(mp.Parts))
			for _, s := range res.Sheets {
				parts += len(s.Placements)
				for _, o := range s.Offcuts {
					if o.Reusable {
						offcuts++
					}
				}
			}
			for _, u := range res.Unplaced {
				unplaced += u.Quantity
			}
		case cutplan.LinearID:
			var res linear.Result
			if json.Unmarshal(mp.Result, &res) != nil {
				continue
			}
			count, use = len(res.Bars), cutreport.BarsUse(res.Bars)
			for _, b := range res.Bars {
				parts += len(b.Cuts)
				if b.Reusable {
					offcuts++
				}
			}
			for _, u := range res.Unplaced {
				unplaced += u.Quantity
			}
		}
		rows = append(rows, []xlsxCell{m.Code, m.Description, kind[m.Kind], m.Thickness, count,
			math.Round(use*10000) / 100, parts, offcuts, unplaced})
	}
	rows = append(rows, nil, h2)
	for _, mp := range st.Materials {
		for _, p := range mp.Parts {
			var cw, fw xlsxCell
			if mp.Material.Kind == KindSheet {
				cw, fw = p.CutWidth, p.Width
			}
			rows = append(rows, []xlsxCell{mp.Material.Code, firstCode(p), p.Desc, strings.Join(p.Items, " "),
				p.CutLength, cw, p.Length, fw, p.Qty})
		}
	}
	return rows
}
