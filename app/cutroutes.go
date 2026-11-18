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
func cutRoutes(mux *http.ServeMux) {
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
