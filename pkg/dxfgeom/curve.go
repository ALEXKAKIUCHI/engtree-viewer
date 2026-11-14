package dxfgeom

import (
	"math"

	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/geom"
)

// CurveKind says which fields of a Curve are in use.
type CurveKind string

const (
	CurveLine    CurveKind = "line"
	CurveArc     CurveKind = "arc"
	CurveEllipse CurveKind = "ellipse"
	CurveSpline  CurveKind = "spline"
)

// Curve is one piece of the drawing as it was drawn, before flattening:
// a straight line, a circular arc (a full circle is an arc that sweeps a
// whole turn), an elliptical arc or a spline. Coordinates are final:
// millimetres, with every block's scale, rotation and position applied.
//
// A polyline comes apart into one line per straight span and one arc per
// bulged span. A circle or arc inside a block scaled unevenly is no longer
// circular and comes back as an ellipse.
type Curve struct {
	Kind  CurveKind `json:"kind"`
	Layer string    `json:"layer,omitempty"`

	// Line: from A to B.
	A geom.Point `json:"a"`
	B geom.Point `json:"b"`

	// Arc: the point at angle t is Center + Radius*(cos t, sin t), with t
	// running from Start to Start+Sweep, in radians. A positive Sweep is
	// counter-clockwise; a full circle sweeps 2*pi.
	//
	// Ellipse: the point at t is Center + U*cos t + V*sin t, over the same
	// range. U and V are two conjugate half-diameters: for an ellipse
	// straight from the file, the major half-axis and the minor one.
	Center geom.Point `json:"center"`
	Radius float64    `json:"radius,omitempty"`
	Start  float64    `json:"start,omitempty"`
	Sweep  float64    `json:"sweep,omitempty"`
	U      geom.Point `json:"u"`
	V      geom.Point `json:"v"`

	// Spline: a B-spline by its control points. Weights is empty unless
	// the spline is rational.
	Degree  int          `json:"degree,omitempty"`
	Knots   []float64    `json:"knots,omitempty"`
	Weights []float64    `json:"weights,omitempty"`
	Control []geom.Point `json:"control,omitempty"`
}

// Matrix is a 2D affine map:
//
//	x' = A*x + C*y + TX
//	y' = B*x + D*y + TY
type Matrix struct {
	A, B, C, D, TX, TY float64
}

// Placing returns the map that mirrors a shape across the Y axis when
// mirror is set (x becomes -x), then turns it deg degrees counter-clockwise
// around the origin, then moves it by dx, dy.
func Placing(mirror bool, deg, dx, dy float64) Matrix {
	t := scaling(1, 1)
	if mirror {
		t = mirrorX()
	}
	t = t.then(rotating(deg)).then(moving(dx, dy))
	return Matrix{A: t.a, B: t.b, C: t.c, D: t.d, TX: t.tx, TY: t.ty}
}

// Apply maps one point.
func (m Matrix) Apply(p geom.Point) geom.Point {
	return transform{m.A, m.B, m.C, m.D, m.TX, m.TY}.apply(p)
}

// Transform returns the curve moved by m. An arc stays an arc under a
// rotation, a move, a mirror or an even scale; anything else makes it an
// ellipse.
func (c Curve) Transform(m Matrix) Curve {
	return c.transformed(transform{m.A, m.B, m.C, m.D, m.TX, m.TY})
}

func lineCurve(a, b geom.Point) Curve { return Curve{Kind: CurveLine, A: a, B: b} }

func arcCurve(center geom.Point, r, start, sweep float64) Curve {
	return Curve{Kind: CurveArc, Center: center, Radius: math.Abs(r), Start: start, Sweep: sweep}
}

// linear applies the map without its translation: for directions.
func (t transform) linear(p geom.Point) geom.Point {
	return geom.Point{X: t.a*p.X + t.c*p.Y, Y: t.b*p.X + t.d*p.Y}
}

func (c Curve) transformed(t transform) Curve {
	switch c.Kind {
	case CurveLine:
		c.A, c.B = t.apply(c.A), t.apply(c.B)
	case CurveArc:
		u := t.linear(geom.Point{X: c.Radius})
		v := t.linear(geom.Point{Y: c.Radius})
		return conic(c.Layer, t.apply(c.Center), u, v, c.Start, c.Sweep)
	case CurveEllipse:
		return conic(c.Layer, t.apply(c.Center), t.linear(c.U), t.linear(c.V), c.Start, c.Sweep)
	case CurveSpline:
		ctrl := make([]geom.Point, len(c.Control))
		for i, p := range c.Control {
			ctrl[i] = t.apply(p)
		}
		c.Control = ctrl
	}
	return c
}

// conic builds the curve Center + u*cos t + v*sin t: an arc when u and v
// are as long as each other and at a right angle, an ellipse otherwise.
func conic(layer string, center, u, v geom.Point, start, sweep float64) Curve {
	lu, lv := math.Hypot(u.X, u.Y), math.Hypot(v.X, v.Y)
	dot := u.X*v.X + u.Y*v.Y
	const eps = 1e-9
	if math.Abs(lu-lv) <= eps*math.Max(lu, lv) && math.Abs(dot) <= eps*lu*lv {
		// A circle. Turned the same way, angles just shift; mirrored, they
		// run backwards.
		phi := math.Atan2(u.Y, u.X)
		c := Curve{Kind: CurveArc, Layer: layer, Center: center, Radius: lu, Start: phi + start, Sweep: sweep}
		if u.X*v.Y-u.Y*v.X < 0 {
			c.Start, c.Sweep = phi-start, -sweep
		}
		return c
	}
	return Curve{Kind: CurveEllipse, Layer: layer, Center: center, U: u, V: v, Start: start, Sweep: sweep}
}

// Axes returns an ellipse as DXF and most CAD programs want it: the major
// half-axis, the minor one at a right angle to it, and the parameter range
// measured from the major axis. The point at t is
// Center + major*cos t + minor*sin t, for t from start to start+sweep.
func (c Curve) Axes() (major, minor geom.Point, start, sweep float64) {
	u, v := c.U, c.V
	// Turning the parameter by delta turns a pair of conjugate
	// half-diameters into the pair that is at a right angle.
	delta := 0.5 * math.Atan2(2*(u.X*v.X+u.Y*v.Y), (u.X*u.X+u.Y*u.Y)-(v.X*v.X+v.Y*v.Y))
	cd, sd := math.Cos(delta), math.Sin(delta)
	major = geom.Point{X: u.X*cd + v.X*sd, Y: u.Y*cd + v.Y*sd}
	minor = geom.Point{X: -u.X*sd + v.X*cd, Y: -u.Y*sd + v.Y*cd}
	start = c.Start - delta
	if math.Hypot(major.X, major.Y) < math.Hypot(minor.X, minor.Y) {
		// The other axis is the long one: a quarter turn further on.
		major, minor = minor, geom.Point{X: -major.X, Y: -major.Y}
		start -= math.Pi / 2
	}
	return major, minor, start, c.Sweep
}

// Ends returns where the curve starts and where it stops.
func (c Curve) Ends() (a, b geom.Point) {
	switch c.Kind {
	case CurveLine:
		return c.A, c.B
	case CurveArc:
		return c.arcAt(c.Start), c.arcAt(c.Start + c.Sweep)
	case CurveEllipse:
		return c.ellipseAt(c.Start), c.ellipseAt(c.Start + c.Sweep)
	}
	pts := c.Points(0)
	if len(pts) == 0 {
		return a, b
	}
	return pts[0], pts[len(pts)-1]
}

func (c Curve) arcAt(t float64) geom.Point {
	return geom.Point{X: c.Center.X + c.Radius*math.Cos(t), Y: c.Center.Y + c.Radius*math.Sin(t)}
}

func (c Curve) ellipseAt(t float64) geom.Point {
	ct, st := math.Cos(t), math.Sin(t)
	return geom.Point{X: c.Center.X + c.U.X*ct + c.V.X*st, Y: c.Center.Y + c.U.Y*ct + c.V.Y*st}
}

// Points flattens the curve into straight pieces that stay within sag
// millimetres of it (0 means the default, 0.05). Both ends are included.
func (c Curve) Points(sag float64) []geom.Point {
	if sag <= 0 {
		sag = 0.05
	}
	switch c.Kind {
	case CurveLine:
		return []geom.Point{c.A, c.B}
	case CurveArc:
		return arcPoints(c.Center, c.Radius, c.Start, c.Sweep, sag)
	case CurveEllipse:
		r := math.Max(math.Hypot(c.U.X, c.U.Y), math.Hypot(c.V.X, c.V.Y))
		n := arcSteps(r, c.Sweep, sag)
		pts := make([]geom.Point, 0, n+1)
		for i := 0; i <= n; i++ {
			pts = append(pts, c.ellipseAt(c.Start+c.Sweep*float64(i)/float64(n)))
		}
		return pts
	case CurveSpline:
		return splineSample(c.Control, c.Degree, c.Knots, c.Weights, sag)
	}
	return nil
}

// ellipseCurve reads an ELLIPSE entity, in its own space.
func ellipseCurve(e entity) Curve {
	mx, my := e.num(11, 0), e.num(21, 0)
	ratio := e.num(40, 1)
	if e.num(230, 1) < 0 {
		ratio = -ratio // seen from behind, the ellipse runs the other way
	}
	start, end := e.num(41, 0), e.num(42, 2*math.Pi)
	for end < start {
		end += 2 * math.Pi
	}
	return Curve{
		Kind:   CurveEllipse,
		Center: geom.Point{X: e.num(10, 0), Y: e.num(20, 0)},
		U:      geom.Point{X: mx, Y: my},
		V:      geom.Point{X: -my * ratio, Y: mx * ratio},
		Start:  start,
		Sweep:  end - start,
	}
}

// splineCurve reads a SPLINE entity, in its own space. ok is false when
// the spline has no control points to be drawn from.
func splineCurve(e entity) (Curve, bool) {
	xs, ys := e.floats(10), e.floats(20)
	n := len(xs)
	if len(ys) < n {
		n = len(ys)
	}
	if n < 2 {
		return Curve{}, false
	}
	ctrl := make([]geom.Point, n)
	for i := 0; i < n; i++ {
		ctrl[i] = geom.Point{X: xs[i], Y: ys[i]}
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
	return Curve{Kind: CurveSpline, Degree: degree, Knots: knots, Weights: weights, Control: ctrl}, true
}
