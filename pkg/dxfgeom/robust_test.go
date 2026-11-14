package dxfgeom

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/geom"
)

// ---- small DXF writer for the tests ----

func doc(entities string, blocks ...string) string {
	s := "0\nSECTION\n2\nHEADER\n9\n$INSUNITS\n70\n4\n0\nENDSEC\n"
	if len(blocks) > 0 {
		s += "0\nSECTION\n2\nBLOCKS\n" + strings.Join(blocks, "") + "0\nENDSEC\n"
	}
	return s + "0\nSECTION\n2\nENTITIES\n" + entities + "0\nENDSEC\n0\nEOF\n"
}

func ln(x1, y1, x2, y2 float64) string {
	return fmt.Sprintf("0\nLINE\n8\n0\n10\n%g\n20\n%g\n11\n%g\n21\n%g\n", x1, y1, x2, y2)
}

func box(x, y, w, h float64) string {
	return ln(x, y, x+w, y) + ln(x+w, y, x+w, y+h) + ln(x+w, y+h, x, y+h) + ln(x, y+h, x, y)
}

func block(name, body string) string {
	return "0\nBLOCK\n2\n" + name + "\n10\n0\n20\n0\n" + body + "0\nENDBLK\n"
}

func parse(t *testing.T, text string) *Drawing {
	t.Helper()
	d, err := Read(strings.NewReader(text), Options{})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func rings(t *testing.T, text string) ([]geom.Contour, []string) {
	t.Helper()
	d := parse(t, text)
	c, w := d.Contours(Options{})
	return c, append(d.Warnings, w...)
}

// ---- joining ----

func TestLineDrawnTwiceStillCloses(t *testing.T) {
	c, warn := rings(t, doc(box(0, 0, 100, 50)+ln(0, 0, 100, 0)+ln(100, 0, 0, 0)))
	if len(c) != 1 || !near(c[0].Area(), 5000, 0.01) {
		t.Fatalf("got %d contours (warnings %v), want the one rectangle", len(c), warn)
	}
	if hasWarning(warn, "open") {
		t.Errorf("a duplicated line was reported as an open contour: %v", warn)
	}
}

func TestOrderInTheFileDoesNotMatter(t *testing.T) {
	// The same two cases with the troublemakers first, which is the order
	// that leads a naive walk astray.
	c, _ := rings(t, doc(ln(100, 0, 0, 0)+ln(0, 0, 100, 0)+box(0, 0, 100, 50)))
	if len(c) != 1 || !near(c[0].Area(), 5000, 0.01) {
		t.Errorf("duplicate first: got %d contours, want the rectangle", len(c))
	}
	c, _ = rings(t, doc(ln(130, 80, 100, 50)+box(0, 0, 100, 50)))
	if len(c) != 1 || !near(c[0].Area(), 5000, 0.01) {
		t.Errorf("stray line first: got %d contours, want the rectangle", len(c))
	}
	c, _ = rings(t, doc(ln(10, 10, 20, 10)+box(0, 0, 10, 10)+ln(20, 10, 20, 20)+ln(20, 20, 10, 20)+ln(10, 20, 10, 10)))
	if len(c) != 2 {
		t.Errorf("touching squares, mixed order: got %d contours, want 2", len(c))
	}
}

func TestStrayLineDoesNotLoseTheOutline(t *testing.T) {
	c, warn := rings(t, doc(box(0, 0, 100, 50)+ln(100, 50, 130, 80)))
	if len(c) != 1 || !near(c[0].Area(), 5000, 0.01) {
		t.Fatalf("got %d contours, want the rectangle kept", len(c))
	}
	if !hasWarning(warn, "open contour") {
		t.Errorf("the hanging line should be reported: %v", warn)
	}
}

func TestShapesTouchingAtACornerAreTwoRings(t *testing.T) {
	c, _ := rings(t, doc(box(0, 0, 10, 10)+box(10, 10, 10, 10)))
	if len(c) != 2 {
		t.Fatalf("got %d contours, want 2 squares", len(c))
	}
	for _, r := range c {
		if !near(r.Area(), 100, 0.01) {
			t.Errorf("area %v, want 100 each", r.Area())
		}
	}
}

func TestPartInsideAHoleIsItsOwnPart(t *testing.T) {
	c, _ := rings(t, doc(box(0, 0, 100, 100)+box(20, 20, 60, 60)+box(40, 40, 20, 20)))
	if len(c) != 2 {
		t.Fatalf("got %d contours, want 2 (frame and island)", len(c))
	}
	if len(c[0].Holes) != 1 || !near(c[0].Area(), 10000-3600, 0.01) {
		t.Errorf("frame: %d hole(s), area %v", len(c[0].Holes), c[0].Area())
	}
	if len(c[1].Holes) != 0 || !near(c[1].Area(), 400, 0.01) {
		t.Errorf("island: %d hole(s), area %v", len(c[1].Holes), c[1].Area())
	}
}

func TestEndsWithinToleranceJoin(t *testing.T) {
	// The last corner misses by 0.005 mm, inside the default 0.01.
	gap := ln(0, 0, 100, 0) + ln(100, 0, 100, 50) + ln(100, 50, 0, 50) + ln(0, 50, 0.005, 0.0)
	if c, warn := rings(t, doc(gap)); len(c) != 1 {
		t.Fatalf("a 0.005 mm gap was not closed: %v", warn)
	}
	wide := ln(0, 0, 100, 0) + ln(100, 0, 100, 50) + ln(100, 50, 0, 50) + ln(0, 50, 0.5, 0.0)
	if c, _ := rings(t, doc(wide)); len(c) != 0 {
		t.Fatal("a 0.5 mm gap was closed; it should be reported instead")
	}
}

// ---- curves ----

func TestArcStaysWithinTheSagitta(t *testing.T) {
	for _, r := range []float64{0.2, 3, 50, 2000} {
		for _, sag := range []float64{0.01, 0.05, 0.5} {
			pts := arcPoints(geom.Point{}, r, 0.3, 2*math.Pi, sag)
			worst := 0.0
			for i := 0; i+1 < len(pts); i++ {
				mid := math.Hypot((pts[i].X+pts[i+1].X)/2, (pts[i].Y+pts[i+1].Y)/2)
				worst = math.Max(worst, r-mid)
			}
			if worst > sag*1.0001 {
				t.Errorf("r=%v sag=%v: chord sits %.5f from the arc", r, sag, worst)
			}
			if len(pts)-1 < 12 {
				t.Errorf("r=%v sag=%v: a full circle in only %d pieces", r, sag, len(pts)-1)
			}
		}
	}
}

func TestBulgeGeometry(t *testing.T) {
	cases := []struct {
		name    string
		a, b    geom.Point
		bulge   float64
		centre  geom.Point
		radius  float64
		through geom.Point // a point the arc must pass by
	}{
		{"quarter turn left", geom.Point{X: 1}, geom.Point{Y: 1}, math.Tan(math.Pi / 8), geom.Point{}, 1,
			geom.Point{X: math.Sqrt2 / 2, Y: math.Sqrt2 / 2}},
		{"quarter turn right", geom.Point{Y: 1}, geom.Point{X: 1}, -math.Tan(math.Pi / 8), geom.Point{}, 1,
			geom.Point{X: math.Sqrt2 / 2, Y: math.Sqrt2 / 2}},
		{"three quarters left", geom.Point{X: 1}, geom.Point{Y: -1}, math.Tan(3 * math.Pi / 8), geom.Point{}, 1,
			geom.Point{X: -1}},
		{"half turn", geom.Point{}, geom.Point{X: 10}, 1, geom.Point{X: 5}, 5, geom.Point{X: 5, Y: -5}},
	}
	for _, c := range cases {
		pts := bulgePoints(c.a, c.b, c.bulge, 0.0005)
		if last := pts[len(pts)-1]; last != c.b {
			t.Errorf("%s: ends at %+v, want exactly %+v", c.name, last, c.b)
		}
		closest := math.MaxFloat64
		for _, p := range pts {
			if d := math.Hypot(p.X-c.centre.X, p.Y-c.centre.Y); !near(d, c.radius, 1e-9) {
				t.Errorf("%s: point %+v is %.6f from the centre, want %v", c.name, p, d, c.radius)
				break
			}
			closest = math.Min(closest, math.Hypot(p.X-c.through.X, p.Y-c.through.Y))
		}
		if closest > 0.05 {
			t.Errorf("%s: the arc never comes near %+v (closest %.3f): it bends the wrong way", c.name, c.through, closest)
		}
	}
}

func TestEllipseArea(t *testing.T) {
	// centre (0,0), major axis 40 along X, minor/major 0.5
	e := "0\nELLIPSE\n8\n0\n10\n0\n20\n0\n11\n40\n21\n0\n40\n0.5\n41\n0\n42\n6.283185307179586\n"
	d, err := Read(strings.NewReader(doc(e)), Options{Sagitta: 0.001})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := d.Contours(Options{})
	want := math.Pi * 40 * 20
	if len(c) != 1 || !near(c[0].Area(), want, want*0.001) {
		t.Fatalf("ellipse: %d contour(s), area %.1f, want about %.1f", len(c), areaOf(c), want)
	}
	if !near(d.Bounds.Width(), 80, 0.01) || !near(d.Bounds.Height(), 40, 0.01) {
		t.Errorf("ellipse box %.2f x %.2f, want 80 x 40", d.Bounds.Width(), d.Bounds.Height())
	}
}

func TestSplineStartsAndEndsOnItsEndPoints(t *testing.T) {
	// Clamped cubic with large knot values, where a fixed epsilon would
	// have fallen off the end.
	sp := "0\nSPLINE\n8\n0\n71\n3\n"
	for _, k := range []float64{0, 0, 0, 0, 50000, 100000, 100000, 100000, 100000} {
		sp += fmt.Sprintf("40\n%g\n", k)
	}
	ctrl := []geom.Point{{X: 0, Y: 0}, {X: 20, Y: 40}, {X: 50, Y: -10}, {X: 80, Y: 40}, {X: 100, Y: 5}}
	for _, p := range ctrl {
		sp += fmt.Sprintf("10\n%g\n20\n%g\n", p.X, p.Y)
	}
	d := parse(t, doc(sp))
	first, last := d.Segments[0].A, d.Segments[len(d.Segments)-1].B
	if math.Hypot(first.X, first.Y) > 1e-6 {
		t.Errorf("starts at %+v, want the first control point", first)
	}
	if math.Hypot(last.X-100, last.Y-5) > 1e-6 {
		t.Errorf("ends at %+v, want the last control point (100, 5)", last)
	}
}

// ---- placement ----

func TestArcSeenFromBehindIsMirrored(t *testing.T) {
	// centre (10,0), radius 5, 0°..90°, extrusion (0,0,-1)
	arc := "0\nARC\n8\n0\n10\n10\n20\n0\n40\n5\n50\n0\n51\n90\n210\n0\n220\n0\n230\n-1\n"
	b := parse(t, doc(arc)).Bounds
	if !near(b.MinX, -15, 0.01) || !near(b.MaxX, -10, 0.01) || !near(b.MinY, 0, 0.01) || !near(b.MaxY, 5, 0.01) {
		t.Errorf("arc landed at %+v, want x -15..-10 and y 0..5", b)
	}
}

func TestMirroredBlockInsideATurnedBlock(t *testing.T) {
	// An L, so that any wrong flip or turn shows.
	l := []geom.Point{{X: 0, Y: 0}, {X: 30, Y: 0}, {X: 30, Y: 10}, {X: 10, Y: 10}, {X: 10, Y: 20}, {X: 0, Y: 20}}
	body := ""
	for i := range l {
		j := (i + 1) % len(l)
		body += ln(l[i].X, l[i].Y, l[j].X, l[j].Y)
	}
	inner := "0\nINSERT\n8\n0\n2\nSHAPE\n10\n5\n20\n7\n41\n-1\n42\n1\n50\n30\n"      // mirrored, turned 30°
	outer := "0\nINSERT\n8\n0\n2\nholder\n10\n100\n20\n50\n41\n2\n42\n0.5\n50\n45\n" // uneven scale, turned 45°
	d := parse(t, doc(outer, block("SHAPE", body), block("HOLDER", inner)))

	place := func(p geom.Point) geom.Point {
		// inner: mirror X, turn 30°, move (5,7)
		x, y := -p.X, p.Y
		s, c := math.Sin(math.Pi/6), math.Cos(math.Pi/6)
		x, y = x*c-y*s+5, x*s+y*c+7
		// outer: scale (2, 0.5), turn 45°, move (100,50)
		x, y = x*2, y*0.5
		s, c = math.Sin(math.Pi/4), math.Cos(math.Pi/4)
		return geom.Point{X: x*c - y*s + 100, Y: x*s + y*c + 50}
	}
	if len(d.Segments) != len(l) {
		t.Fatalf("got %d segments, want %d", len(d.Segments), len(l))
	}
	for i, s := range d.Segments {
		want := place(l[i])
		if math.Hypot(s.A.X-want.X, s.A.Y-want.Y) > 1e-9 {
			t.Fatalf("corner %d at %+v, want %+v", i, s.A, want)
		}
	}
}

func TestRepeatedInsertTurnsWithItsGrid(t *testing.T) {
	// 3 columns 100 apart, the whole thing turned 90°: copies go up, not right.
	ins := "0\nINSERT\n8\n0\n2\nSQ\n10\n0\n20\n0\n50\n90\n70\n3\n71\n1\n44\n100\n45\n0\n"
	b := parse(t, doc(ins, block("SQ", box(0, 0, 10, 10)))).Bounds
	if !near(b.MinX, -10, 1e-6) || !near(b.MaxX, 0, 1e-6) || !near(b.MinY, 0, 1e-6) || !near(b.MaxY, 210, 1e-6) {
		t.Errorf("grid landed at %+v, want x -10..0 and y 0..210", b)
	}
}

// ---- reading ----

func TestPaperSpaceIsLeftOut(t *testing.T) {
	frame := strings.Replace(box(-500, -500, 2000, 2000), "8\n0\n", "8\n0\n67\n1\n", -1)
	b := parse(t, doc(box(0, 0, 100, 50)+frame)).Bounds
	if !near(b.Width(), 100, 0.01) || !near(b.Height(), 50, 0.01) {
		t.Errorf("paper-space frame was read: box %+v", b)
	}
}

func TestPaddedValuesAndCRLF(t *testing.T) {
	text := strings.ReplaceAll(doc(box(0, 0, 10, 10)), "\n", "\r\n")
	text = strings.ReplaceAll(text, "SECTION", "SECTION  ")
	text = strings.ReplaceAll(text, "LINE", " LINE")
	if c, warn := rings(t, text); len(c) != 1 {
		t.Fatalf("padded file not read: %v", warn)
	}
}

func TestBinaryDXFIsRefusedClearly(t *testing.T) {
	_, err := Read(strings.NewReader("AutoCAD Binary DXF\r\n\x1a\x00rest"), Options{})
	if err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatalf("err = %v, want one that says binary DXF", err)
	}
}

func TestUndefinedAndEmptyBlocks(t *testing.T) {
	ins := func(n string) string { return "0\nINSERT\n8\n0\n2\n" + n + "\n10\n0\n20\n0\n" }
	d := parse(t, doc(ins("MISSING")+ins("EMPTY")+box(0, 0, 5, 5), "0\nBLOCK\n2\nEMPTY\n10\n0\n20\n0\n0\nENDBLK\n"))
	n := 0
	for _, w := range d.Warnings {
		if strings.Contains(w, "not defined") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want exactly one 'not defined' warning (for MISSING), got %v", d.Warnings)
	}
}

func areaOf(c []geom.Contour) float64 {
	if len(c) == 0 {
		return 0
	}
	return c[0].Area()
}
