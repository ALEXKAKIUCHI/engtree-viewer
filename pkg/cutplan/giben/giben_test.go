package giben

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ALEXKAKIUCHI/engtree-viewer/pkg/cutplan/rect"
)

// The files in testdata are real exports for a Giben saw; only
// the product codes and the description were changed (same lengths).
//
// The tests read them back into sheets, parts and cuts, as the planner
// would have made them, then write them out with Write: the result must
// be the same files, byte for byte.

type sample struct {
	export Export
	job    rect.Job
	res    rect.Result
}

func readSample(t *testing.T, name string, kerf, trim float64) (sample, []byte, []byte) {
	t.Helper()
	ac, err := os.ReadFile("testdata/" + name + ".AC")
	if err != nil {
		t.Fatal(err)
	}
	ad, err := os.ReadFile("testdata/" + name + ".AD")
	if err != nil {
		t.Fatal(err)
	}
	acLines := strings.Split(strings.TrimSuffix(string(ac), "\r\n"), "\r\n")
	adLines := strings.Split(strings.TrimSuffix(string(ad), "\r\n"), "\r\n")
	num := func(s string) int {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			t.Fatalf("not a number: %q", s)
		}
		return n
	}
	mm := func(s string) float64 { return float64(num(s)) / 10 }

	h := adLines[0]
	date, err := time.Parse("2006Jan02", h[62:66]+strings.ToUpper(h[66:67])+h[67:69]+h[69:71])
	if err != nil {
		t.Fatal(err)
	}
	smp := sample{export: Export{
		Material:    h[:15],
		Thickness:   mm(h[15:19]),
		ProjectCode: strings.TrimSpace(h[19:28]),
		Description: h[37:62],
		Date:        date,
		Parts:       map[string]PartInfo{},
	}}
	smp.job.Settings = rect.Settings{Kerf: kerf, EdgeTrim: trim, Stages: 3}

	// The .AD part lines of each block, in order.
	var blocks [][]string
	for _, l := range adLines {
		switch l[28] {
		case '1':
			blocks = append(blocks, nil)
		case '2':
			blocks[len(blocks)-1] = append(blocks[len(blocks)-1], l)
		}
	}
	quantity := map[string]int{}
	var order []string

	// The .AC, layout by layout.
	type node struct {
		level, code, repeat int
		measure             float64
		kids                []*node
		part                string
	}
	var plans [][]*node
	var sheets []rect.Sheet
	var counts []int
	for _, l := range acLines {
		body := l[18:]
		if body[2] == '1' && len(l) == 37 {
			plans = append(plans, nil)
			counts = append(counts, num(body[3:8]))
			sheets = append(sheets, rect.Sheet{Width: mm(body[8:13]), Length: mm(body[13:18])})
			continue
		}
		pi := len(plans) - 1
		n := &node{level: num(body[2:3]), code: num(body[3:5]), measure: mm(body[5:10]), repeat: num(body[10:12])}
		plans[pi] = append(plans[pi], n)
	}
	// Give each part line of the .AC its .AD line, in order.
	for pi, ns := range plans {
		rows := blocks[pi]
		k := 0
		for i, n := range ns {
			leaf := i+1 == len(ns) || ns[i+1].level <= n.level
			if !leaf {
				continue
			}
			if k == len(rows) {
				t.Fatalf("layout %d: more parts in the .AC than in the .AD", pi+1)
			}
			r := rows[k]
			k++
			code := num(r[29:33])
			if n.code != 0 && n.code != code {
				t.Fatalf("layout %d: .AC code %d, .AD code %d", pi+1, n.code, code)
			}
			if q := num(r[45:50]); q != n.repeat {
				t.Fatalf("layout %d: .AC repeat %d, .AD quantity %d", pi+1, n.repeat, q)
			}
			id := fmt.Sprint(code)
			if _, ok := smp.export.Parts[id]; !ok {
				smp.export.Parts[id] = PartInfo{Code: code, Label: strings.TrimRight(r[130:156], " ")}
				smp.job.Parts = append(smp.job.Parts, rect.Part{ID: id, Width: mm(r[33:39]), Length: mm(r[39:45]), CanRotate: true})
				order = append(order, id)
			}
			n.part = id
			quantity[id] += n.repeat * counts[pi]
		}
		if k != len(rows) {
			t.Fatalf("layout %d: more parts in the .AD than in the .AC", pi+1)
		}
	}
	for i := range smp.job.Parts {
		smp.job.Parts[i].Quantity = quantity[smp.job.Parts[i].ID]
	}
	size := map[string]rect.Part{}
	for _, p := range smp.job.Parts {
		size[p.ID] = p
	}

	// Lay the tree out on the sheet.
	for pi, ns := range plans {
		sh := rect.Sheet{ID: fmt.Sprint("S", pi+1), Length: sheets[pi].Length, Width: sheets[pi].Width}
		smp.job.Stock = append(smp.job.Stock, sh)
		first := ns[0].level
		lay := rect.SheetLayout{StockID: sh.ID, Length: sh.Length, Width: sh.Width,
			FirstCut: rect.AlongLength, Placements: []rect.Placement{}, Cuts: []rect.Cut{}}
		if first == 3 {
			lay.FirstCut = rect.AlongWidth
		}
		var place func(i int, r region) int
		// place lays out the children of ns[i-1] (or the sheet, i = 0)
		// inside r and returns the index after them.
		place = func(i int, r region) int {
			level := ns[i].level
			alongY := level%2 == 0
			lo, hi, olo, ohi := r.x0, r.x1, r.y0, r.y1
			along := rect.AlongWidth
			if alongY {
				lo, hi, olo, ohi = r.y0, r.y1, r.x0, r.x1
				along = rect.AlongLength
			}
			p := lo
			for i < len(ns) && ns[i].level == level {
				n := ns[i]
				i++
				for k := 0; k < n.repeat; k++ {
					sub := region{p, p + n.measure, r.y0, r.y1}
					if alongY {
						sub = region{r.x0, r.x1, p, p + n.measure}
					}
					if n.part != "" {
						pt := size[n.part]
						dx, dy := sub.x1-sub.x0, sub.y1-sub.y0
						rot := false
						switch {
						case near(pt.Length, dx) && near(pt.Width, dy):
						case near(pt.Width, dx) && near(pt.Length, dy):
							rot = true
						default:
							t.Fatalf("layout %d: part %s (%gx%g) does not fill %gx%g", pi+1, n.part, pt.Width, pt.Length, dy, dx)
						}
						lay.Placements = append(lay.Placements, rect.Placement{PartID: n.part, X: sub.x0, Y: sub.y0, Rotated: rot})
					} else {
						end := place(i, sub)
						if k+1 == n.repeat {
							i = end
						}
					}
					if p+n.measure < hi-tol {
						lay.Cuts = append(lay.Cuts, rect.Cut{Stage: level - first + 1, Along: along, Pos: p + n.measure, From: olo, To: ohi})
					}
					p += n.measure + kerf
				}
			}
			return i
		}
		if end := place(0, region{trim, sh.Length - trim, trim, sh.Width - trim}); end != len(ns) {
			t.Fatalf("layout %d: cuts left over at line %d", pi+1, end)
		}
		for k := 0; k < counts[pi]; k++ {
			smp.res.Sheets = append(smp.res.Sheets, lay)
		}
	}
	return smp, ac, ad
}

func near(a, b float64) bool { return a-b < tol && b-a < tol }

func TestReferenceFiles(t *testing.T) {
	for _, name := range []string{"mdf15", "mdf6"} {
		for _, set := range [][2]float64{{4, 0}, {4.4, 10}} {
			t.Run(fmt.Sprintf("%s kerf %g trim %g", name, set[0], set[1]), func(t *testing.T) {
				smp, wantAC, wantAD := readSample(t, name, set[0], set[1])
				ac, ad, err := Write(smp.job, smp.res, smp.export)
				if err != nil {
					t.Fatal(err)
				}
				same(t, "AC", ac, wantAC)
				same(t, "AD", ad, wantAD)
			})
		}
	}
}

func same(t *testing.T, what string, got, want []byte) {
	t.Helper()
	if bytes.Equal(got, want) {
		return
	}
	g, w := strings.Split(string(got), "\r\n"), strings.Split(string(want), "\r\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var a, b string
		if i < len(g) {
			a = g[i]
		}
		if i < len(w) {
			b = w[i]
		}
		if a != b {
			t.Fatalf(".%s line %d:\n got %q\nwant %q", what, i+1, strings.TrimRight(a, " "), strings.TrimRight(b, " "))
		}
	}
}

// A plan from the planner itself must export without errors, and every
// part it places must be on the .AD.
func TestPlannerPlan(t *testing.T) {
	job := rect.Job{
		Parts: []rect.Part{
			{ID: "side", Length: 1705, Width: 785, Quantity: 6, CanRotate: true},
			{ID: "shelf", Length: 968, Width: 745, Quantity: 9, CanRotate: true},
			{ID: "rail", Length: 968, Width: 50, Quantity: 14, CanRotate: true},
			{ID: "clip", Length: 500, Width: 70, Quantity: 20, CanRotate: true},
			{ID: "door", Length: 1993.4, Width: 289, Quantity: 3},
		},
		Stock:    []rect.Sheet{{ID: "MDF", Length: 2750, Width: 1850}},
		Settings: rect.Settings{Kerf: 4.4, EdgeTrim: 10, Stages: 3, TimeLimitSec: 1},
	}
	for _, first := range []string{rect.AlongLength, rect.AlongWidth} {
		job.Settings.FirstCut = first
		res, err := rect.Run(context.Background(), job, nil)
		if err != nil {
			t.Fatal(err)
		}
		ac, ad, err := Write(job, res, Export{Material: "MDF BRANCO 15MM", Thickness: 15, ProjectCode: "TEST", Description: "Test", Date: time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatalf("first cut %s: %v", first, err)
		}
		placed := 0
		for _, s := range res.Sheets {
			placed += len(s.Placements)
		}
		// Each .AD block lists the parts of one layout once; the .AC says
		// on how many sheets that layout is cut.
		var counts []int
		for _, l := range strings.Split(string(ac), "\r\n") {
			if len(l) == 37 {
				c, _ := strconv.Atoi(l[21:26])
				counts = append(counts, c)
			}
		}
		block, onAD := -1, 0
		for _, l := range strings.Split(string(ad), "\r\n") {
			if len(l) != adLineWidth {
				continue
			}
			switch l[28] {
			case '1':
				block++
			case '2':
				q, _ := strconv.Atoi(l[45:50])
				onAD += q * counts[block]
			}
		}
		if onAD != placed {
			t.Fatalf("first cut %s: %d parts on the .AD, %d placed", first, onAD, placed)
		}
	}
}

func TestErrors(t *testing.T) {
	job := rect.Job{Parts: []rect.Part{{ID: "a", Length: 100, Width: 100, Quantity: 1}, {ID: "b", Length: 100, Width: 100, Quantity: 1}}}
	if _, _, err := Write(job, rect.Result{}, Export{Parts: map[string]PartInfo{"a": {Code: 2}}}); err == nil {
		t.Fatal("two parts with code 2 must be refused")
	}
	if _, _, err := Write(job, rect.Result{}, Export{Parts: map[string]PartInfo{"a": {Code: 100}}}); err == nil {
		t.Fatal("code 100 must be refused")
	}
	// Leftover between two parts cannot be written.
	job.Parts[0].Width, job.Parts[1].Width = 500, 500
	res := rect.Result{Sheets: []rect.SheetLayout{{Length: 1000, Width: 500, FirstCut: rect.AlongWidth,
		Placements: []rect.Placement{{PartID: "a", X: 0, Y: 0}, {PartID: "b", X: 300, Y: 0}},
		Cuts:       []rect.Cut{{Stage: 1, Along: rect.AlongWidth, Pos: 100, From: 0, To: 500}, {Stage: 1, Along: rect.AlongWidth, Pos: 200, From: 0, To: 500}, {Stage: 1, Along: rect.AlongWidth, Pos: 400, From: 0, To: 500}}}}}
	if _, _, err := Write(job, res, Export{}); err == nil || !strings.Contains(err.Error(), "leftover") {
		t.Fatalf("leftover between parts: %v", err)
	}
}
