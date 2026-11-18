// Package cutplan gathers the cut planners that ship with EngTree and
// plugs them into the program through the extension package:
//
//   - "linear": bars and tubes (package linear);
//   - "rectangular": sheets on a panel saw (package rect).
//
// Each planner takes its Job as JSON and answers with its Result as JSON.
package cutplan

import (
	"context"
	"encoding/json"

	"github.com/ALEXKAKIUCHI/engtree-viewer/extension"
	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/cutplan/linear"
	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/cutplan/rect"
)

// IDs of the planners, as used in the API and in saved settings.
const (
	LinearID      = "linear"
	RectangularID = "rectangular"
)

// Register adds the built-in planners to the program. The app calls it
// once when it starts.
func Register() {
	extension.RegisterCutPlanner(planner[linear.Job, linear.Result]{LinearID, "Linear (bars and tubes)", linear.Run})
	extension.RegisterCutPlanner(planner[rect.Job, rect.Result]{RectangularID, "Rectangular (sheets)", rect.Run})
}

// planner wraps a typed Run function as an extension.CutPlanner.
type planner[J, R any] struct {
	id, label string
	run       func(context.Context, J, func(R)) (R, error)
}

func (p planner[J, R]) ID() string    { return p.id }
func (p planner[J, R]) Label() string { return p.label }

func (p planner[J, R]) Plan(ctx context.Context, job json.RawMessage, progress func(json.RawMessage)) (json.RawMessage, error) {
	var j J
	if err := json.Unmarshal(job, &j); err != nil {
		return nil, err
	}
	res, err := p.run(ctx, j, func(r R) {
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
}
