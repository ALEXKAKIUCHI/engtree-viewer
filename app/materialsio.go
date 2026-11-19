package app

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The material list as a spreadsheet, to move it between computers or fill
// it in Excel. One row per material; offcuts in one cell, "1000x800x2;
// 600x500" (length x width x quantity, quantity 1 when left out; bars
// have no width: "2500x3").

// materialColumn is one column of the spreadsheet: the header written on
// export and the names recognised on import (case, accents and
// punctuation ignored, as for the BOM).
type materialColumn struct {
	key, header, alternatives string
}

var materialColumns = []materialColumn{
	{"code", "Código", "CODIGO|COD|CODE|COD MP|CODIGO MP"},
	{"alt", "Código alternativo", "CODIGO ALTERNATIVO|COD ALTERNATIVO|COD ALT|ALTERNATIVE CODE|ALT CODE"},
	{"desc", "Descrição", "DESCRICAO|DESCRIPTION|DESC"},
	{"kind", "Tipo", "TIPO|TYPE|KIND"},
	{"thick", "Espessura (mm)", "ESPESSURA|ESP|THICKNESS"},
	{"grain", "Veio", "VEIO|GRAIN"},
	{"len", "Comprimento (mm)", "COMPRIMENTO|COMP|LENGTH"},
	{"wid", "Largura (mm)", "LARGURA|LARG|WIDTH"},
	{"minl", "Sobra mín. comprimento (mm)", "SOBRA MIN COMPRIMENTO|SOBRA MINIMA COMPRIMENTO|MIN OFFCUT LENGTH"},
	{"minw", "Sobra mín. largura (mm)", "SOBRA MIN LARGURA|SOBRA MINIMA LARGURA|MIN OFFCUT WIDTH"},
	{"offcuts", "Retalhos", "RETALHOS|RETALHO|OFFCUTS|OFFCUT"},
}

// materialRows is the list as spreadsheet rows, header first. An empty
// list gives the header alone: a blank template.
func materialRows(l MaterialList) [][]xlsxCell {
	header := make([]xlsxCell, len(materialColumns))
	for i, c := range materialColumns {
		header[i] = c.header
	}
	rows := [][]xlsxCell{header}
	num := func(v float64) xlsxCell {
		if v == 0 {
			return nil
		}
		return v
	}
	for _, m := range l.Materials {
		kind, grain := "chapa", "não"
		if m.Kind == KindBar {
			kind, grain = "barra", ""
		} else if m.Grain {
			grain = "sim"
		}
		rows = append(rows, []xlsxCell{m.Code, m.AltCode, m.Description, kind, num(m.Thickness), grain,
			num(m.Length), num(m.Width), num(m.MinOffcutLength), num(m.MinOffcutWidth), formatOffcuts(m)})
	}
	return rows
}

func formatOffcuts(m Material) string {
	var parts []string
	for _, o := range m.Offcuts {
		s := fmtMM(o.Length)
		if m.Kind == KindSheet {
			s += "x" + fmtMM(o.Width)
		}
		if o.Quantity > 1 {
			s += "x" + strconv.Itoa(o.Quantity)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

// materialsCSV writes the rows for Excel in Portuguese: semicolons, decimal
// commas and a byte order mark so the accents come out right.
func materialsCSV(rows [][]xlsxCell) []byte {
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF")
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	w.UseCRLF = true
	for _, r := range rows {
		rec := make([]string, len(r))
		for i, v := range r {
			switch n := v.(type) {
			case nil:
			case float64:
				rec[i] = fmtMM(n)
			default:
				rec[i] = fmt.Sprint(v)
			}
		}
		_ = w.Write(rec)
	}
	w.Flush()
	return buf.Bytes()
}

// MaterialImport is what reading a spreadsheet gives: the list merged with
// the current one (not saved yet), and what happened row by row.
type MaterialImport struct {
	List     MaterialList    `json:"list"`
	Added    int             `json:"added"`
	Updated  int             `json:"updated"`
	Problems []ImportProblem `json:"problems"` // rows left out, and codes used twice
	Missing  []string        `json:"missing"`  // required columns not found
}

// ImportProblem says why a row of the spreadsheet was left out. Reason is
// a code the screen translates: no_code, bad_type, bad_grain, not_number,
// bad_offcut, no_size, negative_thickness, repeated_code (Value holds the
// other material that has the code).
type ImportProblem struct {
	Row    int    `json:"row"` // spreadsheet row, 1 = first row; 0 for repeated codes
	Code   string `json:"code"`
	Reason string `json:"reason"`
	Value  string `json:"value,omitempty"`
}

// importMaterials reads a spreadsheet (.xlsx, .csv or .txt) and merges its
// rows into current: a row whose code is already in the list replaces that
// material, a new code is added. Materials not in the spreadsheet stay.
func importMaterials(name string, data []byte, current MaterialList) (MaterialImport, error) {
	res := MaterialImport{Problems: []ImportProblem{}, Missing: []string{}}
	dir, err := os.MkdirTemp("", "engtree-materials")
	if err != nil {
		return res, err
	}
	defer os.RemoveAll(dir)
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".xlsx" && ext != ".csv" && ext != ".txt" {
		ext = ".csv"
	}
	path := filepath.Join(dir, "materials"+ext)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return res, err
	}
	rows := readTable(path)
	if len(rows) == 0 {
		return res, fmt.Errorf("the file has no rows (the old .xls format is not read: save it as .xlsx)")
	}

	// The header is the first row, within the first 10, that has a code column.
	head, col := -1, map[string]int{}
	for i := 0; i < len(rows) && i < 10 && head < 0; i++ {
		taken := map[int]bool{}
		found := map[string]int{}
		for _, c := range materialColumns {
			k := findColumn(rows[i], c.alternatives, taken)
			if k >= 0 {
				taken[k] = true
			}
			found[c.key] = k
		}
		if found["code"] >= 0 && found["len"] >= 0 {
			head, col = i, found
		}
	}
	if head < 0 {
		res.Missing = []string{"Código", "Comprimento"}
		return res, nil
	}

	list := current
	list.Materials = append([]Material(nil), current.Materials...)
	index := map[string]int{}
	for i, m := range list.Materials {
		index[materialKey(m.Code)] = i
	}
	get := func(r []string, key string) string { return strings.TrimSpace(cell(r, col[key])) }
	numOf := func(r []string, key string) (float64, bool) {
		s := get(r, key)
		if s == "" {
			return 0, true
		}
		return parseMeasure(s)
	}
	for n, r := range rows[head+1:] {
		rowNo := head + n + 2
		m := Material{
			Code: get(r, "code"), AltCode: get(r, "alt"), Description: get(r, "desc"),
		}
		if m.Code == "" {
			if strings.Join(r, "") != "" {
				res.Problems = append(res.Problems, ImportProblem{Row: rowNo, Reason: "no_code"})
			}
			continue
		}
		var bad []ImportProblem
		problem := func(reason, value string) {
			bad = append(bad, ImportProblem{Row: rowNo, Code: m.Code, Reason: reason, Value: value})
		}
		switch k := normHeader(get(r, "kind")); {
		case k == "" || strings.HasPrefix(k, "CHAPA") || strings.HasPrefix(k, "SHEET") || strings.HasPrefix(k, "PAINEL"):
			m.Kind = KindSheet
		case strings.HasPrefix(k, "BARRA") || strings.HasPrefix(k, "BAR") || strings.HasPrefix(k, "TUBO") ||
			strings.HasPrefix(k, "TUBE") || strings.HasPrefix(k, "PERFIL"):
			m.Kind = KindBar
		default:
			problem("bad_type", get(r, "kind"))
		}
		switch g := normHeader(get(r, "grain")); g {
		case "SIM", "S", "YES", "Y", "X", "1", "TRUE", "VERDADEIRO":
			m.Grain = true
		case "", "NAO", "N", "NO", "0", "FALSE", "FALSO":
		default:
			problem("bad_grain", get(r, "grain"))
		}
		for _, f := range []struct {
			key string
			to  *float64
		}{{"thick", &m.Thickness}, {"len", &m.Length}, {"wid", &m.Width}, {"minl", &m.MinOffcutLength}, {"minw", &m.MinOffcutWidth}} {
			v, ok := numOf(r, f.key)
			if !ok {
				problem("not_number", get(r, f.key))
			}
			*f.to = v
		}
		offcuts, err := parseOffcuts(get(r, "offcuts"), m.Kind)
		if err != nil {
			problem("bad_offcut", get(r, "offcuts"))
		}
		m.Offcuts = offcuts
		// The checks Validate makes on one material.
		if m.Kind == KindSheet && (m.Length <= 0 || m.Width <= 0) || m.Kind == KindBar && m.Length <= 0 {
			problem("no_size", "")
		}
		if m.Thickness < 0 {
			problem("negative_thickness", "")
		}
		if len(bad) > 0 {
			res.Problems = append(res.Problems, bad...)
			continue
		}
		if i, ok := index[materialKey(m.Code)]; ok {
			list.Materials[i] = m
			res.Updated++
		} else {
			index[materialKey(m.Code)] = len(list.Materials)
			list.Materials = append(list.Materials, m)
			res.Added++
		}
	}
	// An alternative code that is another material's code is only seen on
	// the whole list. Saving refuses it; say so now.
	owner := map[string]string{}
	for _, m := range list.Materials {
		for _, c := range []string{m.Code, m.AltCode} {
			k := materialKey(c)
			if k == "" {
				continue
			}
			if other, ok := owner[k]; ok && other != m.Code {
				res.Problems = append(res.Problems, ImportProblem{Code: c, Reason: "repeated_code", Value: other + " / " + m.Code})
				continue
			}
			owner[k] = m.Code
		}
	}
	res.List = list
	return res, nil
}

// parseOffcuts reads "1000x800x2; 600x500" (sheets) or "2500x3; 1200"
// (bars).
func parseOffcuts(s string, kind string) ([]Offcut, error) {
	var out []Offcut
	for _, item := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '|' || r == '\n' }) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		fields := strings.FieldsFunc(strings.ToLower(item), func(r rune) bool { return r == 'x' || r == '×' || r == '*' })
		var n []float64
		for _, f := range fields {
			v, ok := parseMeasure(f)
			if !ok {
				return nil, fmt.Errorf("offcut %q is not written as length x width x quantity", item)
			}
			n = append(n, v)
		}
		o := Offcut{Quantity: 1}
		switch {
		case kind == KindBar && len(n) >= 1 && len(n) <= 2:
			o.Length = n[0]
			if len(n) == 2 {
				o.Quantity = int(n[1])
			}
		case kind != KindBar && len(n) >= 2 && len(n) <= 3:
			o.Length, o.Width = n[0], n[1]
			if len(n) == 3 {
				o.Quantity = int(n[2])
			}
		default:
			return nil, fmt.Errorf("offcut %q is not written as length x width x quantity", item)
		}
		if o.Length <= 0 || o.Quantity <= 0 || (kind != KindBar && o.Width <= 0) {
			return nil, fmt.Errorf("offcut %q has a zero size or quantity", item)
		}
		out = append(out, o)
	}
	return out, nil
}
