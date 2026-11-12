package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Shown in the About box. Bump both on every release and add a line to
// CHANGELOG.md.
const (
	appVersion   = "0.1.0"
	appBuildDate = "2026-11-12"
	repoURL      = "https://github.com/ALEXKAKIUCHI/engtree-viewer"
)

//go:embed ui/index.html assets
var uiFiles embed.FS

var (
	projMu    sync.RWMutex
	products  []Product
	cachePath string

	scanMu      sync.Mutex
	scanning    bool
	scanPending bool   // a rescan was asked for while one was running
	lastScan    string // result of the last scan

	// Codes of each opened product, used to tell "ABC-100-01.pdf" (its own
	// part) from a revision of "ABC-100". Key: product code in upper case.
	codeSetMu sync.RWMutex
	codeSets  = map[string]map[string]bool{}
)

// ---- Product list ----

func loadProductsCache() {
	if b, err := os.ReadFile(cachePath); err == nil {
		projMu.Lock()
		_ = json.Unmarshal(b, &products)
		projMu.Unlock()
	}
}

func refreshProducts() {
	scanMu.Lock()
	if scanning {
		scanPending = true // e.g. settings saved during a scan: run again after it
		scanMu.Unlock()
		return
	}
	scanning = true
	scanMu.Unlock()

	go func() {
		defer func() {
			scanMu.Lock()
			scanning = false
			again := scanPending
			scanPending = false
			scanMu.Unlock()
			if again {
				refreshProducts()
			}
		}()
		cfg := getConfig()
		msg := ""
		switch {
		case !cfg.isConfigured():
			msg = "not_configured"
		case !folderReachable(cfg.BomFolder):
			msg = "offline"
		default:
			invalidateDirCache()
			list := scanProducts(cfg)
			projMu.Lock()
			products = list
			projMu.Unlock()
			if b, err := json.Marshal(list); err == nil {
				_ = os.WriteFile(cachePath, b, 0644)
			}
			msg = "ok"
		}
		scanMu.Lock()
		lastScan = msg
		scanMu.Unlock()
	}()
}

func findProduct(code string) *Product {
	projMu.RLock()
	defer projMu.RUnlock()
	for i := range products {
		if strings.EqualFold(products[i].Code, code) {
			p := products[i]
			return &p
		}
	}
	return nil
}

// ---- Folder reachability ----

// folderReachable checks that a folder can be listed, with a timeout so a
// dropped network share or VPN does not freeze the app. For UNC paths
// (\\server\share) the SMB port is tested first, which fails fast.
func folderReachable(folder string) bool {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return false
	}
	if strings.HasPrefix(folder, `\\`) {
		host := strings.SplitN(strings.TrimPrefix(folder, `\\`), `\`, 2)[0]
		if host != "" {
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, "445"), 1500*time.Millisecond)
			if err != nil {
				return false
			}
			_ = conn.Close()
		}
	}
	ch := make(chan bool, 1)
	go func() {
		info, err := os.Stat(folder)
		ch <- err == nil && info.IsDir()
	}()
	select {
	case ok := <-ch:
		return ok
	case <-time.After(2 * time.Second):
		return false
	}
}

type reachItem struct {
	ts time.Time
	ok bool
}

var (
	reachMu    sync.Mutex
	reachCache = map[string]reachItem{}
)

// folderReachableCached remembers the answer for 3 s, so the status bar and
// a burst of file lookups do not test the network over and over.
func folderReachableCached(folder string) bool {
	reachMu.Lock()
	if it, ok := reachCache[folder]; ok && time.Since(it.ts) < 3*time.Second {
		reachMu.Unlock()
		return it.ok
	}
	reachMu.Unlock()
	ok := folderReachable(folder)
	reachMu.Lock()
	reachCache[folder] = reachItem{time.Now(), ok}
	reachMu.Unlock()
	return ok
}

func bomFolderOnline() bool { return folderReachableCached(getConfig().BomFolder) }

// ---- Security ----
//
// The server listens on 127.0.0.1 only, but any web page open in the same
// browser can send it requests. So files are only served from the folders
// in the settings, and executables are never opened.

var blockedExts = map[string]bool{
	".exe": true, ".bat": true, ".cmd": true, ".com": true, ".scr": true, ".pif": true,
	".msi": true, ".ps1": true, ".vbs": true, ".vbe": true, ".js": true, ".jse": true,
	".wsf": true, ".wsh": true, ".hta": true, ".lnk": true, ".reg": true, ".cpl": true,
	".url": true, ".jar": true, ".dll": true, ".sh": true,
}

func insideFolder(p, root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	absP, err1 := filepath.Abs(p)
	absR, err2 := filepath.Abs(root)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(absR, absP)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func isAllowedPath(p string, cfg Config) bool {
	if p == "" {
		return false
	}
	for _, r := range cfg.allowedRoots() {
		if insideFolder(p, r) {
			return true
		}
	}
	return false
}

// guard rejects requests that do not come from the app's own window: other
// web sites open in the same browser (cross-site requests) and DNS-rebinding
// tricks (a foreign host name pointing at 127.0.0.1).
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		origin := r.Header.Get("Origin")
		if (host != "127.0.0.1" && host != "localhost") ||
			r.Header.Get("Sec-Fetch-Site") == "cross-site" ||
			(origin != "" && origin != "http://"+r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- Helpers ----

// attachment sets a download file name that survives accents and spaces.
func attachment(w http.ResponseWriter, name string) {
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func openWithSystem(target string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}

// launchWindow opens the UI in Edge or Chrome "app mode" (no address bar),
// falling back to the default browser.
func launchWindow(url string) {
	if runtime.GOOS == "windows" {
		local := os.Getenv("LOCALAPPDATA")
		for _, exe := range []string{
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			filepath.Join(local, `Google\Chrome\Application\chrome.exe`),
		} {
			if _, err := os.Stat(exe); err == nil {
				_ = exec.Command(exe, "--app="+url, "--window-size=1400,900").Start()
				return
			}
		}
	}
	_ = openWithSystem(url)
}

// ---- Main ----

func main() {
	port := flag.Int("port", 0, "fixed local port (default: random free port)")
	noWindow := flag.Bool("no-window", false, "do not open a window and do not exit when idle (for development)")
	flag.Parse()

	initPaths()
	loadConfig()
	cachePath = filepath.Join(dataDir, "products_cache.json")
	loadProductsCache()
	refreshProducts()

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, _ := uiFiles.ReadFile("ui/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
	mux.Handle("/assets/", http.FileServer(http.FS(uiFiles)))

	mux.HandleFunc("/api/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"version": appVersion, "date": appBuildDate, "repo": repoURL, "config_path": cfgPath})
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		scanMu.Lock()
		s, msg := scanning, lastScan
		scanMu.Unlock()
		writeJSON(w, map[string]any{
			"configured": cfg.isConfigured(),
			"online":     cfg.isConfigured() && bomFolderOnline(),
			"scanning":   s,
			"message":    msg,
		})
	})

	mux.HandleFunc("/api/products", func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		sortCol := r.URL.Query().Get("sort")
		asc := r.URL.Query().Get("asc") != "0"
		projMu.RLock()
		list := make([]Product, 0, len(products))
		for _, p := range products {
			if q == "" || strings.Contains(strings.ToLower(p.Code), q) || strings.Contains(strings.ToLower(p.Description), q) {
				list = append(list, p)
			}
		}
		projMu.RUnlock()
		less := func(i, j int) bool { return strings.ToLower(list[i].Code) < strings.ToLower(list[j].Code) }
		switch sortCol {
		case "desc":
			less = func(i, j int) bool {
				return strings.ToLower(list[i].Description) < strings.ToLower(list[j].Description)
			}
		case "date":
			less = func(i, j int) bool { return list[i].Mtime < list[j].Mtime }
		}
		sort.SliceStable(list, func(i, j int) bool {
			if asc {
				return less(i, j)
			}
			return less(j, i)
		})
		writeJSON(w, list)
	})

	mux.HandleFunc("/api/refresh", func(w http.ResponseWriter, r *http.Request) {
		refreshProducts()
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var in Config
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				http.Error(w, "invalid settings", 400)
				return
			}
			normalizeConfig(&in)
			cfgMu.Lock()
			if in.ColumnWidths == nil {
				in.ColumnWidths = appCfg.ColumnWidths
			}
			appCfg = in
			saveConfigLocked()
			cfgMu.Unlock()
			reachMu.Lock()
			reachCache = map[string]reachItem{}
			reachMu.Unlock()
			refreshProducts()
		}
		writeJSON(w, getConfig())
	})

	mux.HandleFunc("/api/default_config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, defaultConfig())
	})

	mux.HandleFunc("/api/column_widths", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var in map[string]int
			if json.NewDecoder(r.Body).Decode(&in) == nil {
				cfgMu.Lock()
				if appCfg.ColumnWidths == nil {
					appCfg.ColumnWidths = map[string]int{}
				}
				for k, v := range in {
					appCfg.ColumnWidths[k] = v
				}
				saveConfigLocked()
				cfgMu.Unlock()
			}
		}
		writeJSON(w, getConfig().ColumnWidths)
	})

	mux.HandleFunc("/api/product", func(w http.ResponseWriter, r *http.Request) {
		p := findProduct(r.URL.Query().Get("code"))
		if p == nil {
			http.Error(w, "product not found", 404)
			return
		}
		cfg := getConfig()
		online := folderReachable(cfg.BomFolder)
		res := BomResult{Columns: []string{}, Tree: []*TreeNode{}, Warnings: []string{}}
		var productFiles []string
		if online {
			res = buildBom(*p, cfg)
			others := codeSet(p.Code, res.Tree)
			codeSetMu.Lock()
			codeSets[strings.ToUpper(p.Code)] = others
			codeSetMu.Unlock()
			markAvailability(cfg, res.Tree, others)
			root := &TreeNode{Code: p.Code}
			markAvailability(cfg, []*TreeNode{root}, others)
			productFiles = root.Files
		}
		writeJSON(w, map[string]any{
			"product":       p,
			"bom":           res,
			"product_files": productFiles,
			"online":        online,
			"file_types":    cfg.FileTypes,
		})
	})

	mux.HandleFunc("/api/item_files", func(w http.ResponseWriter, r *http.Request) {
		codeSetMu.RLock()
		others := codeSets[strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("product")))]
		codeSetMu.RUnlock()
		writeJSON(w, findAllFiles(getConfig(), r.URL.Query().Get("code"), others))
	})

	mux.HandleFunc("/api/file", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("path")
		if !isAllowedPath(p, getConfig()) {
			http.Error(w, "file outside the configured folders", 403)
			return
		}
		if r.URL.Query().Get("download") == "1" {
			attachment(w, filepath.Base(p))
		}
		http.ServeFile(w, r, p)
	})

	mux.HandleFunc("/api/open_external", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		p := r.URL.Query().Get("path")
		if !isAllowedPath(p, getConfig()) || blockedExts[strings.ToLower(filepath.Ext(p))] {
			http.Error(w, "file not allowed", 403)
			return
		}
		_ = openWithSystem(p)
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/open_folder", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		p := r.URL.Query().Get("path")
		if !isAllowedPath(p, getConfig()) {
			http.Error(w, "file not allowed", 403)
			return
		}
		if runtime.GOOS == "windows" {
			_ = exec.Command("explorer", "/select,", p).Start()
		} else {
			_ = openWithSystem(filepath.Dir(p))
		}
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/cad_data", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("path")
		if !isAllowedPath(p, getConfig()) {
			http.Error(w, "file outside the configured folders", 403)
			return
		}
		if strings.ToLower(r.URL.Query().Get("kind")) == "dxf" {
			writeJSON(w, parseDXF(p))
		} else {
			writeJSON(w, parseIGES(p))
		}
	})

	mux.HandleFunc("/api/package_zip", func(w http.ResponseWriter, r *http.Request) {
		cfg := getConfig()
		p := findProduct(r.URL.Query().Get("code"))
		if p == nil {
			http.Error(w, "product not found", 404)
			return
		}
		var ft *FileType
		for i := range cfg.FileTypes {
			if strings.EqualFold(cfg.FileTypes[i].Name, r.URL.Query().Get("type")) {
				ft = &cfg.FileTypes[i]
			}
		}
		if ft == nil {
			http.Error(w, "unknown file type", 400)
			return
		}
		if !folderReachable(cfg.BomFolder) {
			http.Error(w, "BOM folder not reachable", 503)
			return
		}
		entries := collectPackage(cfg, *ft, p.Code, buildBom(*p, cfg).Tree)
		if len(entries) == 0 {
			http.Error(w, "no files", 404)
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		attachment(w, p.Code+"_"+ft.Name+".zip")
		writeZip(w, entries)
	})

	mux.HandleFunc("/api/open_url", func(w http.ResponseWriter, r *http.Request) {
		// Only the project page (About / report a problem); nothing else.
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Query().Get("url"), repoURL) {
			_ = openWithSystem(r.URL.Query().Get("url"))
		}
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/closing", func(w http.ResponseWriter, r *http.Request) {
		markClosing()
		w.WriteHeader(http.StatusNoContent)
	})

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	url := "http://" + listener.Addr().String()
	if *noWindow {
		fmt.Println("EngTree Viewer running at", url)
	} else {
		go launchWindow(url)
		startTaskbarIdentity()
		go watchForWindowClose()
	}
	_ = http.Serve(listener, guard(trackActivity(mux)))
}

// ---- Exit when the window closes ----
//
// The window is a separate browser process, so closing it does not stop
// this server. The page sends /api/closing on unload; the server exits a few
// seconds later unless the page comes back (a reload). As a safety net it
// also exits after 3 minutes without any request (the page polls /api/status
// every few seconds while open, even minimized).

var (
	lifeMu      sync.Mutex
	lastSeen    = time.Now()
	closingAt   time.Time
	idleTimeout = 3 * time.Minute
	closeGrace  = 5 * time.Second
)

func trackActivity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/closing" {
			lifeMu.Lock()
			lastSeen = time.Now()
			closingAt = time.Time{}
			lifeMu.Unlock()
		}
		next.ServeHTTP(w, r)
	})
}

func markClosing() {
	lifeMu.Lock()
	closingAt = time.Now()
	lifeMu.Unlock()
}

func watchForWindowClose() {
	for range time.Tick(time.Second) {
		lifeMu.Lock()
		closing := !closingAt.IsZero() && time.Since(closingAt) > closeGrace
		idle := time.Since(lastSeen) > idleTimeout
		lifeMu.Unlock()
		if closing || idle {
			os.Exit(0)
		}
	}
}
