package app

import (
	"encoding/json"
	"net/http"
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
