package dxfgeom

import (
	"fmt"
	"math"
	"sort"

	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/geom"
)

// Shape is one closed contour together with the curves it was drawn with.
type Shape struct {
	geom.Contour
	// OuterCurves are the lines, arcs, ellipses and splines that make up
	// the outline, and HoleCurves those of each hole, in the order of
	// Contour.Holes. They trace the same path as the polygon, but exactly.
	// A list is nil when the ring could not be matched to whole curves (a
	// curve shared with another ring, or one that only partly lies on it):
	// the polygon is then all there is.
	OuterCurves []Curve
	HoleCurves  [][]Curve
}

// Contours joins the drawing's segments end to end into closed rings and
// works out which ring is an outline and which is a hole inside it.
//
// Outlines come back counter-clockwise and holes clockwise, which is what
// cutting and nesting expect. Anything that does not close is reported as a
// warning instead of being silently dropped: an open outline usually means
// a gap in the drawing that the operator has to fix.
func (d *Drawing) Contours(opt Options) ([]geom.Contour, []string) {
	shapes, warn := d.Shapes(opt)
	if shapes == nil {
		return nil, warn
	}
	out := make([]geom.Contour, len(shapes))
	for i, s := range shapes {
		out[i] = s.Contour
	}
	return out, warn
}

// Shapes is Contours with the original curves of every ring attached.
func (d *Drawing) Shapes(opt Options) ([]Shape, []string) {
	tol := opt.tolerance()
	var warn []string

	polys, ringEdges, jn, open := joinRings(d.Segments, tol)
	if open > 0 {
		warn = append(warn, fmt.Sprintf("%d open contour(s): ends further apart than %.3g mm, or a line left hanging", open, tol))
	}

	// Rings too small to be real (a double line, a stray dot).
	type ring struct {
		poly   geom.Polygon
		curves []Curve
	}
	var rings []ring
	for i, r := range polys {
		if r.Area() > tol*tol {
			rings = append(rings, ring{poly: r, curves: d.ringCurves(ringEdges[i], jn)})
		}
	}
	if len(rings) == 0 {
		warn = append(warn, "no closed contour found")
		return nil, warn
	}

	// Biggest first, so a ring's parent is always already placed.
	sort.SliceStable(rings, func(i, j int) bool { return rings[i].poly.Area() > rings[j].poly.Area() })

	depth := make([]int, len(rings))
	parent := make([]int, len(rings))
	for i := range rings {
		parent[i] = -1
		for j := i - 1; j >= 0; j-- {
			if rings[i].poly.Inside(rings[j].poly) {
				parent[i] = j
				depth[i] = depth[j] + 1
				break
			}
		}
	}

	// Even depth is material, odd depth is a hole in the ring around it.
	// A ring inside a hole is material again (a part inside a cut-out).
	var out []Shape
	at := make([]int, len(rings))
	for i := range rings {
		if depth[i]%2 != 0 {
			continue
		}
		at[i] = len(out)
		out = append(out, Shape{Contour: geom.Contour{Outer: rings[i].poly.CCW()}, OuterCurves: rings[i].curves})
	}
	for i := range rings {
		if depth[i]%2 == 0 || parent[i] < 0 {
			continue
		}
		owner := at[parent[i]]
		out[owner].Holes = append(out[owner].Holes, rings[i].poly.CW())
		out[owner].HoleCurves = append(out[owner].HoleCurves, rings[i].curves)
	}
	return out, warn
}

// ringCurves returns the curves a ring is made of, in the order the ring
// runs, or nil when the ring is not made of whole curves of its own.
func (d *Drawing) ringCurves(edges []int, j joined) []Curve {
	onRing := make(map[int]bool, len(edges))
	for _, e := range edges {
		onRing[e] = true
	}
	var order []int
	picked := map[int]bool{}
	for _, e := range edges {
		c := d.Segments[j.edgeSeg[e]].Curve
		if c < 0 || c >= len(d.Curves) {
			return nil
		}
		if !picked[c] {
			picked[c] = true
			order = append(order, c)
		}
	}
	// Every piece of every picked curve has to lie on this ring, and be
	// the piece that stands for its edge: a curve that also runs along
	// another ring, or that was partly drawn over by another, would be
	// cut wrong if written out whole.
	for si, s := range d.Segments {
		if !picked[s.Curve] {
			continue
		}
		e := j.segEdge[si]
		if e < 0 {
			continue // shorter than the tolerance
		}
		if !onRing[e] || d.Segments[j.edgeSeg[e]].Curve != s.Curve {
			return nil
		}
	}
	out := make([]Curve, len(order))
	for i, c := range order {
		out[i] = d.Curves[c]
	}
	return out
}

// Parts reads a file and returns its closed contours in one step.
func Parts(path string, opt Options) ([]geom.Contour, []string, error) {
	d, err := ReadFile(path, opt)
	if err != nil {
		return nil, nil, err
	}
	rings, warn := d.Contours(opt)
	return rings, append(d.Warnings, warn...), nil
}

// Shapes reads a file and returns its closed contours, each with the
// curves it was drawn with, in one step.
func Shapes(path string, opt Options) ([]Shape, []string, error) {
	d, err := ReadFile(path, opt)
	if err != nil {
		return nil, nil, err
	}
	shapes, warn := d.Shapes(opt)
	return shapes, append(d.Warnings, warn...), nil
}

// ---- joining ----

// joined remembers which segment became which edge of the walk.
type joined struct {
	segEdge []int // segment -> its edge, -1 for one shorter than the tolerance
	edgeSeg []int // edge -> the first segment that drew it
}

// joinRings walks the segments end to end. It returns the closed rings,
// the edges each ring runs along, and how many chains never closed.
//
// Real drawings are not tidy, so three things are dealt with before and
// during the walk:
//
//   - the same line drawn twice (very common in exports) counts once;
//     otherwise the two copies close on each other and eat an edge of the
//     outline;
//   - loose ends (a stray line hanging off the outline, or an outline with
//     a gap) are peeled away first and reported, instead of leading the
//     walk into a dead end that would lose the whole outline;
//   - when the walk comes back to a point it has already passed (two
//     shapes touching at a corner), the loop just closed is cut off as its
//     own ring, instead of one ring that crosses itself.
func joinRings(segs []Segment, tol float64) ([]geom.Polygon, [][]int, joined, int) {
	// 1. Ends within tolerance become one node.
	var pts []geom.Point
	index := map[[2]int][]int{} // grid cell -> node indices
	nodeAt := func(p geom.Point) int {
		c := [2]int{int(math.Floor(p.X / tol)), int(math.Floor(p.Y / tol))}
		// Neighbouring cells too: two ends within tolerance can still fall
		// on either side of a cell edge.
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				for _, n := range index[[2]int{c[0] + dx, c[1] + dy}] {
					if math.Hypot(pts[n].X-p.X, pts[n].Y-p.Y) <= tol {
						return n
					}
				}
			}
		}
		pts = append(pts, p)
		index[c] = append(index[c], len(pts)-1)
		return len(pts) - 1
	}

	// 2. One edge per pair of nodes.
	type edge struct{ a, b int }
	var edges []edge
	j := joined{segEdge: make([]int, len(segs))}
	seen := map[edge]int{}
	for i, s := range segs {
		j.segEdge[i] = -1
		a, b := nodeAt(s.A), nodeAt(s.B)
		if a == b {
			continue // shorter than the tolerance
		}
		if a > b {
			a, b = b, a
		}
		if e, dup := seen[edge{a, b}]; dup {
			j.segEdge[i] = e
			continue
		}
		seen[edge{a, b}] = len(edges)
		j.segEdge[i] = len(edges)
		j.edgeSeg = append(j.edgeSeg, i)
		edges = append(edges, edge{a, b})
	}

	at := make([][]int, len(pts)) // node -> edges touching it
	for i, e := range edges {
		at[e.a] = append(at[e.a], i)
		at[e.b] = append(at[e.b], i)
	}

	// 3. Peel away everything that ends in the air.
	gone := make([]bool, len(edges))
	degree := make([]int, len(pts))
	var leaves []int
	for n := range pts {
		degree[n] = len(at[n])
		if degree[n] == 1 {
			leaves = append(leaves, n)
		}
	}
	open := (len(leaves) + 1) / 2 // an open chain has two loose ends, a spur one
	for len(leaves) > 0 {
		n := leaves[len(leaves)-1]
		leaves = leaves[:len(leaves)-1]
		if degree[n] != 1 {
			continue
		}
		for _, e := range at[n] {
			if gone[e] {
				continue
			}
			gone[e] = true
			other := edges[e].a
			if other == n {
				other = edges[e].b
			}
			degree[n]--
			degree[other]--
			if degree[other] == 1 {
				leaves = append(leaves, other)
			}
		}
	}

	// 4. Walk what is left.
	var rings []geom.Polygon
	var ringEdges [][]int
	emit := func(nodes, via []int) {
		if len(nodes) < 3 {
			return
		}
		ring := make(geom.Polygon, len(nodes))
		for i, n := range nodes {
			ring[i] = pts[n]
		}
		rings = append(rings, ring)
		ringEdges = append(ringEdges, append([]int(nil), via...))
	}
	nextAt := func(n int) int {
		for _, e := range at[n] {
			if !gone[e] {
				return e
			}
		}
		return -1
	}

	for start := range edges {
		if gone[start] {
			continue
		}
		cur := edges[start].a
		chain := []int{cur}
		via := []int{-1} // via[i] is the edge that led to chain[i]
		pos := map[int]int{cur: 0}
		for e := start; e >= 0; e = nextAt(cur) {
			gone[e] = true
			if edges[e].a == cur {
				cur = edges[e].b
			} else {
				cur = edges[e].a
			}
			if k, back := pos[cur]; back {
				emit(chain[k:], append(via[k+1:len(via):len(via)], e)) // the loop just closed
				for _, n := range chain[k+1:] {
					delete(pos, n)
				}
				chain, via = chain[:k+1], via[:k+1]
			} else {
				pos[cur] = len(chain)
				chain = append(chain, cur)
				via = append(via, e)
			}
		}
		if len(chain) > 1 {
			open++ // a path between two junctions that closed nothing
		}
	}
	return rings, ringEdges, j, open
}
