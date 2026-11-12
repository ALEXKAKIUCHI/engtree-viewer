package main

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Spreadsheet formats EngTree can read. A ".xls" file is accepted when it is
// really text (tab-separated exports are often saved with that extension) or
// an .xlsx with the wrong extension. The old binary Excel format is not read:
// save it as .xlsx or .csv.
var sheetExts = map[string]bool{".xlsx": true, ".xls": true, ".csv": true, ".txt": true, ".tsv": true}

func isSheetFile(name string) bool {
	return sheetExts[strings.ToLower(filepath.Ext(name))]
}

// readTable reads any supported spreadsheet as rows of trimmed cells.
func readTable(path string) [][]string {
	if path == "" {
		return nil
	}
	if strings.EqualFold(filepath.Ext(path), ".xlsx") {
		return readXLSX(path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if bytes.HasPrefix(raw, []byte("PK\x03\x04")) {
		return readXLSX(path) // an .xlsx saved with another extension
	}
	text := decodeText(raw)
	if text == "" {
		return nil
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")

	rows := parseDelimited(text, detectDelimiter(text))

	var out [][]string
	for _, row := range rows {
		empty := true
		for i := range row {
			row[i] = cleanCellText(row[i])
			if row[i] != "" {
				empty = false
			}
		}
		if !empty {
			out = append(out, row)
		}
	}
	return out
}

// parseDelimited reads CSV-style text (Excel quotes fields that contain the
// delimiter). If the quoting is broken it falls back to a plain split, so a
// stray quote never makes the whole file unreadable.
func parseDelimited(text string, delim rune) [][]string {
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = delim
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	if rows, err := r.ReadAll(); err == nil {
		return rows
	}
	var rows [][]string
	for _, line := range strings.Split(text, "\n") {
		rows = append(rows, strings.Split(line, string(delim)))
	}
	return rows
}

// decodeText accepts UTF-8 (with or without BOM), UTF-16 and falls back to
// Windows-1252/Latin-1, which is what Excel writes on many systems.
func decodeText(raw []byte) string {
	// Old binary Excel (.xls renamed or saved with the wrong extension).
	if len(raw) >= 4 && bytes.Equal(raw[:4], []byte{0xD0, 0xCF, 0x11, 0xE0}) {
		return ""
	}
	if len(raw) >= 2 && (raw[0] == 0xFF && raw[1] == 0xFE || raw[0] == 0xFE && raw[1] == 0xFF) {
		return decodeUTF16(raw)
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	if utf8.Valid(raw) {
		return string(raw)
	}
	runes := make([]rune, len(raw))
	for i, b := range raw {
		runes[i] = rune(b)
	}
	return string(runes)
}

func decodeUTF16(raw []byte) string {
	le := raw[0] == 0xFF
	raw = raw[2:]
	u := make([]uint16, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		if le {
			u = append(u, uint16(raw[i])|uint16(raw[i+1])<<8)
		} else {
			u = append(u, uint16(raw[i])<<8|uint16(raw[i+1]))
		}
	}
	return string(utf16.Decode(u))
}

// detectDelimiter looks at the first lines and picks tab, semicolon or comma
// (Excel uses ";" for CSV in locales where "," is the decimal mark).
func detectDelimiter(text string) rune {
	lines := strings.SplitN(text, "\n", 6)
	count := map[rune]int{}
	for _, l := range lines {
		for _, d := range []rune{'\t', ';', ','} {
			count[d] += strings.Count(l, string(d))
		}
	}
	best, bestN := '\t', 0
	for _, d := range []rune{'\t', ';', ','} {
		if count[d] > bestN {
			best, bestN = d, count[d]
		}
	}
	return best
}
