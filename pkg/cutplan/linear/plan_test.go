package linear

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"testing"
)

func mustRun(t *testing.T, job Job) Result {
	t.Helper()
	res, err := Run(context.Background(), job, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if bad := Verify(job, res); len(bad) != 0 {
		t.Fatalf("plan refused by the verifier: %v\njob: %+v\nplan: %+v", bad, job, res)
	}
	return res
}

func count(res Result, stock string) int {
	n := 0
	for _, b := range res.Bars {
		if b.StockID == stock {
			n++
		}
	}
	return n
}

// Every plan for a random job must pass the verifier.
func TestRandomJobsPassTheVerifier(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 300; i++ {
		job := Job{Settings: Settings{
			Kerf:      float64(rng.Intn(5)),
			StartLoss: float64(rng.Intn(3) * 10),
			EndLoss:   float64(rng.Intn(4) * 50),
			MinOffcut: float64(rng.Intn(4) * 150),
		}}
		for k, n := 0, 1+rng.Intn(8); k < n; k++ {
			job.Parts = append(job.Parts, Part{ID: fmt.Sprintf("P%d", k), Length: float64(50 + rng.Intn(3200)), Quantity: 1 + rng.Intn(6)})
		}
		for k, n := 0, rng.Intn(3); k < n; k++ {
			job.Stock = append(job.Stock, Stock{ID: fmt.Sprintf("offcut%d", k), Length: float64(400 + rng.Intn(2500)), Quantity: 1 + rng.Intn(3)})
		}
		full := Stock{ID: "bar", Length: 3000 + float64(rng.Intn(4))*1000}
		if rng.Intn(4) == 0 {
			full.Quantity = 1 + rng.Intn(3) // sometimes the shelf runs out
		}
		job.Stock = append(job.Stock, full)
		mustRun(t, job)
	}
}

// fewestBars tries every way of sharing the lengths among bars.
func fewestBars(lengths []float64, usable, kerf float64) int {
	best := len(lengths)
	free := []float64{}
	var walk func(i int)
	walk = func(i int) {
		if len(free) >= best {
			return
		}
		if i == len(lengths) {
			best = len(free)
			return
		}
		for b := range free {
			if lengths[i]+kerf <= free[b]+Eps {
				free[b] -= lengths[i] + kerf
				walk(i + 1)
				free[b] += lengths[i] + kerf
			}
		}
		free = append(free, usable+kerf-lengths[i]-kerf)
		walk(i + 1)
		free = free[:len(free)-1]
	}
	walk(0)
	return best
}

// On small jobs the planner must reach the fewest bars possible.
func TestSmallJobsUseTheFewestBars(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for i := 0; i < 200; i++ {
		kerf := float64(rng.Intn(4))
		job := Job{Stock: []Stock{{ID: "bar", Length: 6000}}, Settings: Settings{Kerf: kerf, StartLoss: 10, EndLoss: 40}}
		var lengths []float64
		for k, n := 0, 3+rng.Intn(7); k < n; k++ {
			l := float64(300 + rng.Intn(4200))
			lengths = append(lengths, l)
			job.Parts = append(job.Parts, Part{ID: fmt.Sprintf("P%d", k), Length: l, Quantity: 1})
		}
		res := mustRun(t, job)
		if want := fewestBars(lengths, 5950, kerf); len(res.Bars) != want {
			t.Errorf("job %d: %d bars, %d are enough. lengths %v kerf %g", i, len(res.Bars), want, lengths, kerf)
		}
	}
}

// Taking the longest parts first needs three bars here; two are enough.
func TestBeatsLongestFirst(t *testing.T) {
	job := Job{Stock: []Stock{{ID: "bar", Length: 1000}}}
	for i, l := range []float64{440, 240, 240, 220, 210, 210, 180, 180, 80} {
		job.Parts = append(job.Parts, Part{ID: fmt.Sprintf("P%d", i), Length: l, Quantity: 1})
	}
	// 440+240+240+80 and 220+210+210+180+180 fill two bars exactly.
	if res := mustRun(t, job); len(res.Bars) != 2 {
		t.Errorf("%d bars, 2 are enough", len(res.Bars))
	}
}

func TestOffcutsGoFirst(t *testing.T) {
	job := Job{
		Parts:    []Part{{ID: "A", Length: 900, Quantity: 4}},
		Stock:    []Stock{{ID: "offcut", Length: 2000, Quantity: 2}, {ID: "bar", Length: 6000}},
		Settings: Settings{Kerf: 3},
	}
	res := mustRun(t, job)
	if count(res, "offcut") != 2 || count(res, "bar") != 0 {
		t.Errorf("want the 2 offcuts and no full bar, got %+v", res.Bars)
	}
}

// The full bar has to be opened anyway and the short part fits in it: the
// offcut stays on the shelf.
func TestOffcutIsNotSpentForNothing(t *testing.T) {
	job := Job{
		Parts: []Part{{ID: "long", Length: 5000, Quantity: 1}, {ID: "short", Length: 800, Quantity: 1}},
		Stock: []Stock{{ID: "offcut", Length: 1000, Quantity: 1}, {ID: "bar", Length: 6000}},
	}
	res := mustRun(t, job)
	if count(res, "offcut") != 0 || count(res, "bar") != 1 {
		t.Errorf("want one full bar and the offcut kept, got %+v", res.Bars)
	}
}

// Two bars either way; the plan that leaves one long offcut wins over the
// one that leaves two short ones.
func TestPrefersOneLongRemainder(t *testing.T) {
	job := Job{
		Parts: []Part{{ID: "A", Length: 3000, Quantity: 2}, {ID: "B", Length: 2000, Quantity: 2}},
		Stock: []Stock{{ID: "bar", Length: 6000}},
	}
	res := mustRun(t, job)
	longest := 0.0
	for _, b := range res.Bars {
		longest = math.Max(longest, b.Remainder)
	}
	if len(res.Bars) != 2 || longest != 2000 {
		t.Errorf("want 2 bars with one remainder of 2000, got %+v", res.Bars)
	}
}

func TestLossesAndKerfAreCounted(t *testing.T) {
	// Usable 6000-20-150 = 5830. Three parts of 1940 need 5820 + 2 kerfs
	// of 5 = 5830: an exact fit. One more mm of kerf and only two fit.
	job := Job{
		Parts:    []Part{{ID: "A", Length: 1940, Quantity: 3}},
		Stock:    []Stock{{ID: "bar", Length: 6000}},
		Settings: Settings{Kerf: 5, StartLoss: 20, EndLoss: 150},
	}
	if res := mustRun(t, job); len(res.Bars) != 1 || res.Bars[0].Remainder != 0 {
		t.Errorf("want one bar with no remainder, got %+v", res.Bars)
	}
	job.Settings.Kerf = 6
	if res := mustRun(t, job); len(res.Bars) != 2 {
		t.Errorf("want two bars, got %+v", res.Bars)
	}
}

func TestPartTooLongIsLeftOut(t *testing.T) {
	job := Job{
		Parts: []Part{{ID: "ok", Length: 1000, Quantity: 2}, {ID: "huge", Length: 7000, Quantity: 3}},
		Stock: []Stock{{ID: "bar", Length: 6000}},
	}
	res := mustRun(t, job)
	if len(res.Unplaced) != 1 || res.Unplaced[0] != (Unplaced{PartID: "huge", Quantity: 3}) {
		t.Errorf("unplaced = %+v", res.Unplaced)
	}
}

func TestShelfRunsOut(t *testing.T) {
	job := Job{
		Parts: []Part{{ID: "A", Length: 2500, Quantity: 5}},
		Stock: []Stock{{ID: "bar", Length: 6000, Quantity: 2}},
	}
	res := mustRun(t, job)
	if len(res.Bars) != 2 || len(res.Unplaced) != 1 || res.Unplaced[0].Quantity != 1 {
		t.Errorf("want 2 bars and 1 unit left out, got %+v / %+v", res.Bars, res.Unplaced)
	}
}

func TestCancelledRunStillReturnsAPlan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	job := goodJob()
	calls := 0
	res, err := Run(ctx, job, func(Result) { calls++ })
	if err != nil {
		t.Fatal(err)
	}
	if bad := Verify(job, res); len(bad) != 0 {
		t.Fatalf("refused: %v", bad)
	}
	if calls == 0 {
		t.Error("progress was never called")
	}
}

func TestBadJobIsAnError(t *testing.T) {
	if _, err := Run(context.Background(), Job{}, nil); err == nil {
		t.Error("empty job accepted")
	}
}

func TestLargeJobIsQuick(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	job := Job{Stock: []Stock{{ID: "bar", Length: 6000}}, Settings: Settings{Kerf: 3, StartLoss: 20, EndLoss: 150, TimeLimitSec: 1}}
	total := 0.0
	for k := 0; k < 60; k++ {
		p := Part{ID: fmt.Sprintf("P%d", k), Length: float64(120 + rng.Intn(2800)), Quantity: 5 + rng.Intn(40)}
		total += p.Length * float64(p.Quantity)
		job.Parts = append(job.Parts, p)
	}
	res := mustRun(t, job)
	if res.ElapsedMs > 3000 {
		t.Errorf("took %d ms", res.ElapsedMs)
	}
	floor := int(math.Ceil(total / 5830))
	t.Logf("%d bars, %d is the floor by length, utilization %.1f%%", len(res.Bars), floor, res.Utilization*100)
	if len(res.Bars) > floor+floor/50+1 {
		t.Errorf("%d bars, far from the floor of %d", len(res.Bars), floor)
	}
}
