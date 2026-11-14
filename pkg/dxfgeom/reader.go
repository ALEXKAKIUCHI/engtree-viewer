// Package dxfgeom reads a DXF drawing and gives back flat geometry in
// millimetres: line segments to draw, and closed contours (an outline plus
// its holes) to cut or to nest.
//
// It reads the ASCII DXF that CAD programs export for cutting: LINE, ARC,
// CIRCLE, LWPOLYLINE (bulges included), POLYLINE, ELLIPSE, SPLINE and
// INSERT (blocks, with their scale and rotation). Text, dimensions and
// hatches are skipped, and so are any layers the caller names.
//
// The curves themselves are kept too (see Curve): every segment knows
// which line, arc, ellipse or spline it was cut from, so a program that
// writes a drawing back out can write true arcs instead of short lines.
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
	// Curve is the index in Drawing.Curves of the curve this piece was
	// cut from.
	Curve int `json:"-"`
}

// Drawing is everything useful that came out of the file.
type Drawing struct {
	Segments []Segment `json:"segments"`
	// Curves are the drawing's lines, arcs, ellipses and splines as they
	// were drawn, already in millimetres and with block placement applied.
	Curves   []Curve   `json:"-"`
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
			if cur != nil && cur.typ == "BLOCK" { // a block with nothing in it
				name, _ := cur.first(2)
				blocks[blockKey(name)] = nil
				cur = nil
			}
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
				curBlock = blockKey(name)
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
	st.run(entities, scaling(scale, scale), 0)

	d.Segments = st.segments
	d.Curves = st.curves
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

// blockKey makes block names comparable: DXF does not tell "Door" from "DOOR".
func blockKey(name string) string { return strings.ToUpper(strings.TrimSpace(name)) }

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
	if len(lines) > 0 && strings.HasPrefix(lines[0], "AutoCAD Binary DXF") {
		return nil, fmt.Errorf("dxfgeom: binary DXF is not supported; save the drawing as ASCII DXF")
	}
	out := make([]pair, 0, len(lines)/2)
	for i := 0; i+1 < len(lines); i += 2 {
		code, err := strconv.Atoi(strings.TrimSpace(lines[i]))
		if err != nil {
			continue // a stray line: skip this pair and carry on
		}
		// Some writers pad values with spaces; nothing read here needs them.
		out = append(out, pair{code: code, val: strings.TrimSpace(lines[i+1])})
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

// transform is a full 2D affine map:
//
//	x' = a*x + c*y + tx
//	y' = b*x + d*y + ty
//
// A matrix, not "scale, rotation, move": a mirrored block placed inside a
// rotated one is a shear of neither kind, and only the matrix gets it right.
type transform struct {
	a, b, c, d, tx, ty float64
}

func scaling(sx, sy float64) transform { return transform{a: sx, d: sy} }

func moving(dx, dy float64) transform { return transform{a: 1, d: 1, tx: dx, ty: dy} }

func rotating(deg float64) transform {
	rad := deg * math.Pi / 180
	s, c := math.Sin(rad), math.Cos(rad)
	return transform{a: c, b: s, c: -s, d: c}
}

// mirrorX is what an entity drawn "from behind" needs (extrusion 0,0,-1):
// its X axis points the other way.
func mirrorX() transform { return transform{a: -1, d: 1} }

// then returns the map that applies t first and next after it.
func (t transform) then(next transform) transform {
	return transform{
		a:  next.a*t.a + next.c*t.b,
		b:  next.b*t.a + next.d*t.b,
		c:  next.a*t.c + next.c*t.d,
		d:  next.b*t.c + next.d*t.d,
		tx: next.a*t.tx + next.c*t.ty + next.tx,
		ty: next.b*t.tx + next.d*t.ty + next.ty,
	}
}

func (t transform) apply(p geom.Point) geom.Point {
	return geom.Point{X: t.a*p.X + t.c*p.Y + t.tx, Y: t.b*p.X + t.d*p.Y + t.ty}
}

// scaleOf is the most this map stretches a length, used to keep the
// flattening tolerance right inside scaled blocks.
func (t transform) scaleOf() float64 {
	return math.Max(math.Hypot(t.a, t.b), math.Hypot(t.c, t.d))
}

var skipTypes = map[string]bool{
	"TEXT": true, "MTEXT": true, "ATTDEF": true, "ATTRIB": true,
	"DIMENSION": true, "LEADER": true, "MLEADER": true, "HATCH": true,
	"POINT": true, "SOLID": true, "VIEWPORT": true, "IMAGE": true, "WIPEOUT": true,
}

type flattener struct {
	opt       Options
	blocks    map[string][]entity
	base      map[string]geom.Point
	ignore    map[string]bool
	segments  []Segment
	curves    []Curve
	skipped   map[string]int
	warnings  []string
	fitWarned bool
}

// add records one curve and the straight pieces it was flattened into.
// pts are in the entity's own space; c is already in the drawing's.
func (f *flattener) add(layer string, t transform, pts []geom.Point, c Curve) {
	c.Layer = layer
	idx := len(f.curves)
	used := false
	for i := 0; i+1 < len(pts); i++ {
		a, b := t.apply(pts[i]), t.apply(pts[i+1])
		if a == b {
			continue
		}
		f.segments = append(f.segments, Segment{A: a, B: b, Layer: layer, Curve: idx})
		used = true
	}
	if used {
		f.curves = append(f.curves, c)
	}
}

// addPolyline adds a polyline span by span: each straight span is a line
// and each bulged span an arc of its own.
func (f *flattener) addPolyline(layer string, t transform, verts []vertex, closed bool, sag float64) {
	n := len(verts)
	spans := n - 1
	// Two vertices and a bulge each is how a circle is written as a
	// polyline, so a closing span is drawn from two vertices up.
	if closed && n >= 2 {
		spans = n
	}
	for i := 0; i < spans; i++ {
		v, end := verts[i], verts[(i+1)%n].p
		pts := append([]geom.Point{v.p}, spanPoints(v, end, sag)...)
		c := lineCurve(t.apply(v.p), t.apply(end))
		if cen, r, start, sweep, ok := bulgeArc(v.p, end, v.bulge); ok {
			c = arcCurve(cen, r, start, sweep).transformed(t)
		}
		f.add(layer, t, pts, c)
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
		if e.integer(67, 0) == 1 {
			continue // paper space: title blocks and viewports, not the part
		}
		if skipTypes[e.typ] {
			f.skipped[e.typ]++
			continue
		}
		layer := e.layer()

		// Arcs, circles and polylines are stored in their own plane. Seen
		// "from behind" (extrusion 0,0,-1) that plane's X axis is flipped;
		// ignoring it draws those arcs mirrored, a classic DXF trap.
		flat := t
		if e.num(230, 1) < 0 {
			flat = mirrorX().then(t)
		}

		switch e.typ {
		case "LINE":
			a := geom.Point{X: e.num(10, 0), Y: e.num(20, 0)}
			b := geom.Point{X: e.num(11, 0), Y: e.num(21, 0)}
			f.add(layer, t, []geom.Point{a, b}, lineCurve(t.apply(a), t.apply(b)))

		case "LWPOLYLINE":
			verts := lwVertices(e)
			closed := e.integer(70, 0)&1 == 1
			f.addPolyline(layer, flat, verts, closed, sag)

		case "POLYLINE":
			flags := e.integer(70, 0)
			if flags&(16|64) != 0 {
				f.skipped["POLYLINE (mesh)"]++ // 3D meshes are not outlines
				continue
			}
			f.addPolyline(layer, flat, e.verts, flags&1 == 1, sag)

		case "CIRCLE":
			c := geom.Point{X: e.num(10, 0), Y: e.num(20, 0)}
			r := e.num(40, 0)
			f.add(layer, flat, arcPoints(c, r, 0, 2*math.Pi, sag), arcCurve(c, r, 0, 2*math.Pi).transformed(flat))

		case "ARC":
			c := geom.Point{X: e.num(10, 0), Y: e.num(20, 0)}
			start := e.num(50, 0) * math.Pi / 180
			end := e.num(51, 0) * math.Pi / 180
			for end < start {
				end += 2 * math.Pi
			}
			r := e.num(40, 0)
			f.add(layer, flat, arcPoints(c, r, start, end-start, sag), arcCurve(c, r, start, end-start).transformed(flat))

		case "ELLIPSE":
			f.add(layer, t, ellipsePoints(e, sag), ellipseCurve(e).transformed(t))

		case "SPLINE":
			pts, fitOnly := splinePoints(e, sag)
			if fitOnly && !f.fitWarned {
				f.fitWarned = true
				f.warnings = append(f.warnings, "a spline came with fit points only and was joined with straight lines")
			}
			if sp, ok := splineCurve(e); ok {
				f.add(layer, t, pts, sp.transformed(t))
			} else {
				// no control points: the straight joins are all there is
				for i := 0; i+1 < len(pts); i++ {
					f.add(layer, t, pts[i:i+2], lineCurve(t.apply(pts[i]), t.apply(pts[i+1])))
				}
			}

		case "INSERT":
			f.insert(e, flat, depth)

		case "BLOCK", "ENDBLK", "SEQEND", "VERTEX":
			// structural, nothing to draw

		default:
			f.skipped[e.typ]++
		}
	}
}

func (f *flattener) insert(e entity, t transform, depth int) {
	name, _ := e.first(2)
	key := blockKey(name)
	body, ok := f.blocks[key]
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
	rot := rotating(e.num(50, 0))
	cols, rows := e.integer(70, 1), e.integer(71, 1)
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	colSp, rowSp := e.num(44, 0), e.num(45, 0)
	base := f.base[key]

	// In order: put the block's base point at the origin, scale, turn, and
	// move to where the INSERT says. A repeated insert steps along the
	// turned axes, so the whole grid turns with it.
	body0 := moving(-base.X, -base.Y).then(scaling(sx, sy)).then(rot)
	for c := 0; c < cols; c++ {
		for r := 0; r < rows; r++ {
			step := rot.apply(geom.Point{X: float64(c) * colSp, Y: float64(r) * rowSp})
			place := moving(e.num(10, 0)+step.X, e.num(20, 0)+step.Y)
			f.run(body, body0.then(place).then(t), depth+1)
		}
	}
}

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

// spanPoints returns the points from v to end, not including v itself.
func spanPoints(v vertex, end geom.Point, sag float64) []geom.Point {
	if v.bulge == 0 {
		return []geom.Point{end}
	}
	return bulgePoints(v.p, end, v.bulge, sag)
}

// bulgeArc works out the arc that a polyline bulge stands for. The bulge
// is the tangent of a quarter of the arc's angle; positive turns left.
// ok is false for a straight span.
func bulgeArc(a, b geom.Point, bulge float64) (c geom.Point, r, start, sweep float64, ok bool) {
	dx, dy := b.X-a.X, b.Y-a.Y
	chord := math.Hypot(dx, dy)
	if bulge == 0 || chord == 0 {
		return c, 0, 0, 0, false
	}
	theta := 4 * math.Atan(bulge)
	halfSin := math.Sin(theta / 2)
	if math.Abs(halfSin) < 1e-12 {
		return c, 0, 0, 0, false
	}
	rad := chord / (2 * halfSin)
	// The centre sits on the left of a->b for a positive bulge.
	mx, my := (a.X+b.X)/2, (a.Y+b.Y)/2
	d := rad * math.Cos(theta/2)
	c = geom.Point{X: mx - dy/chord*d, Y: my + dx/chord*d}
	return c, math.Abs(rad), math.Atan2(a.Y-c.Y, a.X-c.X), theta, true
}

// bulgePoints walks that arc from a to b, not including a itself.
func bulgePoints(a, b geom.Point, bulge, sag float64) []geom.Point {
	c, r, start, sweep, ok := bulgeArc(a, b, bulge)
	if !ok {
		return []geom.Point{b}
	}
	pts := arcPoints(c, r, start, sweep, sag)
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
	// Never coarser than 12 pieces to the full turn, however small the
	// radius: a 1 mm hole should still look and nest like a circle.
	if min := int(math.Ceil(sweep / (math.Pi / 6))); n < min {
		n = min
	}
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
	if e.num(230, 1) < 0 {
		ratio = -ratio // seen from behind, the ellipse runs the other way
	}
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
func splinePoints(e entity, sag float64) (pts []geom.Point, fitOnly bool) {
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
		pts = make([]geom.Point, 0, m)
		for i := 0; i < m; i++ {
			pts = append(pts, geom.Point{X: fx[i], Y: fy[i]})
		}
		return pts, m > 0
	}
	if n == 1 {
		return []geom.Point{{X: xs[0], Y: ys[0]}}, false
	}
	c, _ := splineCurve(e)
	return splineSample(c.Control, c.Degree, c.Knots, c.Weights, sag), false
}

// splineSample walks a B-spline given by its control points.
func splineSample(ctrl []geom.Point, degree int, knots, weights []float64, sag float64) []geom.Point {
	n := len(ctrl)
	if n < 2 || degree < 1 || len(knots) != n+degree+1 {
		return ctrl
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
	if steps > 8192 {
		steps = 8192
	}

	lo, hi := knots[degree], knots[n]
	pts := make([]geom.Point, 0, steps+1)
	for i := 0; i <= steps; i++ {
		u := lo + (hi-lo)*float64(i)/float64(steps)
		if i == steps {
			// The basis functions are defined on [knot, next knot), so the
			// very end is taken a hair before it, whatever the knots' size.
			u = math.Nextafter(hi, lo)
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
