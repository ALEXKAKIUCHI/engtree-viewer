package app

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cutRoutes adds the cut plan API:
//
//	GET  /api/materials           the material list
//	POST /api/materials           replace it (refused with 400 and the reasons when invalid)
//	GET  /api/cut_lines?code=&qty= the parts of a product to cut, with total quantities
//	GET  /api/cut_machine          the machine values typed last time
//	POST /api/cut_plan             start a plan (PlanRequest); a new one stops the old
//	GET  /api/cut_plan             the plan as it is now
//	POST /api/cut_plan/stop        stop it; what was found so far is kept
//	GET  /api/cut_plan/pdf         the report; also saved in the reports folder when one is set
//	GET  /api/cut_plan/xlsx        the summary spreadsheet; saved beside the report the same way
func cutRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/cut_machine", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, currentMachine())
	})

	mux.HandleFunc("/api/cut_plan", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, plans.get())
			return
		}
		var req PlanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid plan", http.StatusBadRequest)
			return
		}
		rememberMachine(req.Machine)
		writeJSON(w, plans.start(req, loadMaterials()))
	})

	mux.HandleFunc("/api/cut_plan/pdf", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		st := plans.get()
		name := reportFileName(st)
		path := ""
		if cfg.CutReportsFolder != "" {
			path = filepath.Join(cfg.CutReportsFolder, name)
		}
		b, err := cutReportPDF(st, pickReportLang(cfg.Language), path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if path != "" {
			if err := os.WriteFile(path, b, 0644); err != nil {
				w.Header().Set("X-Save-Error", err.Error())
			}
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `inline; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
		_, _ = w.Write(b)
	})

	mux.HandleFunc("/api/cut_plan/xlsx", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		st := plans.get()
		if len(st.Materials) == 0 {
			http.Error(w, "no plan", http.StatusConflict)
			return
		}
		b, err := writeXLSX("Resumo", cutSummaryRows(st, pickReportLang(cfg.Language)))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		name := strings.TrimSuffix(reportFileName(st), ".pdf") + ".xlsx"
		if cfg.CutReportsFolder != "" {
			if err := os.WriteFile(filepath.Join(cfg.CutReportsFolder, name), b, 0644); err != nil {
				w.Header().Set("X-Save-Error", err.Error())
			}
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
		_, _ = w.Write(b)
	})

	mux.HandleFunc("/api/cut_plan/stop", func(w http.ResponseWriter, r *http.Request) {
		plans.stop()
		writeJSON(w, plans.get())
	})

	mux.HandleFunc("/api/materials", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var l MaterialList
			if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
				http.Error(w, "invalid material list", http.StatusBadRequest)
				return
			}
			if err := saveMaterials(l); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		writeJSON(w, loadMaterials())
	})

	mux.HandleFunc("/api/cut_lines", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		p := findProduct(strings.TrimSpace(r.URL.Query().Get("code")))
		if p == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if !folderReachable(cfg.BomFolder) {
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}
		qty, _ := strconv.Atoi(r.URL.Query().Get("qty"))
		writeJSON(w, cutLines(buildBom(*p, cfg), cfg.CutColumns, loadMaterials(), qty))
	})
}

// pickReportLang follows the language setting; an unset one means
// Portuguese, the language of the shops that use the report today.
func pickReportLang(setting string) string {
	if setting == "en" {
		return "en"
	}
	return "pt-BR"
}
