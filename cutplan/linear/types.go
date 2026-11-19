// Package linear plans cuts along one dimension: parts of a given length
// taken out of bars or tubes.
//
// A Job goes in, a Result comes out. Everything is in millimetres. The
// package knows nothing about the BOM, the screen or files.
//
// How a bar is laid out, from its start:
//
//	| StartLoss | part | kerf | part | kerf | ... | remainder | EndLoss |
//
// StartLoss and EndLoss are the lengths the machine cannot use at each end
// (a trim cut on a saw, the length held by the chuck on a tube laser). They
// may differ. The first part starts right after StartLoss. Two neighbouring
// parts are one kerf apart. What is left before EndLoss, minus one kerf for
// the cut that frees it, is the remainder.
package linear

import (
	"errors"
	"fmt"
)

// Eps is the tolerance for comparing lengths, in mm.
const Eps = 1e-6

type Job struct {
	Parts    []Part   `json:"parts"`
	Stock    []Stock  `json:"stock"` // in order of use: offcuts first, full bars last
	Settings Settings `json:"settings"`
}

type Part struct {
	ID       string  `json:"id"`
	Length   float64 `json:"length"` // mm
	Quantity int     `json:"quantity"`
}

// Stock is one bar length on the shelf.
type Stock struct {
	ID       string  `json:"id"`
	Length   float64 `json:"length"`   // mm
	Quantity int     `json:"quantity"` // 0 = unlimited
}

type Settings struct {
	Kerf      float64 `json:"kerf"`       // width of the cut (mm)
	StartLoss float64 `json:"start_loss"` // unusable length at the start of a bar (mm)
	EndLoss   float64 `json:"end_loss"`   // unusable length at the end of a bar (mm)
	// MinOffcut is the shortest remainder worth keeping. A shorter one is
	// scrap.
	MinOffcut    float64 `json:"min_offcut"`
	TimeLimitSec int     `json:"time_limit_sec"` // 0 = the default search effort
}

type Result struct {
	Bars     []Bar      `json:"bars"`
	Unplaced []Unplaced `json:"unplaced,omitempty"`
	// Utilization is the length of the parts over the usable length of the
	// bars used, 0..1.
	Utilization float64  `json:"utilization"`
	Warnings    []string `json:"warnings,omitempty"`
	ElapsedMs   int64    `json:"elapsed_ms"`
}

// Bar is one bar taken from the stock and what is cut from it.
type Bar struct {
	StockID string  `json:"stock_id"`
	Length  float64 `json:"length"`
	Cuts    []Cut   `json:"cuts"` // from the start of the bar
	// Remainder is the free length after the last part, already without
	// the kerf that frees it and without EndLoss.
	Remainder float64 `json:"remainder"`
	// Reusable says the remainder is at least Settings.MinOffcut: it goes
	// back to the shelf instead of the scrap bin.
	Reusable    bool    `json:"reusable"`
	Utilization float64 `json:"utilization"`
}

// Cut is one part on a bar.
type Cut struct {
	PartID   string  `json:"part_id"`
	Instance int     `json:"instance"` // 1..Quantity
	Start    float64 `json:"start"`    // from the start of the bar (mm)
	Length   float64 `json:"length"`
}

// Unplaced says how many units of a part found no bar.
type Unplaced struct {
	PartID   string `json:"part_id"`
	Quantity int    `json:"quantity"`
}

// Validate reports what makes a job impossible to plan.
func (j Job) Validate() error {
	s := j.Settings
	if s.Kerf < 0 || s.StartLoss < 0 || s.EndLoss < 0 || s.MinOffcut < 0 {
		return errors.New("linear: kerf, losses and minimum offcut cannot be negative")
	}
	if len(j.Parts) == 0 {
		return errors.New("linear: no parts")
	}
	if len(j.Stock) == 0 {
		return errors.New("linear: no stock")
	}
	seen := map[string]bool{}
	for _, p := range j.Parts {
		if p.ID == "" {
			return errors.New("linear: part without id")
		}
		if seen[p.ID] {
			return fmt.Errorf("linear: part %q listed twice", p.ID)
		}
		seen[p.ID] = true
		if !(p.Length > 0) {
			return fmt.Errorf("linear: part %q has no length", p.ID)
		}
		if p.Quantity <= 0 {
			return fmt.Errorf("linear: part %q has no quantity", p.ID)
		}
	}
	seen = map[string]bool{}
	for _, st := range j.Stock {
		if st.ID == "" {
			return errors.New("linear: stock without id")
		}
		if seen[st.ID] {
			return fmt.Errorf("linear: stock %q listed twice", st.ID)
		}
		seen[st.ID] = true
		if !(st.Length > 0) {
			return fmt.Errorf("linear: stock %q has no length", st.ID)
		}
		if st.Quantity < 0 {
			return fmt.Errorf("linear: stock %q has a negative quantity", st.ID)
		}
	}
	return nil
}
