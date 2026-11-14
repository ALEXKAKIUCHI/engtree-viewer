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

type node struct {
	p    geom.Point
	segs []int // segments that touch this node
}

// joinRings walks the segments end to end. It returns the closed rings and
// how many chains never closed.
func joinRings(segs []Segment, tol float64) ([]geom.Polygon, int) {
	type endRef struct{ a, b int } // node index of each end

	nodes := []node{}
	index := map[[2]int][]int{} // grid cell -> node indices
	cell := func(p geom.Point) [2]int {
		return [2]int{int(math.Floor(p.X / tol)), int(math.Floor(p.Y / tol))}
	}
	nodeAt := func(p geom.Point) int {
		c := cell(p)
		// Look in the neighbouring cells too: two ends within tolerance can
		// still fall on either side of a cell edge.
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				for _, n := range index[[2]int{c[0] + dx, c[1] + dy}] {
					if math.Hypot(nodes[n].p.X-p.X, nodes[n].p.Y-p.Y) <= tol {
						return n
					}
				}
			}
		}
		nodes = append(nodes, node{p: p})
		i := len(nodes) - 1
		index[c] = append(index[c], i)
		return i
	}

	refs := make([]endRef, 0, len(segs))
	for i, s := range segs {
		a, b := nodeAt(s.A), nodeAt(s.B)
		if a == b {
			refs = append(refs, endRef{-1, -1}) // zero length after snapping
			continue
		}
		refs = append(refs, endRef{a, b})
		nodes[a].segs = append(nodes[a].segs, i)
		nodes[b].segs = append(nodes[b].segs, i)
	}

	used := make([]bool, len(segs))
	var rings []geom.Polygon
	openCount := 0

	for start := range segs {
		if used[start] || refs[start].a < 0 {
			continue
		}
		used[start] = true
		first := refs[start].a
		cur := refs[start].b
		chain := []geom.Point{nodes[first].p, nodes[cur].p}

		for cur != first {
			next := -1
			for _, s := range nodes[cur].segs {
				if !used[s] && refs[s].a >= 0 {
					next = s
					break
				}
			}
			if next < 0 {
				break // dead end: this chain does not close
			}
			used[next] = true
			if refs[next].a == cur {
				cur = refs[next].b
			} else {
				cur = refs[next].a
			}
			chain = append(chain, nodes[cur].p)
		}

		if cur == first {
			if len(chain) > 3 {
				rings = append(rings, geom.Polygon(chain[:len(chain)-1]))
			}
		} else {
			openCount++
		}
	}
	return rings, openCount
}
