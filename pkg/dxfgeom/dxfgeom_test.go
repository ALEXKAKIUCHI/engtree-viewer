package dxfgeom

import (
	"math"
	"strings"
	"testing"

	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/geom"
)

func read(t *testing.T, name string, opt Options) *Drawing {
	t.Helper()
	d, err := ReadFile("testdata/"+name, opt)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return d
}

func contours(t *testing.T, name string, opt Options) ([]geom.Contour, []string) {
	t.Helper()
	d := read(t, name, opt)
	c, w := d.Contours(opt)
	return c, append(d.Warnings, w...)
}

func near(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

func TestRectangle(t *testing.T) {
	c, warn := contours(t, "rect.dxf", Options{})
	if len(c) != 1 {
		t.Fatalf("got %d contours, want 1 (warnings: %v)", len(c), warn)
	}
	if !near(c[0].Area(), 5000, 0.01) {
		t.Errorf("area = %v, want 5000", c[0].Area())
	}
	if !c[0].Outer.IsCCW() {
		t.Error("the outline should come back counter-clockwise")
	}
	if len(warn) != 0 {
		t.Errorf("unexpected warnings: %v", warn)
	}
}

func TestPartWithHole(t *testing.T) {
	c, _ := contours(t, "hole.dxf", Options{Sagitta: 0.001})
	if len(c) != 1 || len(c[0].Holes) != 1 {
		t.Fatalf("got %d contours with %v holes, want 1 with 1", len(c), holeCounts(c))
	}
	want := 5000 - math.Pi*100
	if !near(c[0].Area(), want, 0.5) {
		t.Errorf("area = %.2f, want about %.2f", c[0].Area(), want)
	}
	if c[0].Holes[0].IsCCW() {
		t.Error("a hole should come back clockwise")
	}
}

func TestBulgeMakesACircle(t *testing.T) {
	c, _ := contours(t, "bulge.dxf", Options{Sagitta: 0.005})
	if len(c) != 1 {
		t.Fatalf("got %d contours, want 1", len(c))
	}
	want := math.Pi * 50 * 50
	if !near(c[0].Area(), want, want*0.001) {
		t.Errorf("area = %.1f, want about %.1f (two bulges of 1 are a full circle)", c[0].Area(), want)
	}
}

func TestSplineStaysInsideItsControlPoints(t *testing.T) {
	d := read(t, "spline.dxf", Options{})
	if len(d.Segments) < 16 {
		t.Fatalf("spline flattened into %d segments, want at least 16", len(d.Segments))
	}
	b := d.Bounds
	if b.MinX < -0.01 || b.MaxX > 100.01 || b.MinY < -20.01 || b.MaxY > 60.01 {
		t.Errorf("curve left the control polygon's box: %+v", b)
	}
}

func TestInchesBecomeMillimetres(t *testing.T) {
	d := read(t, "inches.dxf", Options{})
	if d.Units != "in" {
		t.Errorf("units = %q, want in", d.Units)
	}
	if !near(d.Bounds.Width(), 101.6, 0.001) || !near(d.Bounds.Height(), 50.8, 0.001) {
		t.Errorf("size = %.2f x %.2f mm, want 101.60 x 50.80", d.Bounds.Width(), d.Bounds.Height())
	}
}

func TestOpenContourWarns(t *testing.T) {
	c, warn := contours(t, "open.dxf", Options{})
	if len(c) != 0 {
		t.Errorf("got %d contours from an open outline, want 0", len(c))
	}
	if !hasWarning(warn, "open contour") {
		t.Errorf("warnings = %v, want one about an open contour", warn)
	}
}

func TestBlockInsertIsPlaced(t *testing.T) {
	d := read(t, "block.dxf", Options{})
	// A 10 x 10 square, scaled by 2, turned 90° and moved to (50, 50).
	b := d.Bounds
	if !near(b.MinX, 30, 0.001) || !near(b.MaxX, 50, 0.001) || !near(b.MinY, 50, 0.001) || !near(b.MaxY, 70, 0.001) {
		t.Errorf("block landed at %+v, want x 30..50 and y 50..70", b)
	}
}

func TestIgnoredLayersAndEntities(t *testing.T) {
	c, _ := contours(t, "layers.dxf", Options{IgnoreLayers: []string{"cotas"}})
	if len(c) != 1 {
		t.Fatalf("got %d contours, want 1: the dimension layer should be left out", len(c))
	}
	d := read(t, "layers.dxf", Options{})
	if len(d.Skipped) == 0 || !strings.Contains(strings.Join(d.Skipped, ","), "TEXT") {
		t.Errorf("skipped = %v, want TEXT reported", d.Skipped)
	}
}

func TestMissingUnitsWarns(t *testing.T) {
	d := read(t, "nounits.dxf", Options{})
	if !hasWarning(d.Warnings, "unit") {
		t.Errorf("warnings = %v, want one about the missing unit", d.Warnings)
	}
	if d.Units != "mm" {
		t.Errorf("units = %q, want mm assumed", d.Units)
	}
}

func holeCounts(c []geom.Contour) []int {
	out := make([]int, len(c))
	for i := range c {
		out[i] = len(c[i].Holes)
	}
	return out
}

func hasWarning(warn []string, text string) bool {
	for _, w := range warn {
		if strings.Contains(strings.ToLower(w), text) {
			return true
		}
	}
	return false
}
