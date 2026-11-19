package linear

import (
	"fmt"
	"math"
	"sort"
)

// Verify checks a result against its job and lists everything that is
// wrong with it. An empty list means the plan can go to the machine.
//
// It is written on its own, from the rules in the package comment, and
// shares no code with the planner: every plan the planner makes is run
// through it in the tests.
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
	stock := map[string]Stock{}
	for _, s := range job.Stock {
		stock[s.ID] = s
	}

	placed := map[string]map[int]bool{} // part -> instances seen
	usedBars := map[string]int{}
	var partsLen, usableLen float64

	for bi, bar := range res.Bars {
		name := fmt.Sprintf("bar %d (%s)", bi+1, bar.StockID)
		st, ok := stock[bar.StockID]
		if !ok {
			fail("%s: stock is not in the job", name)
			continue
		}
		usedBars[st.ID]++
		if math.Abs(bar.Length-st.Length) > Eps {
			fail("%s: length %g differs from the stock length %g", name, bar.Length, st.Length)
		}
		if len(bar.Cuts) == 0 {
			fail("%s: no parts on it", name)
			continue
		}
		cuts := append([]Cut(nil), bar.Cuts...)
		sort.SliceStable(cuts, func(i, j int) bool { return cuts[i].Start < cuts[j].Start })
		limit := st.Length - set.EndLoss
		var barParts float64
		prevEnd := math.Inf(-1)
		for ci, c := range cuts {
			p, ok := parts[c.PartID]
			if !ok {
				fail("%s: part %q is not in the job", name, c.PartID)
				continue
			}
			if math.Abs(c.Length-p.Length) > Eps {
				fail("%s: part %q cut at %g, should be %g", name, c.PartID, c.Length, p.Length)
			}
			if c.Instance < 1 || c.Instance > p.Quantity {
				fail("%s: part %q has no unit number %d", name, c.PartID, c.Instance)
			} else {
				if placed[c.PartID] == nil {
					placed[c.PartID] = map[int]bool{}
				}
				if placed[c.PartID][c.Instance] {
					fail("%s: part %q unit %d is cut twice", name, c.PartID, c.Instance)
				}
				placed[c.PartID][c.Instance] = true
			}
			if ci == 0 {
				if c.Start < set.StartLoss-Eps {
					fail("%s: part %q starts at %g, inside the start loss of %g", name, c.PartID, c.Start, set.StartLoss)
				}
			} else if c.Start < prevEnd+set.Kerf-Eps {
				fail("%s: part %q starts at %g, less than one kerf (%g) after the previous part ends at %g",
					name, c.PartID, c.Start, set.Kerf, prevEnd)
			}
			end := c.Start + c.Length
			if end > limit+Eps {
				fail("%s: part %q ends at %g, past the usable end at %g", name, c.PartID, end, limit)
			}
			prevEnd = end
			barParts += c.Length
		}
		if math.IsInf(prevEnd, -1) {
			continue
		}
		wantRem := math.Max(0, limit-prevEnd-set.Kerf)
		if math.Abs(bar.Remainder-wantRem) > Eps {
			fail("%s: remainder reported as %g, it is %g", name, bar.Remainder, wantRem)
		}
		wantReusable := wantRem > Eps && wantRem >= set.MinOffcut-Eps
		if bar.Reusable != wantReusable {
			fail("%s: remainder of %g marked reusable=%v with a minimum offcut of %g", name, wantRem, bar.Reusable, set.MinOffcut)
		}
		usable := st.Length - set.StartLoss - set.EndLoss
		if usable > Eps && math.Abs(bar.Utilization-barParts/usable) > 1e-6 {
			fail("%s: utilization reported as %g, it is %g", name, bar.Utilization, barParts/usable)
		}
		partsLen += barParts
		usableLen += usable
	}

	for _, st := range job.Stock {
		if st.Quantity > 0 && usedBars[st.ID] > st.Quantity {
			fail("stock %s: %d bars used, only %d on the shelf", st.ID, usedBars[st.ID], st.Quantity)
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
		got := len(placed[p.ID]) + left[p.ID]
		if got != p.Quantity {
			fail("part %q: %d cut + %d left out, the job asks for %d", p.ID, len(placed[p.ID]), left[p.ID], p.Quantity)
		}
		if left[p.ID] == 0 {
			continue
		}
		// A part may only be left out when no bar that never runs out can
		// hold it.
		for _, st := range job.Stock {
			if st.Quantity == 0 && p.Length <= st.Length-set.StartLoss-set.EndLoss+Eps {
				fail("part %q left out although it fits stock %s, which is unlimited", p.ID, st.ID)
				break
			}
		}
	}

	want := 0.0
	if usableLen > Eps {
		want = partsLen / usableLen
	}
	if math.Abs(res.Utilization-want) > 1e-6 {
		fail("utilization reported as %g, it is %g", res.Utilization, want)
	}
	return bad
}
