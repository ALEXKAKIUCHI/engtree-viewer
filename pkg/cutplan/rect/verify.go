package rect

import (
	"fmt"
	"math"
	"sort"
)

// box is a rectangle on a sheet, in sheet coordinates.
type box struct{ x0, y0, x1, y1 float64 }

// Verify checks a result against its job and lists everything that is
// wrong with it. An empty list means the plan can go to the saw.
//
// It is written on its own, from the rules in the package comment, and
// shares no code with the planner: every plan the planner makes is run
// through it in the tests. For each sheet it checks, separately:
//
//   - the parts: right size, inside the usable area, a kerf apart;
//   - that the parts can be freed with edge-to-edge cuts within the
//     number of stages, working only from where the parts are;
//   - the cut list: no cut goes through a part, and every side of every
//     part lies on a cut or on the edge of the usable area;
//   - the offcuts, the counts and the utilization.
func Verify(job Job, res Result) []string {
	var bad []string
	fail := func(format string, a ...any) { bad = append(bad, fmt.Sprintf(format, a...)) }

	if err := job.Validate(); err != nil {
		return []string{err.Error()}
	}
	set := job.Settings
	parts := map[string]Part{}
	for _, p := range job.Parts {
		parts[p.ID] = p
	}
	stock := map[string]Sheet{}
	for _, s := range job.Stock {
		stock[s.ID] = s
	}

	placed := map[string]map[int]bool{}
	usedSheets := map[string]int{}
	var partsArea, usableArea float64

	for si, sh := range res.Sheets {
		name := fmt.Sprintf("sheet %d (%s)", si+1, sh.StockID)
		st, ok := stock[sh.StockID]
		if !ok {
			fail("%s: stock is not in the job", name)
			continue
		}
		usedSheets[st.ID]++
		if math.Abs(sh.Length-st.Length) > Eps || math.Abs(sh.Width-st.Width) > Eps {
			fail("%s: size %g x %g differs from the stock size %g x %g", name, sh.Length, sh.Width, st.Length, st.Width)
		}
		if len(sh.Placements) == 0 {
			fail("%s: no parts on it", name)
			continue
		}
		if sh.FirstCut != AlongLength && sh.FirstCut != AlongWidth {
			fail("%s: first cut %q is not a direction", name, sh.FirstCut)
			continue
		}
		if set.FirstCut != "" && sh.FirstCut != set.FirstCut {
			fail("%s: first cut is %s, the job fixes %s", name, sh.FirstCut, set.FirstCut)
		}
		area := box{set.EdgeTrim, set.EdgeTrim, st.Length - set.EdgeTrim, st.Width - set.EdgeTrim}

		// The parts.
		var boxes []box
		var labels []string
		var sheetParts float64
		for _, pl := range sh.Placements {
			p, ok := parts[pl.PartID]
			if !ok {
				fail("%s: part %q is not in the job", name, pl.PartID)
				continue
			}
			label := fmt.Sprintf("%s#%d", pl.PartID, pl.Instance)
			if pl.Instance < 1 || pl.Instance > p.Quantity {
				fail("%s: part %q has no unit number %d", name, pl.PartID, pl.Instance)
			} else {
				if placed[pl.PartID] == nil {
					placed[pl.PartID] = map[int]bool{}
				}
				if placed[pl.PartID][pl.Instance] {
					fail("%s: part %s is cut twice", name, label)
				}
				placed[pl.PartID][pl.Instance] = true
			}
			dx, dy := p.Length, p.Width
			if pl.Rotated {
				if !p.CanRotate {
					fail("%s: part %s is rotated but must follow the grain", name, label)
				}
				dx, dy = dy, dx
			}
			b := box{pl.X, pl.Y, pl.X + dx, pl.Y + dy}
			if b.x0 < area.x0-Eps || b.y0 < area.y0-Eps || b.x1 > area.x1+Eps || b.y1 > area.y1+Eps {
				fail("%s: part %s at %g,%g size %g x %g is outside the usable area", name, label, pl.X, pl.Y, dx, dy)
			}
			boxes = append(boxes, b)
			labels = append(labels, label)
			sheetParts += p.Length * p.Width
		}
		for i := range boxes {
			for k := i + 1; k < len(boxes); k++ {
				a, b := boxes[i], boxes[k]
				gapX := math.Max(b.x0-a.x1, a.x0-b.x1)
				gapY := math.Max(b.y0-a.y1, a.y0-b.y1)
				if gapX < set.Kerf-Eps && gapY < set.Kerf-Eps {
					fail("%s: parts %s and %s are less than one kerf (%g) apart", name, labels[i], labels[k], set.Kerf)
				}
			}
		}

		// Can a guillotine free them within the stages?
		if !canCut(boxes, area, sh.FirstCut, set.MaxStages(), set.Kerf) {
			fail("%s: the parts cannot be freed with edge-to-edge cuts in %d stages starting along the %s",
				name, set.MaxStages(), sh.FirstCut)
		}

		// The cut list.
		for ci, c := range sh.Cuts {
			cn := fmt.Sprintf("%s: cut %d", name, ci+1)
			if c.Stage < 1 || c.Stage > set.MaxStages() {
				fail("%s is of stage %d, the saw has %d", cn, c.Stage, set.MaxStages())
				continue
			}
			want := sh.FirstCut
			if c.Stage%2 == 0 {
				want = other(sh.FirstCut)
			}
			if c.Along != want {
				fail("%s of stage %d runs along the %s, should be the %s", cn, c.Stage, c.Along, want)
				continue
			}
			lo, hi, from, to := area.y0, area.y1, area.x0, area.x1
			if c.Along == AlongWidth {
				lo, hi, from, to = area.x0, area.x1, area.y0, area.y1
			}
			if c.Pos < lo-Eps || c.Pos > hi+Eps || c.From < from-Eps || c.To > to+Eps || c.To <= c.From+Eps {
				fail("%s is outside the usable area", cn)
				continue
			}
			for bi, b := range boxes {
				across0, across1, along0, along1 := b.y0, b.y1, b.x0, b.x1
				if c.Along == AlongWidth {
					across0, across1, along0, along1 = b.x0, b.x1, b.y0, b.y1
				}
				if math.Min(c.To, along1)-math.Max(c.From, along0) <= Eps {
					continue // the cut does not reach this part
				}
				eats := math.Min(c.Pos+set.Kerf, across1)-math.Max(c.Pos, across0) > Eps
				inside := c.Pos > across0+Eps && c.Pos < across1-Eps
				if eats || inside {
					fail("%s goes through part %s", cn, labels[bi])
				}
			}
		}
		for bi, b := range boxes {
			sides := []struct {
				name     string
				along    string
				at       float64
				far      bool // the side away from the origin
				from, to float64
				edge     float64
			}{
				{"left", AlongWidth, b.x0, false, b.y0, b.y1, area.x0},
				{"right", AlongWidth, b.x1, true, b.y0, b.y1, area.x1},
				{"bottom", AlongLength, b.y0, false, b.x0, b.x1, area.y0},
				{"top", AlongLength, b.y1, true, b.x0, b.x1, area.y1},
			}
			for _, s := range sides {
				if math.Abs(s.at-s.edge) <= Eps {
					continue // on the edge of the usable area: the trim cut frees it
				}
				found := false
				for _, c := range sh.Cuts {
					at := c.Pos // a far side lies where a cut starts
					if !s.far {
						at = c.Pos + set.Kerf // a near side lies where a cut ends
					}
					if c.Along == s.along && math.Abs(at-s.at) <= Eps && c.From <= s.from+Eps && c.To >= s.to-Eps {
						found = true
						break
					}
				}
				if !found {
					fail("%s: no cut frees the %s side of part %s", name, s.name, labels[bi])
				}
			}
		}

		// The offcuts.
		var offs []box
		for oi, o := range sh.Offcuts {
			on := fmt.Sprintf("%s: offcut %d", name, oi+1)
			if o.Length <= Eps || o.Width <= Eps {
				fail("%s has no size", on)
				continue
			}
			b := box{o.X, o.Y, o.X + o.Length, o.Y + o.Width}
			if b.x0 < area.x0-Eps || b.y0 < area.y0-Eps || b.x1 > area.x1+Eps || b.y1 > area.y1+Eps {
				fail("%s is outside the usable area", on)
			}
			for bi, pb := range boxes {
				gapX := math.Max(b.x0-pb.x1, pb.x0-b.x1)
				gapY := math.Max(b.y0-pb.y1, pb.y0-b.y1)
				if gapX < set.Kerf-Eps && gapY < set.Kerf-Eps {
					fail("%s is less than one kerf from part %s", on, labels[bi])
				}
			}
			for _, ob := range offs {
				if math.Min(b.x1, ob.x1)-math.Max(b.x0, ob.x0) > Eps && math.Min(b.y1, ob.y1)-math.Max(b.y0, ob.y0) > Eps {
					fail("%s overlaps another offcut", on)
				}
			}
			offs = append(offs, b)
			long, short := math.Max(o.Length, o.Width), math.Min(o.Length, o.Width)
			minLong, minShort := math.Max(set.MinOffcutLength, set.MinOffcutWidth), math.Min(set.MinOffcutLength, set.MinOffcutWidth)
			if want := long >= minLong-Eps && short >= minShort-Eps; o.Reusable != want {
				fail("%s of %g x %g marked reusable=%v with a minimum of %g x %g", on, o.Length, o.Width, o.Reusable, set.MinOffcutLength, set.MinOffcutWidth)
			}
		}

		usable := (area.x1 - area.x0) * (area.y1 - area.y0)
		if math.Abs(sh.Utilization-sheetParts/usable) > 1e-6 {
			fail("%s: utilization reported as %g, it is %g", name, sh.Utilization, sheetParts/usable)
		}
		partsArea += sheetParts
		usableArea += usable
	}

	for _, st := range job.Stock {
		if st.Quantity > 0 && usedSheets[st.ID] > st.Quantity {
			fail("stock %s: %d sheets used, only %d on the shelf", st.ID, usedSheets[st.ID], st.Quantity)
		}
	}

	left := map[string]int{}
	for _, u := range res.Unplaced {
		if _, ok := parts[u.PartID]; !ok {
			fail("unplaced part %q is not in the job", u.PartID)
			continue
		}
		if u.Quantity <= 0 {
			fail("unplaced part %q with quantity %d", u.PartID, u.Quantity)
		}
		left[u.PartID] += u.Quantity
	}
	for _, p := range job.Parts {
		if got := len(placed[p.ID]) + left[p.ID]; got != p.Quantity {
			fail("part %q: %d cut + %d left out, the job asks for %d", p.ID, len(placed[p.ID]), left[p.ID], p.Quantity)
		}
		if left[p.ID] == 0 {
			continue
		}
		// A part may only be left out when no sheet that never runs out
		// can hold it.
		for _, st := range job.Stock {
			if st.Quantity != 0 {
				continue
			}
			l, w := st.Length-2*set.EdgeTrim, st.Width-2*set.EdgeTrim
			if (p.Length <= l+Eps && p.Width <= w+Eps) || (p.CanRotate && p.Width <= l+Eps && p.Length <= w+Eps) {
				fail("part %q left out although it fits stock %s, which is unlimited", p.ID, st.ID)
				break
			}
		}
	}

	want := 0.0
	if usableArea > Eps {
		want = partsArea / usableArea
	}
	if math.Abs(res.Utilization-want) > 1e-6 {
		fail("utilization reported as %g, it is %g", res.Utilization, want)
	}
	return bad
}

func other(along string) string {
	if along == AlongLength {
		return AlongWidth
	}
	return AlongLength
}

// canCut says whether the boxes, lying in the panel, can all be freed by
// edge-to-edge cuts in at most stages stages, the first one running along
// the given direction.
//
// One stage cuts the panel into bands: the boxes are grouped by where
// they lie across the cuts, a new group starting wherever a full kerf of
// free material crosses the whole panel. Each group becomes a panel as
// tight as the group, cut the other way in the next stage. A panel that
// is exactly one box is a finished part.
func canCut(boxes []box, panel box, along string, stages int, kerf float64) bool {
	if len(boxes) == 0 {
		return true
	}
	if len(boxes) == 1 {
		b := boxes[0]
		if math.Abs(b.x0-panel.x0) <= Eps && math.Abs(b.y0-panel.y0) <= Eps &&
			math.Abs(b.x1-panel.x1) <= Eps && math.Abs(b.y1-panel.y1) <= Eps {
			return true
		}
	}
	if stages == 0 {
		return false
	}
	span := func(b box) (float64, float64) { // where the box lies across the cuts
		if along == AlongLength {
			return b.y0, b.y1
		}
		return b.x0, b.x1
	}
	sorted := append([]box(nil), boxes...)
	sort.Slice(sorted, func(i, j int) bool {
		a, _ := span(sorted[i])
		b, _ := span(sorted[j])
		return a < b
	})
	start := 0
	lo, hi := span(sorted[0])
	flush := func(end int) bool {
		sub := panel
		if along == AlongLength {
			sub.y0, sub.y1 = lo, hi
		} else {
			sub.x0, sub.x1 = lo, hi
		}
		return canCut(sorted[start:end], sub, other(along), stages-1, kerf)
	}
	for i := 1; i < len(sorted); i++ {
		a, b := span(sorted[i])
		if a >= hi+kerf-Eps {
			if !flush(i) {
				return false
			}
			start, lo, hi = i, a, b
			continue
		}
		hi = math.Max(hi, b)
	}
	return flush(len(sorted))
}
