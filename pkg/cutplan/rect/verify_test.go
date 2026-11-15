package rect

import (
	"strings"
	"testing"
)

// goodJob and goodResult are a small plan worked out by hand: a sheet of
// 2000 x 1000, 10 of trim, 4 of kerf, strips along the length.
//
//	strip 1, Y 10..510:   A#1 | A#2 | a stack of B#1 and B#2
//	strip 2, Y 514..814:  C#1 | D#1 (rotated)
func goodJob() Job {
	return Job{
		Parts: []Part{
			{ID: "A", Length: 800, Width: 500, Quantity: 2},
			{ID: "B", Length: 360, Width: 240, Quantity: 2},
			{ID: "C", Length: 600, Width: 300, Quantity: 1},
			{ID: "D", Length: 300, Width: 600, Quantity: 1, CanRotate: true},
		},
		Stock:    []Sheet{{ID: "sheet", Length: 2000, Width: 1000}},
		Settings: Settings{Kerf: 4, EdgeTrim: 10, MinOffcutLength: 300, MinOffcutWidth: 150},
	}
}

func goodResult() Result {
	u := (2*800*500 + 2*360*240 + 600*300 + 300*600) / (1980.0 * 980.0)
	return Result{
		Sheets: []SheetLayout{{
			StockID: "sheet", Length: 2000, Width: 1000, FirstCut: AlongLength,
			Placements: []Placement{
				{PartID: "A", Instance: 1, X: 10, Y: 10},
				{PartID: "A", Instance: 2, X: 814, Y: 10},
				{PartID: "B", Instance: 1, X: 1618, Y: 10},
				{PartID: "B", Instance: 2, X: 1618, Y: 254},
				{PartID: "C", Instance: 1, X: 10, Y: 514},
				{PartID: "D", Instance: 1, X: 614, Y: 514, Rotated: true},
			},
			Cuts: []Cut{
				{Stage: 1, Along: AlongLength, Pos: 510, From: 10, To: 1990},
				{Stage: 1, Along: AlongLength, Pos: 814, From: 10, To: 1990},
				{Stage: 2, Along: AlongWidth, Pos: 810, From: 10, To: 510},
				{Stage: 2, Along: AlongWidth, Pos: 1614, From: 10, To: 510},
				{Stage: 2, Along: AlongWidth, Pos: 1978, From: 10, To: 510},
				{Stage: 3, Along: AlongLength, Pos: 250, From: 1618, To: 1978},
				{Stage: 3, Along: AlongLength, Pos: 494, From: 1618, To: 1978},
				{Stage: 2, Along: AlongWidth, Pos: 610, From: 514, To: 814},
				{Stage: 2, Along: AlongWidth, Pos: 1214, From: 514, To: 814},
			},
			Offcuts: []Offcut{
				{X: 1982, Y: 10, Length: 8, Width: 500},
				{X: 1618, Y: 498, Length: 360, Width: 12},
				{X: 1218, Y: 514, Length: 772, Width: 300, Reusable: true},
				{X: 10, Y: 818, Length: 1980, Width: 172, Reusable: true},
			},
			Utilization: u,
		}},
		Utilization: u,
	}
}

func TestVerifyAcceptsGoodPlan(t *testing.T) {
	if bad := Verify(goodJob(), goodResult()); len(bad) != 0 {
		t.Fatalf("good plan refused:\n%s", strings.Join(bad, "\n"))
	}
}

// One part the size of the whole usable area needs no cut at all.
func TestVerifyAcceptsFullSheetPart(t *testing.T) {
	job := Job{
		Parts:    []Part{{ID: "A", Length: 1980, Width: 980, Quantity: 1}},
		Stock:    []Sheet{{ID: "sheet", Length: 2000, Width: 1000, Quantity: 1}},
		Settings: Settings{Kerf: 4, EdgeTrim: 10, Stages: 2},
	}
	res := Result{
		Sheets: []SheetLayout{{StockID: "sheet", Length: 2000, Width: 1000, FirstCut: AlongWidth, Utilization: 1,
			Placements: []Placement{{PartID: "A", Instance: 1, X: 10, Y: 10}}}},
		Utilization: 1,
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
		{"inside the trim", nil, func(r *Result) { r.Sheets[0].Placements[0].X = 5 }, "outside the usable area"},
		{"past the far edge", func(j *Job) { j.Stock[0].Length = 1900 }, func(r *Result) { r.Sheets[0].Length = 1900 }, "outside the usable area"},
		{"parts overlap", nil, func(r *Result) { r.Sheets[0].Placements[1].X = 700 }, "apart"},
		{"no kerf between parts", nil, func(r *Result) { r.Sheets[0].Placements[1].X = 812 }, "apart"},
		{"rotated against the grain", nil, func(r *Result) { r.Sheets[0].Placements[4].Rotated = true }, "must follow the grain"},
		{"not rotated, so wrong size", nil, func(r *Result) { r.Sheets[0].Placements[5].Rotated = false }, "goes through part D#1"},
		{"unit cut twice", nil, func(r *Result) { r.Sheets[0].Placements[1].Instance = 1 }, "cut twice"},
		{"unit number out of range", nil, func(r *Result) { r.Sheets[0].Placements[1].Instance = 3 }, "no unit number"},
		{"part missing", nil, func(r *Result) { r.Sheets[0].Placements = r.Sheets[0].Placements[:5] }, "the job asks for"},
		{"unknown part", nil, func(r *Result) { r.Sheets[0].Placements[4].PartID = "Z" }, "not in the job"},
		{"unknown stock", nil, func(r *Result) { r.Sheets[0].StockID = "other" }, "stock is not in the job"},
		{"wrong sheet size", nil, func(r *Result) { r.Sheets[0].Width = 1200 }, "differs from the stock size"},
		{"needs three stages, saw has two", func(j *Job) { j.Settings.Stages = 2 }, nil, "cannot be freed"},
		{"first cut fixed the other way", func(j *Job) { j.Settings.FirstCut = AlongWidth }, nil, "the job fixes"},
		{"first cut is not a direction", nil, func(r *Result) { r.Sheets[0].FirstCut = "diagonal" }, "not a direction"},
		{"cut through a part", nil, func(r *Result) { r.Sheets[0].Cuts[2].Pos = 400 }, "goes through part A#1"},
		{"cut eats the edge of a part", nil, func(r *Result) { r.Sheets[0].Cuts[2].Pos = 808 }, "goes through part"},
		{"cut missing", nil, func(r *Result) { r.Sheets[0].Cuts = r.Sheets[0].Cuts[:8] }, "no cut frees the right side of part D#1"},
		{"cut too short", nil, func(r *Result) { r.Sheets[0].Cuts[0].To = 1000 }, "no cut frees the top side of part A#2"},
		{"cut of the wrong stage", nil, func(r *Result) { r.Sheets[0].Cuts[5].Stage = 2 }, "should be the"},
		{"cut of a stage the saw lacks", nil, func(r *Result) { r.Sheets[0].Cuts[5].Stage = 4 }, "the saw has"},
		{"cut outside the sheet", nil, func(r *Result) { r.Sheets[0].Cuts[1].To = 2500 }, "outside the usable area"},
		{"offcut over a part", nil, func(r *Result) { r.Sheets[0].Offcuts[2].X = 1000; r.Sheets[0].Offcuts[2].Length = 990 }, "from part D#1"},
		{"offcut outside the sheet", nil, func(r *Result) { r.Sheets[0].Offcuts[3].Width = 300 }, "outside the usable area"},
		{"offcuts overlap", nil, func(r *Result) {
			r.Sheets[0].Offcuts[3].Y = 600
			r.Sheets[0].Offcuts[3].X = 1300
			r.Sheets[0].Offcuts[3].Length = 600
		}, "overlaps another offcut"},
		{"scrap marked reusable", nil, func(r *Result) { r.Sheets[0].Offcuts[0].Reusable = true }, "reusable"},
		{"offcut marked scrap", nil, func(r *Result) { r.Sheets[0].Offcuts[3].Reusable = false }, "reusable"},
		{"wrong sheet utilization", nil, func(r *Result) { r.Sheets[0].Utilization = 0.5 }, "utilization"},
		{"wrong total utilization", nil, func(r *Result) { r.Utilization = 1 }, "utilization"},
		{"empty sheet", nil, func(r *Result) {
			r.Sheets = append(r.Sheets, SheetLayout{StockID: "sheet", Length: 2000, Width: 1000, FirstCut: AlongLength})
		}, "no parts on it"},
		{"more sheets than the shelf has", func(j *Job) { j.Stock[0].Quantity = 1 }, func(r *Result) {
			r.Sheets = append(r.Sheets, r.Sheets[0])
		}, "on the shelf"},
		{"left out although it fits", nil, func(r *Result) {
			r.Sheets[0].Placements = r.Sheets[0].Placements[:5]
			r.Unplaced = []Unplaced{{PartID: "D", Quantity: 1}}
		}, "left out although it fits"},
		{"bad job", func(j *Job) { j.Settings.Stages = 5 }, nil, "stages"},
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

// A pinwheel: four parts around a hole. No edge-to-edge cut can start it,
// however many stages there are. The parts themselves are fine.
func TestVerifyRefusesPinwheel(t *testing.T) {
	job := Job{
		Parts:    []Part{{ID: "P", Length: 200, Width: 100, Quantity: 4, CanRotate: true}},
		Stock:    []Sheet{{ID: "sheet", Length: 300, Width: 300}},
		Settings: Settings{},
	}
	res := Result{
		Sheets: []SheetLayout{{StockID: "sheet", Length: 300, Width: 300, FirstCut: AlongLength, Utilization: 80000.0 / 90000,
			Placements: []Placement{
				{PartID: "P", Instance: 1, X: 0, Y: 0},
				{PartID: "P", Instance: 2, X: 200, Y: 0, Rotated: true},
				{PartID: "P", Instance: 3, X: 100, Y: 200},
				{PartID: "P", Instance: 4, X: 0, Y: 100, Rotated: true},
			}}},
		Utilization: 80000.0 / 90000,
	}
	bad := strings.Join(Verify(job, res), "\n")
	if !strings.Contains(bad, "cannot be freed") {
		t.Errorf("pinwheel accepted: %v", bad)
	}
	if strings.Contains(bad, "apart") || strings.Contains(bad, "outside") {
		t.Errorf("the pinwheel parts are fine, yet: %v", bad)
	}
}

func TestValidate(t *testing.T) {
	bad := []func(*Job){
		func(j *Job) { j.Parts = nil },
		func(j *Job) { j.Stock = nil },
		func(j *Job) { j.Parts[0].Width = 0 },
		func(j *Job) { j.Parts[0].Quantity = 0 },
		func(j *Job) { j.Parts[1].ID = "A" },
		func(j *Job) { j.Stock[0].Width = 15 },
		func(j *Job) { j.Stock[0].Quantity = -1 },
		func(j *Job) { j.Settings.Kerf = -1 },
		func(j *Job) { j.Settings.Stages = 1 },
		func(j *Job) { j.Settings.FirstCut = "x" },
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
