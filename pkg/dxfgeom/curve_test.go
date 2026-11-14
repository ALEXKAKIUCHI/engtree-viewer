package dxfgeom

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/geom"
)

func curveLength(c Curve) float64 { return polylineLength(c.Points(0.001)) }

// distToRing is how far p is from the ring's boundary.
func distToRing(p geom.Point, ring geom.Polygon) float64 {
	best := math.Inf(1)
	for i := range ring {
		a, b := ring[i], ring[(i+1)%len(ring)]
		dx, dy := b.X-a.X, b.Y-a.Y
		t := 0.0
		if l := dx*dx + dy*dy; l > 0 {
			t = math.Max(0, math.Min(1, ((p.X-a.X)*dx+(p.Y-a.Y)*dy)/l))
		}
		best = math.Min(best, math.Hypot(p.X-a.X-t*dx, p.Y-a.Y-t*dy))
	}
	return best
}

// The curves of a ring trace the same path as its polygon: same length,
// and never further from it than the flattening tolerance.
func checkRing(t *testing.T, name string, ring geom.Polygon, curves []Curve) {
	t.Helper()
	if curves == nil {
		t.Errorf("%s: ring came back without its curves", name)
		return
	}
	const sag = 0.05
	total := 0.0
	for _, c := range curves {
		total += curveLength(c)
		for _, p := range c.Points(0.01) {
			if d := distToRing(p, ring); d > sag+0.02 {
				t.Errorf("%s: a %s strays %.3f mm from the ring", name, c.Kind, d)
				return
			}
		}
	}
	if per := ring.Perimeter(); math.Abs(total-per) > 0.01*per {
		t.Errorf("%s: curves are %.2f mm long, the ring %.2f", name, total, per)
	}
}

func TestShapesKeepTheirCurves(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.dxf")
	if len(files) == 0 {
		t.Fatal("no test drawings")
	}
	n := 0
	for _, f := range files {
		shapes, _, err := Shapes(f, Options{})
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		contours, _, _ := Parts(f, Options{})
		if len(shapes) != len(contours) {
			t.Fatalf("%s: %d shapes but %d contours", f, len(shapes), len(contours))
		}
		for i, s := range shapes {
			n++
			if s.Area() != contours[i].Area() {
				t.Errorf("%s: shape %d is not contour %d", f, i, i)
			}
			checkRing(t, f, s.Outer, s.OuterCurves)
			if len(s.HoleCurves) != len(s.Holes) {
				t.Fatalf("%s: %d holes but %d curve lists", f, len(s.Holes), len(s.HoleCurves))
			}
			for h := range s.Holes {
				checkRing(t, f+" (hole)", s.Holes[h], s.HoleCurves[h])
			}
		}
	}
	if n < 5 {
		t.Fatalf("only %d shapes in the test drawings", n)
	}
}

func kinds(curves []Curve) map[CurveKind]int {
	m := map[CurveKind]int{}
	for _, c := range curves {
		m[c.Kind]++
	}
	return m
}

func TestCurveKinds(t *testing.T) {
	// a plate with a round hole: four lines, and one arc that is a circle
	s, _, err := Shapes("testdata/hole.dxf", Options{})
	if err != nil || len(s) != 1 || len(s[0].HoleCurves) != 1 {
		t.Fatalf("hole.dxf: %v, %d shapes", err, len(s))
	}
	if k := kinds(s[0].OuterCurves); k[CurveLine] != 4 || len(k) != 1 {
		t.Errorf("outline curves = %v, want 4 lines", k)
	}
	hole := s[0].HoleCurves[0]
	if len(hole) != 1 || hole[0].Kind != CurveArc || !near(math.Abs(hole[0].Sweep), 2*math.Pi, 1e-9) || !near(hole[0].Radius, 10, 1e-9) {
		t.Errorf("hole curves = %+v, want one full circle of radius 10", hole)
	}

	// bulged polyline spans are arcs, a spline stays a spline
	s, _, _ = Shapes("testdata/bulge.dxf", Options{})
	if len(s) == 0 || kinds(s[0].OuterCurves)[CurveArc] == 0 {
		t.Errorf("bulge.dxf: no arcs in %v", s)
	}
	// (the test spline is an open curve, so it is looked up in the drawing)
	d := read(t, "spline.dxf", Options{})
	if k := kinds(d.Curves); k[CurveSpline] != 1 {
		t.Errorf("spline.dxf: curves = %v, want one spline", k)
	} else if got, want := curveLength(d.Curves[0]), 0.0; got <= want {
		t.Errorf("the spline has no length")
	}
	for _, sg := range d.Segments {
		if sg.Curve < 0 || sg.Curve >= len(d.Curves) {
			t.Fatalf("segment points at curve %d of %d", sg.Curve, len(d.Curves))
		}
	}
}

// The same line drawn twice is one curve, not two: written back out, it
// must not be cut twice.
func TestDoubledLineIsOneCurve(t *testing.T) {
	d := &Drawing{}
	sq := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	add := func(a, b geom.Point) {
		d.Segments = append(d.Segments, Segment{A: a, B: b, Curve: len(d.Curves)})
		d.Curves = append(d.Curves, lineCurve(a, b))
	}
	for i := range sq {
		add(sq[i], sq[(i+1)%4])
	}
	add(sq[1], sq[0]) // the bottom edge again, the other way
	s, _ := d.Shapes(Options{})
	if len(s) != 1 || len(s[0].OuterCurves) != 4 {
		t.Fatalf("got %d shapes with %d curves, want 1 with 4", len(s), len(s[0].OuterCurves))
	}
}

// A curve that runs along two rings cannot be written whole for either.
func TestSharedCurveFallsBack(t *testing.T) {
	d := &Drawing{}
	// one polyline-like curve drawing two squares that touch at a corner
	pts := []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 20, Y: 10}, {X: 20, Y: 20}, {X: 10, Y: 20}, {X: 10, Y: 10}, {X: 0, Y: 10}, {X: 0, Y: 0}}
	d.Curves = []Curve{{Kind: CurveSpline}}
	for i := 0; i+1 < len(pts); i++ {
		d.Segments = append(d.Segments, Segment{A: pts[i], B: pts[i+1], Curve: 0})
	}
	s, _ := d.Shapes(Options{})
	if len(s) != 2 {
		t.Fatalf("got %d shapes, want 2", len(s))
	}
	for _, sh := range s {
		if sh.OuterCurves != nil {
			t.Error("a ring sharing its curve with another should come back without curves")
		}
	}
}

func TestCurveTransform(t *testing.T) {
	arc := arcCurve(geom.Point{X: 5, Y: 0}, 2, 0, math.Pi/2) // from (7,0) up to (5,2)
	check := func(name string, c Curve, wantKind CurveKind, a, b geom.Point) {
		t.Helper()
		ga, gb := c.Ends()
		if c.Kind != wantKind || math.Hypot(ga.X-a.X, ga.Y-a.Y) > 1e-9 || math.Hypot(gb.X-b.X, gb.Y-b.Y) > 1e-9 {
			t.Errorf("%s: %s from %v to %v, want %s from %v to %v", name, c.Kind, ga, gb, wantKind, a, b)
		}
	}
	check("as drawn", arc, CurveArc, geom.Point{X: 7}, geom.Point{X: 5, Y: 2})
	// turned a quarter and moved
	turned := arc.Transform(Placing(false, 90, 100, 0))
	check("turned", turned, CurveArc, geom.Point{X: 100, Y: 7}, geom.Point{X: 98, Y: 5})
	if turned.Sweep <= 0 {
		t.Error("a turned arc keeps its direction")
	}
	// mirrored: same ends mirrored, and the arc runs the other way
	mirrored := arc.Transform(Placing(true, 0, 0, 0))
	check("mirrored", mirrored, CurveArc, geom.Point{X: -7}, geom.Point{X: -5, Y: 2})
	if mirrored.Sweep >= 0 {
		t.Error("a mirrored arc runs the other way")
	}
	mid := mirrored.Points(0.001)
	if p := mid[len(mid)/2]; !near(math.Hypot(p.X+5, p.Y), 2, 1e-9) || p.Y <= 0 {
		t.Errorf("the mirrored arc bulges the wrong way: midpoint %v", p)
	}
	// stretched: no longer a circle
	wide := arc.Transform(Matrix{A: 2, D: 1})
	check("stretched", wide, CurveEllipse, geom.Point{X: 14}, geom.Point{X: 10, Y: 2})
	major, minor, start, sweep := wide.Axes()
	if !near(math.Hypot(major.X, major.Y), 4, 1e-9) || !near(math.Hypot(minor.X, minor.Y), 2, 1e-9) || !near(major.X*minor.X+major.Y*minor.Y, 0, 1e-9) {
		t.Errorf("axes = %v and %v, want lengths 4 and 2 at a right angle", major, minor)
	}
	at := func(u float64) geom.Point {
		return geom.Point{X: wide.Center.X + major.X*math.Cos(u) + minor.X*math.Sin(u), Y: wide.Center.Y + major.Y*math.Cos(u) + minor.Y*math.Sin(u)}
	}
	if a, b := at(start), at(start+sweep); math.Hypot(a.X-14, a.Y) > 1e-9 || math.Hypot(b.X-10, b.Y-2) > 1e-9 {
		t.Errorf("Axes changed the ends: %v to %v", a, b)
	}
	// an ellipse given by slanted conjugate diameters
	slant := Curve{Kind: CurveEllipse, U: geom.Point{X: 3, Y: 1}, V: geom.Point{X: -1, Y: 2}, Sweep: 2 * math.Pi}
	major, minor, start, _ = slant.Axes()
	if math.Abs(major.X*minor.X+major.Y*minor.Y) > 1e-9 || math.Hypot(major.X, major.Y) < math.Hypot(minor.X, minor.Y) {
		t.Errorf("axes of a slanted ellipse = %v, %v", major, minor)
	}
	want := slant.ellipseAt(0.7)
	u := 0.7 + start
	got := geom.Point{X: major.X*math.Cos(u) + minor.X*math.Sin(u), Y: major.Y*math.Cos(u) + minor.Y*math.Sin(u)}
	if math.Hypot(got.X-want.X, got.Y-want.Y) > 1e-9 {
		t.Errorf("Axes moved the curve: %v, want %v", got, want)
	}
}
