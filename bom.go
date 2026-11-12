package main

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Product is one BOM spreadsheet found in the BOM folder.
type Product struct {
	Code        string  `json:"code"`
	Description string  `json:"description"`
	BomFile     string  `json:"bom_file"`
	DetailsFile string  `json:"details_file,omitempty"`
	Mtime       float64 `json:"mtime"`

	DetailsMtime float64  `json:"details_mtime,omitempty"`
	Files        []string `json:"files"` // file types found for the product or its parts
	Refs         int      `json:"refs"`  // number of attachments
	NoBom        bool     `json:"no_bom,omitempty"`
}

// TreeNode is one BOM line. Values line up with BomResult.Columns.
type TreeNode struct {
	Item     string      `json:"item"`
	Code     string      `json:"code"`
	Desc     string      `json:"desc"`
	Qty      string      `json:"qty"`
	Values   []string    `json:"values"`
	Files    []string    `json:"files"`             // names of the file types found for this code
	Missing  bool        `json:"missing,omitempty"` // had files before, has none now
	Children []*TreeNode `json:"children"`
}

type BomResult struct {
	Columns  []string    `json:"columns"`  // extra columns, in display order
	Tree     []*TreeNode `json:"tree"`     // top-level lines
	Warnings []string    `json:"warnings"` // codes translated by the UI (missing columns, etc.)
}

// ---- Header matching ----

// normHeader makes headers comparable: upper case, no accents, punctuation
// turned into spaces, single spaces. "Nº do Item" and "N DO ITEM" match;
// "ITEM NO." and "ITEM NO" match.
func normHeader(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		r = stripAccent(r)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func stripAccent(r rune) rune {
	switch r {
	case 'Á', 'À', 'Â', 'Ã', 'Ä':
		return 'A'
	case 'É', 'È', 'Ê', 'Ë':
		return 'E'
	case 'Í', 'Ì', 'Î', 'Ï':
		return 'I'
	case 'Ó', 'Ò', 'Ô', 'Õ', 'Ö':
		return 'O'
	case 'Ú', 'Ù', 'Û', 'Ü':
		return 'U'
	case 'Ç':
		return 'C'
	case 'Ñ':
		return 'N'
	case 'º', '°':
		return ' '
	}
	return r
}

func splitAlternatives(s string) []string {
	var out []string
	for _, a := range strings.Split(s, "|") {
		if a = normHeader(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// findColumn returns the index of the first header that matches one of the
// alternatives: exact match first, then "starts with", then any word of the
// header starting with the alternative. Indexes in taken are skipped.
//
// An alternative written as "#3" means "the 3rd column", for spreadsheets
// whose headers are missing or change from file to file.
func findColumn(header []string, alternatives string, taken map[int]bool) int {
	for _, a := range strings.Split(alternatives, "|") {
		a = strings.TrimSpace(a)
		if n, err := strconv.Atoi(strings.TrimPrefix(a, "#")); err == nil && strings.HasPrefix(a, "#") {
			if n >= 1 && n <= len(header) && !taken[n-1] {
				return n - 1
			}
		}
	}
	alts := splitAlternatives(alternatives)
	norm := make([]string, len(header))
	for i, h := range header {
		norm[i] = normHeader(h)
	}
	try := func(ok func(h, a string) bool) int {
		for _, a := range alts {
			for i, h := range norm {
				if h != "" && !taken[i] && ok(h, a) {
					return i
				}
			}
		}
		return -1
	}
	if i := try(func(h, a string) bool { return h == a }); i >= 0 {
		return i
	}
	if i := try(func(h, a string) bool { return strings.HasPrefix(h, a+" ") }); i >= 0 {
		return i
	}
	return try(func(h, a string) bool {
		for _, w := range strings.Fields(h) {
			if w == a {
				return true
			}
		}
		return false
	})
}

// findHeaderRow returns the header row (within the first 15 rows) and its
// code column. Some exports put a title or blank lines above the header, so
// a row that also has a description, quantity or item column is preferred
// over one that merely contains a word like "Part" (e.g. a "Part list" title).
func findHeaderRow(rows [][]string, cols Columns) (int, int) {
	first, firstCol := -1, -1
	for i := 0; i < len(rows) && i < 15; i++ {
		c := findColumn(rows[i], cols.Code, nil)
		if c < 0 {
			continue
		}
		taken := map[int]bool{c: true}
		if findColumn(rows[i], cols.Description, taken) >= 0 ||
			findColumn(rows[i], cols.Qty, taken) >= 0 ||
			findColumn(rows[i], cols.Item, taken) >= 0 {
			return i, c
		}
		if first < 0 {
			first, firstCol = i, c
		}
	}
	return first, firstCol
}

func cell(row []string, i int) string {
	if i >= 0 && i < len(row) {
		return strings.TrimSpace(row[i])
	}
	return ""
}

// ---- Product discovery ----

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// splitName turns "BOM - ABC-100 - Side table.xlsx" into ("ABC-100", "Side table").
func splitName(fileName, prefix, sep string) (string, string) {
	name := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if prefix != "" && hasPrefixFold(name, prefix) {
		name = name[len(prefix):]
	}
	parts := strings.SplitN(name, sep, 2)
	code := strings.TrimSpace(parts[0])
	desc := ""
	if len(parts) > 1 {
		desc = strings.TrimSpace(parts[1])
	}
	return code, desc
}

func scanProducts(cfg Config) []Product {
	entries := scanDirCached(cfg.BomFolder)
	isXLSX := func(n string) bool { return strings.EqualFold(filepath.Ext(n), ".xlsx") }
	details := map[string]DirEntryInfo{}
	boms := map[string]DirEntryInfo{}

	for _, e := range entries {
		if strings.HasPrefix(e.Name, "~$") || strings.HasPrefix(e.Name, ".") || !isSheetFile(e.Name) {
			continue // Excel/LibreOffice lock files and anything that is not a spreadsheet
		}
		if cfg.DetailsFile != "" && strings.EqualFold(filepath.Clean(e.FullPath), filepath.Clean(cfg.DetailsFile)) {
			continue // the shared details file is not a product
		}
		if cfg.DetailsPrefix != "" && hasPrefixFold(e.Name, cfg.DetailsPrefix) {
			code, _ := splitName(e.Name, cfg.DetailsPrefix, cfg.NameSeparator)
			key := strings.ToUpper(code)
			if prev, ok := details[key]; !ok || (!isXLSX(prev.Name) && isXLSX(e.Name)) {
				details[key] = e
			}
			continue
		}
		if cfg.BomPrefix != "" && !hasPrefixFold(e.Name, cfg.BomPrefix) {
			continue
		}
		code, _ := splitName(e.Name, cfg.BomPrefix, cfg.NameSeparator)
		if code == "" {
			continue
		}
		key := strings.ToUpper(code)
		// Same code as text and .xlsx: the .xlsx wins.
		if prev, ok := boms[key]; !ok || (!isXLSX(prev.Name) && isXLSX(e.Name)) {
			boms[key] = e
		}
	}

	// Products with no BOM spreadsheet: a folder of whole-product drawings.
	// They have files but no tree. A code that has a BOM wins.
	extra := map[string]Product{}
	for _, e := range scanDirCached(cfg.ProductsFolder) {
		if strings.HasPrefix(e.Name, "~$") || strings.HasPrefix(e.Name, ".") {
			continue
		}
		code, desc := splitName(e.Name, "", cfg.NameSeparator)
		key := strings.ToUpper(code)
		if code == "" || boms[key].Name != "" {
			continue
		}
		if prev, ok := extra[key]; !ok || e.Mtime > prev.Mtime {
			extra[key] = Product{Code: code, Description: desc, Mtime: e.Mtime, NoBom: true, Files: []string{}}
		}
	}

	list := make([]Product, 0, len(boms)+len(extra))
	for _, p := range extra {
		list = append(list, p)
	}
	for key, e := range boms {
		code, desc := splitName(e.Name, cfg.BomPrefix, cfg.NameSeparator)
		list = append(list, Product{
			Code:         code,
			Description:  desc,
			BomFile:      e.FullPath,
			DetailsFile:  details[key].FullPath,
			DetailsMtime: details[key].Mtime,
			Mtime:        e.Mtime,
			Files:        []string{},
		})
	}
	sort.Slice(list, func(i, j int) bool {
		return strings.ToLower(list[i].Code) < strings.ToLower(list[j].Code)
	})
	return list
}

// ---- Tree ----

type detailsTable struct {
	columns []string            // display names
	norm    []string            // normalized names, same order
	rows    map[string][]string // code (upper) -> values aligned with columns
}

func readDetails(path string, cfg Config) *detailsTable {
	rows := readTable(path)
	h, codeCol := findHeaderRow(rows, cfg.Columns)
	if h < 0 {
		return nil
	}
	t := &detailsTable{rows: map[string][]string{}}
	var idx []int
	for i, name := range rows[h] {
		if i == codeCol || strings.TrimSpace(name) == "" {
			continue
		}
		t.columns = append(t.columns, strings.TrimSpace(name))
		t.norm = append(t.norm, normHeader(name))
		idx = append(idx, i)
	}
	for _, r := range rows[h+1:] {
		code := strings.ToUpper(cell(r, codeCol))
		if code == "" {
			continue
		}
		vals := make([]string, len(idx))
		for k, i := range idx {
			vals[k] = cell(r, i)
		}
		t.rows[code] = vals
	}
	return t
}

func buildBom(p Product, cfg Config) BomResult {
	res := BomResult{Columns: []string{}, Tree: []*TreeNode{}, Warnings: []string{}}
	if p.BomFile == "" {
		return res // a product with no BOM: files only, no tree
	}
	rows := readTable(p.BomFile)
	if len(rows) == 0 {
		res.Warnings = append(res.Warnings, "bom_unreadable")
		return res
	}
	h, codeCol := findHeaderRow(rows, cfg.Columns)
	if h < 0 {
		res.Warnings = append(res.Warnings, "no_code_column")
		return res
	}
	header := rows[h]
	taken := map[int]bool{codeCol: true}
	itemCol := findColumn(header, cfg.Columns.Item, taken)
	taken[itemCol] = true
	descCol := findColumn(header, cfg.Columns.Description, taken)
	taken[descCol] = true
	qtyCol := findColumn(header, cfg.Columns.Qty, taken)
	taken[qtyCol] = true
	if itemCol < 0 {
		res.Warnings = append(res.Warnings, "no_item_column")
	}

	hidden := map[string]bool{}
	for _, c := range cfg.HiddenColumns {
		hidden[normHeader(c)] = true
	}

	// Extra columns from the BOM, then the ones only the details sheet has.
	var extraIdx []int
	seen := map[string]bool{}
	for i, name := range header {
		n := normHeader(name)
		if taken[i] || n == "" || hidden[n] || seen[n] {
			continue
		}
		seen[n] = true
		res.Columns = append(res.Columns, strings.TrimSpace(name))
		extraIdx = append(extraIdx, i)
	}

	detailsPath := p.DetailsFile
	if detailsPath == "" {
		detailsPath = cfg.DetailsFile
	}
	var det *detailsTable
	detCol := map[int]int{} // details column -> position in res.Columns
	if detailsPath != "" {
		det = readDetails(detailsPath, cfg)
		if det == nil {
			res.Warnings = append(res.Warnings, "details_no_code")
		} else {
			pos := map[string]int{}
			for k, i := range extraIdx {
				pos[normHeader(header[i])] = k
			}
			for k, n := range det.norm {
				if hidden[n] {
					continue
				}
				if at, ok := pos[n]; ok {
					detCol[k] = at
				} else {
					detCol[k] = len(res.Columns)
					res.Columns = append(res.Columns, det.columns[k])
					pos[n] = detCol[k]
				}
			}
		}
	}

	var nodes []*TreeNode
	byItem := map[string]*TreeNode{}
	for n, r := range rows[h+1:] {
		code := cell(r, codeCol)
		item := strings.ReplaceAll(cell(r, itemCol), ",", ".")
		if itemCol < 0 {
			item = strconv.Itoa(n + 1)
		}
		if code == "" && item == "" {
			continue
		}
		node := &TreeNode{
			Item:     item,
			Code:     code,
			Desc:     cell(r, descCol),
			Qty:      cell(r, qtyCol),
			Values:   make([]string, len(res.Columns)),
			Files:    []string{},
			Children: []*TreeNode{},
		}
		for k, i := range extraIdx {
			node.Values[k] = cell(r, i)
		}
		if det != nil {
			if dv, ok := det.rows[strings.ToUpper(code)]; ok {
				for k, at := range detCol {
					if node.Values[at] == "" {
						node.Values[at] = dv[k]
					}
				}
			}
		}
		nodes = append(nodes, node)
		if item != "" {
			byItem[item] = node
		}
	}

	// Link children to parents by item number (1.2.1 -> 1.2), even when the
	// export is out of order or a level is missing.
	for _, n := range nodes {
		var parent *TreeNode
		if itemCol >= 0 && strings.Contains(n.Item, ".") {
			parts := strings.Split(n.Item, ".")
			for i := len(parts) - 1; i > 0; i-- {
				if p, ok := byItem[strings.Join(parts[:i], ".")]; ok && p != n {
					parent = p
					break
				}
			}
		}
		if parent != nil {
			parent.Children = append(parent.Children, n)
		} else {
			res.Tree = append(res.Tree, n)
		}
	}
	return res
}

// walkTree calls fn for every node.
func walkTree(nodes []*TreeNode, fn func(*TreeNode)) {
	for _, n := range nodes {
		fn(n)
		walkTree(n.Children, fn)
	}
}
