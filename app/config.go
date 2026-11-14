package app

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
	// List shows the files as a list (date, size, download, ZIP) instead of
	// opening the first one: meant for machine programs (CNC and the like),
	// where a part often has several files in formats each company defines.
	List bool `json:"list"`
}

// Columns holds the header names used to find the key BOM columns. Each
// field accepts alternatives separated by "|"; the first one found wins.
// Matching ignores case, accents and punctuation; "#2" means the 2nd column.
// Every other column of the spreadsheet is shown as-is.
type Columns struct {
	Item        string `json:"item"`
	Code        string `json:"code"`
	Description string `json:"description"`
	Qty         string `json:"qty"`
}

// ListLayout is how one list looks after the user dragged its columns: the
// order of the column keys and their widths in pixels.
type ListLayout struct {
	Order  []string       `json:"order,omitempty"`
	Widths map[string]int `json:"widths,omitempty"`
}

// Modes. The mode belongs to the computer, not to a person: there is no
// login. A shop-floor PC stays in production mode and can ask for revisions;
// an engineering PC can also close them.
const (
	ModeProduction  = "production"
	ModeEngineering = "engineering"
)

type Config struct {
	// ---- Settings of this computer ----

	Language string `json:"language"` // "" = follow the system, "en" or "pt-BR"
	Mode     string `json:"mode"`     // production (default) or engineering

	// Optional path to an engtree.json on the network holding the settings
	// below ("shared settings"). Engineering sets the folders once and every
	// computer follows. The last copy read is kept locally, so a dropped
	// network does not leave the program unconfigured.
	SharedConfig string `json:"shared_config"`

	// Name of whoever last asked for a revision on this computer, offered
	// again next time.
	Requester string `json:"requester,omitempty"`

	ColumnLayout map[string]ListLayout `json:"column_layout,omitempty"`

	// ---- Shared settings (see SharedConfig) ----

	// Folder with the BOM spreadsheets (.xlsx, .csv, .txt). Each spreadsheet
	// is one product; code and description come from the file name:
	// "<prefix><code><separator><description>.xlsx".
	BomFolder     string `json:"bom_folder"`
	BomPrefix     string `json:"bom_prefix"`     // e.g. "BOM - "; empty = every spreadsheet is a BOM
	NameSeparator string `json:"name_separator"` // between code and description, default " - "

	// Optional folder whose files are products with no BOM spreadsheet
	// (often a folder of whole-product drawings). A file there becomes a
	// product, with its files but no tree, when no BOM has that code.
	ProductsFolder string `json:"products_folder"`

	// Optional secondary spreadsheet with extra data per part (finish, color,
	// supplier...), joined to the BOM by part code. Either one file per
	// product in the BOM folder, named "<details prefix><code>...", or one
	// shared file for everything. The per-product file wins when both exist.
	DetailsPrefix string `json:"details_prefix"`
	DetailsFile   string `json:"details_file"`

	Columns       Columns    `json:"columns"`
	HiddenColumns []string   `json:"hidden_columns"` // spreadsheet columns not shown in the tree
	FileTypes     []FileType `json:"file_types"`

	// Optional folder with attachments (photos, videos, customer documents).
	// A sub-folder or a loose file belongs to a code when its name is the
	// code or starts with it ("ABC-100 - customer photos").
	RefFolder string `json:"ref_folder"`

	// Spreadsheet columns shown (and filterable) in the Parts tab,
	// e.g. ["Material", "Finish"].
	PartsColumns []string `json:"parts_columns"`

	// Folder where revision requests are stored, one small file each. It is
	// the only folder EngTree writes to. Without it, revisions are off.
	RevisionsFolder string `json:"revisions_folder"`
}

func (c Config) isEngineering() bool { return c.Mode == ModeEngineering }

// applyShared copies the shared settings of src into dst, leaving every
// setting that belongs to this computer untouched.
func applyShared(dst *Config, src Config) {
	dst.BomFolder = src.BomFolder
	dst.BomPrefix = src.BomPrefix
	dst.NameSeparator = src.NameSeparator
	dst.ProductsFolder = src.ProductsFolder
	dst.DetailsPrefix = src.DetailsPrefix
	dst.DetailsFile = src.DetailsFile
	dst.Columns = src.Columns
	dst.HiddenColumns = src.HiddenColumns
	dst.FileTypes = src.FileTypes
	dst.RefFolder = src.RefFolder
	dst.PartsColumns = src.PartsColumns
	dst.RevisionsFolder = src.RevisionsFolder
}

func defaultColumns() Columns {
	return Columns{
		Item:        "ITEM NO|ITEM|Nº DO ITEM|N DO ITEM|NUMERO DO ITEM",
		Code:        "PART NUMBER|PART NO|PART|CODE|CODIGO|Nº DA PEÇA|NUMERO DA PECA|REFERENCIA",
		Description: "DESCRIPTION|DESCRICAO|DESCRIPCION|DESC",
		Qty:         "QTY|QUANTITY|QTD|QTDE|QUANTIDADE|CANTIDAD",
	}
}

func defaultConfig() Config {
	return Config{
		Mode:          ModeProduction,
		NameSeparator: " - ",
		Columns:       defaultColumns(),
		FileTypes: []FileType{
			{Name: "PDF", Extensions: []string{".pdf"}, Match: "prefix"},
			{Name: "DXF", Extensions: []string{".dxf"}, Match: "exact"},
			{Name: "IGES", Extensions: []string{".igs", ".iges"}, Match: "exact"},
		},
	}
}

// sharedStatus is what the settings screen shows about the shared file.
type sharedStatus struct {
	Active   bool   `json:"active"`   // settings are coming from the shared file
	Path     string `json:"path"`     //
	Editable bool   `json:"editable"` // engineering mode and the file can be written
	Offline  bool   `json:"offline"`  // set, but unreachable: using the last copy
}

var (
	cfgMu     sync.RWMutex
	appCfg    Config
	sharedAt  sharedStatus
	cfgPath   string
	dataDir   string
	portable  bool
	exeFolder string
)

// initPaths picks where settings live. A file named engtree.json next to the
// executable makes the install portable (handy on a pen drive, or to copy
// the same setup to several shop-floor PCs); otherwise the per-user config
// folder is used.
func initPaths() {
	if exe, err := os.Executable(); err == nil {
		exeFolder = filepath.Dir(exe)
		p := filepath.Join(exeFolder, "engtree.json")
		if _, err := os.Stat(p); err == nil {
			cfgPath, dataDir, portable = p, exeFolder, true
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

func sharedCachePath() string { return filepath.Join(dataDir, "shared_config_cache.json") }

func readConfigFile(path string) (Config, bool) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, false
	}
	if json.Unmarshal(b, &c) != nil {
		return c, false
	}
	return c, true
}

func loadConfig() {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	appCfg = defaultConfig()
	if local, ok := readConfigFile(cfgPath); ok {
		appCfg = local
	} else {
		saveLocalLocked()
	}
	loadSharedLocked()
	normalizeConfig(&appCfg)
}

// loadSharedLocked pulls the shared settings in, falling back to the last
// copy read when the network is down.
func loadSharedLocked() {
	sharedAt = sharedStatus{}
	path := strings.TrimSpace(appCfg.SharedConfig)
	if path == "" {
		return
	}
	sharedAt.Active, sharedAt.Path = true, path
	if shared, ok := readConfigFile(path); ok {
		applyShared(&appCfg, shared)
		if b, err := json.MarshalIndent(shared, "", "  "); err == nil {
			_ = os.WriteFile(sharedCachePath(), b, 0644)
		}
		sharedAt.Editable = appCfg.isEngineering() && isWritable(path)
		return
	}
	sharedAt.Offline = true
	if cached, ok := readConfigFile(sharedCachePath()); ok {
		applyShared(&appCfg, cached)
	}
}

func isWritable(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
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
	if c.Mode != ModeEngineering {
		c.Mode = ModeProduction
	}
	var types []FileType
	for _, ft := range c.FileTypes {
		ft.Name = strings.TrimSpace(ft.Name)
		if ft.Name == "" || ft.Name == refTypeName {
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
	var pc []string
	for _, p := range c.PartsColumns {
		if p = strings.TrimSpace(p); p != "" {
			pc = append(pc, p)
		}
	}
	c.PartsColumns = pc
	c.RefFolder = strings.TrimSpace(c.RefFolder)
	c.ProductsFolder = strings.TrimSpace(c.ProductsFolder)
	c.RevisionsFolder = strings.TrimSpace(c.RevisionsFolder)
	c.SharedConfig = strings.TrimSpace(c.SharedConfig)
}

// saveLocalLocked writes this computer's file. When the settings come from a
// shared file, the shared part is left out, so a copy of it is never
// mistaken for the settings themselves.
func saveLocalLocked() {
	out := appCfg
	if sharedAt.Active {
		applyShared(&out, Config{})
	}
	if b, err := json.MarshalIndent(out, "", "  "); err == nil {
		_ = os.WriteFile(cfgPath, b, 0644)
	}
}

// saveConfig stores incoming settings and returns what is in force now.
func saveConfig(in Config) Config {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	normalizeConfig(&in)
	// Settings that are not on the settings screen are kept.
	if in.ColumnLayout == nil {
		in.ColumnLayout = appCfg.ColumnLayout
	}
	if in.Requester == "" {
		in.Requester = appCfg.Requester
	}
	writeShared := sharedAt.Active && !sharedAt.Offline && sharedAt.Editable && in.SharedConfig == sharedAt.Path
	sharedPath := sharedAt.Path
	appCfg = in
	if writeShared {
		shared, _ := readConfigFile(sharedPath)
		applyShared(&shared, in)
		if b, err := json.MarshalIndent(shared, "", "  "); err == nil {
			_ = os.WriteFile(sharedPath, b, 0644)
		}
	}
	loadSharedLocked() // re-read: the path may have just changed
	normalizeConfig(&appCfg)
	saveLocalLocked()
	return appCfg
}

// setLocal changes settings of this computer only (mode, layout, requester).
func setLocal(fn func(c *Config)) {
	cfgMu.Lock()
	fn(&appCfg)
	saveLocalLocked()
	cfgMu.Unlock()
}

func getConfig() Config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return appCfg
}

func getSharedStatus() sharedStatus {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return sharedAt
}

// isConfigured is false until the user has set at least one source folder.
func (c Config) isConfigured() bool {
	return strings.TrimSpace(c.BomFolder) != "" || strings.TrimSpace(c.ProductsFolder) != ""
}

// allowedRoots lists every folder the app may serve files from.
func (c Config) allowedRoots() []string {
	roots := []string{c.BomFolder, c.RefFolder, c.ProductsFolder}
	if c.DetailsFile != "" {
		roots = append(roots, filepath.Dir(c.DetailsFile))
	}
	for _, ft := range c.FileTypes {
		roots = append(roots, ft.Folders...)
	}
	return roots
}
