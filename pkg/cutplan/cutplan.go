// Package cutplan plugs the cut planners of the cutplan module
// (github.com/ALEXKAKIUCHI/engtree-viewer/cutplan) into the program
// through the extension package:
//
//   - "linear": bars and tubes;
//   - "rectangular": sheets on a panel saw.
//
// The planners themselves live in that module and know nothing about
// EngTree; this package is only the adapter.
package cutplan

import (
	"context"
	"encoding/json"

	planners "github.com/ALEXKAKIUCHI/engtree-viewer/cutplan"
	"github.com/ALEXKAKIUCHI/engtree-viewer/extension"
)

// IDs of the planners, as used in the API and in saved settings.
const (
	LinearID      = planners.LinearID
	RectangularID = planners.RectangularID
)

// Register adds the built-in planners to the program. The app calls it
// once when it starts.
func Register() {
	for _, p := range planners.Planners() {
		extension.RegisterCutPlanner(adapter{p})
	}
}

// adapter shows a planners.Planner as an extension.CutPlanner.
type adapter struct{ p planners.Planner }

func (a adapter) ID() string    { return a.p.ID }
func (a adapter) Label() string { return a.p.Label }

func (a adapter) Plan(ctx context.Context, job json.RawMessage, progress func(json.RawMessage)) (json.RawMessage, error) {
	return a.p.Plan(ctx, job, progress)
}
