package app

import (
	"math"
	"strconv"
	"strings"
)

// unitToMM converts the unit of the BOM measures to millimetres.
var unitToMM = map[string]float64{"mm": 1, "cm": 10, "m": 1000, "in": 25.4}

// CutLine is one part of a product that goes to the cut plan: a BOM line
// that names a material, with its total quantity.
type CutLine struct {
	Item     string  `json:"item"`     // BOM item number, e.g. "1.2.3"
	Code     string  `json:"code"`     // part code
	Desc     string  `json:"desc"`     //
	Material string  `json:"material"` // material code as written in the BOM
	Length   float64 `json:"length"`   // mm, finished size, along the grain
	Width    float64 `json:"width"`    // mm; 0 for bars
	Qty      int     `json:"qty"`      // line × assemblies above it × product quantity

	// Filled when the material is in the list.
	Kind      string  `json:"kind,omitempty"`
	Thickness float64 `json:"thickness,omitempty"`
	Grain     bool    `json:"grain,omitempty"`

	// Codes translated by the UI: no_length, no_width, bad_qty,
	// unknown_material, too_big.
	Warnings []string `json:"warnings"`
}

// CutLinesResult is what the cut plan reads from one product.
type CutLinesResult struct {
	Lines []CutLine `json:"lines"`
	// Codes translated by the UI: no_material_column, no_length_column,
	// no_width_column (and the BOM's own warnings).
	Warnings []string `json:"warnings"`
}

// cutLines picks the parts to cut out of a product's BOM. A line goes to
// the plan when its material column is filled and it has no children (an
// assembly is not cut; its parts are). productQty is how many products are
// made.
//
// The material, length and width columns are looked up among the columns
// of the BOM and of the details sheet, so the material code can come from
// either.
func cutLines(bom BomResult, cc CutColumns, mats MaterialList, productQty int) CutLinesResult {
	res := CutLinesResult{Lines: []CutLine{}, Warnings: append([]string{}, bom.Warnings...)}
	taken := map[int]bool{}
	matCol := findColumn(bom.Columns, cc.Material, taken)
	taken[matCol] = true
	lenCol := findColumn(bom.Columns, cc.Length, taken)
	taken[lenCol] = true
	widCol := findColumn(bom.Columns, cc.Width, taken)
	if matCol < 0 {
		res.Warnings = append(res.Warnings, "no_material_column")
		return res
	}
	if lenCol < 0 {
		res.Warnings = append(res.Warnings, "no_length_column")
	}
	if widCol < 0 {
		res.Warnings = append(res.Warnings, "no_width_column")
	}
	scale, ok := unitToMM[cc.Unit]
	if !ok {
		scale = 1
	}
	if productQty < 1 {
		productQty = 1
	}

	var walk func(nodes []*TreeNode, mult float64, multOK bool)
	walk = func(nodes []*TreeNode, mult float64, multOK bool) {
		for _, n := range nodes {
			q, qOK := parseQty(n.Qty)
			total, totalOK := mult*q, multOK && qOK
			if len(n.Children) > 0 {
				walk(n.Children, total, totalOK)
				continue
			}
			mat := strings.TrimSpace(value(n, matCol))
			if mat == "" {
				continue
			}
			line := CutLine{Item: n.Item, Code: n.Code, Desc: n.Desc, Material: mat, Warnings: []string{}}
			if l, ok := parseMeasure(value(n, lenCol)); ok {
				line.Length = round3(l * scale)
			}
			if w, ok := parseMeasure(value(n, widCol)); ok {
				line.Width = round3(w * scale)
			}
			// A quantity that is missing, not a number or not whole
			// somewhere up the tree is rounded up and flagged.
			whole := math.Ceil(total*float64(productQty) - 1e-9)
			if !totalOK || whole != total*float64(productQty) {
				line.Warnings = append(line.Warnings, "bad_qty")
			}
			line.Qty = int(math.Max(whole, 1))

			m, found := mats.Find(mat)
			if found {
				line.Kind, line.Thickness, line.Grain = m.Kind, m.Thickness, m.Kind == KindSheet && m.Grain
				if m.Kind == KindBar {
					line.Width = 0
				}
			} else {
				line.Warnings = append(line.Warnings, "unknown_material")
			}
			if line.Length <= 0 {
				line.Warnings = append(line.Warnings, "no_length")
			}
			if line.Width <= 0 && (!found || m.Kind == KindSheet) {
				line.Warnings = append(line.Warnings, "no_width")
			}
			if found && line.Length > 0 && !fits(line, m) {
				line.Warnings = append(line.Warnings, "too_big")
			}
			res.Lines = append(res.Lines, line)
		}
	}
	walk(bom.Tree, 1, true)
	return res
}

// fits says whether the part fits in a whole sheet or bar of m, turned if
// the material has no grain.
func fits(l CutLine, m Material) bool {
	if m.Kind == KindBar {
		return l.Length <= m.Length+1e-6
	}
	if l.Length <= m.Length+1e-6 && l.Width <= m.Width+1e-6 {
		return true
	}
	return !m.Grain && l.Width <= m.Length+1e-6 && l.Length <= m.Width+1e-6
}

func value(n *TreeNode, col int) string {
	if col < 0 || col >= len(n.Values) {
		return ""
	}
	return n.Values[col]
}

// parseQty reads a BOM quantity. Empty means 1 and is fine (many exports
// leave it blank for single parts); text that is not a number counts as 1
// and is flagged.
func parseQty(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 1, true
	}
	v, ok := parseMeasure(s)
	if !ok || v <= 0 {
		return 1, false
	}
	return v, true
}

// parseMeasure reads a number written either way: "730", "730,5",
// "730.5", "730 mm".
func parseMeasure(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(s, "mm"), "un"))
	s = strings.ReplaceAll(s, ",", ".")
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
