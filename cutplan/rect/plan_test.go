package rect

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
)

func mustRun(t *testing.T, job Job) Result {
	t.Helper()
	res, err := Run(context.Background(), job, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if bad := Verify(job, res); len(bad) != 0 {
		t.Fatalf("plan refused by the verifier:\n%s\njob: %+v\nplan: %+v", strings.Join(bad, "\n"), job, res)
	}
	return res
}

func count(res Result, stock string) int {
	n := 0
	for _, s := range res.Sheets {
		if s.StockID == stock {
			n++
		}
	}
	return n
}

// Every plan for a random job must pass the verifier.
func TestRandomJobsPassTheVerifier(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	for i := 0; i < 80; i++ {
		job := Job{Settings: Settings{
			Kerf:            float64(rng.Intn(3)) * 2.2,
			EdgeTrim:        float64(rng.Intn(3)) * 5,
			Stages:          []int{0, 2, 3}[rng.Intn(3)],
			FirstCut:        []string{"", AlongLength, AlongWidth}[rng.Intn(3)],
			MinOffcutLength: float64(rng.Intn(3)) * 200,
			MinOffcutWidth:  float64(rng.Intn(3)) * 100,
			TimeLimitSec:    1,
		}}
		for k, n := 0, 1+rng.Intn(10); k < n; k++ {
			job.Parts = append(job.Parts, Part{
				ID:     fmt.Sprintf("P%d", k),
				Length: float64(60+rng.Intn(1500)) + float64(rng.Intn(2))*0.5, Width: float64(60 + rng.Intn(900)),
				Quantity: 1 + rng.Intn(8), CanRotate: rng.Intn(2) == 0})
		}
		for k, n := 0, rng.Intn(3); k < n; k++ {
			job.Stock = append(job.Stock, Sheet{ID: fmt.Sprintf("offcut%d", k),
				Length: float64(500 + rng.Intn(1500)), Width: float64(300 + rng.Intn(900)), Quantity: 1 + rng.Intn(2)})
		}
		full := Sheet{ID: "sheet", Length: 2750, Width: 1830}
		if rng.Intn(4) == 0 {
			full.Quantity = 1 + rng.Intn(2) // sometimes the shelf runs out
		}
		job.Stock = append(job.Stock, full)
		mustRun(t, job)
	}
}

// Jobs whose answer is known: the parts tile the sheet exactly.
func TestKnownAnswers(t *testing.T) {
	cases := []struct {
		name   string
		job    Job
		sheets int
		full   bool // utilization of exactly 1
	}{
		{"four quarters per sheet", Job{
			Parts: []Part{{ID: "A", Length: 1000, Width: 500, Quantity: 8}},
			Stock: []Sheet{{ID: "s", Length: 2000, Width: 1000}},
		}, 2, true},
		{"kerf and trim counted: an exact fit", Job{
			Parts:    []Part{{ID: "A", Length: 1000, Width: 500, Quantity: 4}},
			Stock:    []Sheet{{ID: "s", Length: 2024, Width: 1024}},
			Settings: Settings{Kerf: 4, EdgeTrim: 10},
		}, 1, false},
		{"one mm short: a second sheet", Job{
			Parts:    []Part{{ID: "A", Length: 1000, Width: 500, Quantity: 4}},
			Stock:    []Sheet{{ID: "s", Length: 2023, Width: 1024}},
			Settings: Settings{Kerf: 4, EdgeTrim: 10},
		}, 2, false},
		{"strips of different heights", Job{
			Parts: []Part{{ID: "A", Length: 2750, Width: 600, Quantity: 2}, {ID: "B", Length: 1375, Width: 630, Quantity: 2}},
			Stock: []Sheet{{ID: "s", Length: 2750, Width: 1830}},
		}, 1, true},
		{"a stack beside a tall part", Job{
			Parts: []Part{{ID: "A", Length: 1000, Width: 800, Quantity: 1}, {ID: "B", Length: 500, Width: 400, Quantity: 2}},
			Stock: []Sheet{{ID: "s", Length: 1500, Width: 800}},
		}, 1, true},
		{"rotation makes it fit", Job{
			Parts: []Part{{ID: "A", Length: 1000, Width: 2000, Quantity: 1, CanRotate: true}},
			Stock: []Sheet{{ID: "s", Length: 2000, Width: 1000}},
		}, 1, true},
		{"strips across the width are the only way", Job{
			Parts: []Part{{ID: "A", Length: 700, Width: 1000, Quantity: 2}, {ID: "B", Length: 600, Width: 500, Quantity: 2}},
			Stock: []Sheet{{ID: "s", Length: 2000, Width: 1000}},
		}, 1, true},
	}
	for _, c := range cases {
		res := mustRun(t, c.job)
		if len(res.Sheets) != c.sheets || len(res.Unplaced) != 0 {
			t.Errorf("%s: %d sheets (%d parts left out), want %d", c.name, len(res.Sheets), len(res.Unplaced), c.sheets)
		}
		if c.full && math.Abs(res.Utilization-1) > 1e-9 {
			t.Errorf("%s: utilization %g, want 1", c.name, res.Utilization)
		}
	}
}

// With two stages a part cannot be trimmed or stacked. Strips along the
// length, 800 high: the two small parts fit as a stack with three stages
// and need a second sheet with two.
func TestTwoStagesCannotStack(t *testing.T) {
	job := Job{
		Parts:    []Part{{ID: "A", Length: 1000, Width: 800, Quantity: 1}, {ID: "B", Length: 500, Width: 400, Quantity: 2}},
		Stock:    []Sheet{{ID: "s", Length: 1500, Width: 800}},
		Settings: Settings{Stages: 2, FirstCut: AlongLength},
	}
	if res := mustRun(t, job); len(res.Sheets) != 2 {
		t.Errorf("two stages: %d sheets, want 2", len(res.Sheets))
	}
	job.Settings.Stages = 3
	if res := mustRun(t, job); len(res.Sheets) != 1 {
		t.Errorf("three stages: %d sheets, want 1", len(res.Sheets))
	}
}

func TestGrainIsKept(t *testing.T) {
	job := Job{
		Parts: []Part{{ID: "A", Length: 1000, Width: 2000, Quantity: 1}},
		Stock: []Sheet{{ID: "s", Length: 2000, Width: 1000}},
	}
	res := mustRun(t, job)
	if len(res.Sheets) != 0 || len(res.Unplaced) != 1 {
		t.Errorf("a part that only fits across the grain was placed: %+v", res)
	}
}

func TestFirstCutIsObeyed(t *testing.T) {
	for _, first := range []string{AlongLength, AlongWidth} {
		job := goodJob()
		job.Settings.FirstCut = first
		res := mustRun(t, job) // the verifier checks the direction
		for _, s := range res.Sheets {
			if s.FirstCut != first {
				t.Errorf("first cut %s, want %s", s.FirstCut, first)
			}
		}
	}
}

// One small part on a big sheet: cutting a strip across the width leaves
// 1500 x 1000 whole; a strip along the length would leave two halves.
func TestPrefersOneBigOffcut(t *testing.T) {
	job := Job{
		Parts:    []Part{{ID: "A", Length: 500, Width: 500, Quantity: 1}},
		Stock:    []Sheet{{ID: "s", Length: 2000, Width: 1000}},
		Settings: Settings{MinOffcutLength: 300, MinOffcutWidth: 300},
	}
	res := mustRun(t, job)
	best := 0.0
	for _, o := range res.Sheets[0].Offcuts {
		if o.Reusable {
			best = math.Max(best, o.Length*o.Width)
		}
	}
	if best != 1500*1000 {
		t.Errorf("largest offcut %g mm2, want 1500 x 1000. offcuts: %+v", best, res.Sheets[0].Offcuts)
	}
}

func TestOffcutsGoFirst(t *testing.T) {
	job := Job{
		Parts: []Part{{ID: "A", Length: 600, Width: 400, Quantity: 4}},
		Stock: []Sheet{{ID: "offcut", Length: 1300, Width: 900, Quantity: 1}, {ID: "sheet", Length: 2750, Width: 1830}},
	}
	res := mustRun(t, job)
	if count(res, "offcut") != 1 || count(res, "sheet") != 0 {
		t.Errorf("want the offcut and no full sheet, got %d and %d", count(res, "offcut"), count(res, "sheet"))
	}
}

// The full sheet has to be opened anyway and the small part fits on it:
// the offcut stays on the shelf.
func TestOffcutIsNotSpentForNothing(t *testing.T) {
	job := Job{
		Parts: []Part{{ID: "big", Length: 2000, Width: 1500, Quantity: 1}, {ID: "small", Length: 400, Width: 300, Quantity: 1}},
		Stock: []Sheet{{ID: "offcut", Length: 500, Width: 400, Quantity: 1}, {ID: "sheet", Length: 2750, Width: 1830}},
	}
	res := mustRun(t, job)
	if count(res, "offcut") != 0 || count(res, "sheet") != 1 {
		t.Errorf("want one full sheet and the offcut kept, got %d and %d", count(res, "sheet"), count(res, "offcut"))
	}
}

func TestPartTooBigIsLeftOut(t *testing.T) {
	job := Job{
		Parts: []Part{{ID: "ok", Length: 500, Width: 500, Quantity: 2}, {ID: "huge", Length: 3000, Width: 500, Quantity: 2, CanRotate: true}},
		Stock: []Sheet{{ID: "s", Length: 2750, Width: 1830}},
	}
	res := mustRun(t, job)
	if len(res.Unplaced) != 1 || res.Unplaced[0] != (Unplaced{PartID: "huge", Quantity: 2}) {
		t.Errorf("unplaced = %+v", res.Unplaced)
	}
}

func TestShelfRunsOut(t *testing.T) {
	job := Job{
		Parts: []Part{{ID: "A", Length: 1500, Width: 1000, Quantity: 3}},
		Stock: []Sheet{{ID: "s", Length: 2750, Width: 1830, Quantity: 2}},
	}
	res := mustRun(t, job)
	if len(res.Sheets) != 2 || len(res.Unplaced) != 1 || res.Unplaced[0].Quantity != 1 {
		t.Errorf("want 2 sheets and 1 unit left out, got %d / %+v", len(res.Sheets), res.Unplaced)
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

// The test page example sent four times over: sheets cut alike repeat and
// come one after the other.
func TestRepeatsSheetLayouts(t *testing.T) {
	job := Job{
		Parts: []Part{
			{ID: "Lateral", Length: 2100, Width: 550, Quantity: 16},
			{ID: "Base", Length: 1164, Width: 550, Quantity: 16, CanRotate: true},
			{ID: "Prateleira", Length: 1164, Width: 520, Quantity: 40, CanRotate: true},
			{ID: "Porta", Length: 2090, Width: 395, Quantity: 24},
			{ID: "Fundo", Length: 2100, Width: 1190, Quantity: 8, CanRotate: true},
			{ID: "Rodape", Length: 1164, Width: 100, Quantity: 16, CanRotate: true},
		},
		Stock:    []Sheet{{ID: "s", Length: 2750, Width: 1830}},
		Settings: Settings{Kerf: 4, EdgeTrim: 10},
	}
	res := mustRun(t, job)
	key := func(sh SheetLayout) string {
		k := sh.FirstCut
		for _, p := range sh.Placements {
			k += fmt.Sprintf("|%s,%g,%g,%v", p.PartID, p.X, p.Y, p.Rotated)
		}
		return k
	}
	keys := map[string]bool{}
	prev := ""
	for i, sh := range res.Sheets {
		k := key(sh)
		if keys[k] && k != prev {
			t.Errorf("sheet %d repeats an earlier layout but is not next to it", i+1)
		}
		keys[k], prev = true, k
	}
	t.Logf("%d sheets, %d different layouts, utilization %.1f%%", len(res.Sheets), len(keys), res.Utilization*100)
	if len(keys)*2 > len(res.Sheets) {
		t.Errorf("%d different layouts in %d sheets", len(keys), len(res.Sheets))
	}
}

// A furniture-sized job: how full the sheets get, and how long it takes.
func TestLargeJob(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	job := Job{
		Stock:    []Sheet{{ID: "s", Length: 2750, Width: 1830}},
		Settings: Settings{Kerf: 4, EdgeTrim: 10, TimeLimitSec: 2},
	}
	area := 0.0
	for k := 0; k < 40; k++ {
		p := Part{ID: fmt.Sprintf("P%d", k), Length: float64(200 + rng.Intn(1800)), Width: float64(80 + rng.Intn(620)),
			Quantity: 2 + rng.Intn(14), CanRotate: k%2 == 0}
		area += p.Length * p.Width * float64(p.Quantity)
		job.Parts = append(job.Parts, p)
	}
	res := mustRun(t, job)
	floor := int(math.Ceil(area / (2730 * 1810)))
	t.Logf("%d sheets, %d is the floor by area, utilization %.1f%%, %d ms", len(res.Sheets), floor, res.Utilization*100, res.ElapsedMs)
	if res.ElapsedMs > 4000 {
		t.Errorf("took %d ms", res.ElapsedMs)
	}
	if float64(len(res.Sheets)) > float64(floor)*1.2+1 {
		t.Errorf("%d sheets, far from the floor of %d", len(res.Sheets), floor)
	}
}
