package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// Parts and assemblies index
//
// The index is derived: it is rebuilt from the BOM spreadsheets and is never
// edited. Each BOM is parsed once and kept in a local cache with its
// modification time; on the next scan only the spreadsheets that changed are
// read again, which matters on slow network shares.
// ---------------------------------------------------------------------------

type cachedRow struct {
	Code string   `json:"c"`
	Desc string   `json:"d,omitempty"`
	Vals []string `json:"v,omitempty"`
	Kids bool     `json:"k,omitempty"`
}

type bomCacheEntry struct {
	Mtime    float64     `json:"mtime"`
	DetMtime float64     `json:"det_mtime"`
	Columns  []string    `json:"columns"`
	Rows     []cachedRow `json:"rows"`
}

type partsCacheFile struct {
	Sig  string                   `json:"sig"` // settings that change how a BOM is read
	Boms map[string]bomCacheEntry `json:"boms"`
}

// PartRec is one distinct part code found in any BOM.
type PartRec struct {
	Code     string   `json:"code"`
	Desc     string   `json:"desc"`
	Vals     []string `json:"vals"` // aligned with Config.PartsColumns
	Used     []string `json:"used"` // products whose BOM contains it
	Assembly bool     `json:"assembly"`
	Files    []string `json:"files"`
	Mtime    float64  `json:"mtime"` // date of its main file
	Src      string   `json:"src"`   // product used to show its sub-tree
	srcKids  bool
	blob     string // lower-case text for searching
}

var (
	partsMu     sync.RWMutex
	partsList   []*PartRec
	partsByCode = map[string]*PartRec{}

	// Every known code (lower case): products and parts. Used to tell a
	// part's own file from a revision of a shorter code. Replaced, never
	// modified in place, so readers need no lock after fetching it.
	knownMu    sync.RWMutex
	knownCodes = map[string]bool{}
)

func getKnownCodes() map[string]bool {
	knownMu.RLock()
	defer knownMu.RUnlock()
	return knownCodes
}

func addKnownCodes(extra map[string]bool) {
	knownMu.Lock()
	defer knownMu.Unlock()
	merged := make(map[string]bool, len(knownCodes)+len(extra))
	for k := range knownCodes {
		merged[k] = true
	}
	for k := range extra {
		merged[k] = true
	}
	knownCodes = merged
}

func partsCachePath() string { return filepath.Join(dataDir, "parts_cache.json") }
func partsIndexPath() string { return filepath.Join(dataDir, "parts_index.json") }

func configSig(cfg Config) string {
	b, _ := json.Marshal([]any{cfg.Columns, cfg.HiddenColumns, cfg.BomPrefix, cfg.DetailsPrefix, cfg.DetailsFile, cfg.NameSeparator})
	return string(b)
}

func (p *PartRec) makeBlob() {
	p.blob = strings.ToLower(p.Code + "\x00" + p.Desc + "\x00" + strings.Join(p.Vals, "\x00") + "\x00" + strings.Join(p.Used, "\x00"))
}

// loadPartsIndex restores the last built index so the Parts tab works at
// once (and offline) while the first scan runs.
func loadPartsIndex() {
	b, err := os.ReadFile(partsIndexPath())
	if err != nil {
		return
	}
	var list []*PartRec
	if json.Unmarshal(b, &list) != nil {
		return
	}
	setParts(list)
}

func setParts(list []*PartRec) {
	by := make(map[string]*PartRec, len(list))
	codes := make(map[string]bool, len(list))
	for _, p := range list {
		p.makeBlob()
		by[strings.ToUpper(p.Code)] = p
		codes[strings.ToLower(p.Code)] = true
	}
	partsMu.Lock()
	partsList, partsByCode = list, by
	partsMu.Unlock()
	addKnownCodes(codes)
}

func findPart(code string) *PartRec {
	partsMu.RLock()
	defer partsMu.RUnlock()
	return partsByCode[strings.ToUpper(strings.TrimSpace(code))]
}

func statMtime(path string) float64 {
	if path == "" {
		return 0
	}
	if info, err := os.Stat(path); err == nil {
		return float64(info.ModTime().Unix())
	}
	return 0
}

// buildPartsIndex parses new or changed BOMs, rebuilds the parts index and
// fills in, for every product, which file types exist for it or its parts.
func buildPartsIndex(cfg Config, list []Product, progress func(done, total int)) {
	var cache partsCacheFile
	if b, err := os.ReadFile(partsCachePath()); err == nil {
		_ = json.Unmarshal(b, &cache)
	}
	sig := configSig(cfg)
	if cache.Sig != sig || cache.Boms == nil {
		cache = partsCacheFile{Sig: sig, Boms: map[string]bomCacheEntry{}}
	}
	next := map[string]bomCacheEntry{}
	sharedMt := statMtime(cfg.DetailsFile)

	for i, p := range list {
		detMt := p.DetailsMtime
		if p.DetailsFile == "" {
			detMt = sharedMt
		}
		ent, ok := cache.Boms[p.BomFile]
		if !ok || ent.Mtime != p.Mtime || ent.DetMtime != detMt {
			res := buildBom(p, cfg)
			ent = bomCacheEntry{Mtime: p.Mtime, DetMtime: detMt, Columns: res.Columns}
			walkTree(res.Tree, func(n *TreeNode) {
				if strings.TrimSpace(n.Code) != "" {
					ent.Rows = append(ent.Rows, cachedRow{Code: strings.TrimSpace(n.Code), Desc: n.Desc, Vals: n.Values, Kids: len(n.Children) > 0})
				}
			})
			unreadable := false
			for _, w := range res.Warnings {
				unreadable = unreadable || w == "bom_unreadable"
			}
			if unreadable {
				ok = false // do not remember a failed read (file open in Excel, network hiccup)
			} else {
				ok = true
			}
		}
		if ok {
			next[p.BomFile] = ent
		}
		if progress != nil {
			progress(i+1, len(list))
		}
	}
	cache.Boms = next
	if b, err := json.Marshal(cache); err == nil {
		_ = os.WriteFile(partsCachePath(), b, 0644)
	}

	// Merge all BOMs into one record per code.
	wanted := make([]string, len(cfg.PartsColumns))
	for i, c := range cfg.PartsColumns {
		wanted[i] = normHeader(c)
	}
	by := map[string]*PartRec{}
	var parts []*PartRec
	codes := map[string]bool{}
	for _, p := range list {
		codes[strings.ToLower(p.Code)] = true
		ent, ok := next[p.BomFile]
		if !ok {
			continue
		}
		colAt := map[string]int{}
		for i, c := range ent.Columns {
			colAt[normHeader(c)] = i
		}
		for _, r := range ent.Rows {
			key := strings.ToUpper(r.Code)
			rec := by[key]
			if rec == nil {
				rec = &PartRec{Code: r.Code, Vals: make([]string, len(wanted)), Used: []string{}, Files: []string{}, Src: p.Code, srcKids: r.Kids}
				by[key] = rec
				parts = append(parts, rec)
				codes[strings.ToLower(r.Code)] = true
			}
			if rec.Desc == "" {
				rec.Desc = r.Desc
			}
			for i, w := range wanted {
				if at, ok := colAt[w]; ok && at < len(r.Vals) && rec.Vals[i] == "" {
					rec.Vals[i] = r.Vals[at]
				}
			}
			if len(rec.Used) == 0 || rec.Used[len(rec.Used)-1] != p.Code {
				rec.Used = append(rec.Used, p.Code)
			}
			if r.Kids {
				rec.Assembly = true
				if !rec.srcKids {
					rec.Src, rec.srcKids = p.Code, true
				}
			}
		}
	}
	knownMu.Lock()
	knownCodes = codes
	knownMu.Unlock()

	// Which files exist for each part.
	typesOf := func(code string) ([]string, float64) {
		var names []string
		var mt float64
		for _, ft := range cfg.FileTypes {
			if f := findFiles(ft, code, codes); len(f) > 0 {
				names = append(names, ft.Name)
				if mt == 0 {
					mt = f[0].Mtime
				}
			}
		}
		return names, mt
	}
	for _, rec := range parts {
		rec.Files, rec.Mtime = typesOf(rec.Code)
		if rec.Files == nil {
			rec.Files = []string{}
		}
		if hasRefs(cfg, rec.Code, codes) {
			rec.Files = append(rec.Files, refTypeName)
		}
	}
	sort.Slice(parts, func(i, j int) bool { return strings.ToLower(parts[i].Code) < strings.ToLower(parts[j].Code) })
	setParts(parts)
	if b, err := json.Marshal(parts); err == nil {
		_ = os.WriteFile(partsIndexPath(), b, 0644)
	}

	// Products: a type is listed when the product or any of its parts has it.
	for i := range list {
		has := map[string]bool{}
		names, _ := typesOf(list[i].Code)
		for _, n := range names {
			has[n] = true
		}
		if ent, ok := next[list[i].BomFile]; ok {
			for _, r := range ent.Rows {
				if rec := by[strings.ToUpper(r.Code)]; rec != nil {
					for _, n := range rec.Files {
						has[n] = true
					}
				}
			}
		}
		list[i].Files = []string{}
		for _, ft := range cfg.FileTypes {
			if has[ft.Name] {
				list[i].Files = append(list[i].Files, ft.Name)
			}
		}
		list[i].Refs = len(findRefs(cfg, list[i].Code, codes))
	}
}

// ---- Showing a product or a part ----

type itemView struct {
	Product    Product
	Bom        BomResult
	RootValues []string
	Part       bool
	Found      bool
}

func findNode(tree []*TreeNode, code string) *TreeNode {
	var first, withKids *TreeNode
	walkTree(tree, func(n *TreeNode) {
		if strings.EqualFold(strings.TrimSpace(n.Code), code) {
			if first == nil {
				first = n
			}
			if withKids == nil && len(n.Children) > 0 {
				withKids = n
			}
		}
	})
	if withKids != nil {
		return withKids
	}
	return first
}

// resolveItem returns the tree to show for a code: the whole BOM for a
// product, or the part with its own sub-tree for an assembly or part.
func resolveItem(code string, cfg Config, online bool) itemView {
	v := itemView{Bom: BomResult{Columns: []string{}, Tree: []*TreeNode{}, Warnings: []string{}}, RootValues: []string{}}
	if p := findProduct(code); p != nil {
		v.Product, v.Found = *p, true
		if online {
			v.Bom = buildBom(*p, cfg)
		}
		return v
	}
	rec := findPart(code)
	if rec == nil {
		return v
	}
	v.Found, v.Part = true, true
	v.Product = Product{Code: rec.Code, Description: rec.Desc}
	if src := findProduct(rec.Src); src != nil && online {
		res := buildBom(*src, cfg)
		if n := findNode(res.Tree, rec.Code); n != nil {
			v.Product.Description = n.Desc
			v.Bom.Columns = res.Columns
			v.Bom.Tree = n.Children
			v.RootValues = n.Values
		}
	}
	return v
}

// ---- Routes ----

func registerPartsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/parts_facets", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		type facet struct {
			Name   string   `json:"name"`
			Values []string `json:"values"`
		}
		facets := []facet{}
		partsMu.RLock()
		for i, c := range cfg.PartsColumns {
			seen := map[string]bool{}
			var vals []string
			for _, p := range partsList {
				if i < len(p.Vals) && p.Vals[i] != "" && !seen[p.Vals[i]] {
					seen[p.Vals[i]] = true
					vals = append(vals, p.Vals[i])
				}
			}
			sort.Slice(vals, func(a, b int) bool { return strings.ToLower(vals[a]) < strings.ToLower(vals[b]) })
			if len(vals) > 500 {
				vals = vals[:500]
			}
			facets = append(facets, facet{Name: c, Values: vals})
		}
		partsMu.RUnlock()
		types := []string{}
		for _, ft := range cfg.FileTypes {
			types = append(types, ft.Name)
		}
		writeJSON(w, map[string]any{"columns": facets, "types": types})
	})

	mux.HandleFunc("/api/parts", func(w http.ResponseWriter, r *http.Request) {
		qs := r.URL.Query()
		q := strings.ToLower(strings.TrimSpace(qs.Get("q")))
		var has []string
		for _, h := range strings.Split(qs.Get("has"), ",") {
			if h = strings.TrimSpace(h); h != "" {
				has = append(has, h)
			}
		}
		withFiles := qs.Get("withfiles") == "1"
		ncols := len(getConfig().PartsColumns)

		partsMu.RLock()
		list := make([]*PartRec, 0, 256)
	next:
		for _, p := range partsList {
			if withFiles && len(p.Files) == 0 {
				continue
			}
			if q != "" && !strings.Contains(p.blob, q) {
				continue
			}
			for _, h := range has {
				found := false
				for _, f := range p.Files {
					found = found || f == h
				}
				if !found {
					continue next
				}
			}
			for i := 0; i < ncols; i++ {
				if want := qs.Get(fmt.Sprintf("f%d", i)); want != "" && (i >= len(p.Vals) || p.Vals[i] != want) {
					continue next
				}
			}
			list = append(list, p)
		}
		partsMu.RUnlock()

		asc := qs.Get("asc") != "0"
		less := func(i, j int) bool { return strings.ToLower(list[i].Code) < strings.ToLower(list[j].Code) }
		switch qs.Get("sort") {
		case "desc":
			less = func(i, j int) bool { return strings.ToLower(list[i].Desc) < strings.ToLower(list[j].Desc) }
		case "date":
			less = func(i, j int) bool { return list[i].Mtime < list[j].Mtime }
		}
		sort.SliceStable(list, func(i, j int) bool {
			if asc {
				return less(i, j)
			}
			return less(j, i)
		})
		total := len(list)
		if len(list) > 500 {
			list = list[:500]
		}
		writeJSON(w, map[string]any{"total": total, "rows": list})
	})

	// ZIP with every file of one type (or the attachments) of a single code.
	mux.HandleFunc("/api/item_zip", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		typ := r.URL.Query().Get("type")
		var files []FoundFile
		for _, tf := range findAllFiles(cfg, code, getKnownCodes()) {
			if strings.EqualFold(tf.Type, typ) {
				files = tf.Files
			}
		}
		if len(files) == 0 {
			http.Error(w, "no files", 404)
			return
		}
		var entries []ZipEntry
		used := map[string]bool{}
		for _, f := range files {
			arc := f.Name
			base := strings.TrimSuffix(arc, filepath.Ext(arc))
			for n := 1; used[strings.ToLower(arc)]; n++ {
				arc = fmt.Sprintf("%s_%d%s", base, n, filepath.Ext(f.Name))
			}
			used[strings.ToLower(arc)] = true
			entries = append(entries, ZipEntry{Path: f.Path, ArcName: arc})
		}
		label := typ
		if typ == refTypeName {
			label = "files"
		}
		w.Header().Set("Content-Type", "application/zip")
		attachment(w, code+"_"+label+".zip")
		writeZip(w, entries)
	})
}
