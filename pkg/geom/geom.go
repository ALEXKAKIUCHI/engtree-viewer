// Package geom holds the plain 2D shapes EngTree passes around: points,
// rings and a ring with its holes. Everything is in millimetres.
//
// It is deliberately small and depends on nothing: a cut planner, a DXF
// reader and the viewer all speak these types.
package geom

import "math"

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Rect struct {
	MinX float64 `json:"min_x"`
	MinY float64 `json:"min_y"`
	MaxX float64 `json:"max_x"`
	MaxY float64 `json:"max_y"`
}

func (r Rect) Width() float64  { return r.MaxX - r.MinX }
func (r Rect) Height() float64 { return r.MaxY - r.MinY }

// EmptyRect is the zero-size rectangle every Bounds starts from.
func EmptyRect() Rect {
	return Rect{MinX: math.MaxFloat64, MinY: math.MaxFloat64, MaxX: -math.MaxFloat64, MaxY: -math.MaxFloat64}
}

func (r *Rect) Add(p Point) {
	r.MinX = math.Min(r.MinX, p.X)
	r.MinY = math.Min(r.MinY, p.Y)
	r.MaxX = math.Max(r.MaxX, p.X)
	r.MaxY = math.Max(r.MaxY, p.Y)
}

func (r Rect) Valid() bool { return r.MaxX >= r.MinX && r.MaxY >= r.MinY }

// Polygon is a closed ring: the last point is NOT a copy of the first.
type Polygon []Point

// SignedArea is positive when the ring runs counter-clockwise.
func (p Polygon) SignedArea() float64 {
	if len(p) < 3 {
		return 0
	}
	sum := 0.0
	for i := range p {
		j := (i + 1) % len(p)
		sum += p[i].X*p[j].Y - p[j].X*p[i].Y
	}
	return sum / 2
}

func (p Polygon) Area() float64 { return math.Abs(p.SignedArea()) }
func (p Polygon) IsCCW() bool   { return p.SignedArea() > 0 }

// Reversed returns the ring running the other way. The original is untouched.
func (p Polygon) Reversed() Polygon {
	out := make(Polygon, len(p))
	for i, pt := range p {
		out[len(p)-1-i] = pt
	}
	return out
}

// CCW and CW return the ring in the given direction, reversing only when
// needed. Outer rings are kept counter-clockwise and holes clockwise.
func (p Polygon) CCW() Polygon {
	if p.IsCCW() {
		return p
	}
	return p.Reversed()
}

func (p Polygon) CW() Polygon {
	if !p.IsCCW() {
		return p
	}
	return p.Reversed()
}

func (p Polygon) Bounds() Rect {
	r := EmptyRect()
	for _, pt := range p {
		r.Add(pt)
	}
	return r
}

func (p Polygon) Perimeter() float64 {
	if len(p) < 2 {
		return 0
	}
	sum := 0.0
	for i := range p {
		j := (i + 1) % len(p)
		sum += math.Hypot(p[j].X-p[i].X, p[j].Y-p[i].Y)
	}
	return sum
}

// Contains reports whether pt is inside the ring (ray casting). A point
// exactly on an edge may land either way.
func (p Polygon) Contains(pt Point) bool {
	in := false
	for i, j := 0, len(p)-1; i < len(p); j, i = i, i+1 {
		if (p[i].Y > pt.Y) != (p[j].Y > pt.Y) &&
			pt.X < (p[j].X-p[i].X)*(pt.Y-p[i].Y)/(p[j].Y-p[i].Y)+p[i].X {
			in = !in
		}
	}
	return in
}

// Inside reports whether the whole ring sits inside other. The box is
// checked first: a point of a ring that surrounds another can land inside
// that other ring, and the box tells the two cases apart.
func (p Polygon) Inside(other Polygon) bool {
	if len(p) == 0 || len(other) < 3 {
		return false
	}
	const eps = 1e-9
	a, b := p.Bounds(), other.Bounds()
	if a.MinX < b.MinX-eps || a.MinY < b.MinY-eps || a.MaxX > b.MaxX+eps || a.MaxY > b.MaxY+eps {
		return false
	}
	return other.Contains(p.PointInside())
}

// PointInside returns a point that is inside the ring, not just on it: the
// middle of the shortest diagonal from the first vertex that stays in.
func (p Polygon) PointInside() Point {
	if len(p) < 3 {
		if len(p) > 0 {
			return p[0]
		}
		return Point{}
	}
	// The middle of a diagonal across one corner; the first that lands
	// inside wins (on a concave corner it does not).
	for i := range p {
		a, c := p[i], p[(i+2)%len(p)]
		mid := Point{X: (a.X + c.X) / 2, Y: (a.Y + c.Y) / 2}
		if p.Contains(mid) {
			return mid
		}
	}
	// Degenerate ring (all points in a line): the centroid of the box.
	r := p.Bounds()
	return Point{X: (r.MinX + r.MaxX) / 2, Y: (r.MinY + r.MaxY) / 2}
}

func (p Polygon) Translate(dx, dy float64) Polygon {
	out := make(Polygon, len(p))
	for i, pt := range p {
		out[i] = Point{X: pt.X + dx, Y: pt.Y + dy}
	}
	return out
}

// Rotate turns the ring around the origin, in degrees, counter-clockwise.
func (p Polygon) Rotate(deg float64) Polygon {
	rad := deg * math.Pi / 180
	s, c := math.Sin(rad), math.Cos(rad)
	out := make(Polygon, len(p))
	for i, pt := range p {
		out[i] = Point{X: pt.X*c - pt.Y*s, Y: pt.X*s + pt.Y*c}
	}
	return out
}

// Contour is one piece of material: its outline and the holes in it.
// Outer runs counter-clockwise and Holes run clockwise.
type Contour struct {
	Outer Polygon   `json:"outer"`
	Holes []Polygon `json:"holes,omitempty"`
}

// Area is the material left: the outline minus its holes.
func (c Contour) Area() float64 {
	a := c.Outer.Area()
	for _, h := range c.Holes {
		a -= h.Area()
	}
	return a
}

func (c Contour) Bounds() Rect { return c.Outer.Bounds() }
