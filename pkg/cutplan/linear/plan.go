package linear

import (
	"context"
	"math"
	"math/rand"
	"sort"
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
//  2. fewer bars of the last stock in the list, then of the one before
//     it, and so on: the list goes from offcuts to full bars, so full bars
//     are spared first and an offcut is not spent when the part fits a bar
//     that is already being cut;
//  3. the longest single reusable remainder: one long offcut is worth more
//     than several short ones.
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

	offer(p.fillBarByBar())
	offer(p.firstFit(p.order, false))
	offer(p.firstFit(p.order, true))

	limit := defaultSearchTime
	if job.Settings.TimeLimitSec > 0 {
		limit = time.Duration(job.Settings.TimeLimitSec) * time.Second
	}
	rng := rand.New(rand.NewSource(1))
	order := append([]int(nil), p.order...)
	for i := 0; i < maxShuffles && len(order) > 1; i++ {
		if ctx.Err() != nil || time.Since(began) > limit {
			break
		}
		rng.Shuffle(len(order), func(a, b int) { order[a], order[b] = order[b], order[a] })
		offer(p.firstFit(order, i%2 == 1))
	}
	return p.result(best, began), nil
}

const (
	defaultSearchTime = 2 * time.Second
	maxShuffles       = 3000
	maxFillNodes      = 200000
)

// planner holds the job turned into numbers: one entry per unit to cut,
// longest first.
//
// A bar holds parts l1..lk when l1+..+lk + (k-1)*kerf fits its usable
// length. Adding one kerf to every part and to the bar gives the same test
// as a plain sum, so that is how sizes and room are stored.
type planner struct {
	job   Job
	part  []int     // unit -> index in job.Parts
	size  []float64 // unit -> length + kerf
	room  []float64 // stock -> usable length + kerf
	order []int     // units that fit some stock, longest first
	lost  []int     // units longer than every stock
}

// layout is one candidate plan: which units go on which bar.
type layout struct {
	bars     []barLoad
	unplaced []int
}

type barLoad struct {
	stock int
	units []int
	used  float64 // sum of sizes
}

func newPlanner(job Job) *planner {
	p := &planner{job: job}
	set := job.Settings
	maxRoom := 0.0
	for _, st := range job.Stock {
		r := st.Length - set.StartLoss - set.EndLoss + set.Kerf
		p.room = append(p.room, r)
		maxRoom = math.Max(maxRoom, r)
	}
	for pi, part := range job.Parts {
		for q := 0; q < part.Quantity; q++ {
			p.part = append(p.part, pi)
			p.size = append(p.size, part.Length+set.Kerf)
		}
	}
	for u := range p.part {
		if p.size[u] <= maxRoom+Eps {
			p.order = append(p.order, u)
		} else {
			p.lost = append(p.lost, u)
		}
	}
	sort.SliceStable(p.order, func(a, b int) bool { return p.size[p.order[a]] > p.size[p.order[b]] })
	return p
}

// firstFit takes the units in the given order. Each goes on a bar already
// open (the first with room, or the tightest when tight is set); when none
// has room, a new bar is taken from the first stock in the list that still
// has bars and is long enough.
func (p *planner) firstFit(order []int, tight bool) *layout {
	l := &layout{unplaced: append([]int(nil), p.lost...)}
	taken := make([]int, len(p.job.Stock))
	for _, u := range order {
		pick := -1
		for bi := range l.bars {
			free := p.room[l.bars[bi].stock] - l.bars[bi].used
			if p.size[u] > free+Eps {
				continue
			}
			if !tight {
				pick = bi
				break
			}
			if pick < 0 || free < p.room[l.bars[pick].stock]-l.bars[pick].used {
				pick = bi
			}
		}
		if pick < 0 {
			for si, st := range p.job.Stock {
				if (st.Quantity == 0 || taken[si] < st.Quantity) && p.size[u] <= p.room[si]+Eps {
					taken[si]++
					l.bars = append(l.bars, barLoad{stock: si})
					pick = len(l.bars) - 1
					break
				}
			}
		}
		if pick < 0 {
			l.unplaced = append(l.unplaced, u)
			continue
		}
		l.bars[pick].units = append(l.bars[pick].units, u)
		l.bars[pick].used += p.size[u]
	}
	return l
}

// fillBarByBar walks the stock in list order and, for each bar, picks the
// group of remaining units that fills it best. It moves to the next stock
// when the bars run out or nothing left fits.
func (p *planner) fillBarByBar() *layout {
	l := &layout{unplaced: append([]int(nil), p.lost...)}
	rest := append([]int(nil), p.order...)
	for si, st := range p.job.Stock {
		for n := 0; len(rest) > 0 && (st.Quantity == 0 || n < st.Quantity); n++ {
			chosen := p.bestFill(rest, p.room[si])
			if len(chosen) == 0 {
				break
			}
			load := barLoad{stock: si}
			in := map[int]bool{}
			for _, k := range chosen {
				in[k] = true
				load.units = append(load.units, rest[k])
				load.used += p.size[rest[k]]
			}
			l.bars = append(l.bars, load)
			keep := rest[:0]
			for k, u := range rest {
				if !in[k] {
					keep = append(keep, u)
				}
			}
			rest = keep
		}
	}
	l.unplaced = append(l.unplaced, rest...)
	return l
}

// bestFill returns the positions in units (longest first) of the group
// with the largest total size that fits in room. Among equal totals it
// keeps the first found, which favours long parts. The search gives up
// after maxFillNodes steps and returns the best so far.
func (p *planner) bestFill(units []int, room float64) []int {
	n := len(units)
	tail := make([]float64, n+1) // tail[i] = sum of sizes from i on
	for i := n - 1; i >= 0; i-- {
		tail[i] = tail[i+1] + p.size[units[i]]
	}
	var best, cur []int
	bestSum := 0.0
	nodes := 0
	var walk func(i int, sum float64) bool // true = stop the search
	walk = func(i int, sum float64) bool {
		if sum > bestSum+Eps {
			bestSum = sum
			best = append(best[:0], cur...)
			if bestSum >= room-Eps {
				return true
			}
		}
		if i >= n || sum+tail[i] <= bestSum+Eps {
			return false
		}
		if nodes++; nodes > maxFillNodes {
			return true
		}
		s := p.size[units[i]]
		if sum+s <= room+Eps {
			cur = append(cur, i)
			stop := walk(i+1, sum+s)
			cur = cur[:len(cur)-1]
			if stop {
				return true
			}
		}
		// Leaving this unit out: units of the same size after it would
		// only repeat the same groups.
		j := i + 1
		for j < n && math.Abs(p.size[units[j]]-s) <= Eps {
			j++
		}
		return walk(j, sum)
	}
	walk(0, 0)
	return best
}

// remainder is the free length of a loaded bar, as Bar.Remainder defines
// it.
func (p *planner) remainder(b barLoad) float64 {
	return math.Max(0, p.room[b.stock]-b.used-p.job.Settings.Kerf)
}

func (p *planner) reusable(rem float64) bool {
	return rem > Eps && rem >= p.job.Settings.MinOffcut-Eps
}

// better says whether a beats b by the order in Run's comment.
func (p *planner) better(a, b *layout) bool {
	la, lb := len(a.unplaced), len(b.unplaced)
	if la != lb {
		return la < lb
	}
	ca, cb := make([]int, len(p.room)), make([]int, len(p.room))
	var ra, rb float64
	for _, bar := range a.bars {
		ca[bar.stock]++
		if r := p.remainder(bar); p.reusable(r) {
			ra = math.Max(ra, r)
		}
	}
	for _, bar := range b.bars {
		cb[bar.stock]++
		if r := p.remainder(bar); p.reusable(r) {
			rb = math.Max(rb, r)
		}
	}
	for si := len(ca) - 1; si >= 0; si-- {
		if ca[si] != cb[si] {
			return ca[si] < cb[si]
		}
	}
	return ra > rb+Eps
}

// result writes a layout out as positions on each bar.
func (p *planner) result(l *layout, began time.Time) Result {
	set := p.job.Settings
	bars := append([]barLoad(nil), l.bars...)
	// Stock order, fullest bar first: the bar with the long offcut comes
	// last within its stock.
	sort.SliceStable(bars, func(i, j int) bool {
		if bars[i].stock != bars[j].stock {
			return bars[i].stock < bars[j].stock
		}
		return bars[i].used > bars[j].used+Eps
	})
	res := Result{Bars: []Bar{}}
	next := make([]int, len(p.job.Parts)) // units already numbered, per part
	var partsLen, usableLen float64
	for _, load := range bars {
		st := p.job.Stock[load.stock]
		units := append([]int(nil), load.units...)
		sort.SliceStable(units, func(i, j int) bool {
			if d := p.size[units[i]] - p.size[units[j]]; math.Abs(d) > Eps {
				return d > 0
			}
			return p.part[units[i]] < p.part[units[j]]
		})
		bar := Bar{StockID: st.ID, Length: st.Length}
		pos := set.StartLoss
		var sum float64
		for _, u := range units {
			part := p.job.Parts[p.part[u]]
			next[p.part[u]]++
			bar.Cuts = append(bar.Cuts, Cut{PartID: part.ID, Instance: next[p.part[u]], Start: pos, Length: part.Length})
			pos += part.Length + set.Kerf
			sum += part.Length
		}
		end := pos - set.Kerf // where the last part ends
		bar.Remainder = math.Max(0, st.Length-set.EndLoss-end-set.Kerf)
		bar.Reusable = p.reusable(bar.Remainder)
		usable := st.Length - set.StartLoss - set.EndLoss
		if usable > Eps {
			bar.Utilization = sum / usable
		}
		partsLen += sum
		usableLen += usable
		res.Bars = append(res.Bars, bar)
	}
	if usableLen > Eps {
		res.Utilization = partsLen / usableLen
	}
	left := make([]int, len(p.job.Parts))
	for _, u := range l.unplaced {
		left[p.part[u]]++
	}
	for pi, n := range left {
		if n > 0 {
			res.Unplaced = append(res.Unplaced, Unplaced{PartID: p.job.Parts[pi].ID, Quantity: n})
		}
	}
	res.ElapsedMs = time.Since(began).Milliseconds()
	return res
}
