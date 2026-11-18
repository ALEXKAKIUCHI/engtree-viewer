// Package rect plans rectangular parts on sheets for a panel saw: every
// cut goes from one edge of the piece being cut to the other (a guillotine
// cut).
//
// A Job goes in, a Result comes out. Everything is in millimetres. The
// package knows nothing about the BOM, the screen or files.
//
// Coordinates: X runs along the sheet length, Y along its width, with
// (0,0) at a corner of the raw sheet. EdgeTrim is taken off all four
// sides; parts live in what is left, the usable area.
//
// The saw works in stages:
//
//  1. cuts across the whole sheet, which give strips;
//  2. cuts across each strip, the other way, which give parts or stacks;
//  3. cuts across each stack, the first way again, which give the parts
//     of a stack, or trim a part that is narrower than its strip.
//
// Every cut eats one kerf, so two neighbouring parts are at least one
// kerf apart.
package rect

import (
	"errors"
	"fmt"
)

// Eps is the tolerance for comparing lengths, in mm.
const Eps = 1e-6

// Direction a cut runs in.
const (
	AlongLength = "length" // parallel to the sheet length: a line of constant Y
	AlongWidth  = "width"  // parallel to the sheet width: a line of constant X
)

type Job struct {
	Parts    []Part   `json:"parts"`
	Stock    []Sheet  `json:"stock"` // in order of use: offcuts first, full sheets last
	Settings Settings `json:"settings"`
}

type Part struct {
	ID       string  `json:"id"`
	Length   float64 `json:"length"` // mm, along the grain when the material has one
	Width    float64 `json:"width"`  // mm
	Quantity int     `json:"quantity"`
	// CanRotate lets the part lie with its length along the sheet width.
	// False keeps its length along the sheet length (the grain).
	CanRotate bool `json:"can_rotate"`
}

// Sheet is one sheet size on the shelf.
type Sheet struct {
	ID       string  `json:"id"`
	Length   float64 `json:"length"`
	Width    float64 `json:"width"`
	Quantity int     `json:"quantity"` // 0 = unlimited
}

type Settings struct {
	Kerf     float64 `json:"kerf"`      // width of the cut (mm)
	EdgeTrim float64 `json:"edge_trim"` // taken off each of the four sides (mm)
	// Stages is how many stages the saw may use: 2 or 3. 0 means 3.
	Stages int `json:"stages"`
	// FirstCut fixes the direction of the first stage: AlongLength or
	// AlongWidth. Empty lets the planner choose per sheet.
	FirstCut string `json:"first_cut"`
	// A remainder is worth keeping when it holds a rectangle of
	// MinOffcutLength x MinOffcutWidth, either way round.
	MinOffcutLength float64 `json:"min_offcut_length"`
	MinOffcutWidth  float64 `json:"min_offcut_width"`
	TimeLimitSec    int     `json:"time_limit_sec"` // 0 = the default search effort
}

// MaxStages is Stages with its default applied.
func (s Settings) MaxStages() int {
	if s.Stages == 0 {
		return 3
	}
	return s.Stages
}

type Result struct {
	Sheets   []SheetLayout `json:"sheets"`
	Unplaced []Unplaced    `json:"unplaced,omitempty"`
	// Utilization is the area of the parts over the usable area of the
	// sheets used, 0..1.
	Utilization float64  `json:"utilization"`
	Warnings    []string `json:"warnings,omitempty"`
	ElapsedMs   int64    `json:"elapsed_ms"`
}

// SheetLayout is one sheet taken from the stock and what is cut from it.
type SheetLayout struct {
	StockID     string      `json:"stock_id"`
	Length      float64     `json:"length"`
	Width       float64     `json:"width"`
	FirstCut    string      `json:"first_cut"` // direction of the stage 1 cuts
	Placements  []Placement `json:"placements"`
	Cuts        []Cut       `json:"cuts"`    // in the order the operator makes them
	Offcuts     []Offcut    `json:"offcuts"` // what is left, piece by piece
	Utilization float64     `json:"utilization"`
}

// Placement is one part on a sheet. X, Y is its corner nearest the
// origin. It takes Length along X and Width along Y, or the other way
// round when Rotated.
type Placement struct {
	PartID   string  `json:"part_id"`
	Instance int     `json:"instance"` // 1..Quantity
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Rotated  bool    `json:"rotated,omitempty"`
}

// Cut is one pass of the saw. It eats the material from Pos to Pos+Kerf,
// measured across the cut, and runs from From to To along itself.
//
// The four trim cuts that take EdgeTrim off the raw sheet are not listed.
type Cut struct {
	Stage int     `json:"stage"` // 1, 2 or 3
	Along string  `json:"along"` // AlongLength: Pos is a Y, From/To are X. AlongWidth: the reverse.
	Pos   float64 `json:"pos"`
	From  float64 `json:"from"`
	To    float64 `json:"to"`
}

// Offcut is a rectangle left over after the cuts.
type Offcut struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Length float64 `json:"length"` // along X
	Width  float64 `json:"width"`  // along Y
	// Reusable says it is at least the minimum offcut: it goes back to
	// the shelf instead of the scrap bin.
	Reusable bool `json:"reusable"`
}

// Unplaced says how many units of a part found no sheet.
type Unplaced struct {
	PartID   string `json:"part_id"`
	Quantity int    `json:"quantity"`
}

// Validate reports what makes a job impossible to plan.
func (j Job) Validate() error {
	s := j.Settings
	if s.Kerf < 0 || s.EdgeTrim < 0 || s.MinOffcutLength < 0 || s.MinOffcutWidth < 0 {
		return errors.New("rect: kerf, edge trim and minimum offcut cannot be negative")
	}
	if s.Stages != 0 && s.Stages != 2 && s.Stages != 3 {
		return errors.New("rect: stages must be 2 or 3")
	}
	if s.FirstCut != "" && s.FirstCut != AlongLength && s.FirstCut != AlongWidth {
		return fmt.Errorf("rect: first cut %q is neither %q nor %q", s.FirstCut, AlongLength, AlongWidth)
	}
	if len(j.Parts) == 0 {
		return errors.New("rect: no parts")
	}
	if len(j.Stock) == 0 {
		return errors.New("rect: no stock")
	}
	seen := map[string]bool{}
	for _, p := range j.Parts {
		if p.ID == "" {
			return errors.New("rect: part without id")
		}
		if seen[p.ID] {
			return fmt.Errorf("rect: part %q listed twice", p.ID)
		}
		seen[p.ID] = true
		if !(p.Length > 0) || !(p.Width > 0) {
			return fmt.Errorf("rect: part %q needs a length and a width", p.ID)
		}
		if p.Quantity <= 0 {
			return fmt.Errorf("rect: part %q has no quantity", p.ID)
		}
	}
	seen = map[string]bool{}
	for _, st := range j.Stock {
		if st.ID == "" {
			return errors.New("rect: stock without id")
		}
		if seen[st.ID] {
			return fmt.Errorf("rect: stock %q listed twice", st.ID)
		}
		seen[st.ID] = true
		if !(st.Length-2*s.EdgeTrim > 0) || !(st.Width-2*s.EdgeTrim > 0) {
			return fmt.Errorf("rect: stock %q has no usable area", st.ID)
		}
		if st.Quantity < 0 {
			return fmt.Errorf("rect: stock %q has a negative quantity", st.ID)
		}
	}
	return nil
}
