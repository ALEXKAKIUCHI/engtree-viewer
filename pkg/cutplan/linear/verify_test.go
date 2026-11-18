package linear

import (
	"strings"
	"testing"
)

// goodJob and goodResult are a small plan worked out by hand:
// a 6000 bar, 20 lost at the start, 150 at the end, 3 of kerf.
//
//	20..2020 A1 | 2023..4023 A2 | 4026..5526 B1 | remainder 5529..5850 = 321
func goodJob() Job {
	return Job{
		Parts:    []Part{{ID: "A", Length: 2000, Quantity: 2}, {ID: "B", Length: 1500, Quantity: 1}},
		Stock:    []Stock{{ID: "bar", Length: 6000}},
		Settings: Settings{Kerf: 3, StartLoss: 20, EndLoss: 150, MinOffcut: 300},
	}
}

func goodResult() Result {
	u := 5500.0 / 5830.0
	return Result{
		Bars: []Bar{{
			StockID: "bar", Length: 6000,
			Cuts: []Cut{
				{PartID: "A", Instance: 1, Start: 20, Length: 2000},
				{PartID: "A", Instance: 2, Start: 2023, Length: 2000},
				{PartID: "B", Instance: 1, Start: 4026, Length: 1500},
			},
			Remainder: 321, Reusable: true, Utilization: u,
		}},
		Utilization: u,
	}
}

func TestVerifyAcceptsGoodPlan(t *testing.T) {
	if bad := Verify(goodJob(), goodResult()); len(bad) != 0 {
		t.Fatalf("good plan refused: %v", bad)
	}
}

func TestVerifyAcceptsExactFit(t *testing.T) {
	// No losses, no kerf needed after the last part: 2 x 3000 in 6000
	// with a zero kerf, and 2997 + 3 + 3000 with a 3 mm kerf.
	job := Job{
		Parts:    []Part{{ID: "A", Length: 2997, Quantity: 1}, {ID: "B", Length: 3000, Quantity: 1}},
		Stock:    []Stock{{ID: "bar", Length: 6000, Quantity: 1}},
		Settings: Settings{Kerf: 3},
	}
	res := Result{
		Bars: []Bar{{StockID: "bar", Length: 6000, Utilization: 5997.0 / 6000,
			Cuts: []Cut{{PartID: "A", Instance: 1, Start: 0, Length: 2997}, {PartID: "B", Instance: 1, Start: 3000, Length: 3000}}}},
		Utilization: 5997.0 / 6000,
	}
	if bad := Verify(job, res); len(bad) != 0 {
		t.Fatalf("exact fit refused: %v", bad)
	}
}

func TestVerifyAcceptsLeftOutWhenStockRunsOut(t *testing.T) {
	job := Job{
		Parts:    []Part{{ID: "A", Length: 900, Quantity: 2}},
		Stock:    []Stock{{ID: "offcut", Length: 1000, Quantity: 1}},
		Settings: Settings{},
	}
	res := Result{
		Bars: []Bar{{StockID: "offcut", Length: 1000, Remainder: 100, Reusable: true, Utilization: 0.9,
			Cuts: []Cut{{PartID: "A", Instance: 1, Start: 0, Length: 900}}}},
		Unplaced:    []Unplaced{{PartID: "A", Quantity: 1}},
		Utilization: 0.9,
	}
	if bad := Verify(job, res); len(bad) != 0 {
		t.Fatalf("refused: %v", bad)
	}
}

// Each case breaks the good plan in one way; the verifier must say so.
func TestVerifyRefusesBrokenPlans(t *testing.T) {
	cases := []struct {
		name string
		job  func(*Job)
		res  func(*Result)
		want string
	}{
		{"inside start loss", nil, func(r *Result) { r.Bars[0].Cuts[0].Start = 10 }, "start loss"},
		{"no kerf between parts", nil, func(r *Result) { r.Bars[0].Cuts[1].Start = 2021 }, "kerf"},
		{"parts overlap", nil, func(r *Result) { r.Bars[0].Cuts[1].Start = 1500 }, "kerf"},
		{"past the usable end", func(j *Job) { j.Settings.EndLoss = 500 }, nil, "past the usable end"},
		{"wrong part length", nil, func(r *Result) { r.Bars[0].Cuts[2].Length = 1400 }, "should be"},
		{"unit cut twice", nil, func(r *Result) { r.Bars[0].Cuts[1].Instance = 1 }, "cut twice"},
		{"unit number out of range", nil, func(r *Result) { r.Bars[0].Cuts[1].Instance = 3 }, "no unit number"},
		{"part missing", nil, func(r *Result) { r.Bars[0].Cuts = r.Bars[0].Cuts[:2] }, "the job asks for"},
		{"unknown part", nil, func(r *Result) { r.Bars[0].Cuts[2].PartID = "Z" }, "not in the job"},
		{"unknown stock", nil, func(r *Result) { r.Bars[0].StockID = "other" }, "stock is not in the job"},
		{"wrong bar length", nil, func(r *Result) { r.Bars[0].Length = 6500 }, "differs from the stock length"},
		{"wrong remainder", nil, func(r *Result) { r.Bars[0].Remainder = 324 }, "remainder reported"},
		{"scrap marked reusable", func(j *Job) { j.Settings.MinOffcut = 400 }, nil, "reusable"},
		{"offcut marked scrap", nil, func(r *Result) { r.Bars[0].Reusable = false }, "reusable"},
		{"wrong bar utilization", nil, func(r *Result) { r.Bars[0].Utilization = 0.5 }, "utilization"},
		{"wrong total utilization", nil, func(r *Result) { r.Utilization = 1 }, "utilization"},
		{"empty bar", nil, func(r *Result) { r.Bars = append(r.Bars, Bar{StockID: "bar", Length: 6000}) }, "no parts on it"},
		{"more bars than the shelf has", func(j *Job) { j.Stock[0].Quantity = 1 }, func(r *Result) {
			r.Bars[0].Cuts = r.Bars[0].Cuts[:2]
			r.Bars = append(r.Bars, Bar{StockID: "bar", Length: 6000, Cuts: []Cut{{PartID: "B", Instance: 1, Start: 20, Length: 1500}}})
		}, "on the shelf"},
		{"left out although it fits", nil, func(r *Result) {
			r.Bars[0].Cuts = r.Bars[0].Cuts[:2]
			r.Unplaced = []Unplaced{{PartID: "B", Quantity: 1}}
		}, "left out although it fits"},
		{"left out and cut", nil, func(r *Result) { r.Unplaced = []Unplaced{{PartID: "B", Quantity: 1}} }, "the job asks for"},
		{"bad job", func(j *Job) { j.Settings.Kerf = -1 }, nil, "negative"},
	}
	for _, c := range cases {
		job, res := goodJob(), goodResult()
		if c.job != nil {
			c.job(&job)
		}
		if c.res != nil {
			c.res(&res)
		}
		bad := Verify(job, res)
		if !strings.Contains(strings.Join(bad, "\n"), c.want) {
			t.Errorf("%s: want a complaint with %q, got %v", c.name, c.want, bad)
		}
	}
}

func TestValidate(t *testing.T) {
	bad := []func(*Job){
		func(j *Job) { j.Parts = nil },
		func(j *Job) { j.Stock = nil },
		func(j *Job) { j.Parts[0].Length = 0 },
		func(j *Job) { j.Parts[0].Quantity = 0 },
		func(j *Job) { j.Parts[1].ID = "A" },
		func(j *Job) { j.Stock[0].Length = -1 },
		func(j *Job) { j.Stock[0].Quantity = -1 },
		func(j *Job) { j.Settings.EndLoss = -5 },
	}
	for i, f := range bad {
		j := goodJob()
		f(&j)
		if j.Validate() == nil {
			t.Errorf("case %d: bad job accepted", i)
		}
	}
	if err := goodJob().Validate(); err != nil {
		t.Errorf("good job refused: %v", err)
	}
}
