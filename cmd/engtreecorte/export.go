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

// exportRequest is a finished plan with what the report prints about it.
type exportRequest struct {
	Mode   string          `json:"mode"` // cutplan.LinearID or cutplan.RectangularID
	Job    json.RawMessage `json:"job"`
	Result json.RawMessage `json:"result"`
	Lang   string          `json:"lang"` // "pt" or "en"
	Info   struct {
		Product      string  `json:"product"`
		ProductDesc  string  `json:"product_desc"`
		Order        string  `json:"order"`
		User         string  `json:"user"`
		MaterialCode string  `json:"material_code"`
		MaterialDesc string  `json:"material_desc"`
		Thickness    float64 `json:"thickness"`
	} `json:"info"`
	// Parts adds a description and a project to the parts of the job, by ID.
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

// build checks the plan with the verifier and writes the report and, when
// production is asked for a sheet plan, the Giben files.
func (req exportRequest) build(production bool) ([]exportFile, string, error) {
	p, ok := cutplan.ByID(req.Mode)
	if !ok {
		return nil, "", fmt.Errorf("unknown mode %q", req.Mode)
	}
	if problems := p.Verify(req.Job, req.Result); len(problems) > 0 {
		return nil, "", fmt.Errorf("the plan did not pass the verifier: %s", strings.Join(problems, "; "))
	}
	meta := map[string]int{}
	for i, m := range req.Parts {
		meta[m.ID] = i
	}
	info := req.Info
	plan := cutreport.Plan{
		Material: cutreport.Material{Code: info.MaterialCode, Description: info.MaterialDesc, Thickness: info.Thickness},
		Planner:  req.Mode, Job: req.Job, Result: req.Result,
	}
	part := func(id string, length, width float64, qty int) cutreport.Part {
		cp := cutreport.Part{ID: id, Code: id, CutLength: length, CutWidth: width, Qty: qty}
		if i, ok := meta[id]; ok {
			cp.Desc, cp.Project = req.Parts[i].Desc, req.Parts[i].Project
		}
		return cp
	}
	var rjob rect.Job
	var rres rect.Result
	switch req.Mode {
	case cutplan.RectangularID:
		if err := json.Unmarshal(req.Job, &rjob); err != nil {
			return nil, "", err
		}
		if err := json.Unmarshal(req.Result, &rres); err != nil {
			return nil, "", err
		}
		for _, jp := range rjob.Parts {
			plan.Parts = append(plan.Parts, part(jp.ID, jp.Length, jp.Width, jp.Quantity))
		}
	case cutplan.LinearID:
		var job linear.Job
		if err := json.Unmarshal(req.Job, &job); err != nil {
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

	head := cutreport.Header{Product: info.Product, ProductDesc: info.ProductDesc, ProductQty: 1,
		Order: info.Order, User: info.User, Created: time.Now()}
	lang := "pt-BR"
	if req.Lang == "en" {
		lang = "en"
	}
	base := cutreport.FileName(head)
	pdf, err := cutreport.PDF(head, []cutreport.Plan{plan}, lang, "")
	if err != nil {
		return nil, "", err
	}
	files := []exportFile{{base + ".pdf", pdf}}
	if production && req.Mode == cutplan.RectangularID {
		labels := map[string]giben.PartInfo{}
		for i, jp := range rjob.Parts {
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
		ac, ad, err := giben.Write(rjob, rres, giben.Export{
			Material: info.MaterialDesc, Thickness: info.Thickness, ProjectCode: info.Product,
			Description: desc, Date: time.Now(), Parts: labels,
		})
		if err != nil {
			return nil, "", err
		}
		// The saw files keep the short name GVision is used to.
		saw := fmt.Sprintf("%s_%.1f", strings.Join(strings.Fields(info.MaterialDesc), "_"), info.Thickness)
		if strings.TrimSpace(info.MaterialDesc) == "" {
			saw = "plano"
		}
		files = append(files, exportFile{saw + ".AC", ac}, exportFile{saw + ".AD", ad})
	}
	return files, base, nil
}

func readExport(w http.ResponseWriter, r *http.Request) (exportRequest, bool) {
	var req exportRequest
	if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&req) != nil || len(req.Job) == 0 || len(req.Result) == 0 {
		http.Error(w, "POST {mode, job, result, info, parts}", http.StatusBadRequest)
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
