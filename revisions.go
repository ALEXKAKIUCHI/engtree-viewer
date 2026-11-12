package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Revision requests
//
// The shop floor asks engineering to fix a drawing; engineering answers and
// closes the request. Each request is one small JSON file in a shared folder
// on the network: no server, no database, and two computers writing at the
// same time never touch the same file. Nothing is ever deleted, so the
// history stays.
//
// This folder is the only place EngTree writes to.
// ---------------------------------------------------------------------------

type Revision struct {
	ID        string  `json:"id"`
	Code      string  `json:"code"`    // part or product the request is about
	Desc      string  `json:"desc"`    // its description when asked
	Product   string  `json:"product"` // product being looked at
	Requester string  `json:"requester"`
	Assignee  string  `json:"assignee,omitempty"`
	Kind      string  `json:"kind"` // measure | missing_info | material | file | other
	Note      string  `json:"note,omitempty"`
	Created   string  `json:"created"` // RFC3339
	Computer  string  `json:"computer,omitempty"`
	FileName  string  `json:"file_name,omitempty"`  // main file when asked
	FileMtime float64 `json:"file_mtime,omitempty"` // its date, to notice a new version
	ClosedAt  string  `json:"closed_at,omitempty"`
	ClosedBy  string  `json:"closed_by,omitempty"`
	Answer    string  `json:"answer,omitempty"`

	// Worked out when read, never stored.
	Status string `json:"status,omitempty"` // open | updated | closed
	File   string `json:"-"`                // path of the record
}

var revKinds = map[string]bool{"measure": true, "missing_info": true, "material": true, "file": true, "other": true}

var (
	revMu      sync.Mutex
	revCache   []Revision
	revCacheAt time.Time
	revOnline  bool
	revTTL     = 15 * time.Second
)

func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return time.Now().UTC().Format("20060102T150405") + "_" + hex.EncodeToString(b)
}

// revisionsEnabled is true when a folder is set; usable only if it answers.
func revisionsEnabled(cfg Config) bool { return cfg.RevisionsFolder != "" }

// readRevisions lists the folder (cached) and returns every record. The
// second value is false when the folder is set but cannot be reached.
func readRevisions(cfg Config) ([]Revision, bool) {
	if !revisionsEnabled(cfg) {
		return nil, false
	}
	revMu.Lock()
	defer revMu.Unlock()
	if time.Since(revCacheAt) < revTTL {
		return revCache, revOnline
	}
	if !folderReachableCached(cfg.RevisionsFolder) {
		revCacheAt, revOnline = time.Now(), false
		return revCache, false
	}
	entries, err := os.ReadDir(cfg.RevisionsFolder)
	if err != nil {
		revCacheAt, revOnline = time.Now(), false
		return revCache, false
	}
	var list []Revision
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(cfg.RevisionsFolder, e.Name()))
		if err != nil {
			continue
		}
		var r Revision
		if json.Unmarshal(b, &r) != nil || r.Code == "" {
			continue // not one of ours, or half-written: ignored
		}
		r.File = filepath.Join(cfg.RevisionsFolder, e.Name())
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Created > list[j].Created })
	revCache, revCacheAt, revOnline = list, time.Now(), true
	return list, true
}

func invalidateRevisions() {
	revMu.Lock()
	revCacheAt = time.Time{}
	revMu.Unlock()
}

// mainFile is the file EngTree shows first for a code: used to notice that a
// drawing was replaced after a revision was asked for.
func mainFile(cfg Config, code string) (string, float64) {
	for _, ft := range cfg.FileTypes {
		if f := findFiles(ft, code, getKnownCodes()); len(f) > 0 {
			return f[0].Name, f[0].Mtime
		}
	}
	return "", 0
}

// withStatus works out each record's status now: closed by engineering, or
// settled on its own because the drawing was replaced after the request.
func withStatus(cfg Config, list []Revision) []Revision {
	type cur struct {
		name  string
		mtime float64
	}
	seen := map[string]cur{}
	out := make([]Revision, 0, len(list))
	for _, r := range list {
		switch {
		case r.ClosedAt != "":
			r.Status = "closed"
		default:
			key := strings.ToUpper(r.Code)
			c, ok := seen[key]
			if !ok {
				c.name, c.mtime = mainFile(cfg, r.Code)
				seen[key] = c
			}
			if c.mtime > r.FileMtime || (r.FileMtime == 0 && c.name != "") {
				r.Status = "updated"
			} else {
				r.Status = "open"
			}
		}
		out = append(out, r)
	}
	return out
}

// openCounts maps an upper-case code to how many revisions are still open,
// so the lists and the tree can show it in red.
func openCounts(cfg Config) map[string]int {
	list, ok := readRevisions(cfg)
	if !ok && len(list) == 0 {
		return map[string]int{}
	}
	counts := map[string]int{}
	for _, r := range withStatus(cfg, list) {
		if r.Status == "open" {
			counts[strings.ToUpper(r.Code)]++
		}
	}
	return counts
}

// ---- Labels for the exported spreadsheet ----

var revLabels = map[string]map[string]string{
	"en": {
		"measure": "Wrong measurement", "missing_info": "Missing dimension or information",
		"material": "Material or finish", "file": "Wrong or missing file", "other": "Other",
		"open": "Open", "updated": "Settled (file replaced)", "closed": "Settled",
		"head": "Part;Description;Product;Requester;Date;Type;Reason;Assigned to;Status;Settled on;Answer",
	},
	"pt-BR": {
		"measure": "Medida errada", "missing_info": "Falta cota ou informação",
		"material": "Material ou acabamento", "file": "Arquivo errado ou faltando", "other": "Outro",
		"open": "Aberta", "updated": "Sanada (arquivo substituído)", "closed": "Sanada",
		"head": "Peça;Descrição;Produto;Solicitante;Data;Tipo;Motivo;Responsável;Status;Data da solução;Resposta",
	},
}

func label(lang, key string) string {
	if m, ok := revLabels[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return revLabels["en"][key]
}

func fmtWhen(s string) string {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Local().Format("02/01/2006 15:04")
	}
	return s
}

// csvCell quotes a value for a semicolon-separated file, which is what
// Excel opens with a double click.
func csvCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if strings.ContainsAny(s, ";\"") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// ---- Routes ----

func registerRevisionRoutes(mux *http.ServeMux) {
	// State for the toolbar, the alert and the legend.
	mux.HandleFunc("/api/revisions/state", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		list, online := readRevisions(cfg)
		var open []Revision
		for _, rev := range withStatus(cfg, list) {
			if rev.Status == "open" {
				open = append(open, rev)
			}
		}
		sort.Slice(open, func(i, j int) bool { return open[i].Created < open[j].Created })
		if open == nil {
			open = []Revision{}
		}
		writeJSON(w, map[string]any{
			"enabled":     revisionsEnabled(cfg),
			"online":      online,
			"engineering": cfg.isEngineering(),
			"requester":   cfg.Requester,
			"open":        open,
		})
	})

	// History: of one code, or of a whole product (the code plus its tree).
	mux.HandleFunc("/api/revisions", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		want := map[string]bool{strings.ToUpper(code): true}
		if r.URL.Query().Get("scope") == "product" {
			v := resolveItem(code, cfg, true)
			for c := range codeSet(v.Product.Code, v.Bom.Tree) {
				want[strings.ToUpper(c)] = true
			}
		}
		list, online := readRevisions(cfg)
		out := []Revision{}
		for _, rev := range withStatus(cfg, list) {
			if want[strings.ToUpper(rev.Code)] {
				out = append(out, rev)
			}
		}
		writeJSON(w, map[string]any{"online": online, "rows": out})
	})

	// New request. Production and engineering can both ask.
	mux.HandleFunc("/api/revisions/new", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		cfg := getConfig()
		if !revisionsEnabled(cfg) || !folderReachableCached(cfg.RevisionsFolder) {
			http.Error(w, "revisions folder not reachable", 503)
			return
		}
		var in Revision
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		in.Code = strings.TrimSpace(in.Code)
		in.Requester = strings.TrimSpace(in.Requester)
		in.Note = strings.TrimSpace(in.Note)
		if in.Code == "" || in.Requester == "" || !revKinds[in.Kind] || (in.Kind == "other" && in.Note == "") {
			http.Error(w, "missing fields", 400)
			return
		}
		in.ID = newID()
		in.Created = time.Now().Format(time.RFC3339)
		in.Computer, _ = os.Hostname()
		in.ClosedAt, in.ClosedBy, in.Answer, in.Status = "", "", "", ""
		in.FileName, in.FileMtime = mainFile(cfg, in.Code)

		b, _ := json.MarshalIndent(in, "", "  ")
		path := filepath.Join(cfg.RevisionsFolder, "rev_"+in.ID+".json")
		if err := os.WriteFile(path, b, 0644); err != nil {
			http.Error(w, "could not write to the revisions folder", 503)
			return
		}
		setLocal(func(c *Config) { c.Requester = in.Requester })
		invalidateRevisions()
		writeJSON(w, map[string]bool{"ok": true})
	})

	// Close a request, with a short answer. Engineering only.
	mux.HandleFunc("/api/revisions/close", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		if !cfg.isEngineering() {
			http.Error(w, "engineering mode only", 403)
			return
		}
		var in struct{ ID, Answer, By string }
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.ID == "" {
			http.Error(w, "invalid request", 400)
			return
		}
		list, online := readRevisions(cfg)
		if !online {
			http.Error(w, "revisions folder not reachable", 503)
			return
		}
		for _, rev := range list {
			if rev.ID != in.ID {
				continue
			}
			rev.ClosedAt = time.Now().Format(time.RFC3339)
			rev.ClosedBy = strings.TrimSpace(in.By)
			rev.Answer = strings.TrimSpace(in.Answer)
			rev.Status = ""
			b, _ := json.MarshalIndent(rev, "", "  ")
			if err := os.WriteFile(rev.File, b, 0644); err != nil {
				http.Error(w, "could not write to the revisions folder", 503)
				return
			}
			invalidateRevisions()
			writeJSON(w, map[string]bool{"ok": true})
			return
		}
		http.Error(w, "revision not found", 404)
	})

	// Spreadsheet of every request, open and settled.
	mux.HandleFunc("/api/revisions/export", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		if !cfg.isEngineering() {
			http.Error(w, "engineering mode only", 403)
			return
		}
		lang := r.URL.Query().Get("lang")
		if _, ok := revLabels[lang]; !ok {
			lang = "en"
		}
		list, _ := readRevisions(cfg)
		rows := withStatus(cfg, list)
		sort.Slice(rows, func(i, j int) bool { return rows[i].Created < rows[j].Created })

		var b strings.Builder
		b.WriteString("\ufeff") // so Excel sees UTF-8
		b.WriteString(label(lang, "head") + "\r\n")
		for _, rev := range rows {
			settled := rev.ClosedAt
			if rev.Status == "updated" {
				settled = ""
			}
			cells := []string{rev.Code, rev.Desc, rev.Product, rev.Requester, fmtWhen(rev.Created),
				label(lang, rev.Kind), rev.Note, rev.Assignee, label(lang, rev.Status), fmtWhen(settled), rev.Answer}
			for i, c := range cells {
				if i > 0 {
					b.WriteByte(';')
				}
				b.WriteString(csvCell(c))
			}
			b.WriteString("\r\n")
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		attachment(w, fmt.Sprintf("revisions_%s.csv", time.Now().Format("2006-01-02")))
		_, _ = w.Write([]byte(b.String()))
	})
}
