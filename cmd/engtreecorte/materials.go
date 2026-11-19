package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/linear"
	"github.com/ALEXKAKIUCHI/engtree-viewer/cutplan/rect"
)

// The screen sends one table of parts and one of stock, each row with its
// material code (Cód. MP). Parts are cut from the stock with the same code:
// every code is a calculation of its own. A part without a code uses the
// stock without a code, so a screen that never fills the column works as
// one material.

// screenPart and screenStock are rows as the screen sends them.
type screenPart struct {
	ID        string  `json:"id"`
	Length    float64 `json:"length"`
	Width     float64 `json:"width"`
	Quantity  int     `json:"quantity"`
	CanRotate bool    `json:"can_rotate"`
	Material  string  `json:"material"`
}

type screenStock struct {
	ID           string  `json:"id"`
	Length       float64 `json:"length"`
	Width        float64 `json:"width"`
	Quantity     int     `json:"quantity"`
	Material     string  `json:"material"`
	MaterialDesc string  `json:"material_desc"`
	Thickness    float64 `json:"thickness"`
}

type screenJob struct {
	Parts    []screenPart    `json:"parts"`
	Stock    []screenStock   `json:"stock"`
	Settings json.RawMessage `json:"settings"`
}

// materialJob is the planner job of one material.
type materialJob struct {
	Code      string          `json:"code"` // as written in the stock (or the parts)
	Desc      string          `json:"desc"`
	Thickness float64         `json:"thickness"`
	Job       json.RawMessage `json:"job"`
}

// noStock lists the parts of a material that has no stock.
type noStock struct {
	Code  string   `json:"code"`
	Parts []string `json:"parts"`
}

func materialKey(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

// splitByMaterial turns the screen's tables into one planner job per
// material, in the order the materials first appear among the parts.
// Materials whose parts find no stock with their code come back in noStock;
// stock of a material no part uses is left out.
func splitByMaterial(mode string, raw json.RawMessage) ([]materialJob, []noStock, error) {
	var in screenJob
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, nil, err
	}
	if mode != cutplan.LinearID && mode != cutplan.RectangularID {
		return nil, nil, fmt.Errorf("unknown mode %q", mode)
	}
	var order []string
	parts := map[string][]screenPart{}
	spelling := map[string]string{}
	for _, p := range in.Parts {
		k := materialKey(p.Material)
		if _, ok := parts[k]; !ok {
			order = append(order, k)
			spelling[k] = strings.TrimSpace(p.Material)
		}
		parts[k] = append(parts[k], p)
	}
	stock := map[string][]screenStock{}
	for _, s := range in.Stock {
		k := materialKey(s.Material)
		stock[k] = append(stock[k], s)
	}

	var jobs []materialJob
	var missing []noStock
	for _, k := range order {
		st := stock[k]
		if len(st) == 0 {
			ids := make([]string, len(parts[k]))
			for i, p := range parts[k] {
				ids[i] = p.ID
			}
			missing = append(missing, noStock{Code: spelling[k], Parts: ids})
			continue
		}
		mj := materialJob{Code: strings.TrimSpace(st[0].Material)}
		if mj.Code == "" {
			mj.Code = spelling[k]
		}
		for _, s := range st {
			if mj.Desc == "" && strings.TrimSpace(s.MaterialDesc) != "" {
				mj.Desc = strings.TrimSpace(s.MaterialDesc)
			}
			if mj.Thickness == 0 && s.Thickness > 0 {
				mj.Thickness = s.Thickness
			}
		}
		var job any
		switch mode {
		case cutplan.RectangularID:
			j := rect.Job{}
			if err := json.Unmarshal(in.Settings, &j.Settings); err != nil {
				return nil, nil, err
			}
			for _, p := range parts[k] {
				j.Parts = append(j.Parts, rect.Part{ID: p.ID, Length: p.Length, Width: p.Width, Quantity: p.Quantity, CanRotate: p.CanRotate})
			}
			for _, s := range st {
				j.Stock = append(j.Stock, rect.Sheet{ID: s.ID, Length: s.Length, Width: s.Width, Quantity: s.Quantity})
			}
			job = j
		case cutplan.LinearID:
			j := linear.Job{}
			if err := json.Unmarshal(in.Settings, &j.Settings); err != nil {
				return nil, nil, err
			}
			for _, p := range parts[k] {
				j.Parts = append(j.Parts, linear.Part{ID: p.ID, Length: p.Length, Quantity: p.Quantity})
			}
			for _, s := range st {
				j.Stock = append(j.Stock, linear.Stock{ID: s.ID, Length: s.Length, Quantity: s.Quantity})
			}
			job = j
		}
		var err error
		if mj.Job, err = json.Marshal(job); err != nil {
			return nil, nil, err
		}
		jobs = append(jobs, mj)
	}
	return jobs, missing, nil
}
