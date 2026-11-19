// Package cutplan lists the cut planners of this module so that a program
// can offer them without knowing their types:
//
//   - "linear": bars and tubes (package linear);
//   - "rectangular": sheets on a panel saw (package rect).
//
// Each planner takes its Job as JSON and answers with its Result as JSON.
// The module depends on nothing but the standard library; a program plugs
// the planners into its own registry (EngTree does it in pkg/cutplan).
package cutplan

import (
	"context"
	"encoding/json"

	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/linear"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/rect"
)

// IDs of the planners, as used in APIs and in saved settings.
const (
	LinearID      = "linear"
	RectangularID = "rectangular"
)

// Planner is one planner seen through JSON.
type Planner struct {
	ID    string // never changes once released
	Label string // in English
	// Plan runs a job. It reports every better layout through progress
	// (which may be nil) and returns the best one when it is done or when
	// ctx is cancelled (a cancelled run is not an error).
	Plan func(ctx context.Context, job json.RawMessage, progress func(json.RawMessage)) (json.RawMessage, error)
	// Verify runs the planner's independent verifier over a result. An
	// empty list means the plan passed; JSON that does not decode is
	// reported as a problem.
	Verify func(job, result json.RawMessage) []string
}

// Planners lists the planners of this module, by ID.
func Planners() []Planner {
	return []Planner{
		wrap(LinearID, "Linear (bars and tubes)", linear.Run, linear.Verify),
		wrap(RectangularID, "Rectangular (sheets)", rect.Run, rect.Verify),
	}
}

// ByID returns one planner.
func ByID(id string) (Planner, bool) {
	for _, p := range Planners() {
		if p.ID == id {
			return p, true
		}
	}
	return Planner{}, false
}

// wrap turns typed Run and Verify functions into a Planner.
func wrap[J, R any](id, label string, run func(context.Context, J, func(R)) (R, error), verify func(J, R) []string) Planner {
	return Planner{
		ID:    id,
		Label: label,
		Plan: func(ctx context.Context, job json.RawMessage, progress func(json.RawMessage)) (json.RawMessage, error) {
			var j J
			if err := json.Unmarshal(job, &j); err != nil {
				return nil, err
			}
			res, err := run(ctx, j, func(r R) {
				if progress == nil {
					return
				}
				if b, err := json.Marshal(r); err == nil {
					progress(b)
				}
			})
			if err != nil {
				return nil, err
			}
			return json.Marshal(res)
		},
		Verify: func(job, result json.RawMessage) []string {
			var j J
			var r R
			if err := json.Unmarshal(job, &j); err != nil {
				return []string{err.Error()}
			}
			if err := json.Unmarshal(result, &r); err != nil {
				return []string{err.Error()}
			}
			if problems := verify(j, r); problems != nil {
				return problems
			}
			return []string{}
		},
	}
}
