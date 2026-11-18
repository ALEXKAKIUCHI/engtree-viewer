// Package giben writes a rectangular cut plan as the two files a Giben
// panel saw (GVision) loads:
//
//   - .AC, the cuts: for each sheet layout, the tree of edge-to-edge cuts,
//     one line per cut, stage by stage;
//   - .AD, the parts and labels: for each layout, the parts in the order
//     the .AC cuts them, and the list of layouts.
//
// Both files are plain text in fixed columns, Windows line ends, Latin-1.
// Give them the same name: only the extension changes.
//
// The layout was worked out from real files the saw loads. A few
// columns never changed in those files and their meaning is not known;
// they are written with the same values (see the constants below).
package giben

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/cutplan/rect"
)

// Columns that were the same in every reference file. Their
// meaning is not known, so they are copied as they are.
const (
	sheetFlag   = "0"      // .AC sheet line, last column
	unitFlag    = "1"      // .AD part and layout lines, after the quantity
	partFlags   = " 0 0"   // .AD part line, after unitFlag
	labelPrefix = "02.732" // .AD part line, just before the label
	labelFlag   = "0"      // .AD part line, after the label
	layoutFlag  = "0"      // .AD layout line, column 125
)

// Widths of the text fields, in characters.
const (
	materialWidth    = 15
	projectWidth     = 9
	descriptionWidth = 25
	labelWidth       = 26
	layoutMatWidth   = 9
	adLineWidth      = 490
)

// tol is how far two lengths may differ and still be the same, in mm.
// The files hold tenths of a millimetre.
const tol = 0.05

// Export is what the files say besides the cuts.
type Export struct {
	Material    string    // the first 15 characters are written
	Thickness   float64   // mm
	ProjectCode string    // up to 9 characters, on the labels
	Description string    // up to 25 characters, on the labels
	Date        time.Time // on the labels
	// Parts gives each part of the job its code on the saw (1 to 99) and
	// the text on its label. A part left out gets its place in job.Parts
	// (1, 2, ...) as its code and its ID as its label. Codes must be
	// different from one another.
	Parts map[string]PartInfo
}

type PartInfo struct {
	Code  int
	Label string
}

// Write turns the plan for one material into the .AC and .AD files. job
// is the job the plan was made from: its parts give the sizes and its
// settings the kerf and the edge trim.
func Write(job rect.Job, res rect.Result, e Export) (ac, ad []byte, err error) {
	parts, err := partRows(job, e)
	if err != nil {
		return nil, nil, err
	}
	var plans []plan
	for i, sh := range res.Sheets {
		p, err := sheetPlan(sh, job.Settings, parts)
		if err != nil {
			return nil, nil, fmt.Errorf("giben: sheet %d: %w", i+1, err)
		}
		if n := len(plans); n > 0 && plans[n-1].sameAs(p) {
			plans[n-1].count++
			continue
		}
		plans = append(plans, p)
	}
	if len(plans) == 0 {
		return nil, nil, errors.New("giben: the plan has no sheets")
	}
	return format(plans, e)
}

// plan is one sheet layout, cut on count sheets.
type plan struct {
	count         int
	width, length float64 // the sheet, width first as on the saw
	lines         []line
}

func (p plan) sameAs(q plan) bool {
	if math.Abs(p.width-q.width) > tol || math.Abs(p.length-q.length) > tol || len(p.lines) != len(q.lines) {
		return false
	}
	for i := range p.lines {
		a, b := p.lines[i], q.lines[i]
		if a.level != b.level || a.code != b.code || a.repeat != b.repeat ||
			tenths(a.measure) != tenths(b.measure) || (a.part == nil) != (b.part == nil) ||
			(a.part != nil && a.part.code != b.part.code) {
			return false
		}
	}
	return true
}

// line is one cut of the .AC, repeated repeat times. part is set when the
// cut gives a part, which then has a line in the .AD too.
type line struct {
	level   int // 2 and 4: measured across the sheet width; 3 and 5: along its length
	code    int // 0 for a piece that is cut further, and see sheetPlan
	measure float64
	repeat  int
	part    *partRow
}

type partRow struct {
	code          int
	width, length float64
	label         string
}

func partRows(job rect.Job, e Export) (map[string]*partRow, error) {
	rows := map[string]*partRow{}
	used := map[int]string{}
	for i, p := range job.Parts {
		r := &partRow{code: i + 1, width: p.Width, length: p.Length, label: p.ID}
		if info, ok := e.Parts[p.ID]; ok {
			r.code = info.Code
			if info.Label != "" {
				r.label = info.Label
			}
		}
		if r.code < 1 || r.code > 99 {
			return nil, fmt.Errorf("giben: part %q: the code must be between 1 and 99, not %d", p.ID, r.code)
		}
		if other, ok := used[r.code]; ok {
			return nil, fmt.Errorf("giben: parts %q and %q have the same code %d", other, p.ID, r.code)
		}
		used[r.code] = p.ID
		rows[p.ID] = r
	}
	return rows, nil
}

// box is a placed part in sheet coordinates: X along the sheet length, Y
// along its width.
type box struct {
	x0, x1, y0, y1 float64
	part           *partRow
}

// sheetPlan reads the tree of cuts out of a sheet layout.
//
// The files number the levels by direction: level 2 cuts are measured
// across the sheet width, level 3 along its length, and so on. A sheet
// whose first cuts are along the width starts at level 3.
//
// One rule copied from its files: at the third level from the top, the
// first part of each group is written with code 00 and on a line of its
// own, even when the next parts are the same. The .AD still names it.
func sheetPlan(sh rect.SheetLayout, set rect.Settings, parts map[string]*partRow) (plan, error) {
	p := plan{count: 1, width: sh.Width, length: sh.Length}
	var boxes []box
	for _, pl := range sh.Placements {
		r, ok := parts[pl.PartID]
		if !ok {
			return p, fmt.Errorf("part %q is not in the job", pl.PartID)
		}
		lx, ly := r.length, r.width
		if pl.Rotated {
			lx, ly = ly, lx
		}
		boxes = append(boxes, box{pl.X, pl.X + lx, pl.Y, pl.Y + ly, r})
	}
	if len(boxes) == 0 {
		return p, errors.New("no parts on the sheet")
	}
	first := 2
	if sh.FirstCut == rect.AlongWidth {
		first = 3
	}
	t := set.EdgeTrim
	s := &splitter{cuts: sh.Cuts, kerf: set.Kerf, first: first}
	r := region{t, sh.Length - t, t, sh.Width - t}
	if err := s.split(r, 1, boxes); err != nil {
		return p, err
	}
	p.lines = s.lines
	return p, nil
}

type region struct{ x0, x1, y0, y1 float64 }

type splitter struct {
	cuts  []rect.Cut
	kerf  float64
	first int
	lines []line
}

// split cuts region r at the given stage and writes the pieces, then the
// pieces inside them.
func (s *splitter) split(r region, stage int, boxes []box) error {
	level := s.first + stage - 1
	if level > 9 {
		return errors.New("more than 9 levels of cuts")
	}
	// Even levels are cuts along the sheet length, measured along Y.
	alongY := level%2 == 0
	along := rect.AlongWidth
	lo, hi, olo, ohi := r.x0, r.x1, r.y0, r.y1
	if alongY {
		along = rect.AlongLength
		lo, hi, olo, ohi = r.y0, r.y1, r.x0, r.x1
	}
	span := func(b box) (float64, float64) {
		if alongY {
			return b.y0, b.y1
		}
		return b.x0, b.x1
	}
	var pos []float64
	for _, c := range s.cuts {
		if c.Stage != stage || c.Along != along || c.Pos <= lo+tol || c.Pos >= hi-tol ||
			c.From >= ohi-tol || c.To <= olo+tol {
			continue
		}
		if c.From > olo+tol || c.To < ohi-tol {
			return fmt.Errorf("a stage %d cut at %.1f does not go from edge to edge", stage, c.Pos)
		}
		pos = append(pos, c.Pos)
	}
	sort.Float64s(pos)
	if len(pos) == 0 && stage > 1 {
		return errors.New("a part does not fill its piece and no cut trims it")
	}
	type seg struct {
		lo, hi float64
		boxes  []box
	}
	segs := []seg{}
	start := lo
	for _, p := range pos {
		if len(segs) > 0 && p < start-tol {
			continue // the same cut listed twice
		}
		segs = append(segs, seg{lo: start, hi: p})
		start = p + s.kerf
	}
	segs = append(segs, seg{lo: start, hi: -1}) // up to what it holds
	for _, b := range boxes {
		b0, b1 := span(b)
		in := false
		for i := range segs {
			sg := &segs[i]
			if b0 >= sg.lo-tol && (sg.hi < 0 || b1 <= sg.hi+tol) {
				sg.boxes = append(sg.boxes, b)
				in = true
				break
			}
		}
		if !in {
			return fmt.Errorf("a part at %.1f, %.1f crosses a cut", b.x0, b.y0)
		}
	}
	waste := false
	merge := -1 // index in s.lines of the last part line that may take repeats
	for i, sg := range segs {
		if len(sg.boxes) == 0 {
			waste = true
			continue
		}
		if waste {
			// The saw measures each cut from the one before; leftover
			// in between would need a cut of its own.
			return errors.New("leftover between parts: the leftover must come last")
		}
		if sg.hi < 0 {
			for _, b := range sg.boxes {
				_, b1 := span(b)
				sg.hi = math.Max(sg.hi, b1)
			}
		}
		m := sg.hi - sg.lo
		if b := sg.boxes[0]; len(sg.boxes) == 1 {
			b0, b1 := span(b)
			o0, o1 := b.x0, b.x1
			if !alongY {
				o0, o1 = b.y0, b.y1
			}
			if math.Abs(b1-b0-m) < tol && math.Abs(o1-o0-(ohi-olo)) < tol {
				code := b.part.code
				special := stage == 3 && i == 0
				if special {
					code = 0
				}
				if merge >= 0 && !special {
					last := &s.lines[len(s.lines)-1]
					if last.code == code && tenths(last.measure) == tenths(m) && last.repeat < 99 {
						last.repeat++
						continue
					}
				}
				s.lines = append(s.lines, line{level: level, code: code, measure: m, repeat: 1, part: b.part})
				if special {
					merge = -1
				} else {
					merge = len(s.lines) - 1
				}
				continue
			}
		}
		merge = -1
		s.lines = append(s.lines, line{level: level, measure: m, repeat: 1})
		sub := region{sg.lo, sg.hi, r.y0, r.y1}
		if alongY {
			sub = region{r.x0, r.x1, sg.lo, sg.hi}
		}
		if err := s.split(sub, stage+1, sg.boxes); err != nil {
			return err
		}
	}
	return nil
}

// format writes the two files.
func format(plans []plan, e Export) (ac, ad []byte, err error) {
	if len(plans) > 99 {
		return nil, nil, fmt.Errorf("giben: %d sheet layouts, at most 99 fit in one file", len(plans))
	}
	mat := text(e.Material, materialWidth)
	th := tenths(e.Thickness)
	if th < 0 || th > 999 {
		return nil, nil, fmt.Errorf("giben: thickness %.1f mm does not fit", e.Thickness)
	}
	acHead := mat + fmt.Sprintf("%3d", th)
	adHead := mat + fmt.Sprintf("%04d", th)
	var acb, adb bytes.Buffer
	var bad error
	num := func(x float64, digits int, what string) string {
		n := tenths(x)
		if n < 0 || n >= int(math.Pow10(digits)) {
			bad = fmt.Errorf("giben: %s %.1f does not fit in the file", what, x)
		}
		return fmt.Sprintf("%0*d", digits, n)
	}
	count := func(n, digits int, what string) string {
		if n < 0 || n >= int(math.Pow10(digits)) {
			bad = fmt.Errorf("giben: %s %d does not fit in the file", what, n)
		}
		return fmt.Sprintf("%0*d", digits, n)
	}
	crlf := func(b *bytes.Buffer, s string) {
		b.WriteString(s)
		b.WriteString("\r\n")
	}
	// The list of layouts, repeated in every .AD block.
	var layouts []string
	for i, p := range plans {
		layouts = append(layouts, pad(pad(pad(adHead, 28)+"4"+fmt.Sprintf("%4d", i+1)+
			num(p.width, 6, "sheet width")+num(p.length, 6, "sheet length")+
			count(p.count, 5, "number of sheets")+unitFlag, 60)+
			text(e.Material, layoutMatWidth), 125)+layoutFlag)
	}
	header := pad(adHead+text(e.ProjectCode, projectWidth)+"1", 37) +
		text(e.Description, descriptionWidth) + date(e.Date)
	for i, p := range plans {
		plan := fmt.Sprintf("%02d", i+1)
		crlf(&acb, acHead+plan+"1"+count(p.count, 5, "number of sheets")+
			num(p.width, 5, "sheet width")+num(p.length, 5, "sheet length")+sheetFlag)
		crlf(&adb, pad(header, adLineWidth))
		for _, l := range p.lines {
			if l.level < 1 || l.level > 9 {
				return nil, nil, fmt.Errorf("giben: cut level %d", l.level)
			}
			crlf(&acb, acHead+plan+fmt.Sprint(l.level)+count(l.code, 2, "part code")+
				num(l.measure, 5, "cut")+count(l.repeat, 2, "repeat"))
			if r := l.part; r != nil {
				crlf(&adb, pad(pad(pad(adHead, 28)+"2"+fmt.Sprintf("%4d", r.code)+
					num(r.width, 6, "part width")+num(r.length, 6, "part length")+
					count(l.repeat, 5, "quantity")+unitFlag+partFlags, 124)+
					labelPrefix+text(r.label, labelWidth)+labelFlag, 449)+
					size(r.width)+size(r.length)+strings.Repeat(" ", adLineWidth-461))
			}
		}
		for _, s := range layouts {
			crlf(&adb, pad(s, adLineWidth))
		}
	}
	if bad != nil {
		return nil, nil, bad
	}
	return acb.Bytes(), adb.Bytes(), nil
}

func tenths(x float64) int { return int(math.Round(x * 10)) }

// size is a length as 0200.0, as on the .AD part lines.
func size(x float64) string { return fmt.Sprintf("%06.1f", float64(tenths(x))/10) }

func date(t time.Time) string {
	return fmt.Sprintf("%04d%s%02d", t.Year(), strings.ToLower(t.Month().String()[:3]), t.Day())
}

// text is s in Latin-1, cut or padded to n characters. Characters
// outside Latin-1 become '?'.
func text(s string, n int) string {
	b := make([]byte, 0, n)
	for _, r := range s {
		if len(b) == n {
			break
		}
		if r > 0xff || r < 0x20 {
			r = '?'
		}
		b = append(b, byte(r))
	}
	for len(b) < n {
		b = append(b, ' ')
	}
	return string(b)
}

// pad fills s with spaces up to n bytes.
func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
