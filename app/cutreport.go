package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/linear"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/rect"
	"github.com/go-pdf/fpdf"
)

// The cut plan report: an A4 PDF laid out like the Corte Certo sheets the
// shop floor already knows. One page per different sheet layout, with its
// header (plan i/n, how many sheets, material, sheet size and code), the
// figures (parts, use, loss, cuts), the drawing with numbered parts and the
// part list; one page per bar material with a line per bar. Every page
// ends with the product, "File location:" and "page 2 of 5".

// reportText is the wording of the report, by language.
var reportText = map[string]map[string]string{
	"pt-BR": {
		"plan": "Plano de corte %d / %d", "qty": "Quantidade: %d", "material": "Material: %s", "thick": "%s MM",
		"sheet": "Chapa: %s mm x %s mm", "sheet_code": "Código (chapa): %s", "bar_code": "Código (barra): %s", "offcut_code": "Retalho: %s (material %s)",
		"desc": "Descrição: %s", "first": "Corte inicial: %s ; %d fases", "first_l": "Horizontal", "first_w": "Vertical",
		"parts": "Quantidade de peças: %d (%s m²)", "use": "Aproveitamento: %s %%", "loss": "Perda: %s %%",
		"cuts": "Quantidade de cortes: %d ; %s m", "when": "Data: %s", "order": "Pedido: %s", "user": "Usuário: %s",
		"h_n": "N", "h_proj": "Projeto", "h_code": "Código", "h_desc": "Descrição", "h_leg": "Leg", "h_w": "Largura",
		"h_h": "Altura", "h_len": "Comprimento", "h_qt": "Qt", "footer": "PLANO DE CORTE PARA %d %s - (%s)",
		"piece": "PEÇA", "pieces": "PEÇAS", "file": "Local do arquivo: %s", "page": "folha %d de {nb}",
		"bar": "Barra", "bars_plan": "Plano de corte - barras", "bar_size": "Barra: %s mm", "bar_line": "%s (%s mm): %s",
		"remain": "retalho %s", "scrap": "descarte %s", "bars_n": "Quantidade de barras: %d", "ret": "RET %d",
		"unplaced": "Não couberam: %s",
	},
	"en": {
		"plan": "Cut plan %d / %d", "qty": "Quantity: %d", "material": "Material: %s", "thick": "%s MM",
		"sheet": "Sheet: %s mm x %s mm", "sheet_code": "Code (sheet): %s", "bar_code": "Code (bar): %s", "offcut_code": "Offcut: %s (material %s)",
		"desc": "Description: %s", "first": "First cut: %s ; %d stages", "first_l": "Horizontal", "first_w": "Vertical",
		"parts": "Parts: %d (%s m²)", "use": "Use: %s %%", "loss": "Loss: %s %%",
		"cuts": "Cuts: %d ; %s m", "when": "Date: %s", "order": "Order: %s", "user": "User: %s",
		"h_n": "N", "h_proj": "Project", "h_code": "Code", "h_desc": "Description", "h_leg": "Tag", "h_w": "Width",
		"h_h": "Height", "h_len": "Length", "h_qt": "Qty", "footer": "CUT PLAN FOR %d %s - (%s)",
		"piece": "PIECE", "pieces": "PIECES", "file": "File location: %s", "page": "page %d of {nb}",
		"bar": "Bar", "bars_plan": "Cut plan - bars", "bar_size": "Bar: %s mm", "bar_line": "%s (%s mm): %s",
		"remain": "offcut %s", "scrap": "scrap %s", "bars_n": "Bars: %d", "ret": "OFF %d",
		"unplaced": "Not placed: %s",
	},
}

// report holds what every page needs.
type report struct {
	pdf  *fpdf.Fpdf
	tr   func(string) string // UTF-8 to the PDF's code page
	txt  map[string]string
	st   PlanStatus
	path string // where the file is saved, for the footer
}

func (r *report) t(key string, args ...any) string {
	s, ok := r.txt[key]
	if !ok {
		s = key
	}
	if len(args) > 0 {
		s = fmt.Sprintf(s, args...)
	}
	return r.tr(s)
}

// fmtMM writes a size the way the shop reads it: no decimals when whole,
// a comma otherwise.
func fmtMM(v float64) string {
	s := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.1f", v), "0"), ".")
	return strings.Replace(s, ".", ",", 1)
}

func fmtPct(v float64) string { return strings.Replace(fmt.Sprintf("%.2f", v), ".", ",", 1) }
func fmtM(v float64) string   { return strings.Replace(fmt.Sprintf("%.3f", v), ".", ",", 1) }

// cutReportPDF writes the report of a plan. path only appears in the
// footer; the caller saves the bytes where it wants.
func cutReportPDF(st PlanStatus, language, path string) ([]byte, error) {
	txt := reportText["pt-BR"]
	if language == "en" {
		txt = reportText["en"]
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(12, 10, 12)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AliasNbPages("{nb}")
	r := &report{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor(""), txt: txt, st: st, path: path}
	pdf.SetFooterFunc(r.footer)

	pages := 0
	for _, mp := range st.Materials {
		if mp.State != "done" && mp.State != "stopped" || len(mp.Result) == 0 {
			continue
		}
		switch mp.Planner {
		case cutplan.RectangularID:
			var job rect.Job
			var res rect.Result
			if json.Unmarshal(mp.Job, &job) != nil || json.Unmarshal(mp.Result, &res) != nil {
				continue
			}
			pages += r.sheetPages(mp, job, res)
		case cutplan.LinearID:
			var job linear.Job
			var res linear.Result
			if json.Unmarshal(mp.Job, &job) != nil || json.Unmarshal(mp.Result, &res) != nil {
				continue
			}
			r.barPage(mp, job, res)
			pages++
		}
	}
	if pages == 0 {
		return nil, fmt.Errorf("no finished plan to print")
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (r *report) footer() {
	pdf := r.pdf
	req := r.st.Request
	qty := req.ProductQty
	if qty < 1 {
		qty = 1
	}
	piece := r.txt["piece"]
	if qty > 1 {
		piece = r.txt["pieces"]
	}
	pdf.SetY(-24)
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(80, 80, 80)
	if req.Product != "" {
		pdf.CellFormat(0, 6, r.tr(fmt.Sprintf(r.txt["footer"], qty, piece, req.Product)), "", 1, "C", false, 0, "")
	}
	pdf.SetFont("Helvetica", "", 12)
	pdf.CellFormat(0, 6, r.tr(req.ProductDesc), "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(120, 120, 120)
	file := ""
	if r.path != "" {
		// A long path loses its middle, so the start (the drive) and the
		// end (the file name) stay readable.
		p := []rune(r.path)
		file = r.t("file", r.path)
		for cut := 1; pdf.GetStringWidth(file) > 148 && cut < len(p)-10; cut++ {
			keep := len(p) - cut
			file = r.t("file", string(p[:keep/3])+"..."+string(p[len(p)-(keep-keep/3):]))
		}
	}
	pdf.CellFormat(150, 4, file, "", 0, "L", false, 0, "")
	pdf.CellFormat(0, 4, r.tr(fmt.Sprintf(r.txt["page"], pdf.PageNo())), "", 0, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
}

// header writes the left block of a page and returns where it ends.
func (r *report) header(lines []string, right []string) float64 {
	pdf := r.pdf
	pdf.SetXY(12, 10)
	pdf.SetFont("Helvetica", "", 10.5)
	for i, l := range lines {
		if i == 0 {
			pdf.SetFont("Helvetica", "B", 12)
		} else {
			pdf.SetFont("Helvetica", "", 10)
		}
		pdf.SetX(12)
		pdf.CellFormat(110, 4.8, l, "", 1, "L", false, 0, "")
	}
	end := pdf.GetY()
	// Right block in a light box.
	pdf.SetFillColor(242, 242, 242)
	pdf.Rect(124, 10, 74, float64(len(right))*4.8+9, "F")
	pdf.SetXY(126, 11)
	for _, l := range right {
		pdf.SetX(126)
		pdf.SetFont("Helvetica", "", 9)
		pdf.CellFormat(70, 4.8, l, "", 1, "L", false, 0, "")
	}
	pdf.SetX(126)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(70, 6, "EngTree", "", 1, "L", false, 0, "")
	if y := pdf.GetY(); y > end {
		end = y
	}
	return end + 3
}

// stamp is the date, order and user line.
func (r *report) stamp() string {
	parts := []string{r.t("when", r.st.Created.Format("02/01/2006 15:04"))}
	if o := r.st.Request.Machine.Order; o != "" {
		parts = append(parts, r.t("order", o))
	}
	if u := r.st.Request.Machine.User; u != "" {
		parts = append(parts, r.t("user", u))
	}
	return strings.Join(parts, "   ")
}

// sheetGroup is one layout and how many sheets are cut that way.
type sheetGroup struct {
	sheet rect.SheetLayout
	count int
}

func groupLayouts(sheets []rect.SheetLayout) []sheetGroup {
	var groups []sheetGroup
	index := map[string]int{}
	for _, s := range sheets {
		var b strings.Builder
		fmt.Fprintf(&b, "%s|%g|%g", s.StockID, s.Length, s.Width)
		for _, p := range s.Placements {
			fmt.Fprintf(&b, ";%s@%g,%g,%v", p.PartID, p.X, p.Y, p.Rotated)
		}
		k := b.String()
		if i, ok := index[k]; ok {
			groups[i].count++
			continue
		}
		index[k] = len(groups)
		groups = append(groups, sheetGroup{s, 1})
	}
	return groups
}

// partRow is one line of the part list of a layout.
type partRow struct {
	n       int
	part    JobPart
	qty     int
	rotated bool // as first placed on the sheet: width and height follow the drawing
}

func (r *report) sheetPages(mp MaterialPlan, job rect.Job, res rect.Result) int {
	pdf := r.pdf
	m := mp.Material
	parts := map[string]JobPart{}
	for _, p := range mp.Parts {
		parts[p.ID] = p
	}
	groups := groupLayouts(res.Sheets)
	for gi, g := range groups {
		s := g.sheet
		pdf.AddPage()

		// The part list of this layout: one row per part, numbered in the
		// order the parts first appear on the sheet.
		var rows []partRow
		rowOf := map[string]int{}
		for _, p := range s.Placements {
			if i, ok := rowOf[p.PartID]; ok {
				rows[i].qty++
				continue
			}
			rowOf[p.PartID] = len(rows)
			rows = append(rows, partRow{n: len(rows) + 1, part: parts[p.PartID], qty: 1, rotated: p.Rotated})
		}
		area := sheetsUse([]rect.SheetLayout{s}, mp.Parts) * s.Length * s.Width / 1e6
		var cutLen float64
		for _, c := range s.Cuts {
			cutLen += math.Abs(c.To-c.From) / 1000
		}
		use := sheetsUse([]rect.SheetLayout{s}, mp.Parts) * 100
		first := r.txt["first_l"]
		if s.FirstCut == rect.AlongWidth {
			first = r.txt["first_w"]
		}
		code := r.t("sheet_code", m.Code)
		if s.StockID != m.Code {
			code = r.t("offcut_code", s.StockID, m.Code)
		}
		left := []string{
			r.t("plan", gi+1, len(groups)),
			r.t("qty", g.count),
			r.t("material", m.Description),
			r.t("thick", fmtMM(m.Thickness)),
			r.t("sheet", fmtMM(s.Width), fmtMM(s.Length)),
			code,
			r.t("first", first, job.Settings.MaxStages()),
			r.stamp(),
		}
		right := []string{
			r.t("parts", len(s.Placements), fmtM(area)),
			r.t("use", fmtPct(use)),
			r.t("loss", fmtPct(100-use)),
			r.t("cuts", len(s.Cuts), fmtM(cutLen)),
		}
		top := r.header(left, right)

		// Room for the drawing: what is left above the table and footer.
		tableH := float64(len(rows)+1)*4.6 + 4
		bottom := 297 - 28 - tableH
		r.drawSheet(s, parts, rowOf, 14, top+2, 182, bottom-top-4)
		r.partTable(rows, bottom)
	}
	return len(groups)
}

// drawSheet draws a layout with the sheet length running down the page,
// as Corte Certo does: page x is the sheet Y, page y is the sheet X.
func (r *report) drawSheet(s rect.SheetLayout, parts map[string]JobPart, rowOf map[string]int, x0, y0, w, h float64) {
	pdf := r.pdf
	const dimRoom = 8 // for the overall sizes on the left and below
	scale := math.Min((w-dimRoom)/s.Width, (h-dimRoom)/s.Length)
	if scale <= 0 {
		return
	}
	ox := x0 + dimRoom + (w-dimRoom-s.Width*scale)/2
	oy := y0
	px := func(y float64) float64 { return ox + y*scale }
	py := func(x float64) float64 { return oy + x*scale }

	pdf.SetLineWidth(0.3)
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetFillColor(255, 255, 255)
	pdf.Rect(ox, oy, s.Width*scale, s.Length*scale, "D")

	// Offcuts and scrap, hatched; offcuts named and sized.
	ret := 0
	for _, o := range s.Offcuts {
		x, y, ww, hh := px(o.Y), py(o.X), o.Width*scale, o.Length*scale
		r.hatch(x, y, ww, hh, o.Reusable)
		if o.Reusable {
			ret++
			pdf.SetFont("Helvetica", "B", 7)
			pdf.Text(x+1, y+3, r.t("ret", ret))
			r.sizeLabels(x, y, ww, hh, o.Width, o.Length, 6.5)
		}
	}

	// Parts: number and code at the top, sizes along the edges.
	pdf.SetLineWidth(0.25)
	for _, p := range s.Placements {
		jp := parts[p.PartID]
		alongX, alongY := jp.CutLength, jp.CutWidth
		if p.Rotated {
			alongX, alongY = alongY, alongX
		}
		x, y, ww, hh := px(p.Y), py(p.X), alongY*scale, alongX*scale
		pdf.SetFillColor(255, 255, 255)
		pdf.Rect(x, y, ww, hh, "FD")
		fs := math.Max(5, math.Min(9, math.Min(ww, hh)/3.5))
		pdf.SetFont("Helvetica", "B", fs)
		label := fmt.Sprintf("%d", rowOf[p.PartID]+1)
		pdf.Text(x+1, y+fs*0.42, label)
		pdf.SetFont("Helvetica", "", fs)
		code := r.tr(firstCode(jp))
		if pdf.GetStringWidth(code) < ww-2 && hh > fs*0.9 {
			pdf.Text(x+(ww-pdf.GetStringWidth(code))/2, y+math.Min(hh/2, fs*0.9)+fs*0.2, code)
		}
		r.sizeLabels(x, y, ww, hh, alongY, alongX, math.Max(5, fs-1))
	}

	// Overall sizes.
	pdf.SetFont("Helvetica", "", 8)
	wTxt := fmtMM(s.Width)
	pdf.Text(ox+(s.Width*scale-pdf.GetStringWidth(wTxt))/2, oy+s.Length*scale+5, wTxt)
	lTxt := fmtMM(s.Length)
	pdf.TransformBegin()
	pdf.TransformRotate(90, ox-3, oy+s.Length*scale/2+pdf.GetStringWidth(lTxt)/2)
	pdf.Text(ox-3, oy+s.Length*scale/2+pdf.GetStringWidth(lTxt)/2, lTxt)
	pdf.TransformEnd()
}

// sizeLabels writes the width under the top-right area of a rectangle and
// the height turned along its left edge, when they fit.
func (r *report) sizeLabels(x, y, w, h, across, down float64, fs float64) {
	pdf := r.pdf
	pdf.SetFont("Helvetica", "", fs)
	a := fmtMM(across)
	if tw := pdf.GetStringWidth(a); tw < w-2 && h > fs*0.8 {
		pdf.Text(x+(w-tw)/2, y+h-1, a)
	}
	d := fmtMM(down)
	if tw := pdf.GetStringWidth(d); tw < h-2 && w > fs*0.9 {
		bx, by := x+fs*0.38+0.6, y+(h+tw)/2
		pdf.TransformBegin()
		pdf.TransformRotate(90, bx, by)
		pdf.Text(bx, by, d)
		pdf.TransformEnd()
	}
}

// hatch fills a rectangle with diagonal lines: dense for an offcut, light
// for scrap.
func (r *report) hatch(x, y, w, h float64, offcut bool) {
	pdf := r.pdf
	if w <= 0 || h <= 0 {
		return
	}
	step := 2.0
	pdf.SetDrawColor(150, 150, 150)
	if !offcut {
		step = 3.5
		pdf.SetDrawColor(190, 190, 190)
	}
	pdf.SetLineWidth(0.15)
	pdf.ClipRect(x, y, w, h, false)
	for d := -h; d < w; d += step {
		pdf.Line(x+d, y+h, x+d+h, y)
	}
	pdf.ClipEnd()
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.2)
	pdf.Rect(x, y, w, h, "D")
}

func firstCode(p JobPart) string {
	if len(p.Codes) > 0 && p.Codes[0] != "" {
		return p.Codes[0]
	}
	return p.ID
}

// partTable is the list under the drawing: N, project, code, description,
// tag, width, height, quantity on this layout.
func (r *report) partTable(rows []partRow, y float64) {
	pdf := r.pdf
	cols := []struct {
		key   string
		w     float64
		align string
	}{{"h_n", 9, "C"}, {"h_proj", 22, "C"}, {"h_code", 34, "C"}, {"h_desc", 63, "L"}, {"h_leg", 10, "C"}, {"h_w", 17, "R"}, {"h_h", 17, "R"}, {"h_qt", 10, "C"}}
	pdf.SetXY(12, y)
	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.SetFillColor(200, 200, 200)
	for _, c := range cols {
		pdf.CellFormat(c.w, 4.6, r.t(c.key), "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Helvetica", "", 8.5)
	for i, row := range rows {
		pdf.SetX(12)
		fill := i%2 == 1
		pdf.SetFillColor(235, 235, 235)
		w, h := row.part.CutWidth, row.part.CutLength
		if row.rotated {
			w, h = h, w
		}
		vals := []string{
			fmt.Sprint(row.n), r.st.Request.Product, firstCode(row.part), row.part.Desc,
			legend(row.n - 1), fmtMM(w), fmtMM(h), fmt.Sprint(row.qty),
		}
		for k, c := range cols {
			v := r.tr(vals[k])
			for pdf.GetStringWidth(v) > c.w-1.5 && len(v) > 1 {
				v = v[:len(v)-1]
			}
			pdf.CellFormat(c.w, 4.6, v, "1", 0, c.align, fill, 0, "")
		}
		pdf.Ln(-1)
	}
}

// legend gives the part tags A, B, ..., Z, AA, AB...
func legend(i int) string {
	s := ""
	for i >= 0 {
		s = string(rune('A'+i%26)) + s
		i = i/26 - 1
	}
	return s
}

// barPage lists the bars of one material: a line per bar layout, and a
// drawing of each.
func (r *report) barPage(mp MaterialPlan, job linear.Job, res linear.Result) {
	pdf := r.pdf
	m := mp.Material
	pdf.AddPage()
	use := barsUse(res.Bars) * 100
	left := []string{
		r.t("bars_plan"),
		r.t("material", m.Description),
		r.t("thick", fmtMM(m.Thickness)),
		r.t("bar_size", fmtMM(m.Length)),
		r.t("bar_code", m.Code),
		r.stamp(),
	}
	right := []string{
		r.t("bars_n", len(res.Bars)),
		r.t("use", fmtPct(use)),
		r.t("loss", fmtPct(100-use)),
	}
	y := r.header(left, right) + 2

	// Group equal bars.
	type group struct {
		bar   linear.Bar
		count int
	}
	var groups []group
	index := map[string]int{}
	for _, b := range res.Bars {
		var k strings.Builder
		fmt.Fprintf(&k, "%s|%g", b.StockID, b.Length)
		for _, c := range b.Cuts {
			fmt.Fprintf(&k, ";%s", c.PartID)
		}
		if i, ok := index[k.String()]; ok {
			groups[i].count++
			continue
		}
		index[k.String()] = len(groups)
		groups = append(groups, group{b, 1})
	}
	longest := 0.0
	for _, g := range groups {
		longest = math.Max(longest, g.bar.Length)
	}
	n := 0
	for _, g := range groups {
		if y > 297-60 {
			pdf.AddPage()
			y = 14
		}
		from := n + 1
		n += g.count
		name := fmt.Sprintf("%s %d", r.txt["bar"], from)
		if g.count > 1 {
			name = fmt.Sprintf("%s %d-%d", r.txt["bar"], from, n)
		}
		var cuts []string
		for _, c := range g.bar.Cuts {
			cuts = append(cuts, fmtMM(c.Length))
		}
		rest := r.txt["scrap"]
		if g.bar.Reusable {
			rest = r.txt["remain"]
		}
		line := fmt.Sprintf(r.txt["bar_line"], name, fmtMM(g.bar.Length), strings.Join(cuts, " + ")) + ", " + fmt.Sprintf(rest, fmtMM(g.bar.Remainder))
		pdf.SetXY(12, y)
		pdf.SetFont("Helvetica", "", 9.5)
		pdf.MultiCell(186, 4.6, r.tr(line), "", "L", false)
		y = pdf.GetY() + 1
		// The bar, to scale.
		scale := 186 / longest
		pdf.SetLineWidth(0.25)
		pdf.Rect(12, y, g.bar.Length*scale, 6, "D")
		if s := job.Settings.StartLoss; s > 0 {
			r.hatch(12, y, s*scale, 6, false)
		}
		if e := job.Settings.EndLoss; e > 0 {
			r.hatch(12+(g.bar.Length-e)*scale, y, e*scale, 6, false)
		}
		for _, c := range g.bar.Cuts {
			x := 12 + c.Start*scale
			pdf.SetFillColor(255, 255, 255)
			pdf.Rect(x, y, c.Length*scale, 6, "FD")
			pdf.SetFont("Helvetica", "", 6.5)
			if t := fmtMM(c.Length); pdf.GetStringWidth(t) < c.Length*scale-1 {
				pdf.Text(x+(c.Length*scale-pdf.GetStringWidth(t))/2, y+4.2, t)
			}
		}
		if g.bar.Remainder > 0 {
			end := g.bar.Length - job.Settings.EndLoss
			r.hatch(12+(end-g.bar.Remainder)*scale, y, g.bar.Remainder*scale, 6, g.bar.Reusable)
		}
		y += 10
	}

	// Part list.
	parts := append([]JobPart(nil), mp.Parts...)
	sort.SliceStable(parts, func(i, j int) bool { return parts[i].CutLength > parts[j].CutLength })
	rows := make([]partRow, len(parts))
	for i, p := range parts {
		rows[i] = partRow{n: i + 1, part: p, qty: p.Qty}
	}
	if y+float64(len(rows)+1)*4.6 > 297-28 {
		pdf.AddPage()
		y = 14
	}
	r.barTable(rows, y+2)
	if len(res.Unplaced) > 0 {
		var u []string
		for _, x := range res.Unplaced {
			u = append(u, fmt.Sprintf("%s x%d", x.PartID, x.Quantity))
		}
		pdf.SetX(12)
		pdf.SetFont("Helvetica", "B", 9)
		pdf.CellFormat(0, 6, r.t("unplaced", strings.Join(u, ", ")), "", 1, "L", false, 0, "")
	}
}

func (r *report) barTable(rows []partRow, y float64) {
	pdf := r.pdf
	cols := []struct {
		key   string
		w     float64
		align string
	}{{"h_n", 9, "C"}, {"h_proj", 24, "C"}, {"h_code", 38, "C"}, {"h_desc", 80, "L"}, {"h_len", 23, "R"}, {"h_qt", 12, "C"}}
	pdf.SetXY(12, y)
	pdf.SetFont("Helvetica", "B", 8.5)
	pdf.SetFillColor(200, 200, 200)
	for _, c := range cols {
		pdf.CellFormat(c.w, 4.6, r.t(c.key), "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Helvetica", "", 8.5)
	for i, row := range rows {
		pdf.SetX(12)
		pdf.SetFillColor(235, 235, 235)
		vals := []string{fmt.Sprint(row.n), r.st.Request.Product, firstCode(row.part), row.part.Desc, fmtMM(row.part.CutLength), fmt.Sprint(row.qty)}
		for k, c := range cols {
			pdf.CellFormat(c.w, 4.6, r.tr(vals[k]), "1", 0, c.align, i%2 == 1, 0, "")
		}
		pdf.Ln(-1)
	}
}

// reportFileName is the name of a saved report.
func reportFileName(st PlanStatus) string {
	name := "Plano de corte"
	if st.Request.Product != "" {
		name += " - " + st.Request.Product
	}
	if st.Request.ProductDesc != "" {
		name += " - " + st.Request.ProductDesc
	}
	created := st.Created
	if created.IsZero() {
		created = time.Now()
	}
	name += " - " + created.Format("2006-01-02 1504")
	return safeFileName(name) + ".pdf"
}

// safeFileName drops the characters Windows does not allow in names.
func safeFileName(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, strings.TrimSpace(s))
}

// cutSummaryRows is the summary spreadsheet: one row per material, then
// the parts of each material.
func cutSummaryRows(st PlanStatus, language string) [][]xlsxCell {
	h := []xlsxCell{"Material", "Descrição", "Tipo", "Espessura (mm)", "Chapas/barras", "Aproveitamento (%)", "Peças", "Retalhos", "Não couberam"}
	h2 := []xlsxCell{"Material", "Código", "Descrição", "Itens", "Comp. corte (mm)", "Larg. corte (mm)", "Comp. final (mm)", "Larg. final (mm)", "Qtd"}
	kind := map[string]string{KindSheet: "chapa", KindBar: "barra"}
	if language == "en" {
		h = []xlsxCell{"Material", "Description", "Type", "Thickness (mm)", "Sheets/bars", "Use (%)", "Parts", "Offcuts", "Not placed"}
		h2 = []xlsxCell{"Material", "Code", "Description", "Items", "Cut length (mm)", "Cut width (mm)", "Final length (mm)", "Final width (mm)", "Qty"}
		kind = map[string]string{KindSheet: "sheet", KindBar: "bar"}
	}
	rows := [][]xlsxCell{h}
	for _, mp := range st.Materials {
		m := mp.Material
		var count, parts, offcuts, unplaced int
		var use float64
		switch mp.Planner {
		case cutplan.RectangularID:
			var res rect.Result
			if json.Unmarshal(mp.Result, &res) != nil {
				continue
			}
			count, use = len(res.Sheets), sheetsUse(res.Sheets, mp.Parts)
			for _, s := range res.Sheets {
				parts += len(s.Placements)
				for _, o := range s.Offcuts {
					if o.Reusable {
						offcuts++
					}
				}
			}
			for _, u := range res.Unplaced {
				unplaced += u.Quantity
			}
		case cutplan.LinearID:
			var res linear.Result
			if json.Unmarshal(mp.Result, &res) != nil {
				continue
			}
			count, use = len(res.Bars), barsUse(res.Bars)
			for _, b := range res.Bars {
				parts += len(b.Cuts)
				if b.Reusable {
					offcuts++
				}
			}
			for _, u := range res.Unplaced {
				unplaced += u.Quantity
			}
		}
		rows = append(rows, []xlsxCell{m.Code, m.Description, kind[m.Kind], m.Thickness, count,
			math.Round(use*10000) / 100, parts, offcuts, unplaced})
	}
	rows = append(rows, nil, h2)
	for _, mp := range st.Materials {
		for _, p := range mp.Parts {
			var cw, fw xlsxCell
			if mp.Material.Kind == KindSheet {
				cw, fw = p.CutWidth, p.Width
			}
			rows = append(rows, []xlsxCell{mp.Material.Code, firstCode(p), p.Desc, strings.Join(p.Items, " "),
				p.CutLength, cw, p.Length, fw, p.Qty})
		}
	}
	return rows
}

// sheetsUse is the area of the parts over the area of the whole sheets,
// 0..1: the figure Corte Certo prints as "Aproveitamento". The planner's
// own Utilization leaves the edge trim out, so it reads a little higher.
func sheetsUse(sheets []rect.SheetLayout, parts []JobPart) float64 {
	size := map[string]JobPart{}
	for _, p := range parts {
		size[p.ID] = p
	}
	var used, total float64
	for _, s := range sheets {
		total += s.Length * s.Width
		for _, pl := range s.Placements {
			used += size[pl.PartID].CutLength * size[pl.PartID].CutWidth
		}
	}
	if total == 0 {
		return 0
	}
	return used / total
}

// barsUse is the length of the parts over the length of the whole bars.
func barsUse(bars []linear.Bar) float64 {
	var used, total float64
	for _, b := range bars {
		total += b.Length
		for _, c := range b.Cuts {
			used += c.Length
		}
	}
	if total == 0 {
		return 0
	}
	return used / total
}
