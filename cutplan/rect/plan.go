package rect

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"
)

// Run plans the job. It reports every better plan it finds through
// progress (which may be nil) and returns the best one when the search is
// over, when the time limit is reached or when ctx is cancelled. A
// cancelled run is not an error: it returns the best plan so far.
//
// What "better" means, in this order:
//
//  1. fewer parts left out;
//  2. fewer sheets of the last stock in the list, then of the one before
//     it, and so on: the list goes from offcuts to full sheets, so full
//     sheets are spared first;
//  3. fewer different sheet layouts: the same layout cut on several
//     sheets means fewer set-ups at the saw;
//  4. the largest single reusable offcut: one big piece is worth more
//     than several small ones;
//  5. fewer cuts.
//
// How a plan is built: sheets are taken in stock order. A sheet is filled
// strip by strip; a strip is as high as one of the parts still to cut and
// is filled, column by column, with the parts that waste least of it. A
// column holds one part or, with three stages, a stack of parts of the
// same width. The first plan always takes the densest choice; the search
// then repeats the construction with some chance in the choices and keeps
// the best plan. Half of the constructions repeat each sheet layout for as
// long as the parts left allow, before working out a new one.
func Run(ctx context.Context, job Job, progress func(Result)) (Result, error) {
	if err := job.Validate(); err != nil {
		return Result{}, err
	}
	began := time.Now()
	p := newPlanner(job)

	var best *layout
	offer := func(l *layout) {
		if best != nil && !p.better(l, best) {
			return
		}
		best = l
		if progress != nil {
			progress(p.result(best, began))
		}
	}

	limit := defaultSearchTime
	if job.Settings.TimeLimitSec > 0 {
		limit = time.Duration(job.Settings.TimeLimitSec) * time.Second
	}
	rng := rand.New(rand.NewSource(1))
	skip := make([]bool, len(job.Stock))
	offer(p.build(rng, 0, skip, true))
	offer(p.build(rng, 0, skip, false))
	for i := 1; i < maxRounds; i++ {
		if ctx.Err() != nil || time.Since(began) > limit {
			break
		}
		// Now and then leave some of the earlier stock on the shelf: an
		// offcut is not worth spending when the parts fit a sheet that
		// has to be opened anyway.
		for si := range skip {
			skip[si] = i%4 == 3 && si < len(skip)-1 && rng.Intn(2) == 0
		}
		offer(p.build(rng, 0.1+0.5*rng.Float64(), skip, i%2 == 0))
	}
	return p.result(best, began), nil
}

const (
	defaultSearchTime = 2 * time.Second
	maxRounds         = 600
	maxStripHeights   = 12
)

// The planner works in a frame of its own, per sheet: u runs along the
// strips and v across them. With the first cut along the length, u is X
// and v is Y; with the first cut along the width it is the reverse.

// option is one way a part can lie in a frame.
type option struct {
	part  int
	du    float64 // size along the strip
	dv    float64 // size across the strip
	rot   bool
	group int // options of the same du share a group: they can share a column
}

type frame struct {
	first   string
	options []option // highest (dv) first
	groups  [][]int  // group -> its options, highest first
}

type planner struct {
	job    Job
	frames []frame // the first-cut directions the job allows
	units  int     // parts to cut, counting quantities
}

// layout is one candidate plan.
type layout struct {
	sheets   []sheetPlan
	left     []int // units of each part that found no sheet
	unplaced int
	// worked out once, for comparing layouts
	perStock   []int
	plans      int // different sheet layouts
	bestOffcut float64
	cuts       int
}

type sheetPlan struct {
	stock  int
	frame  *frame
	u, v   float64 // usable size in the frame
	strips []strip
	area   float64
}

type strip struct {
	dv   float64
	cols []column
}

type column struct {
	du    float64
	items []option // from the start of the strip upwards
	dv    float64  // height taken by the items, with the kerfs between them
	area  float64
}

func newPlanner(job Job) *planner {
	p := &planner{job: job}
	for _, part := range job.Parts {
		p.units += part.Quantity
	}
	firsts := []string{AlongLength, AlongWidth}
	if job.Settings.FirstCut != "" {
		firsts = []string{job.Settings.FirstCut}
	}
	for _, first := range firsts {
		f := frame{first: first}
		for pi, part := range job.Parts {
			du, dv := part.Length, part.Width
			if first == AlongWidth {
				du, dv = dv, du
			}
			f.options = append(f.options, option{part: pi, du: du, dv: dv})
			if part.CanRotate && math.Abs(du-dv) > Eps {
				f.options = append(f.options, option{part: pi, du: dv, dv: du, rot: true})
			}
		}
		sort.SliceStable(f.options, func(a, b int) bool { return f.options[a].dv > f.options[b].dv })
		byDu := append([]int(nil), make([]int, len(f.options))...)
		for i := range byDu {
			byDu[i] = i
		}
		sort.SliceStable(byDu, func(a, b int) bool { return f.options[byDu[a]].du < f.options[byDu[b]].du })
		for k, i := range byDu {
			if k == 0 || f.options[i].du-f.options[byDu[k-1]].du > Eps {
				f.groups = append(f.groups, nil)
			}
			f.options[i].group = len(f.groups) - 1
		}
		for i, o := range f.options { // options are highest first, so each group is too
			f.groups[o.group] = append(f.groups[o.group], i)
		}
		p.frames = append(p.frames, f)
	}
	return p
}

// build makes one plan. noise is how much chance enters the choices: 0
// always takes the densest. Stock marked in skip is left on the shelf.
// With repeat, each sheet layout is cut again for as long as the stock
// and the parts left allow.
func (p *planner) build(rng *rand.Rand, noise float64, skip []bool, repeat bool) *layout {
	set := p.job.Settings
	left := make([]int, len(p.job.Parts))
	for pi, part := range p.job.Parts {
		left[pi] = part.Quantity
	}
	remaining := p.units
	l := &layout{}
	for si, st := range p.job.Stock {
		if skip[si] {
			continue
		}
		for n := 0; remaining > 0 && (st.Quantity == 0 || n < st.Quantity); n++ {
			var pick *sheetPlan
			var pickLeft []int
			var pickOffcut float64
			for fi := range p.frames {
				f := &p.frames[fi]
				sp := sheetPlan{stock: si, frame: f, u: st.Length - 2*set.EdgeTrim, v: st.Width - 2*set.EdgeTrim}
				if f.first == AlongWidth {
					sp.u, sp.v = sp.v, sp.u
				}
				tryLeft := append([]int(nil), left...)
				p.fill(&sp, tryLeft, rng, noise)
				if sp.area <= Eps {
					continue
				}
				off := p.largestOffcut(&sp)
				if pick == nil || sp.area > pick.area+Eps || (sp.area > pick.area-Eps && off > pickOffcut+Eps) {
					s := sp
					pick, pickLeft, pickOffcut = &s, tryLeft, off
				}
			}
			if pick == nil {
				break // nothing left fits this stock
			}
			used := make([]int, len(left))
			for pi := range left {
				used[pi] = left[pi] - pickLeft[pi]
				remaining -= used[pi]
			}
			left = pickLeft
			l.sheets = append(l.sheets, *pick)
			for repeat && remaining > 0 && (st.Quantity == 0 || n+1 < st.Quantity) && covers(left, used) {
				n++
				for pi := range left {
					left[pi] -= used[pi]
					remaining -= used[pi]
				}
				l.sheets = append(l.sheets, *pick)
			}
		}
	}
	l.left = left
	l.unplaced = remaining
	l.perStock = make([]int, len(p.job.Stock))
	keys := map[string]bool{}
	for i := range l.sheets {
		sp := &l.sheets[i]
		keys[sheetKey(sp)] = true
		l.perStock[sp.stock]++
		l.bestOffcut = math.Max(l.bestOffcut, p.largestOffcut(sp))
		l.cuts += len(p.cuts(sp))
	}
	l.plans = len(keys)
	return l
}

// covers says whether left still holds every unit in used.
func covers(left, used []int) bool {
	for i := range used {
		if left[i] < used[i] {
			return false
		}
	}
	return true
}

// sheetKey is the same for two sheets cut exactly alike.
func sheetKey(sp *sheetPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%s", sp.stock, sp.frame.first)
	for _, s := range sp.strips {
		fmt.Fprintf(&b, "|%g:", s.dv)
		for _, c := range s.cols {
			fmt.Fprintf(&b, "(%g", c.du)
			for _, it := range c.items {
				fmt.Fprintf(&b, ",%d%v", it.part, it.rot)
			}
			b.WriteByte(')')
		}
	}
	return b.String()
}

// fill lays strips on an empty sheet until no part fits, taking the units
// out of left.
func (p *planner) fill(sp *sheetPlan, left []int, rng *rand.Rand, noise float64) {
	kerf := p.job.Settings.Kerf
	f := sp.frame
	v := 0.0 // where the next strip starts
	for {
		room := sp.v - v
		// A strip is as high as one of the parts that still fit.
		var heights []float64
		for _, o := range f.options {
			if left[o.part] == 0 || o.dv > room+Eps || o.du > sp.u+Eps {
				continue
			}
			if n := len(heights); n == 0 || heights[n-1]-o.dv > Eps {
				heights = append(heights, o.dv)
			}
		}
		if len(heights) == 0 {
			return
		}
		if len(heights) > maxStripHeights {
			// The highest ones and a sample of the rest.
			half := maxStripHeights / 2
			rest := heights[half:]
			rng.Shuffle(len(rest), func(a, b int) { rest[a], rest[b] = rest[b], rest[a] })
			heights = heights[:maxStripHeights]
		}
		var best strip
		var bestLeft []int
		bestScore := -1.0
		for _, h := range heights {
			tryLeft := append([]int(nil), left...)
			s := p.strip(f, h, sp.u, tryLeft, rng, noise)
			if len(s.cols) == 0 {
				continue
			}
			area := 0.0
			for _, c := range s.cols {
				area += c.area
			}
			score := area / (sp.u * (s.dv + kerf)) * (1 + noise*rng.Float64())
			if score > bestScore {
				best, bestLeft, bestScore = s, tryLeft, score
			}
		}
		if bestScore < 0 {
			return
		}
		copy(left, bestLeft)
		sp.strips = append(sp.strips, best)
		for _, c := range best.cols {
			sp.area += c.area
		}
		v += best.dv + kerf
	}
}

// strip fills one strip of the given height, column by column, taking the
// units out of left. The strip comes back as high as its highest column.
func (p *planner) strip(f *frame, height, length float64, left []int, rng *rand.Rand, noise float64) strip {
	set := p.job.Settings
	kerf := set.Kerf
	stacks := set.MaxStages() >= 3
	var s strip
	u := 0.0 // where the next column starts
	taken := make([]int, len(left))
	for {
		var best column
		bestScore := -1.0
		for _, o := range f.options {
			if left[o.part] == 0 || o.dv > height+Eps || o.du > length-u+Eps {
				continue
			}
			if !stacks && height-o.dv > Eps {
				continue // two stages: the part must be as high as its strip
			}
			c := column{du: o.du}
			for i := range taken {
				taken[i] = 0
			}
			add := func(o option) bool {
				need := o.dv
				if len(c.items) > 0 {
					need += kerf
				}
				if left[o.part]-taken[o.part] == 0 || c.dv+need > height+Eps {
					return false
				}
				taken[o.part]++
				c.items = append(c.items, o)
				c.dv += need
				c.area += o.du * o.dv
				return true
			}
			add(o)
			if stacks {
				for add(o) { // more of the same part on top
				}
				for _, gi := range f.groups[o.group] { // then others of the same width
					for add(f.options[gi]) {
					}
				}
			}
			score := c.area / ((c.du + kerf) * (height + kerf)) * (1 + noise*rng.Float64())
			if score > bestScore+1e-12 || (score > bestScore-1e-12 && c.area > best.area) {
				best, bestScore = c, score
			}
		}
		if bestScore < 0 {
			break
		}
		for _, it := range best.items {
			left[it.part]--
		}
		s.cols = append(s.cols, best)
		s.dv = math.Max(s.dv, best.dv)
		u += best.du + kerf
	}
	return s
}

// A rectangle in the planner's frame.
type piece struct{ u, v, du, dv float64 }

// offcuts lists what a sheet plan leaves over, in the frame.
func (p *planner) offcuts(sp *sheetPlan) []piece {
	kerf := p.job.Settings.Kerf
	var out []piece
	v := 0.0
	for _, s := range sp.strips {
		u := 0.0
		for _, c := range s.cols {
			if free := s.dv - c.dv - kerf; free > Eps { // above a column
				out = append(out, piece{u, v + c.dv + kerf, c.du, free})
			}
			u += c.du + kerf
		}
		if free := sp.u - u; free > Eps { // after the last column
			out = append(out, piece{u, v, free, s.dv})
		}
		v += s.dv + kerf
	}
	if free := sp.v - v; free > Eps { // after the last strip
		out = append(out, piece{0, v, sp.u, free})
	}
	return out
}

func (p *planner) reusable(a, b float64) bool {
	set := p.job.Settings
	return math.Max(a, b) >= math.Max(set.MinOffcutLength, set.MinOffcutWidth)-Eps &&
		math.Min(a, b) >= math.Min(set.MinOffcutLength, set.MinOffcutWidth)-Eps
}

// largestOffcut is the area of the biggest reusable offcut of a sheet.
func (p *planner) largestOffcut(sp *sheetPlan) float64 {
	best := 0.0
	for _, o := range p.offcuts(sp) {
		if p.reusable(o.du, o.dv) {
			best = math.Max(best, o.du*o.dv)
		}
	}
	return best
}

// A cut in the planner's frame: across says it runs across the strips
// (stage 2); otherwise it runs along them (stages 1 and 3).
type sawCut struct {
	stage    int
	across   bool
	pos      float64
	from, to float64
}

// cuts lists the passes of the saw for a sheet plan: the strips first,
// then, strip by strip, its columns and the stacks inside them. A cut
// that would fall on the edge of the usable area is not needed.
func (p *planner) cuts(sp *sheetPlan) []sawCut {
	kerf := p.job.Settings.Kerf
	var out []sawCut
	v := 0.0
	for _, s := range sp.strips {
		if top := v + s.dv; top < sp.v-Eps {
			out = append(out, sawCut{stage: 1, pos: top, from: 0, to: sp.u})
		}
		v += s.dv + kerf
	}
	v = 0.0
	for _, s := range sp.strips {
		top := v + s.dv
		u := 0.0
		for _, c := range s.cols {
			if end := u + c.du; end < sp.u-Eps {
				out = append(out, sawCut{stage: 2, across: true, pos: end, from: v, to: top})
			}
			w := v
			for _, it := range c.items {
				if itTop := w + it.dv; itTop < top-Eps {
					out = append(out, sawCut{stage: 3, pos: itTop, from: u, to: u + c.du})
				}
				w += it.dv + kerf
			}
			u += c.du + kerf
		}
		v += s.dv + kerf
	}
	return out
}

// better says whether a beats b by the order in Run's comment.
func (p *planner) better(a, b *layout) bool {
	if a.unplaced != b.unplaced {
		return a.unplaced < b.unplaced
	}
	for si := len(a.perStock) - 1; si >= 0; si-- {
		if a.perStock[si] != b.perStock[si] {
			return a.perStock[si] < b.perStock[si]
		}
	}
	if a.plans != b.plans {
		return a.plans < b.plans
	}
	if math.Abs(a.bestOffcut-b.bestOffcut) > Eps {
		return a.bestOffcut > b.bestOffcut
	}
	return a.cuts < b.cuts
}

// result writes a layout out in sheet coordinates.
func (p *planner) result(l *layout, began time.Time) Result {
	set := p.job.Settings
	trim := set.EdgeTrim
	res := Result{Sheets: []SheetLayout{}}
	next := make([]int, len(p.job.Parts)) // units already numbered, per part
	var partsArea, usableArea float64
	// Sheets cut alike come one after the other, in stock order.
	first := map[string]int{}
	order := make([]int, len(l.sheets))
	for i := range l.sheets {
		order[i] = i
		if _, ok := first[sheetKey(&l.sheets[i])]; !ok {
			first[sheetKey(&l.sheets[i])] = i
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		sa, sb := &l.sheets[order[a]], &l.sheets[order[b]]
		if sa.stock != sb.stock {
			return sa.stock < sb.stock
		}
		return first[sheetKey(sa)] < first[sheetKey(sb)]
	})
	for _, i := range order {
		sp := &l.sheets[i]
		st := p.job.Stock[sp.stock]
		lengthwise := sp.frame.first == AlongLength
		// From the frame to the sheet.
		xy := func(u, v float64) (float64, float64) {
			if lengthwise {
				return trim + u, trim + v
			}
			return trim + v, trim + u
		}
		out := SheetLayout{StockID: st.ID, Length: st.Length, Width: st.Width, FirstCut: sp.frame.first,
			Placements: []Placement{}, Cuts: []Cut{}, Offcuts: []Offcut{}}
		v := 0.0
		for _, s := range sp.strips {
			u := 0.0
			for _, c := range s.cols {
				w := v
				for _, it := range c.items {
					next[it.part]++
					x, y := xy(u, w)
					out.Placements = append(out.Placements, Placement{
						PartID: p.job.Parts[it.part].ID, Instance: next[it.part], X: x, Y: y, Rotated: it.rot})
					w += it.dv + set.Kerf
				}
				u += c.du + set.Kerf
			}
			v += s.dv + set.Kerf
		}
		for _, c := range p.cuts(sp) {
			along := sp.frame.first
			if c.across {
				along = other(along)
			}
			out.Cuts = append(out.Cuts, Cut{Stage: c.stage, Along: along, Pos: trim + c.pos, From: trim + c.from, To: trim + c.to})
		}
		for _, o := range p.offcuts(sp) {
			x, y := xy(o.u, o.v)
			length, width := o.du, o.dv
			if !lengthwise {
				length, width = width, length
			}
			out.Offcuts = append(out.Offcuts, Offcut{X: x, Y: y, Length: length, Width: width, Reusable: p.reusable(length, width)})
		}
		usable := sp.u * sp.v
		out.Utilization = sp.area / usable
		partsArea += sp.area
		usableArea += usable
		res.Sheets = append(res.Sheets, out)
	}
	if usableArea > Eps {
		res.Utilization = partsArea / usableArea
	}
	for pi, n := range l.left {
		if n > 0 {
			res.Unplaced = append(res.Unplaced, Unplaced{PartID: p.job.Parts[pi].ID, Quantity: n})
		}
	}
	res.ElapsedMs = time.Since(began).Milliseconds()
	return res
}
