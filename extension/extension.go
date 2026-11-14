// Package extension is where extra features plug into EngTree.
//
// The free program registers nothing: the features simply do not appear.
// Another build (the paid one) imports this package, registers what it
// has and starts the same app. That is why there is no licence check, no
// flag and no "if pro" anywhere in this repository: the free version
// does not know that the other one exists.
package extension

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
)

// A CutPlanner turns a cutting job into a layout: which part goes where on
// which sheet. The job and the result travel as JSON, so this package does
// not need to know what a planner's settings look like.
type CutPlanner interface {
	// ID is the name used in the API and in the saved settings, e.g.
	// "freeform". It never changes once released.
	ID() string
	// Label is what the person reads in the menu, in English. The page
	// translates it when it knows the ID.
	Label() string
	// Plan runs the job. It reports every better layout it finds through
	// progress, and returns the best one when it is done or when ctx is
	// cancelled (a cancelled run is not an error).
	Plan(ctx context.Context, job json.RawMessage, progress func(json.RawMessage)) (json.RawMessage, error)
}

var (
	mu       sync.RWMutex
	planners = map[string]CutPlanner{}
)

// RegisterCutPlanner adds a planner. Registering the same ID twice replaces
// the first one. Call it from main, before the app starts.
func RegisterCutPlanner(p CutPlanner) {
	if p == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	planners[p.ID()] = p
}

// CutPlanners lists what is registered, by ID.
func CutPlanners() []CutPlanner {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]CutPlanner, 0, len(planners))
	for _, p := range planners {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// CutPlannerByID returns one planner.
func CutPlannerByID(id string) (CutPlanner, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := planners[id]
	return p, ok
}
