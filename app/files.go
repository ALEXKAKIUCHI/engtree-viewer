package app

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---- Directory cache ----
//
// Network shares are slow to list, so each folder listing is kept in memory
// for a short time. Opening several parts in a row then costs one listing.

type DirEntryInfo struct {
	Name      string
	NameLower string
	FullPath  string
	Mtime     float64
	Size      int64
}

type dirCacheItem struct {
	ts      time.Time
	entries []DirEntryInfo
	subdirs []DirEntryInfo
}

var (
	dirCacheMu sync.Mutex
	dirCache   = map[string]dirCacheItem{}
	cacheTTL   = 45 * time.Second
)

func invalidateDirCache() {
	dirCacheMu.Lock()
	dirCache = map[string]dirCacheItem{}
	dirCacheMu.Unlock()
	fileIndexMu.Lock()
	fileIndex = map[string]fileIndexItem{}
	refIndexes = map[string]refIndexItem{}
	fileIndexMu.Unlock()
}

func scanDirCached(folder string) []DirEntryInfo {
	files, _ := scanFolder(folder)
	return files
}

// scanFolder lists the files and the sub-folders of a folder (cached).
func scanFolder(folder string) ([]DirEntryInfo, []DirEntryInfo) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return nil, nil
	}
	dirCacheMu.Lock()
	if item, ok := dirCache[folder]; ok && time.Since(item.ts) < cacheTTL {
		dirCacheMu.Unlock()
		return item.entries, item.subdirs
	}
	dirCacheMu.Unlock()

	// Listing a dead network share can block for a long time, so check first.
	if !folderReachableCached(folder) {
		return nil, nil
	}
	raw, err := os.ReadDir(folder)
	if err != nil {
		return nil, nil
	}
	var files, dirs []DirEntryInfo
	for _, e := range raw {
		item := DirEntryInfo{Name: e.Name(), NameLower: strings.ToLower(e.Name()), FullPath: filepath.Join(folder, e.Name())}
		if e.IsDir() {
			dirs = append(dirs, item)
			continue
		}
		if info, err := e.Info(); err == nil {
			item.Mtime = float64(info.ModTime().Unix())
			item.Size = info.Size()
		}
		files = append(files, item)
	}
	dirCacheMu.Lock()
	dirCache[folder] = dirCacheItem{ts: time.Now(), entries: files, subdirs: dirs}
	dirCacheMu.Unlock()
	return files, dirs
}

// ---- Matching files to part codes ----

func isAlnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || c >= 0x80
}

// codeKeys lists every code a file name (lower case, no extension) can stand
// for under "prefix" matching: the full name, plus the name cut at each
// separator. "abc-100_rev2" -> "abc-100_rev2", "abc-100", "abc".
// "abc-1000" never yields "abc-100", so similar codes do not mix.
func codeKeys(stem string) []string {
	keys := []string{stem}
	for i := 1; i < len(stem); i++ {
		if !isAlnum(stem[i]) {
			if k := strings.TrimSpace(stem[:i]); k != "" && k != keys[len(keys)-1] {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

type FoundFile struct {
	Path   string  `json:"path"`
	Name   string  `json:"name"`
	Ext    string  `json:"ext"`
	Mtime  float64 `json:"mtime"`
	Size   int64   `json:"size"`
	Viewer string  `json:"viewer"` // pdf, dxf, iges, image, text or external
	exact  bool
}

type fileIndexItem struct {
	ts    time.Time
	byKey map[string][]FoundFile
}

var (
	fileIndexMu sync.Mutex
	fileIndex   = map[string]fileIndexItem{} // file type name -> index
)

func viewerFor(ext string) string {
	switch ext {
	case ".pdf":
		return "pdf"
	case ".dxf":
		return "dxf"
	case ".igs", ".iges":
		return "iges"
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return "image"
	case ".txt", ".csv", ".nc", ".mpr", ".gcode", ".tap", ".cnc", ".ngc", ".iso", ".xml", ".log":
		return "text"
	case ".mp4", ".webm", ".ogv":
		return "video"
	case ".mp3", ".wav", ".m4a", ".ogg":
		return "audio"
	}
	return "external"
}

// indexFor builds (or reuses) a lookup table code -> files for one file type.
func indexFor(ft FileType) map[string][]FoundFile {
	key := ft.Name + "\x00" + strings.Join(ft.Folders, "\x00") + "\x00" + strings.Join(ft.Extensions, ",") + ft.Match
	fileIndexMu.Lock()
	if it, ok := fileIndex[key]; ok && time.Since(it.ts) < cacheTTL {
		fileIndexMu.Unlock()
		return it.byKey
	}
	fileIndexMu.Unlock()

	exts := map[string]bool{}
	for _, e := range ft.Extensions {
		exts[e] = true
	}
	byKey := map[string][]FoundFile{}
	seen := map[string]bool{}
	for _, folder := range ft.Folders {
		for _, e := range scanDirCached(folder) {
			ext := filepath.Ext(e.NameLower)
			if !exts[ext] || seen[e.FullPath] || strings.HasPrefix(e.Name, "~$") {
				continue
			}
			seen[e.FullPath] = true
			stem := strings.TrimSpace(strings.TrimSuffix(e.NameLower, ext))
			f := FoundFile{Path: e.FullPath, Name: e.Name, Ext: ext, Mtime: e.Mtime, Size: e.Size, Viewer: viewerFor(ext)}
			keys := []string{stem}
			if ft.Match == "prefix" {
				keys = codeKeys(stem)
			}
			for _, k := range keys {
				byKey[k] = append(byKey[k], f)
			}
		}
	}
	fileIndexMu.Lock()
	fileIndex[key] = fileIndexItem{ts: time.Now(), byKey: byKey}
	fileIndexMu.Unlock()
	return byKey
}

// findFiles returns the files of one type for a code: exact name first,
// then newest first.
//
// others is the set of all codes (lower case) of the same product. With
// prefix matching, "abc-100-01.pdf" would also look like a revision of
// "abc-100"; when "abc-100-01" is itself a code of the product, the file
// belongs to that part only and is left out here.
func findFiles(ft FileType, code string, others map[string]bool) []FoundFile {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return nil
	}
	var list []FoundFile
	for _, f := range indexFor(ft)[code] {
		stem := strings.TrimSpace(strings.TrimSuffix(strings.ToLower(f.Name), f.Ext))
		f.exact = stem == code
		if !f.exact && belongsToLongerCode(stem, code, others) {
			continue
		}
		list = append(list, f)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].exact != list[j].exact {
			return list[i].exact
		}
		return list[i].Mtime > list[j].Mtime
	})
	return list
}

func belongsToLongerCode(stem, code string, others map[string]bool) bool {
	if len(others) == 0 {
		return false
	}
	for _, k := range codeKeys(stem) {
		if len(k) > len(code) && others[k] {
			return true
		}
	}
	return false
}

// codeSet collects every code of a product (lower case).
func codeSet(productCode string, tree []*TreeNode) map[string]bool {
	set := map[string]bool{strings.ToLower(strings.TrimSpace(productCode)): true}
	walkTree(tree, func(n *TreeNode) {
		if c := strings.ToLower(strings.TrimSpace(n.Code)); c != "" {
			set[c] = true
		}
	})
	return set
}

type TypeFiles struct {
	Type  string      `json:"type"`
	Files []FoundFile `json:"files"`
	List  bool        `json:"list"` // show as a file list
	Ref   bool        `json:"ref"`  // attachments, not a configured file type
}

func findAllFiles(cfg Config, code string, others map[string]bool) []TypeFiles {
	out := []TypeFiles{}
	for _, ft := range cfg.FileTypes {
		files := findFiles(ft, code, others)
		if files == nil {
			files = []FoundFile{}
		}
		out = append(out, TypeFiles{Type: ft.Name, Files: files, List: ft.List})
	}
	if cfg.RefFolder != "" {
		refs := findRefs(cfg, code, others)
		if refs == nil {
			refs = []FoundFile{}
		}
		out = append(out, TypeFiles{Type: refTypeName, Files: refs, List: true, Ref: true})
	}
	return out
}

// markAvailability fills TreeNode.Files with the file types found per code,
// so the tree can show at a glance what exists for each part.
func markAvailability(cfg Config, nodes []*TreeNode, others map[string]bool) {
	walkTree(nodes, func(n *TreeNode) {
		n.Files = []string{}
		for _, ft := range cfg.FileTypes {
			if len(findFiles(ft, n.Code, others)) > 0 {
				n.Files = append(n.Files, ft.Name)
			}
		}
		if hasRefs(cfg, n.Code, others) {
			n.Files = append(n.Files, refTypeName)
		}
		if rec := findPart(n.Code); rec != nil {
			n.Missing = rec.Missing
		}
	})
}

// ---- Attachments ("reference files") ----

// Name of the pseudo file type for attachments: shown as a paper clip.
const refTypeName = "📎"

type refEntry struct {
	e    DirEntryInfo
	dir  bool
	stem string
}

type refIndexItem struct {
	ts    time.Time
	byKey map[string][]refEntry
}

var refIndexes = map[string]refIndexItem{} // guarded by fileIndexMu

func refIndex(folder string) map[string][]refEntry {
	fileIndexMu.Lock()
	if it, ok := refIndexes[folder]; ok && time.Since(it.ts) < cacheTTL {
		fileIndexMu.Unlock()
		return it.byKey
	}
	fileIndexMu.Unlock()
	files, dirs := scanFolder(folder)
	byKey := map[string][]refEntry{}
	add := func(e DirEntryInfo, dir bool) {
		stem := e.NameLower
		if !dir {
			stem = strings.TrimSuffix(stem, filepath.Ext(stem))
		}
		stem = strings.TrimSpace(stem)
		for _, k := range codeKeys(stem) {
			byKey[k] = append(byKey[k], refEntry{e: e, dir: dir, stem: stem})
		}
	}
	for _, d := range dirs {
		add(d, true)
	}
	for _, f := range files {
		add(f, false)
	}
	fileIndexMu.Lock()
	refIndexes[folder] = refIndexItem{ts: time.Now(), byKey: byKey}
	fileIndexMu.Unlock()
	return byKey
}

func refMatches(cfg Config, code string, others map[string]bool) []refEntry {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" || cfg.RefFolder == "" {
		return nil
	}
	var out []refEntry
	for _, r := range refIndex(cfg.RefFolder)[code] {
		if r.stem != code && belongsToLongerCode(r.stem, code, others) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func hasRefs(cfg Config, code string, others map[string]bool) bool {
	return len(refMatches(cfg, code, others)) > 0
}

// findRefs lists the attachments of a code: loose files named after it and
// everything inside sub-folders named after it.
func findRefs(cfg Config, code string, others map[string]bool) []FoundFile {
	var out []FoundFile
	add := func(e DirEntryInfo) {
		if strings.HasPrefix(e.Name, "~$") || e.NameLower == "thumbs.db" || e.NameLower == "desktop.ini" {
			return
		}
		ext := filepath.Ext(e.NameLower)
		out = append(out, FoundFile{Path: e.FullPath, Name: e.Name, Ext: ext, Mtime: e.Mtime, Size: e.Size, Viewer: viewerFor(ext)})
	}
	for _, r := range refMatches(cfg, code, others) {
		if !r.dir {
			add(r.e)
			continue
		}
		for _, f := range scanDirCached(r.e.FullPath) {
			add(f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// ---- ZIP package ----

type ZipEntry struct {
	Path    string
	ArcName string
}

// collectPackage picks the best file of one type for the product and every
// distinct code in its BOM (every file, for types shown as a list).
func collectPackage(cfg Config, ft FileType, productCode string, tree []*TreeNode) []ZipEntry {
	codes := []string{productCode}
	seenCode := map[string]bool{strings.ToLower(productCode): true}
	walkTree(tree, func(n *TreeNode) {
		k := strings.ToLower(n.Code)
		if k != "" && !seenCode[k] {
			seenCode[k] = true
			codes = append(codes, n.Code)
		}
	})

	others := codeSet(productCode, tree)
	var out []ZipEntry
	usedPath := map[string]bool{}
	usedName := map[string]bool{}
	for _, c := range codes {
		files := findFiles(ft, c, others)
		if len(files) > 1 && !ft.List {
			files = files[:1]
		}
		for _, f := range files {
			if usedPath[f.Path] {
				continue
			}
			usedPath[f.Path] = true
			arc := f.Name
			base := strings.TrimSuffix(arc, f.Ext)
			for n := 1; usedName[strings.ToLower(arc)]; n++ {
				arc = fmt.Sprintf("%s_%d%s", base, n, f.Ext)
			}
			usedName[strings.ToLower(arc)] = true
			out = append(out, ZipEntry{Path: f.Path, ArcName: arc})
		}
	}
	return out
}

func writeZip(w io.Writer, entries []ZipEntry) {
	zw := zip.NewWriter(w)
	defer zw.Close()
	for _, item := range entries {
		f, err := os.Open(item.Path)
		if err != nil {
			continue
		}
		hdr := &zip.FileHeader{Name: item.ArcName, Method: zip.Deflate}
		if info, err := f.Stat(); err == nil {
			hdr.Modified = info.ModTime() // keep the file date inside the ZIP
		}
		if fw, err := zw.CreateHeader(hdr); err == nil {
			_, _ = io.Copy(fw, f)
		}
		_ = f.Close()
	}
}
