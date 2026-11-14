// Package dxfgeom reads a DXF drawing and gives back flat geometry in
// millimetres: line segments to draw, and closed contours (an outline plus
// its holes) to cut or to nest.
//
// It reads the ASCII DXF that CAD programs export for cutting: LINE, ARC,
// CIRCLE, LWPOLYLINE (bulges included), POLYLINE, ELLIPSE, SPLINE and
// INSERT (blocks, with their scale and rotation). Text, dimensions and
// hatches are skipped, and so are any layers the caller names.
package dxfgeom

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/geom"
)

// Options tune how a drawing is read. The zero value is sensible.
type Options struct {
	// Tolerance is how far apart two ends may be and still count as the
	// same point when contours are joined (mm). Default 0.01.
	Tolerance float64
	// Sagitta is how far a flattened curve may sit from the true arc (mm).
	// Smaller means more segments. Default 0.05.
	Sagitta float64
	// IgnoreLayers are layer names to skip entirely (dimensions, engraving).
	// Case and surrounding spaces do not matter.
	IgnoreLayers []string
	// Units forces the drawing's unit instead of reading $INSUNITS:
	// "mm", "cm", "m", "in" or "ft".
	Units string
}

func (o Options) tolerance() float64 {
	if o.Tolerance > 0 {
		return o.Tolerance
	}
	return 0.01
}

func (o Options) sagitta() float64 {
	if o.Sagitta > 0 {
		return o.Sagitta
	}
	return 0.05
}

// Segment is a straight piece of the drawing, after curves are flattened.
type Segment struct {
	A     geom.Point `json:"a"`
	B     geom.Point `json:"b"`
	Layer string     `json:"layer,omitempty"`
}

// Drawing is everything useful that came out of the file.
type Drawing struct {
	Segments []Segment `json:"segments"`
	Bounds   geom.Rect `json:"bounds"`
	Units    string    `json:"units"`              // what the coordinates were converted from
	Warnings []string  `json:"warnings,omitempty"` // things the user should know
	Skipped  []string  `json:"skipped,omitempty"`  // entity types left out, with counts
}

// ---- group codes ----

type pair struct {
	code int
	val  string
}

type entity struct {
	typ   string
	pairs []pair
	// filled in for POLYLINE, which carries its vertices as separate entities
	verts []vertex
}

type vertex struct {
	p     geom.Point
	bulge float64
}

func (e entity) first(code int) (string, bool) {
	for _, p := range e.pairs {
		if p.code == code {
			return p.val, true
		}
	}
	return "", false
}

func (e entity) num(code int, def float64) float64 {
	if v, ok := e.first(code); ok {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return def
}

func (e entity) integer(code int, def int) int {
	if v, ok := e.first(code); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func (e entity) layer() string {
	v, _ := e.first(8)
	return v
}

// floats returns every value of one code, in file order.
func (e entity) floats(code int) []float64 {
	var out []float64
	for _, p := range e.pairs {
		if p.code == code {
			if f, err := strconv.ParseFloat(strings.TrimSpace(p.val), 64); err == nil {
				out = append(out, f)
			}
		}
	}
	return out
}

// ---- file reading ----

// ReadFile reads a DXF file from disk.
func ReadFile(path string, opt Options) (*Drawing, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f, opt)
}

// Read reads a DXF drawing from r.
func Read(r io.Reader, opt Options) (*Drawing, error) {
	pairs, err := readPairs(r)
	if err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return nil, fmt.Errorf("dxfgeom: empty file")
	}

	insunits := 0
	blocks := map[string][]entity{}
	blockBase := map[string]geom.Point{}
	var entities []entity

	section := ""
	curBlock := ""
	var cur *entity
	flush := func() {
		if cur == nil {
			return
		}
		switch {
		case section == "BLOCKS" && curBlock != "":
			blocks[curBlock] = append(blocks[curBlock], *cur)
		case section == "ENTITIES":
			entities = append(entities, *cur)
		}
		cur = nil
	}

	for i := 0; i < len(pairs); i++ {
		p := pairs[i]

		// Header variables arrive as code 9 with the name, then the value.
		if section == "HEADER" && p.code == 9 && strings.EqualFold(p.val, "$INSUNITS") && i+1 < len(pairs) {
			insunits, _ = strconv.Atoi(strings.TrimSpace(pairs[i+1].val))
			continue
		}
		if p.code != 0 {
			if cur != nil {
				cur.pairs = append(cur.pairs, p)
			}
			continue
		}

		switch p.val {
		case "SECTION":
			flush()
			section = ""
			if i+1 < len(pairs) && pairs[i+1].code == 2 {
				section = strings.ToUpper(strings.TrimSpace(pairs[i+1].val))
				i++
			}
		case "ENDSEC":
			flush()
			section, curBlock = "", ""
		case "BLOCK":
			flush()
			curBlock = ""
			cur = &entity{typ: "BLOCK"}
		case "ENDBLK":
			flush()
			curBlock = ""
		case "EOF":
			flush()
			i = len(pairs)
		default:
			// A BLOCK header ends when its first entity starts: its name
			// (code 2) and base point are known by then.
			if cur != nil && cur.typ == "BLOCK" {
				name, _ := cur.first(2)
				curBlock = strings.TrimSpace(name)
				blockBase[curBlock] = geom.Point{X: cur.num(10, 0), Y: cur.num(20, 0)}
				if _, ok := blocks[curBlock]; !ok {
					blocks[curBlock] = nil
				}
				cur = nil
			}
			flush()
			cur = &entity{typ: strings.ToUpper(strings.TrimSpace(p.val))}
		}
	}
	flush()

	entities = groupPolylines(entities)
	for name, list := range blocks {
		blocks[name] = groupPolylines(list)
	}

	d := &Drawing{Bounds: geom.EmptyRect()}
	scale, unitName := unitScale(insunits, opt.Units)
	d.Units = unitName
	if unitName == "unknown" {
		d.Units = "mm"
		d.Warnings = append(d.Warnings, "the drawing does not say its unit ($INSUNITS); millimetres assumed")
	}

	st := &flattener{
		opt:     opt,
		blocks:  blocks,
		base:    blockBase,
		skipped: map[string]int{},
	}
	st.ignore = map[string]bool{}
	for _, l := range opt.IgnoreLayers {
		st.ignore[strings.ToLower(strings.TrimSpace(l))] = true
	}
	st.run(entities, transform{sx: scale, sy: scale}, 0)

	d.Segments = st.segments
	for _, s := range d.Segments {
		d.Bounds.Add(s.A)
		d.Bounds.Add(s.B)
	}
	if len(d.Segments) == 0 {
		d.Bounds = geom.Rect{}
		d.Warnings = append(d.Warnings, "no geometry found in the file")
	}
	if names := sortedCounts(st.skipped); len(names) > 0 {
		d.Skipped = names
	}
	d.Warnings = append(d.Warnings, st.warnings...)
	return d, nil
}

func readPairs(r io.Reader) ([]pair, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	var lines []string
	for sc.Scan() {
		lines = append(lines, strings.TrimRight(sc.Text(), "\r"))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	out := make([]pair, 0, len(lines)/2)
	for i := 0; i+1 < len(lines); i += 2 {
		code, err := strconv.Atoi(strings.TrimSpace(lines[i]))
		if err != nil {
			continue // a stray line: skip this pair and carry on
		}
		out = append(out, pair{code: code, val: lines[i+1]})
	}
	return out, nil
}

// groupPolylines folds VERTEX entities into the POLYLINE they belong to.
func groupPolylines(in []entity) []entity {
	var out []entity
	for i := 0; i < len(in); i++ {
		e := in[i]
		if e.typ != "POLYLINE" {
			out = append(out, e)
			continue
		}
		for i+1 < len(in) {
			n := in[i+1]
			if n.typ == "VERTEX" {
				e.verts = append(e.verts, vertex{
					p:     geom.Point{X: n.num(10, 0), Y: n.num(20, 0)},
					bulge: n.num(42, 0),
				})
				i++
				continue
			}
			if n.typ == "SEQEND" {
				i++
			}
			break
		}
		out = append(out, e)
	}
	return out
}

func unitScale(insunits int, force string) (float64, string) {
	switch strings.ToLower(strings.TrimSpace(force)) {
	case "mm":
		return 1, "mm"
	case "cm":
		return 10, "cm"
	case "m":
		return 1000, "m"
	case "in", "inch", "inches":
		return 25.4, "in"
	case "ft", "feet":
		return 304.8, "ft"
	}
	switch insunits {
	case 1:
		return 25.4, "in"
	case 2:
		return 304.8, "ft"
	case 4:
		return 1, "mm"
	case 5:
		return 10, "cm"
	case 6:
		return 1000, "m"
	}
	return 1, "unknown"
}

func sortedCounts(m map[string]int) []string {
	var out []string
	for k, v := range m {
		out = append(out, fmt.Sprintf("%s x%d", k, v))
	}
	sort.Strings(out)
	return out
}

// ---- turning entities into segments ----

// transform is the placement of a block: scale, then rotation, then move.
type transform struct {
	sx, sy float64
	rotDeg float64
	tx, ty float64
}

func (t transform) apply(p geom.Point) geom.Point {
	x, y := p.X*t.sx, p.Y*t.sy
	if t.rotDeg != 0 {
		rad := t.rotDeg * math.Pi / 180
		s, c := math.Sin(rad), math.Cos(rad)
		x, y = x*c-y*s, x*s+y*c
	}
	return geom.Point{X: x + t.tx, Y: y + t.ty}
}

// scaleOf is how much lengths grow under this transform, used to keep the
// flattening tolerance right inside scaled blocks.
func (t transform) scaleOf() float64 {
	return math.Max(math.Abs(t.sx), math.Abs(t.sy))
}

var skipTypes = map[string]bool{
	"TEXT": true, "MTEXT": true, "ATTDEF": true, "ATTRIB": true,
	"DIMENSION": true, "LEADER": true, "MLEADER": true, "HATCH": true,
	"POINT": true, "SOLID": true, "VIEWPORT": true, "IMAGE": true, "WIPEOUT": true,
}

type flattener struct {
	opt      Options
	blocks   map[string][]entity
	base     map[string]geom.Point
	ignore   map[string]bool
	segments []Segment
	skipped  map[string]int
	warnings []string
	openWarn bool
}

func (f *flattener) add(layer string, t transform, pts []geom.Point) {
	for i := 0; i+1 < len(pts); i++ {
		a, b := t.apply(pts[i]), t.apply(pts[i+1])
		if a == b {
			continue
		}
		f.segments = append(f.segments, Segment{A: a, B: b, Layer: layer})
	}
}

func (f *flattener) run(entities []entity, t transform, depth int) {
	if depth > 8 {
		f.warnings = append(f.warnings, "blocks nested more than 8 deep were not expanded")
		return
	}
	sag := f.opt.sagitta()
	if s := t.scaleOf(); s > 0 {
		sag /= s // the tolerance is in the final drawing, not in block space
	}

	for _, e := range entities {
		if f.ignore[strings.ToLower(strings.TrimSpace(e.layer()))] {
			continue
		}
		if skipTypes[e.typ] {
			f.skipped[e.typ]++
			continue
		}
		layer := e.layer()

		switch e.typ {
		case "LINE":
			f.add(layer, t, []geom.Point{
				{X: e.num(10, 0), Y: e.num(20, 0)},
				{X: e.num(11, 0), Y: e.num(21, 0)},
			})

		case "LWPOLYLINE":
			verts := lwVertices(e)
			closed := e.integer(70, 0)&1 == 1
			f.add(layer, t, polylinePoints(verts, closed, sag))

		case "POLYLINE":
			closed := e.integer(70, 0)&1 == 1
			f.add(layer, t, polylinePoints(e.verts, closed, sag))

		case "CIRCLE":
			c := geom.Point{X: e.num(10, 0), Y: e.num(20, 0)}
			f.add(layer, t, arcPoints(c, e.num(40, 0), 0, 2*math.Pi, sag))

		case "ARC":
			c := geom.Point{X: e.num(10, 0), Y: e.num(20, 0)}
			start := e.num(50, 0) * math.Pi / 180
			end := e.num(51, 0) * math.Pi / 180
			for end < start {
				end += 2 * math.Pi
			}
			f.add(layer, t, arcPoints(c, e.num(40, 0), start, end-start, sag))

		case "ELLIPSE":
			f.add(layer, t, ellipsePoints(e, sag))

		case "SPLINE":
			f.add(layer, t, splinePoints(e, sag))

		case "INSERT":
			f.insert(e, t, depth)

		case "BLOCK", "ENDBLK", "SEQEND", "VERTEX":
			// structural, nothing to draw

		default:
			f.skipped[e.typ]++
		}
	}
}

func (f *flattener) insert(e entity, t transform, depth int) {
	name := strings.TrimSpace(mustStr(e.first(2)))
	body, ok := f.blocks[name]
	if !ok {
		f.warnings = append(f.warnings, fmt.Sprintf("block %q used but not defined in the file", name))
		return
	}
	sx, sy := e.num(41, 1), e.num(42, 1)
	if sx == 0 {
		sx = 1
	}
	if sy == 0 {
		sy = 1
	}
	rot := e.num(50, 0)
	cols, rows := e.integer(70, 1), e.integer(71, 1)
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	colSp, rowSp := e.num(44, 0), e.num(45, 0)
	base := f.base[name]

	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			// The block's base point goes where the INSERT says, so the
			// body is shifted by -base before being scaled and turned.
			local := transform{sx: sx, sy: sy, rotDeg: rot,
				tx: e.num(10, 0) + float64(c)*colSp,
				ty: e.num(20, 0) + float64(r)*rowSp,
			}
			shifted := transform{sx: 1, sy: 1, tx: -base.X, ty: -base.Y}
			f.run(body, compose(t, compose(local, shifted)), depth+1)
		}
	}
}

// compose returns the transform that applies inner first, then outer.
func compose(outer, inner transform) transform {
	// Only the cases EngTree produces are needed: a scale and rotation
	// around the origin, then a move. Composing those keeps that shape as
	// long as the scales are uniform; where they are not, the inner scale
	// wins, which is what CAD files do in practice.
	rad := outer.rotDeg * math.Pi / 180
	s, c := math.Sin(rad), math.Cos(rad)
	x, y := inner.tx*outer.sx, inner.ty*outer.sy
	return transform{
		sx:     outer.sx * inner.sx,
		sy:     outer.sy * inner.sy,
		rotDeg: outer.rotDeg + inner.rotDeg,
		tx:     outer.tx + x*c - y*s,
		ty:     outer.ty + x*s + y*c,
	}
}

func mustStr(v string, _ bool) string { return v }

// lwVertices reads an LWPOLYLINE in file order: x, then y, with an optional
// bulge belonging to the vertex it follows.
func lwVertices(e entity) []vertex {
	var out []vertex
	for _, p := range e.pairs {
		f, err := strconv.ParseFloat(strings.TrimSpace(p.val), 64)
		if err != nil {
			continue
		}
		switch p.code {
		case 10:
			out = append(out, vertex{p: geom.Point{X: f}})
		case 20:
			if len(out) > 0 {
				out[len(out)-1].p.Y = f
			}
		case 42:
			if len(out) > 0 {
				out[len(out)-1].bulge = f
			}
		}
	}
	return out
}

func polylinePoints(verts []vertex, closed bool, sag float64) []geom.Point {
	if len(verts) == 0 {
		return nil
	}
	pts := []geom.Point{verts[0].p}
	last := len(verts) - 1
	for i := 0; i < last; i++ {
		pts = append(pts, spanPoints(verts[i], verts[i+1].p, sag)...)
	}
	// Two vertices and a bulge each is how a circle is written as a
	// polyline, so a closing span is drawn from two vertices up.
	if closed && len(verts) >= 2 {
		pts = append(pts, spanPoints(verts[last], verts[0].p, sag)...)
	}
	return pts
}

// spanPoints returns the points from v to end, not including v itself.
func spanPoints(v vertex, end geom.Point, sag float64) []geom.Point {
	if v.bulge == 0 {
		return []geom.Point{end}
	}
	return bulgePoints(v.p, end, v.bulge, sag)
}

// bulgePoints walks the arc that a polyline bulge stands for. The bulge is
// the tangent of a quarter of the arc's angle; positive turns left.
func bulgePoints(a, b geom.Point, bulge, sag float64) []geom.Point {
	dx, dy := b.X-a.X, b.Y-a.Y
	chord := math.Hypot(dx, dy)
	if chord == 0 {
		return []geom.Point{b}
	}
	theta := 4 * math.Atan(bulge)
	halfSin := math.Sin(theta / 2)
	if math.Abs(halfSin) < 1e-12 {
		return []geom.Point{b}
	}
	r := chord / (2 * halfSin)
	// The centre sits on the left of a->b for a positive bulge.
	mx, my := (a.X+b.X)/2, (a.Y+b.Y)/2
	d := r * math.Cos(theta/2)
	cx, cy := mx-dy/chord*d, my+dx/chord*d
	c := geom.Point{X: cx, Y: cy}
	start := math.Atan2(a.Y-cy, a.X-cx)
	pts := arcPoints(c, math.Abs(r), start, theta, sag)
	if len(pts) > 0 {
		pts[len(pts)-1] = b // land exactly on the vertex
		return pts[1:]
	}
	return []geom.Point{b}
}

// arcSteps is how many straight pieces an arc needs so that it never sits
// more than sag away from the true curve.
func arcSteps(r, sweep, sag float64) int {
	r, sweep = math.Abs(r), math.Abs(sweep)
	if r <= 0 || sweep == 0 {
		return 1
	}
	if sag <= 0 {
		sag = 0.05
	}
	if sag > r/2 {
		sag = r / 2
	}
	maxAng := 2 * math.Acos(1-sag/r)
	if maxAng <= 0 || math.IsNaN(maxAng) {
		return 64
	}
	n := int(math.Ceil(sweep / maxAng))
	if n < 1 {
		n = 1
	}
	if n > 4096 {
		n = 4096
	}
	return n
}

// arcPoints includes both ends.
func arcPoints(c geom.Point, r, start, sweep, sag float64) []geom.Point {
	n := arcSteps(r, sweep, sag)
	pts := make([]geom.Point, 0, n+1)
	for i := 0; i <= n; i++ {
		a := start + sweep*float64(i)/float64(n)
		pts = append(pts, geom.Point{X: c.X + r*math.Cos(a), Y: c.Y + r*math.Sin(a)})
	}
	return pts
}

func ellipsePoints(e entity, sag float64) []geom.Point {
	c := geom.Point{X: e.num(10, 0), Y: e.num(20, 0)}
	mx, my := e.num(11, 0), e.num(21, 0)
	ratio := e.num(40, 1)
	start, end := e.num(41, 0), e.num(42, 2*math.Pi)
	major := math.Hypot(mx, my)
	if major == 0 {
		return nil
	}
	for end < start {
		end += 2 * math.Pi
	}
	n := arcSteps(major, end-start, sag)
	pts := make([]geom.Point, 0, n+1)
	for i := 0; i <= n; i++ {
		t := start + (end-start)*float64(i)/float64(n)
		ct, st := math.Cos(t), math.Sin(t)
		// major axis vector, plus the minor axis at a right angle to it
		x := mx*ct - my*ratio*st
		y := my*ct + mx*ratio*st
		pts = append(pts, geom.Point{X: c.X + x, Y: c.Y + y})
	}
	return pts
}

// splinePoints walks a B-spline. Rational splines (with weights) are
// handled too; a spline given only by fit points is joined straight.
func splinePoints(e entity, sag float64) []geom.Point {
	xs, ys := e.floats(10), e.floats(20)
	n := len(xs)
	if len(ys) < n {
		n = len(ys)
	}
	if n == 0 {
		fx, fy := e.floats(11), e.floats(21)
		m := len(fx)
		if len(fy) < m {
			m = len(fy)
		}
		pts := make([]geom.Point, 0, m)
		for i := 0; i < m; i++ {
			pts = append(pts, geom.Point{X: fx[i], Y: fy[i]})
		}
		return pts
	}
	ctrl := make([]geom.Point, n)
	for i := 0; i < n; i++ {
		ctrl[i] = geom.Point{X: xs[i], Y: ys[i]}
	}
	if n == 1 {
		return ctrl
	}
	degree := e.integer(71, 3)
	if degree < 1 {
		degree = 3
	}
	if degree > n-1 {
		degree = n - 1
	}
	knots := e.floats(40)
	if len(knots) != n+degree+1 {
		knots = uniformKnots(n, degree)
	}
	weights := e.floats(41)
	if len(weights) != n {
		weights = nil
	}

	// Enough samples for the control polygon to be followed closely; the
	// curve is never further from it than the polygon's own corners.
	steps := len(ctrl) * 12
	if l := polylineLength(ctrl); l > 0 && sag > 0 {
		if want := int(math.Ceil(l / math.Max(sag*8, 0.01))); want > steps {
			steps = want
		}
	}
	if steps < 16 {
		steps = 16
	}
	if steps > 2048 {
		steps = 2048
	}

	lo, hi := knots[degree], knots[n]
	pts := make([]geom.Point, 0, steps+1)
	for i := 0; i <= steps; i++ {
		u := lo + (hi-lo)*float64(i)/float64(steps)
		if i == steps {
			u = hi - 1e-12
		}
		pts = append(pts, deBoor(u, degree, knots, ctrl, weights))
	}
	return pts
}

func uniformKnots(n, degree int) []float64 {
	k := make([]float64, 0, n+degree+1)
	for i := 0; i <= degree; i++ {
		k = append(k, 0)
	}
	for i := 1; i < n-degree; i++ {
		k = append(k, float64(i)/float64(n-degree))
	}
	for i := 0; i <= degree; i++ {
		k = append(k, 1)
	}
	return k
}

func polylineLength(p []geom.Point) float64 {
	l := 0.0
	for i := 0; i+1 < len(p); i++ {
		l += math.Hypot(p[i+1].X-p[i].X, p[i+1].Y-p[i].Y)
	}
	return l
}

// deBoor evaluates the spline at u with the basis functions worked out
// directly, which is short and fast enough for drawing.
func deBoor(u float64, degree int, knots []float64, ctrl []geom.Point, weights []float64) geom.Point {
	var x, y, wsum float64
	for i := range ctrl {
		b := basis(i, degree, u, knots)
		if b == 0 {
			continue
		}
		w := 1.0
		if weights != nil {
			w = weights[i]
		}
		x += ctrl[i].X * b * w
		y += ctrl[i].Y * b * w
		wsum += b * w
	}
	if wsum == 0 {
		return ctrl[len(ctrl)-1]
	}
	return geom.Point{X: x / wsum, Y: y / wsum}
}

func basis(i, p int, u float64, knots []float64) float64 {
	if p == 0 {
		if knots[i] <= u && u < knots[i+1] {
			return 1
		}
		return 0
	}
	var left, right float64
	if d := knots[i+p] - knots[i]; d > 0 {
		left = (u - knots[i]) / d * basis(i, p-1, u, knots)
	}
	if d := knots[i+p+1] - knots[i+1]; d > 0 {
		right = (knots[i+p+1] - u) / d * basis(i+1, p-1, u, knots)
	}
	return left + right
}
