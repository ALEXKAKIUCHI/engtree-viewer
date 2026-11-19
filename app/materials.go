package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Material kinds.
const (
	KindSheet = "sheet" // cut on a panel saw: rectangular plan
	KindBar   = "bar"   // bars and tubes: linear plan
)

// Material is one entry of the material list. The list belongs to this
// computer and is the same for every project. A BOM line is cut from the
// material whose Code (or AltCode) is the line's material code.
type Material struct {
	Code        string  `json:"code"`
	AltCode     string  `json:"alt_code,omitempty"` // a second code for the same stock (old system, other supplier)
	Description string  `json:"description"`
	Kind        string  `json:"kind"`      // KindSheet or KindBar
	Thickness   float64 `json:"thickness"` // mm
	Grain       bool    `json:"grain"`     // sheets only: the grain runs along the sheet length, parts may not turn

	// Size of a whole sheet or bar, mm. Bars use Length only.
	Length float64 `json:"length"`
	Width  float64 `json:"width,omitempty"`

	// Offcuts in stock, used before whole sheets or bars.
	Offcuts []Offcut `json:"offcuts,omitempty"`

	// What is left over counts as an offcut from this size up; below it is
	// scrap. Zero means "use the general default".
	MinOffcutLength float64 `json:"min_offcut_length,omitempty"`
	MinOffcutWidth  float64 `json:"min_offcut_width,omitempty"`
}

// Offcut is a piece of a material left from earlier plans.
type Offcut struct {
	Length   float64 `json:"length"`
	Width    float64 `json:"width,omitempty"` // sheets only
	Quantity int     `json:"quantity"`
}

// MaterialList is the whole list as stored.
type MaterialList struct {
	// General defaults for the smallest useful offcut, mm.
	MinOffcutLength float64    `json:"min_offcut_length"`
	MinOffcutWidth  float64    `json:"min_offcut_width"`
	Materials       []Material `json:"materials"`
}

// materialKey makes codes comparable: the BOM and the list must agree on
// the code, but not on case or surrounding spaces.
func materialKey(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

// Validate cleans the list and refuses what cannot be used: a material
// without code, kind or size, or a code (main or alternative) used by two
// materials.
func (l *MaterialList) Validate() error {
	seen := map[string]string{} // key -> code of the material that has it
	var errs []error
	for i := range l.Materials {
		m := &l.Materials[i]
		m.Code = strings.TrimSpace(m.Code)
		m.AltCode = strings.TrimSpace(m.AltCode)
		m.Description = strings.TrimSpace(m.Description)
		name := m.Code
		if name == "" {
			name = fmt.Sprintf("#%d", i+1)
			errs = append(errs, fmt.Errorf("material %s: no code", name))
		}
		switch m.Kind {
		case KindSheet:
			if m.Length <= 0 || m.Width <= 0 {
				errs = append(errs, fmt.Errorf("material %s: sheet size missing", name))
			}
		case KindBar:
			if m.Length <= 0 {
				errs = append(errs, fmt.Errorf("material %s: bar length missing", name))
			}
			m.Width, m.Grain, m.MinOffcutWidth = 0, false, 0
		default:
			errs = append(errs, fmt.Errorf("material %s: kind must be %q or %q", name, KindSheet, KindBar))
		}
		if m.Thickness < 0 {
			errs = append(errs, fmt.Errorf("material %s: negative thickness", name))
		}
		for _, code := range []string{m.Code, m.AltCode} {
			k := materialKey(code)
			if k == "" {
				continue
			}
			if other, ok := seen[k]; ok {
				errs = append(errs, fmt.Errorf("code %s is used by materials %s and %s", code, other, name))
				continue
			}
			seen[k] = name
		}
		var offcuts []Offcut
		for _, o := range m.Offcuts {
			if o.Length > 0 && o.Quantity > 0 && (m.Kind == KindBar || o.Width > 0) {
				if m.Kind == KindBar {
					o.Width = 0
				}
				offcuts = append(offcuts, o)
			}
		}
		m.Offcuts = offcuts
	}
	return errors.Join(errs...)
}

// Find returns the material with this code, main or alternative.
func (l MaterialList) Find(code string) (Material, bool) {
	k := materialKey(code)
	if k == "" {
		return Material{}, false
	}
	for _, m := range l.Materials {
		if materialKey(m.Code) == k || materialKey(m.AltCode) == k {
			return m, true
		}
	}
	return Material{}, false
}

// MinOffcut is the smallest useful offcut for m: its own size, or the
// general default.
func (l MaterialList) MinOffcut(m Material) (length, width float64) {
	length, width = m.MinOffcutLength, m.MinOffcutWidth
	if length <= 0 {
		length = l.MinOffcutLength
	}
	if width <= 0 && m.Kind == KindSheet {
		width = l.MinOffcutWidth
	}
	return length, width
}

// ---- Storage ----

var materialsMu sync.Mutex

func materialsPath() string { return filepath.Join(dataDir, "materials.json") }

// loadMaterials reads the list. A missing file is an empty list.
func loadMaterials() MaterialList {
	materialsMu.Lock()
	defer materialsMu.Unlock()
	var l MaterialList
	if b, err := os.ReadFile(materialsPath()); err == nil {
		_ = json.Unmarshal(b, &l)
	}
	if l.Materials == nil {
		l.Materials = []Material{}
	}
	return l
}

// saveMaterials checks and writes the whole list.
func saveMaterials(l MaterialList) error {
	if err := l.Validate(); err != nil {
		return err
	}
	sort.SliceStable(l.Materials, func(i, j int) bool {
		return materialKey(l.Materials[i].Code) < materialKey(l.Materials[j].Code)
	})
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	materialsMu.Lock()
	defer materialsMu.Unlock()
	tmp := materialsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, materialsPath())
}
