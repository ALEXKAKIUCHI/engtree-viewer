package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/linear"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/rect"
	"github.com/go-pdf/fpdf"
)

// The cut plan report: an A4 PDF in black and white, made to print on any
// office printer without wasting toner.
//
//   - Header: EngTree, the product, order, user and first cut on the left;
//     plan i/n with the number of sheets, the date and the sheet code on
//     the right.
//   - Four boxes: material and raw size, use, loss, saw plan (cuts and
//     their length).
//   - The drawing: parts in white with their number, code and size;
//     offcuts and scrap dotted, offcuts named "SOBRA n".
//   - The part list, and a footer with the file location and page i of n.
//
// One page per different sheet layout, one per bar material.

// reportText is the wording of the report, by language.
var reportText = map[string]map[string]string{
	"pt-BR": {
		"title": "PLANO DE CORTE", "order": "Pedido: ", "user": "Usuário: ", "first": "Corte inicial: ",
		"first_l": "Horizontal", "first_w": "Vertical", "stages": " (%d fases)",
		"badge": "PLANO %d / %d  •  QTD: %d %s", "sheet1": "CHAPA", "sheetN": "CHAPAS", "bar1": "BARRA", "barN": "BARRAS",
		"bars_badge": "BARRAS  •  QTD: %d %s", "issued": "Emissão: ", "sheet_code": "Cód. chapa: ", "bar_code": "Cód. barra: ",
		"offcut_of": "Retalho %s de ", "k_material": "MATERIAL & DIMENSÃO BRUTA", "k_use": "APROVEITAMENTO",
		"k_loss": "PERDA / RETALHO", "k_saw": "PLANO DE SERRA", "sheet_size": "Chapa: %s × %s mm (%s m²)",
		"bar_size": "Barra: %s mm", "parts_area": "%d peças (%s m²)", "parts_len": "%d peças (%s m)",
		"cuts": "%d cortes", "path": "Percurso: %s m", "offcut": "SOBRA %d", "scrap": "DESCARTE",
		"h_item": "ITEM", "h_leg": "LEG.", "h_code": "CÓD. PEÇA", "h_desc": "DESCRIÇÃO", "h_proj": "PROJETO",
		"h_size": "MEDIDAS (L × A)", "h_len": "COMPRIMENTO", "h_qty": "QTD.", "file": "Arquivo: %s",
		"page": "EngTree  •  Página %d de {nb}", "bars_n": "Barras %d–%d", "bar_n": "Barra %d",
		"unplaced": "Não couberam: %s",
	},
	"en": {
		"title": "CUT PLAN", "order": "Order: ", "user": "User: ", "first": "First cut: ",
		"first_l": "Horizontal", "first_w": "Vertical", "stages": " (%d stages)",
		"badge": "PLAN %d / %d  •  QTY: %d %s", "sheet1": "SHEET", "sheetN": "SHEETS", "bar1": "BAR", "barN": "BARS",
		"bars_badge": "BARS  •  QTY: %d %s", "issued": "Issued: ", "sheet_code": "Sheet code: ", "bar_code": "Bar code: ",
		"offcut_of": "Offcut %s of ", "k_material": "MATERIAL & RAW SIZE", "k_use": "USE",
		"k_loss": "LOSS / OFFCUT", "k_saw": "SAW PLAN", "sheet_size": "Sheet: %s × %s mm (%s m²)",
		"bar_size": "Bar: %s mm", "parts_area": "%d parts (%s m²)", "parts_len": "%d parts (%s m)",
		"cuts": "%d cuts", "path": "Path: %s m", "offcut": "OFFCUT %d", "scrap": "SCRAP",
		"h_item": "ITEM", "h_leg": "TAG", "h_code": "PART CODE", "h_desc": "DESCRIPTION", "h_proj": "PROJECT",
		"h_size": "SIZE (W × H)", "h_len": "LENGTH", "h_qty": "QTY", "file": "File: %s",
		"page": "EngTree  •  Page %d of {nb}", "bars_n": "Bars %d–%d", "bar_n": "Bar %d",
		"unplaced": "Not placed: %s",
	},
}

// Page geometry, mm.
const (
	pgLeft   = 10.0
	pgRight  = 200.0
	pgWidth  = pgRight - pgLeft
	pgTop    = 8.0
	pgFooter = 286.0 // the footer line
	rowH     = 4.4   // a table row
)

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
	pdf.SetMargins(pgLeft, pgTop, 210-pgRight)
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

// footer: a thin line, the file location on the left, the page on the right.
func (r *report) footer() {
	pdf := r.pdf
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.25)
	pdf.Line(pgLeft, pgFooter, pgRight, pgFooter)
	pdf.SetFont("Helvetica", "", 7.5)
	page := r.tr(fmt.Sprintf(r.txt["page"], pdf.PageNo()))
	pageW := pdf.GetStringWidth(page) + 2
	file := ""
	if r.path != "" {
		// A long path loses its middle, so the start (the drive) and the
		// end (the file name) stay readable.
		p := []rune(r.path)
		file = r.t("file", r.path)
		for cut := 1; pdf.GetStringWidth(file) > pgWidth-pageW-4 && cut < len(p)-10; cut++ {
			keep := len(p) - cut
			file = r.t("file", string(p[:keep/3])+"..."+string(p[len(p)-(keep-keep/3):]))
		}
	}
	pdf.SetXY(pgLeft, pgFooter+1)
	pdf.CellFormat(pgWidth-pageW, 4, file, "", 0, "L", false, 0, "")
	pdf.CellFormat(pageW, 4, page, "", 0, "R", false, 0, "")
}

// header draws the top of a page: brand, product and the line under it on
// the left; badge, date and code on the right. It returns where it ends.
func (r *report) header(badge, codeLabel, code string, firstCut string) float64 {
	pdf := r.pdf
	req := r.st.Request

	// Brand and title box.
	pdf.SetXY(pgLeft, pgTop)
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(pdf.GetStringWidth("EngTree")+2.5, 7, "EngTree", "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 8)
	title := r.t("title")
	tw := pdf.GetStringWidth(title) + 3
	pdf.SetLineWidth(0.25)
	pdf.CellFormat(tw, 4.2, title, "1", 0, "C", false, 0, "")

	// Product.
	pdf.SetXY(pgLeft, pgTop+8)
	pdf.SetFont("Helvetica", "B", 11)
	product := req.Product
	if req.ProductDesc != "" {
		product += " — " + req.ProductDesc
	}
	pdf.CellFormat(120, 5, r.tr(product), "", 1, "L", false, 0, "")

	// Order | user | first cut, values in bold.
	pdf.SetXY(pgLeft, pgTop+13.5)
	var bits [][2]string
	if o := req.Machine.Order; o != "" {
		bits = append(bits, [2]string{r.txt["order"], o})
	}
	if u := req.Machine.User; u != "" {
		bits = append(bits, [2]string{r.txt["user"], u})
	}
	if firstCut != "" {
		bits = append(bits, [2]string{r.txt["first"], firstCut})
	}
	for i, b := range bits {
		if i > 0 {
			pdf.SetFont("Helvetica", "", 8)
			pdf.Write(4, "  |  ")
		}
		pdf.SetFont("Helvetica", "", 8)
		pdf.Write(4, r.tr(b[0]))
		pdf.SetFont("Helvetica", "B", 8)
		pdf.Write(4, r.tr(b[1]))
	}

	// Right: badge, date, code.
	pdf.SetFont("Helvetica", "B", 8.5)
	bw := pdf.GetStringWidth(badge) + 4
	pdf.SetLineWidth(0.4)
	pdf.SetXY(pgRight-bw, pgTop+1)
	pdf.CellFormat(bw, 5, badge, "1", 0, "C", false, 0, "")
	r.rightLine(pgTop+9, r.txt["issued"], r.st.Created.Format("02/01/2006 15:04"))
	r.rightLine(pgTop+13.5, codeLabel, code)

	// The thick rule under the header.
	pdf.SetLineWidth(0.6)
	pdf.Line(pgLeft, pgTop+19, pgRight, pgTop+19)
	pdf.SetLineWidth(0.25)
	return pgTop + 21
}

// rightLine writes "label value" flush right, the value in bold.
func (r *report) rightLine(y float64, label, value string) {
	pdf := r.pdf
	label, value = r.tr(label), r.tr(value)
	pdf.SetFont("Helvetica", "B", 8)
	vw := pdf.GetStringWidth(value)
	pdf.SetFont("Helvetica", "", 8)
	lw := pdf.GetStringWidth(label)
	pdf.Text(pgRight-vw-lw, y+3, label)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.Text(pgRight-vw, y+3, value)
}

// kpi is one of the four boxes under the header.
type kpi struct{ label, value, sub string }

// kpis draws the four boxes and returns where they end.
func (r *report) kpis(y float64, boxes [4]kpi) float64 {
	pdf := r.pdf
	weights := [4]float64{2.3, 1, 1, 1.1}
	const gap, h = 2.0, 14.0
	unit := (pgWidth - 3*gap) / (weights[0] + weights[1] + weights[2] + weights[3])
	x := pgLeft
	for i, b := range boxes {
		w := weights[i] * unit
		pdf.SetLineWidth(0.25)
		pdf.RoundedRect(x, y, w, h, 0.8, "1234", "D")
		pdf.SetFont("Helvetica", "B", 6.5)
		pdf.Text(x+2.2, y+4, r.tr(b.label))
		pdf.SetFont("Helvetica", "B", 10)
		pdf.Text(x+2.2, y+8.6, fitText(pdf, r.tr(b.value), w-4))
		pdf.SetFont("Helvetica", "", 7.5)
		pdf.Text(x+2.2, y+12.2, fitText(pdf, r.tr(b.sub), w-4))
		x += w + gap
	}
	return y + h + 2.5
}

// fitText shortens s until it fits in w at the current font.
func fitText(pdf *fpdf.Fpdf, s string, w float64) string {
	for pdf.GetStringWidth(s) > w && len(s) > 1 {
		s = s[:len(s)-1]
	}
	return s
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

// drawnSize is the part as drawn: across the page (the sheet width) and
// down it (the sheet length).
func (row partRow) drawnSize() (across, down float64) {
	if row.rotated {
		return row.part.CutLength, row.part.CutWidth
	}
	return row.part.CutWidth, row.part.CutLength
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
		sheetArea := s.Length * s.Width / 1e6
		use := sheetsUse([]rect.SheetLayout{s}, mp.Parts)
		var cutLen float64
		for _, c := range s.Cuts {
			cutLen += math.Abs(c.To-c.From) / 1000
		}

		first := r.txt["first_l"]
		if s.FirstCut == rect.AlongWidth {
			first = r.txt["first_w"]
		}
		first += fmt.Sprintf(r.txt["stages"], job.Settings.MaxStages())
		unit := r.txt["sheet1"]
		if g.count > 1 {
			unit = r.txt["sheetN"]
		}
		codeLabel := r.txt["sheet_code"]
		if s.StockID != m.Code {
			codeLabel += fmt.Sprintf(r.txt["offcut_of"], s.StockID)
		}
		y := r.header(r.t("badge", gi+1, len(groups), g.count, unit), codeLabel, m.Code, first)

		material := strings.TrimSpace(m.Description)
		if m.Thickness > 0 {
			material += " " + fmtMM(m.Thickness) + "MM"
		}
		y = r.kpis(y, [4]kpi{
			{r.txt["k_material"], material, fmt.Sprintf(r.txt["sheet_size"], fmtMM(s.Width), fmtMM(s.Length), fmtM(sheetArea))},
			{r.txt["k_use"], fmtPct(use*100) + "%", fmt.Sprintf(r.txt["parts_area"], len(s.Placements), fmtM(use*sheetArea))},
			{r.txt["k_loss"], fmtPct(100-use*100) + "%", fmtM((1-use)*sheetArea) + " m²"},
			{r.txt["k_saw"], fmt.Sprintf(r.txt["cuts"], len(s.Cuts)), fmt.Sprintf(r.txt["path"], fmtM(cutLen))},
		})

		// The drawing takes what the table leaves; a long table goes on
		// to the next page.
		tableH := 3 + rowH + 0.6 + float64(len(rows))*rowH + 1
		drawH := pgFooter - 2 - tableH - y
		if drawH < 110 {
			drawH = 110
		}
		r.drawSheet(s, parts, rowOf, y, drawH)
		r.partTable(rows, y+drawH+3)
	}
	return len(groups)
}

// drawSheet draws a layout inside a framed box, with the sheet length
// running down the page: page x is the sheet Y, page y is the sheet X.
func (r *report) drawSheet(s rect.SheetLayout, parts map[string]JobPart, rowOf map[string]int, y0, h float64) {
	pdf := r.pdf
	pdf.SetLineWidth(0.25)
	pdf.Rect(pgLeft, y0, pgWidth, h, "D")
	const top, left, pad = 7.0, 9.0, 3.0 // room for the overall sizes
	scale := math.Min((pgWidth-left-2*pad)/s.Width, (h-top-2*pad)/s.Length)
	if scale <= 0 {
		return
	}
	sw, sh := s.Width*scale, s.Length*scale
	ox := pgLeft + left + (pgWidth-left-sw)/2
	oy := y0 + top + pad
	px := func(y float64) float64 { return ox + y*scale }
	py := func(x float64) float64 { return oy + x*scale }

	// Overall sizes: width above, length turned on the left.
	pdf.SetFont("Helvetica", "B", 8)
	wTxt := fmtMM(s.Width) + " mm"
	pdf.Text(ox+(sw-pdf.GetStringWidth(wTxt))/2, oy-2, wTxt)
	lTxt := fmtMM(s.Length) + " mm"
	lx, ly := ox-3, oy+(sh+pdf.GetStringWidth(lTxt))/2
	pdf.TransformBegin()
	pdf.TransformRotate(90, lx, ly)
	pdf.Text(lx, ly, lTxt)
	pdf.TransformEnd()

	// Offcuts and scrap: dotted, dashed outline; offcuts named and sized.
	n := 0
	for _, o := range s.Offcuts {
		x, y, w, hh := px(o.Y), py(o.X), o.Width*scale, o.Length*scale
		r.dotted(x, y, w, hh)
		if o.Reusable {
			n++
			r.centredLabels(x, y, w, hh, []textLine{{r.t("offcut", n), "B", 7}, {r.tr(fmtMM(o.Width) + " × " + fmtMM(o.Length)), "", 6.5}}, true)
		}
	}

	// Parts: white with a black outline, number in a small box at the
	// corner, code in bold and the size in the middle.
	for _, p := range s.Placements {
		jp := parts[p.PartID]
		down, across := jp.CutLength, jp.CutWidth
		if p.Rotated {
			down, across = across, down
		}
		x, y, w, hh := px(p.Y), py(p.X), across*scale, down*scale
		pdf.SetFillColor(255, 255, 255)
		pdf.SetDrawColor(0, 0, 0)
		pdf.SetLineWidth(0.25)
		pdf.Rect(x, y, w, hh, "FD")
		r.partLabels(x, y, w, hh, rowOf[p.PartID]+1, r.tr(firstCode(jp)), fmtMM(across)+" × "+fmtMM(down))
	}
	pdf.SetLineWidth(0.5)
	pdf.Rect(ox, oy, sw, sh, "D")
	pdf.SetLineWidth(0.25)
}

// partLabels fits the number, code and size into a part. A roomy part gets
// the number boxed at its corner and two centred lines; a thin one gets
// everything on one line; a tiny one only its number.
func (r *report) partLabels(x, y, w, h float64, n int, code, size string) {
	pdf := r.pdf
	size = r.tr(size)
	num := fmt.Sprint(n)
	badge := func(bx, by, fs float64) float64 {
		pdf.SetFont("Helvetica", "B", fs)
		bw := pdf.GetStringWidth(num) + 1.4
		bh := fs * 0.42
		pdf.SetFillColor(255, 255, 255)
		pdf.SetLineWidth(0.2)
		pdf.Rect(bx, by, bw, bh, "FD")
		pdf.Text(bx+0.7, by+bh*0.78, num)
		return bw
	}
	for _, fs := range []float64{8.5, 7.5, 6.5, 5.5} {
		lineH := fs * 0.42
		pdf.SetFont("Helvetica", "B", fs)
		cw := pdf.GetStringWidth(code)
		pdf.SetFont("Helvetica", "", fs-1)
		zw := pdf.GetStringWidth(size)
		// Two lines under the corner badge.
		if h >= 2*lineH+fs*0.42+1.5 && math.Max(cw, zw) < w-1.5 {
			badge(x+0.4, y+0.4, fs-1.5)
			mid := y + h/2 + 0.6
			pdf.SetFont("Helvetica", "B", fs)
			pdf.Text(x+(w-cw)/2, mid, code)
			pdf.SetFont("Helvetica", "", fs-1)
			pdf.Text(x+(w-zw)/2, mid+lineH, size)
			return
		}
		// One line: badge, code, (size).
		one := "(" + size + ")"
		pdf.SetFont("Helvetica", "", fs-1)
		ow := pdf.GetStringWidth(one)
		pdf.SetFont("Helvetica", "B", fs-1.5)
		nw := pdf.GetStringWidth(num) + 1.4
		if h >= lineH+0.6 && nw+cw+ow+4 < w {
			start := x + (w-(nw+cw+ow+3))/2
			base := y + h/2 + lineH*0.35
			badge(start, base-lineH*0.8, fs-1.5)
			pdf.SetFont("Helvetica", "B", fs)
			pdf.Text(start+nw+1.5, base, code)
			pdf.SetFont("Helvetica", "", fs-1)
			pdf.Text(start+nw+cw+3, base, one)
			return
		}
	}
	// Only the number, if even that fits.
	pdf.SetFont("Helvetica", "B", 5)
	if nw := pdf.GetStringWidth(num) + 1.4; nw < w && h > 2.4 {
		badge(x+(w-nw)/2, y+(h-2.1)/2, 5)
	}
}

type textLine struct {
	text  string
	style string
	size  float64
}

// centredLabels writes lines in the middle of a box, each on a white patch
// so it reads over the dots. Lines that do not fit are left out.
func (r *report) centredLabels(x, y, w, h float64, lines []textLine, patch bool) {
	pdf := r.pdf
	total := 0.0
	for _, l := range lines {
		total += l.size * 0.42
	}
	if total > h-1 {
		lines = lines[:1]
		total = lines[0].size * 0.42
		if total > h-1 {
			return
		}
	}
	cy := y + (h-total)/2
	for _, l := range lines {
		lh := l.size * 0.42
		pdf.SetFont("Helvetica", l.style, l.size)
		tw := pdf.GetStringWidth(l.text)
		if tw > w-1 {
			return
		}
		if patch {
			pdf.SetFillColor(255, 255, 255)
			pdf.Rect(x+(w-tw)/2-0.6, cy, tw+1.2, lh, "F")
		}
		pdf.Text(x+(w-tw)/2, cy+lh*0.8, l.text)
		cy += lh
	}
}

// dotted fills a rectangle with a grid of small dots and a dashed outline:
// what is not a part.
func (r *report) dotted(x, y, w, h float64) {
	pdf := r.pdf
	if w <= 0 || h <= 0 {
		return
	}
	const step = 1.3
	pdf.SetFillColor(85, 85, 85)
	pdf.ClipRect(x, y, w, h, false)
	for dy := step / 2; dy < h; dy += step {
		for dx := step / 2; dx < w; dx += step {
			pdf.Circle(x+dx, y+dy, 0.17, "F")
		}
	}
	pdf.ClipEnd()
	pdf.SetDrawColor(0, 0, 0)
	pdf.SetLineWidth(0.2)
	pdf.SetDashPattern([]float64{0.8, 0.6}, 0)
	pdf.Rect(x, y, w, h, "D")
	pdf.SetDashPattern([]float64{}, 0)
	pdf.SetFillColor(255, 255, 255)
}

func firstCode(p JobPart) string {
	if len(p.Codes) > 0 && p.Codes[0] != "" {
		return p.Codes[0]
	}
	return p.ID
}

// tableCol is one column of a part list.
type tableCol struct {
	key   string
	w     float64
	align string
	bold  bool
}

// table draws a part list: a header between two rules, rows with a thin
// grey line under each. It goes on to a new page when it runs out of room,
// and repeats its header there.
func (r *report) table(cols []tableCol, rows [][]string, y float64) float64 {
	pdf := r.pdf
	head := func(y float64) float64 {
		pdf.SetLineWidth(0.4)
		pdf.SetDrawColor(0, 0, 0)
		pdf.Line(pgLeft, y, pgRight, y)
		pdf.SetXY(pgLeft, y)
		pdf.SetFont("Helvetica", "B", 7.5)
		for _, c := range cols {
			pdf.CellFormat(c.w, rowH+0.6, r.t(c.key), "", 0, c.align, false, 0, "")
		}
		y += rowH + 0.6
		pdf.Line(pgLeft, y, pgRight, y)
		pdf.SetLineWidth(0.25)
		return y
	}
	y = head(y)
	for _, row := range rows {
		if y+rowH > pgFooter-2 {
			pdf.AddPage()
			y = head(pgTop + 2)
		}
		pdf.SetXY(pgLeft, y)
		for k, c := range cols {
			style := ""
			if c.bold {
				style = "B"
			}
			pdf.SetFont("Helvetica", style, 8)
			pdf.CellFormat(c.w, rowH, fitText(pdf, r.tr(row[k]), c.w-1.5), "", 0, c.align, false, 0, "")
		}
		y += rowH
		pdf.SetDrawColor(153, 153, 153)
		pdf.SetLineWidth(0.2)
		pdf.Line(pgLeft, y, pgRight, y)
		pdf.SetDrawColor(0, 0, 0)
	}
	return y
}

// partTable is the list under the drawing: item, tag, part code,
// description, project, size as drawn, quantity on this layout.
func (r *report) partTable(rows []partRow, y float64) {
	cols := []tableCol{{"h_item", 13, "C", true}, {"h_leg", 12, "C", false}, {"h_code", 34, "L", true},
		{"h_desc", 58, "L", false}, {"h_proj", 28, "L", false}, {"h_size", 31, "R", false}, {"h_qty", 14, "C", true}}
	var cells [][]string
	for _, row := range rows {
		across, down := row.drawnSize()
		cells = append(cells, []string{fmt.Sprint(row.n), legend(row.n - 1), firstCode(row.part), row.part.Desc,
			r.st.Request.Product, fmtMM(across) + " × " + fmtMM(down) + " mm", fmt.Sprint(row.qty)})
	}
	r.table(cols, cells, y)
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

// barPage lists the bars of one material: the same header and boxes, then
// each bar layout drawn to scale, then the part list.
func (r *report) barPage(mp MaterialPlan, job linear.Job, res linear.Result) {
	pdf := r.pdf
	m := mp.Material
	pdf.AddPage()

	var stockLen, partsLen float64
	cuts := 0
	for _, b := range res.Bars {
		stockLen += b.Length
		cuts += len(b.Cuts)
		for _, c := range b.Cuts {
			partsLen += c.Length
		}
	}
	use := barsUse(res.Bars)
	unit := r.txt["bar1"]
	if len(res.Bars) > 1 {
		unit = r.txt["barN"]
	}
	y := r.header(r.t("bars_badge", len(res.Bars), unit), r.txt["bar_code"], m.Code, "")
	material := strings.TrimSpace(m.Description)
	if m.Thickness > 0 {
		material += " " + fmtMM(m.Thickness) + "MM"
	}
	pieces := 0
	for _, p := range mp.Parts {
		pieces += p.Qty
	}
	y = r.kpis(y, [4]kpi{
		{r.txt["k_material"], material, fmt.Sprintf(r.txt["bar_size"], fmtMM(m.Length))},
		{r.txt["k_use"], fmtPct(use*100) + "%", fmt.Sprintf(r.txt["parts_len"], pieces, fmtM(partsLen/1000))},
		{r.txt["k_loss"], fmtPct(100-use*100) + "%", fmtM((stockLen-partsLen)/1000) + " m"},
		{r.txt["k_saw"], fmt.Sprintf(r.txt["cuts"], cuts), ""},
	})

	// Equal bars together.
	type group struct {
		bar   linear.Bar
		count int
	}
	var groups []group
	index := map[string]int{}
	longest := 0.0
	for _, b := range res.Bars {
		longest = math.Max(longest, b.Length)
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
	partOf := map[string]int{}
	for i, p := range mp.Parts {
		partOf[p.ID] = i + 1
	}

	const barH = 9.0
	scale := (pgWidth - 4) / longest
	n := 0
	for _, g := range groups {
		if y+barH+9 > pgFooter-10 {
			pdf.AddPage()
			y = pgTop + 2
		}
		from := n + 1
		n += g.count
		name := fmt.Sprintf(r.txt["bar_n"], from)
		if g.count > 1 {
			name = fmt.Sprintf(r.txt["bars_n"], from, n)
		}
		pdf.SetFont("Helvetica", "B", 8.5)
		pdf.Text(pgLeft, y+3.5, r.tr(name))
		pdf.SetFont("Helvetica", "", 8)
		pdf.Text(pgLeft+pdf.GetStringWidth(r.tr(name))+3, y+3.5, r.tr(fmt.Sprintf("•  %s  •  %s mm", g.bar.StockID, fmtMM(g.bar.Length))))
		y += 5
		x0 := pgLeft + 2
		if s := job.Settings.StartLoss; s > 0 {
			r.dotted(x0, y, s*scale, barH)
		}
		if e := job.Settings.EndLoss; e > 0 {
			r.dotted(x0+(g.bar.Length-e)*scale, y, e*scale, barH)
		}
		for _, c := range g.bar.Cuts {
			x := x0 + c.Start*scale
			w := c.Length * scale
			pdf.SetFillColor(255, 255, 255)
			pdf.SetLineWidth(0.25)
			pdf.Rect(x, y, w, barH, "FD")
			r.partLabels(x, y, w, barH, partOf[c.PartID], r.tr(firstCodeOf(mp.Parts, c.PartID)), fmtMM(c.Length))
		}
		if g.bar.Remainder > 0 {
			end := g.bar.Length - job.Settings.EndLoss
			x, w := x0+(end-g.bar.Remainder)*scale, g.bar.Remainder*scale
			r.dotted(x, y, w, barH)
			name := r.txt["scrap"]
			if g.bar.Reusable {
				name = r.t("offcut", from)
			}
			r.centredLabels(x, y, w, barH, []textLine{{r.tr(name), "B", 7}, {fmtMM(g.bar.Remainder), "", 6.5}}, true)
		}
		pdf.SetLineWidth(0.5)
		pdf.Rect(x0, y, g.bar.Length*scale, barH, "D")
		pdf.SetLineWidth(0.25)
		y += barH + 4
	}

	// Part list, in the numbering of the drawings.
	cols := []tableCol{{"h_item", 13, "C", true}, {"h_code", 40, "L", true}, {"h_desc", 72, "L", false},
		{"h_proj", 30, "L", false}, {"h_len", 21, "R", false}, {"h_qty", 14, "C", true}}
	var cells [][]string
	for i, p := range mp.Parts {
		cells = append(cells, []string{fmt.Sprint(i + 1), firstCode(p), p.Desc, r.st.Request.Product, fmtMM(p.CutLength) + " mm", fmt.Sprint(p.Qty)})
	}
	y = r.table(cols, cells, y+1)
	if len(res.Unplaced) > 0 {
		var u []string
		for _, x := range res.Unplaced {
			u = append(u, fmt.Sprintf("%s x%d", x.PartID, x.Quantity))
		}
		pdf.SetFont("Helvetica", "B", 8.5)
		pdf.Text(pgLeft, y+5, r.t("unplaced", strings.Join(u, ", ")))
	}
}

func firstCodeOf(parts []JobPart, id string) string {
	for _, p := range parts {
		if p.ID == id {
			return firstCode(p)
		}
	}
	return id
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
// 0..1: the "use" printed on the report. The planner's own Utilization
// leaves the edge trim out, so it reads a little higher.
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
