package app

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/linear"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/rect"
)

// CutMachine holds the machine values typed on the cut plan screen. They
// belong to this computer and are offered again next time.
type CutMachine struct {
	// Panel saw.
	SawKerf  float64 `json:"saw_kerf"`
	EdgeTrim float64 `json:"edge_trim"`
	Stages   int     `json:"stages"`    // 2 or 3
	FirstCut string  `json:"first_cut"` // "", rect.AlongLength or rect.AlongWidth

	// Bars and tubes.
	BarKerf   float64 `json:"bar_kerf"`
	StartLoss float64 `json:"start_loss"`
	EndLoss   float64 `json:"end_loss"`

	TimeLimitSec int `json:"time_limit_sec"` // per material; 0 = the planner's default

	// Report header, typed and remembered.
	Order string `json:"order"`
	User  string `json:"user"`
}

func defaultCutMachine() CutMachine {
	return CutMachine{SawKerf: 4, EdgeTrim: 10, Stages: 3, BarKerf: 3}
}

// PlanLine is one part as it leaves the conference screen: the cut line
// with the corrections made for this plan only.
type PlanLine struct {
	Item     string  `json:"item"`
	Code     string  `json:"code"`
	Desc     string  `json:"desc"`
	Material string  `json:"material"`
	Length   float64 `json:"length"` // finished size, mm
	Width    float64 `json:"width"`
	Qty      int     `json:"qty"`
	// CanRotate lets the part turn 90° on the sheet. Nil follows the
	// material: it may turn when the material has no grain.
	CanRotate *bool `json:"can_rotate,omitempty"`
	// Edge banding taken off the finished size, mm: C1 and C2 run along
	// the length (so they come off the width), L1 and L2 along the width
	// (they come off the length).
	C1 float64 `json:"c1,omitempty"`
	C2 float64 `json:"c2,omitempty"`
	L1 float64 `json:"l1,omitempty"`
	L2 float64 `json:"l2,omitempty"`
}

// CutLength and CutWidth are the sizes the saw cuts: finished size minus
// the edge banding.
func (l PlanLine) CutLength() float64 { return round3(l.Length - l.L1 - l.L2) }
func (l PlanLine) CutWidth() float64  { return round3(l.Width - l.C1 - l.C2) }

// JobPart ties a part of a planner job back to the BOM.
type JobPart struct {
	ID        string   `json:"id"` // part ID in the job
	Codes     []string `json:"codes"`
	Items     []string `json:"items"`
	Desc      string   `json:"desc"`
	Length    float64  `json:"length"` // finished size
	Width     float64  `json:"width"`
	CutLength float64  `json:"cut_length"`
	CutWidth  float64  `json:"cut_width"`
	Qty       int      `json:"qty"`
	CanRotate bool     `json:"can_rotate"`
}

// MaterialJob is the planner job of one material.
type MaterialJob struct {
	Material Material        `json:"material"`
	Planner  string          `json:"planner"` // cutplan.LinearID or cutplan.RectangularID
	Job      json.RawMessage `json:"job"`
	Parts    []JobPart       `json:"parts"`
}

// LineProblem says why a line was left out of the plan. Reason is a code
// the screen translates: unknown_material, no_qty, no_size.
type LineProblem struct {
	Code     string `json:"code"`
	Material string `json:"material"`
	Reason   string `json:"reason"`
}

func (p LineProblem) String() string { return p.Code + ": " + p.Reason + " " + p.Material }

// buildJobs turns the plan lines into one job per material, in the order
// the materials first appear. Lines of a material that is not in the list,
// or with a size that cannot be cut, are refused with the reason.
func buildJobs(lines []PlanLine, mats MaterialList, mc CutMachine) ([]MaterialJob, []LineProblem) {
	var problems []LineProblem
	var order []string
	groups := map[string][]PlanLine{}
	found := map[string]Material{}
	for _, l := range lines {
		m, ok := mats.Find(l.Material)
		if !ok {
			problems = append(problems, LineProblem{l.Code, l.Material, "unknown_material"})
			continue
		}
		if l.Qty < 1 {
			problems = append(problems, LineProblem{l.Code, l.Material, "no_qty"})
			continue
		}
		if l.CutLength() <= 0 || (m.Kind == KindSheet && l.CutWidth() <= 0) {
			problems = append(problems, LineProblem{l.Code, l.Material, "no_size"})
			continue
		}
		k := materialKey(m.Code)
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], l)
		found[k] = m
	}

	var jobs []MaterialJob
	for _, k := range order {
		m := found[k]
		parts := jobParts(groups[k], m)
		mj := MaterialJob{Material: m, Parts: parts}
		minL, minW := mats.MinOffcut(m)
		var job any
		if m.Kind == KindBar {
			mj.Planner = cutplan.LinearID
			j := linear.Job{Settings: linear.Settings{
				Kerf: mc.BarKerf, StartLoss: mc.StartLoss, EndLoss: mc.EndLoss,
				MinOffcut: minL, TimeLimitSec: mc.TimeLimitSec,
			}}
			for _, p := range parts {
				j.Parts = append(j.Parts, linear.Part{ID: p.ID, Length: p.CutLength, Quantity: p.Qty})
			}
			for i, o := range m.Offcuts {
				j.Stock = append(j.Stock, linear.Stock{ID: fmt.Sprintf("RET %d", i+1), Length: o.Length, Quantity: o.Quantity})
			}
			j.Stock = append(j.Stock, linear.Stock{ID: m.Code, Length: m.Length})
			job = j
		} else {
			mj.Planner = cutplan.RectangularID
			stages := mc.Stages
			if stages != 2 {
				stages = 3
			}
			j := rect.Job{Settings: rect.Settings{
				Kerf: mc.SawKerf, EdgeTrim: mc.EdgeTrim, Stages: stages, FirstCut: mc.FirstCut,
				MinOffcutLength: minL, MinOffcutWidth: minW, TimeLimitSec: mc.TimeLimitSec,
			}}
			for _, p := range parts {
				j.Parts = append(j.Parts, rect.Part{ID: p.ID, Length: p.CutLength, Width: p.CutWidth, Quantity: p.Qty, CanRotate: p.CanRotate})
			}
			for i, o := range m.Offcuts {
				j.Stock = append(j.Stock, rect.Sheet{ID: fmt.Sprintf("RET %d", i+1), Length: o.Length, Width: o.Width, Quantity: o.Quantity})
			}
			j.Stock = append(j.Stock, rect.Sheet{ID: m.Code, Length: m.Length, Width: m.Width})
			job = j
		}
		mj.Job, _ = json.Marshal(job)
		jobs = append(jobs, mj)
	}
	return jobs, problems
}

// jobParts joins the lines of one material that are the same part to cut
// (same code, sizes, banding and rotation) and gives each part a unique ID:
// its code, or "code (2)" when the same code is cut at two sizes.
func jobParts(lines []PlanLine, m Material) []JobPart {
	type key struct {
		code         string
		l, w, cl, cw float64
		rot          bool
	}
	var parts []JobPart
	index := map[key]int{}
	usedID := map[string]int{}
	for _, l := range lines {
		rot := !m.Grain
		if l.CanRotate != nil {
			rot = *l.CanRotate
		}
		if m.Kind == KindBar {
			rot = false
		}
		cw := l.CutWidth()
		if m.Kind == KindBar {
			cw = 0
		}
		k := key{strings.ToUpper(strings.TrimSpace(l.Code)), l.Length, l.Width, l.CutLength(), cw, rot}
		if i, ok := index[k]; ok {
			parts[i].Qty += l.Qty
			parts[i].Items = appendNew(parts[i].Items, l.Item)
			continue
		}
		id := strings.TrimSpace(l.Code)
		if id == "" {
			id = "?"
		}
		usedID[strings.ToUpper(id)]++
		if n := usedID[strings.ToUpper(id)]; n > 1 {
			id = fmt.Sprintf("%s (%d)", id, n)
		}
		index[k] = len(parts)
		parts = append(parts, JobPart{
			ID: id, Codes: []string{l.Code}, Items: appendNew(nil, l.Item), Desc: l.Desc,
			Length: l.Length, Width: l.Width, CutLength: l.CutLength(), CutWidth: cw,
			Qty: l.Qty, CanRotate: rot,
		})
	}
	// Biggest first, as Corte Certo lists them.
	sort.SliceStable(parts, func(i, j int) bool {
		return parts[i].CutLength*math.Max(parts[i].CutWidth, 1) > parts[j].CutLength*math.Max(parts[j].CutWidth, 1)
	})
	return parts
}

func appendNew(list []string, s string) []string {
	if s == "" {
		return list
	}
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}
