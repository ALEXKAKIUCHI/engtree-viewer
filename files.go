package main

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
	fileIndexMu.Unlock()
}

func scanDirCached(folder string) []DirEntryInfo {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return nil
	}
	dirCacheMu.Lock()
	if item, ok := dirCache[folder]; ok && time.Since(item.ts) < cacheTTL {
		dirCacheMu.Unlock()
		return item.entries
	}
	dirCacheMu.Unlock()

	raw, err := os.ReadDir(folder)
	if err != nil {
		return nil
	}
	var files []DirEntryInfo
	for _, e := range raw {
		if e.IsDir() {
			continue
		}
		var mtime float64
		var size int64
		if info, err := e.Info(); err == nil {
			mtime = float64(info.ModTime().Unix())
			size = info.Size()
		}
		files = append(files, DirEntryInfo{
			Name:      e.Name(),
			NameLower: strings.ToLower(e.Name()),
			FullPath:  filepath.Join(folder, e.Name()),
			Mtime:     mtime,
			Size:      size,
		})
	}
	dirCacheMu.Lock()
	dirCache[folder] = dirCacheItem{ts: time.Now(), entries: files}
	dirCacheMu.Unlock()
	return files
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
	case ".txt", ".csv", ".nc", ".mpr", ".gcode", ".tap":
		return "text"
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
func findFiles(ft FileType, code string) []FoundFile {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return nil
	}
	list := append([]FoundFile(nil), indexFor(ft)[code]...)
	for i := range list {
		list[i].exact = strings.TrimSuffix(strings.ToLower(list[i].Name), list[i].Ext) == code
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].exact != list[j].exact {
			return list[i].exact
		}
		return list[i].Mtime > list[j].Mtime
	})
	return list
}

type TypeFiles struct {
	Type  string      `json:"type"`
	Files []FoundFile `json:"files"`
}

func findAllFiles(cfg Config, code string) []TypeFiles {
	out := []TypeFiles{}
	for _, ft := range cfg.FileTypes {
		files := findFiles(ft, code)
		if files == nil {
			files = []FoundFile{}
		}
		out = append(out, TypeFiles{Type: ft.Name, Files: files})
	}
	return out
}

// markAvailability fills TreeNode.Files with the file types found per code,
// so the tree can show at a glance what exists for each part.
func markAvailability(cfg Config, nodes []*TreeNode) {
	walkTree(nodes, func(n *TreeNode) {
		n.Files = []string{}
		for _, ft := range cfg.FileTypes {
			if len(findFiles(ft, n.Code)) > 0 {
				n.Files = append(n.Files, ft.Name)
			}
		}
	})
}

// ---- ZIP package ----

type ZipEntry struct {
	Path    string
	ArcName string
}

// collectPackage picks the best file of one type for the product and every
// distinct code in its BOM.
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

	var out []ZipEntry
	usedPath := map[string]bool{}
	usedName := map[string]bool{}
	for _, c := range codes {
		files := findFiles(ft, c)
		if len(files) == 0 || usedPath[files[0].Path] {
			continue
		}
		f := files[0]
		usedPath[f.Path] = true
		arc := f.Name
		base := strings.TrimSuffix(arc, f.Ext)
		for n := 1; usedName[strings.ToLower(arc)]; n++ {
			arc = fmt.Sprintf("%s_%d%s", base, n, f.Ext)
		}
		usedName[strings.ToLower(arc)] = true
		out = append(out, ZipEntry{Path: f.Path, ArcName: arc})
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
		if fw, err := zw.Create(item.ArcName); err == nil {
			_, _ = io.Copy(fw, f)
		}
		_ = f.Close()
	}
}
