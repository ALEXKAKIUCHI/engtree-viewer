package dxfgeom

import (
	"fmt"
	"math"
	"sort"

	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/geom"
)

// Contours joins the drawing's segments end to end into closed rings and
// works out which ring is an outline and which is a hole inside it.
//
// Outlines come back counter-clockwise and holes clockwise, which is what
// cutting and nesting expect. Anything that does not close is reported as a
// warning instead of being silently dropped: an open outline usually means
// a gap in the drawing that the operator has to fix.
func (d *Drawing) Contours(opt Options) ([]geom.Contour, []string) {
	tol := opt.tolerance()
	var warn []string

	rings, open := joinRings(d.Segments, tol)
	if open > 0 {
		warn = append(warn, fmt.Sprintf("%d open contour(s): ends further apart than %.3g mm, or a line left hanging", open, tol))
	}

	// Rings too small to be real (a double line, a stray dot).
	kept := rings[:0]
	for _, r := range rings {
		if r.Area() > tol*tol {
			kept = append(kept, r)
		}
	}
	rings = kept
	if len(rings) == 0 {
		warn = append(warn, "no closed contour found")
		return nil, warn
	}

	// Biggest first, so a ring's parent is always already placed.
	sort.SliceStable(rings, func(i, j int) bool { return rings[i].Area() > rings[j].Area() })

	depth := make([]int, len(rings))
	parent := make([]int, len(rings))
	for i := range rings {
		parent[i] = -1
		for j := i - 1; j >= 0; j-- {
			if rings[i].Inside(rings[j]) {
				parent[i] = j
				depth[i] = depth[j] + 1
				break
			}
		}
	}

	// Even depth is material, odd depth is a hole in the ring around it.
	// A ring inside a hole is material again (a part inside a cut-out).
	var out []geom.Contour
	at := make([]int, len(rings))
	for i := range rings {
		if depth[i]%2 != 0 {
			continue
		}
		at[i] = len(out)
		out = append(out, geom.Contour{Outer: rings[i].CCW()})
	}
	for i := range rings {
		if depth[i]%2 == 0 || parent[i] < 0 {
			continue
		}
		owner := at[parent[i]]
		out[owner].Holes = append(out[owner].Holes, rings[i].CW())
	}
	return out, warn
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

// ---- joining ----

// joinRings walks the segments end to end. It returns the closed rings and
// how many chains never closed.
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
func joinRings(segs []Segment, tol float64) ([]geom.Polygon, int) {
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
	seen := map[edge]bool{}
	for _, s := range segs {
		a, b := nodeAt(s.A), nodeAt(s.B)
		if a == b {
			continue // shorter than the tolerance
		}
		if a > b {
			a, b = b, a
		}
		if seen[edge{a, b}] {
			continue
		}
		seen[edge{a, b}] = true
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
	emit := func(nodes []int) {
		if len(nodes) < 3 {
			return
		}
		ring := make(geom.Polygon, len(nodes))
		for i, n := range nodes {
			ring[i] = pts[n]
		}
		rings = append(rings, ring)
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
		pos := map[int]int{cur: 0}
		for e := start; e >= 0; e = nextAt(cur) {
			gone[e] = true
			if edges[e].a == cur {
				cur = edges[e].b
			} else {
				cur = edges[e].a
			}
			if k, back := pos[cur]; back {
				emit(chain[k:]) // the loop just closed
				for _, n := range chain[k+1:] {
					delete(pos, n)
				}
				chain = chain[:k+1]
			} else {
				pos[cur] = len(chain)
				chain = append(chain, cur)
			}
		}
		if len(chain) > 1 {
			open++ // a path between two junctions that closed nothing
		}
	}
	return rings, open
}
