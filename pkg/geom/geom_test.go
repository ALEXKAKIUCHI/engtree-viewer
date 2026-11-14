package geom

import (
	"math"
	"testing"
)

func rect(w, h float64) Polygon {
	return Polygon{{0, 0}, {w, 0}, {w, h}, {0, h}}
}

func TestAreaAndDirection(t *testing.T) {
	r := rect(10, 4)
	if got := r.Area(); math.Abs(got-40) > 1e-9 {
		t.Fatalf("area = %v, want 40", got)
	}
	if !r.IsCCW() {
		t.Fatal("rectangle built counter-clockwise reported as clockwise")
	}
	if r.Reversed().IsCCW() {
		t.Fatal("reversed ring still counter-clockwise")
	}
	if !r.CW().Reversed().IsCCW() {
		t.Fatal("CW/Reversed do not round-trip")
	}
}

func TestContainsAndInside(t *testing.T) {
	outer := rect(10, 10)
	hole := rect(2, 2).Translate(4, 4)
	if !outer.Contains(Point{X: 5, Y: 5}) {
		t.Fatal("centre reported outside")
	}
	if outer.Contains(Point{X: 20, Y: 5}) {
		t.Fatal("point to the right reported inside")
	}
	if !hole.Inside(outer) {
		t.Fatal("hole not detected inside the outline")
	}
	if outer.Inside(hole) {
		t.Fatal("outline reported inside its own hole")
	}
}

func TestContourArea(t *testing.T) {
	c := Contour{Outer: rect(10, 10), Holes: []Polygon{rect(2, 2).Translate(1, 1)}}
	if got := c.Area(); math.Abs(got-96) > 1e-9 {
		t.Fatalf("area = %v, want 96", got)
	}
}

func TestRotateAndTranslate(t *testing.T) {
	p := rect(2, 1).Rotate(90)
	b := p.Bounds()
	if math.Abs(b.Width()-1) > 1e-9 || math.Abs(b.Height()-2) > 1e-9 {
		t.Fatalf("after 90°, box = %v x %v, want 1 x 2", b.Width(), b.Height())
	}
	if got := rect(2, 1).Translate(5, 5).Bounds(); got.MinX != 5 || got.MinY != 5 {
		t.Fatalf("translate = %+v", got)
	}
}

func TestPointInsideIsInside(t *testing.T) {
	// An L shape: the middle of the first diagonal falls outside, so
	// PointInside has to keep looking.
	l := Polygon{{0, 0}, {6, 0}, {6, 2}, {2, 2}, {2, 6}, {0, 6}}
	if !l.Contains(l.PointInside()) {
		t.Fatal("PointInside returned a point outside the ring")
	}
}
