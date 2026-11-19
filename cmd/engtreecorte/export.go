package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/giben"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/linear"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/rect"
	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/cutreport"
)

// exportRequest is a finished plan, one entry per material, with what the
// report prints about it.
type exportRequest struct {
	Mode string `json:"mode"` // cutplan.LinearID or cutplan.RectangularID
	Lang string `json:"lang"` // "pt" or "en"
	Info struct {
		Product     string `json:"product"`
		ProductDesc string `json:"product_desc"`
		Order       string `json:"order"`
		User        string `json:"user"`
	} `json:"info"`
	Materials []struct {
		Code      string          `json:"code"`
		Desc      string          `json:"desc"`
		Thickness float64         `json:"thickness"`
		Job       json.RawMessage `json:"job"`
		Result    json.RawMessage `json:"result"`
	} `json:"materials"`
	// Parts adds a description and a project to the parts of the jobs, by ID.
	Parts []struct {
		ID      string `json:"id"`
		Desc    string `json:"desc"`
		Project string `json:"project"`
	} `json:"parts"`
}

// exportFile is one file of an export.
type exportFile struct {
	name string
	data []byte
}

// build checks every material with the verifier and writes one report for
// all of them and, when production is asked for, the Giben files of each
// sheet material.
func (req exportRequest) build(production bool) ([]exportFile, string, error) {
	p, ok := cutplan.ByID(req.Mode)
	if !ok {
		return nil, "", fmt.Errorf("unknown mode %q", req.Mode)
	}
	if len(req.Materials) == 0 {
		return nil, "", fmt.Errorf("no plan to export")
	}
	meta := map[string]int{}
	for i, m := range req.Parts {
		meta[m.ID] = i
	}
	part := func(id string, length, width float64, qty int) cutreport.Part {
		cp := cutreport.Part{ID: id, Code: id, CutLength: length, CutWidth: width, Qty: qty}
		if i, ok := meta[id]; ok {
			cp.Desc, cp.Project = req.Parts[i].Desc, req.Parts[i].Project
		}
		return cp
	}
	info := req.Info
	head := cutreport.Header{Product: info.Product, ProductDesc: info.ProductDesc, ProductQty: 1,
		Order: info.Order, User: info.User, Created: time.Now()}
	base := cutreport.FileName(head)

	var plans []cutreport.Plan
	var saw []exportFile
	usedNames := map[string]int{}
	for _, m := range req.Materials {
		name := m.Code
		if name == "" {
			name = m.Desc
		}
		if problems := p.Verify(m.Job, m.Result); len(problems) > 0 {
			return nil, "", fmt.Errorf("%s: the plan did not pass the verifier: %s", name, strings.Join(problems, "; "))
		}
		plan := cutreport.Plan{
			Material: cutreport.Material{Code: m.Code, Description: m.Desc, Thickness: m.Thickness},
			Planner:  req.Mode, Job: m.Job, Result: m.Result,
		}
		switch req.Mode {
		case cutplan.RectangularID:
			var job rect.Job
			var res rect.Result
			if err := json.Unmarshal(m.Job, &job); err != nil {
				return nil, "", err
			}
			if err := json.Unmarshal(m.Result, &res); err != nil {
				return nil, "", err
			}
			for _, jp := range job.Parts {
				plan.Parts = append(plan.Parts, part(jp.ID, jp.Length, jp.Width, jp.Quantity))
			}
			if production && len(res.Sheets) > 0 {
				labels := map[string]giben.PartInfo{}
				for i, jp := range job.Parts {
					label := jp.ID
					if k, ok := meta[jp.ID]; ok && req.Parts[k].Desc != "" {
						label = req.Parts[k].Desc
					}
					labels[jp.ID] = giben.PartInfo{Code: i + 1, Label: label}
				}
				desc := info.ProductDesc
				if desc == "" {
					desc = info.Product
				}
				matText := m.Desc
				if strings.TrimSpace(matText) == "" {
					matText = m.Code
				}
				ac, ad, err := giben.Write(job, res, giben.Export{
					Material: matText, Thickness: m.Thickness, ProjectCode: info.Product,
					Description: desc, Date: time.Now(), Parts: labels,
				})
				if err != nil {
					return nil, "", fmt.Errorf("%s: %w", name, err)
				}
				// The saw files keep the short name GVision is used to:
				// material and thickness. Two materials with the same name get
				// a number.
				file := fmt.Sprintf("%s_%.1f", strings.Join(strings.Fields(matText), "_"), m.Thickness)
				if strings.TrimSpace(matText) == "" {
					file = "plano"
				}
				usedNames[file]++
				if n := usedNames[file]; n > 1 {
					file = fmt.Sprintf("%s_%d", file, n)
				}
				saw = append(saw, exportFile{file + ".AC", ac}, exportFile{file + ".AD", ad})
			}
		case cutplan.LinearID:
			var job linear.Job
			if err := json.Unmarshal(m.Job, &job); err != nil {
				return nil, "", err
			}
			for _, jp := range job.Parts {
				plan.Parts = append(plan.Parts, part(jp.ID, jp.Length, 0, jp.Quantity))
			}
			for _, st := range job.Stock {
				if st.Length > plan.Material.Length {
					plan.Material.Length = st.Length
				}
			}
		}
		plans = append(plans, plan)
	}

	lang := "pt-BR"
	if req.Lang == "en" {
		lang = "en"
	}
	pdf, err := cutreport.PDF(head, plans, lang, "")
	if err != nil {
		return nil, "", err
	}
	return append([]exportFile{{base + ".pdf", pdf}}, saw...), base, nil
}

func readExport(w http.ResponseWriter, r *http.Request) (exportRequest, bool) {
	var req exportRequest
	if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&req) != nil || len(req.Materials) == 0 {
		http.Error(w, "POST {mode, info, materials: [{code, desc, thickness, job, result}], parts}", http.StatusBadRequest)
		return req, false
	}
	return req, true
}

// handlePDF sends the report of a finished plan.
func handlePDF(w http.ResponseWriter, r *http.Request) {
	req, ok := readExport(w, r)
	if !ok {
		return
	}
	files, _, err := req.build(false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": files[0].name}))
	_, _ = w.Write(files[0].data)
}

// handleProduction sends what the shop floor needs in one ZIP: the report
// and, for sheets, the .AC and .AD files of the Giben saw.
func handleProduction(w http.ResponseWriter, r *http.Request) {
	req, ok := readExport(w, r)
	if !ok {
		return
	}
	files, base, err := req.build(true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		fw, err := zw.Create(f.name)
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
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": base + " - producao.zip"}))
	_, _ = w.Write(buf.Bytes())
}
