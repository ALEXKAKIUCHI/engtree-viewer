package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileType tells EngTree where to look for one kind of file (drawings, DXF,
// 3D, CNC programs...) and how a file name is matched to a part code.
type FileType struct {
	Name       string   `json:"name"`       // label shown in the UI, e.g. "PDF"
	Extensions []string `json:"extensions"` // e.g. [".igs", ".iges"]
	Folders    []string `json:"folders"`    // one or more folders; may be shared with other types
	// "exact":  file name (without extension) must equal the code.
	// "prefix": file name may continue after the code with a separator,
	//           e.g. "ABC-100 - Side panel.pdf" or "ABC-100_rev2.pdf".
	//           "ABC-1000.pdf" does NOT match "ABC-100".
	Match string `json:"match"`
}

// Columns holds the header names used to find the key BOM columns. Each
// field accepts alternatives separated by "|"; the first one found wins.
// Matching ignores case, accents and punctuation. Every other column of the
// spreadsheet is shown as-is.
type Columns struct {
	Item        string `json:"item"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Qty         string `json:"qty"`
}

type Config struct {
	// Folder with the BOM spreadsheets (.xlsx, .csv, .txt). Each BOM file is
	// one product. The product code and description come from the file name:
	// "<prefix><code><separator><description>.xlsx".
	BomFolder     string `json:"bom_folder"`
	BomPrefix     string `json:"bom_prefix"`     // e.g. "BOM - "; empty = every spreadsheet is a BOM
	NameSeparator string `json:"name_separator"` // between code and description, default " - "

	// Optional secondary spreadsheet with extra data per part (finish, color,
	// supplier...), joined to the BOM by part code. Either one file per
	// product in the BOM folder, named "<details prefix><code>...", or one
	// shared file for everything. The per-product file wins when both exist.
	DetailsPrefix string `json:"details_prefix"`
	DetailsFile   string `json:"details_file"`

	Columns       Columns    `json:"columns"`
	HiddenColumns []string   `json:"hidden_columns"` // spreadsheet columns not shown in the tree
	FileTypes     []FileType `json:"file_types"`

	// Column widths of the lists. Kept here (not in the browser) because the
	// local address changes on every start.
	ColumnWidths map[string]int `json:"column_widths,omitempty"`
}

func defaultColumns() Columns {
	return Columns{
		Item:        "ITEM NO|ITEM|Nº DO ITEM|N DO ITEM|NUMERO DO ITEM",
		Code:        "PART NUMBER|PART NO|PART|CODE|CODIGO|NUMERO DA PECA|REFERENCIA",
		Description: "DESCRIPTION|DESCRICAO|DESCRIPCION|DESC",
		Qty:         "QTY|QUANTITY|QTD|QTDE|QUANTIDADE|CANTIDAD",
	}
}

func defaultConfig() Config {
	return Config{
		NameSeparator: " - ",
		Columns:       defaultColumns(),
		FileTypes: []FileType{
			{Name: "PDF", Extensions: []string{".pdf"}, Match: "prefix"},
			{Name: "DXF", Extensions: []string{".dxf"}, Match: "exact"},
			{Name: "IGES", Extensions: []string{".igs", ".iges"}, Match: "exact"},
		},
	}
}

var (
	cfgMu   sync.RWMutex
	appCfg  Config
	cfgPath string
	dataDir string
)

// initPaths picks where settings live. A file named engtree.json next to the
// executable makes the install portable (handy to copy the same setup to
// several shop-floor PCs); otherwise the per-user config folder is used.
func initPaths() {
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "engtree.json")
		if _, err := os.Stat(p); err == nil {
			cfgPath = p
			dataDir = filepath.Dir(exe)
			return
		}
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base, _ = os.UserHomeDir()
	}
	dataDir = filepath.Join(base, "EngTreeViewer")
	_ = os.MkdirAll(dataDir, 0755)
	cfgPath = filepath.Join(dataDir, "engtree.json")
}

func loadConfig() {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	appCfg = defaultConfig()
	if b, err := os.ReadFile(cfgPath); err == nil {
		_ = json.Unmarshal(b, &appCfg)
	} else {
		saveConfigLocked()
	}
	normalizeConfig(&appCfg)
}

// normalizeConfig fills blanks so the rest of the code can rely on them.
func normalizeConfig(c *Config) {
	def := defaultColumns()
	if strings.TrimSpace(c.Columns.Item) == "" {
		c.Columns.Item = def.Item
	}
	if strings.TrimSpace(c.Columns.Code) == "" {
		c.Columns.Code = def.Code
	}
	if strings.TrimSpace(c.Columns.Description) == "" {
		c.Columns.Description = def.Description
	}
	if strings.TrimSpace(c.Columns.Qty) == "" {
		c.Columns.Qty = def.Qty
	}
	if c.NameSeparator == "" {
		c.NameSeparator = " - "
	}
	var types []FileType
	for _, ft := range c.FileTypes {
		ft.Name = strings.TrimSpace(ft.Name)
		if ft.Name == "" {
			continue
		}
		var exts []string
		for _, e := range ft.Extensions {
			e = strings.ToLower(strings.TrimSpace(e))
			if e == "" {
				continue
			}
			if !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			exts = append(exts, e)
		}
		ft.Extensions = exts
		var folders []string
		for _, f := range ft.Folders {
			if f = strings.TrimSpace(f); f != "" {
				folders = append(folders, f)
			}
		}
		ft.Folders = folders
		if ft.Match != "exact" {
			ft.Match = "prefix"
		}
		types = append(types, ft)
	}
	c.FileTypes = types
}

func saveConfigLocked() {
	if b, err := json.MarshalIndent(appCfg, "", "  "); err == nil {
		_ = os.WriteFile(cfgPath, b, 0644)
	}
}

func getConfig() Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return appCfg
}

// isConfigured is false until the user has set at least the BOM folder.
func (c Config) isConfigured() bool {
	return strings.TrimSpace(c.BomFolder) != ""
}

// allowedRoots lists every folder the app may serve files from.
func (c Config) allowedRoots() []string {
	roots := []string{c.BomFolder}
	if c.DetailsFile != "" {
		roots = append(roots, filepath.Dir(c.DetailsFile))
	}
	for _, ft := range c.FileTypes {
		roots = append(roots, ft.Folders...)
	}
	return roots
}
