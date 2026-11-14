package app

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"path"
	"strconv"
	"strings"
)

// Minimal .xlsx reader using only the standard library.
// An .xlsx file is a ZIP with XML inside: we read the first sheet and return
// the rows as [][]string, the same shape the text/CSV readers return, so the
// rest of the program does not care where a spreadsheet came from.

type xlsxSharedStrings struct {
	Items []struct {
		T    string `xml:"t"`
		Runs []struct {
			T string `xml:"t"`
		} `xml:"r"`
	} `xml:"si"`
}

type xlsxSheet struct {
	Rows []struct {
		Cells []struct {
			Ref    string `xml:"r,attr"`
			Type   string `xml:"t,attr"`
			Value  string `xml:"v"`
			Inline struct {
				T    string `xml:"t"`
				Runs []struct {
					T string `xml:"t"`
				} `xml:"r"`
			} `xml:"is"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

type xlsxWorkbook struct {
	Sheets []struct {
		RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
	} `xml:"sheets>sheet"`
}

type xlsxRels struct {
	Rels []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

func readZipXML(files map[string]*zip.File, name string, v any) bool {
	f, ok := files[name]
	if !ok {
		return false
	}
	rc, err := f.Open()
	if err != nil {
		return false
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return false
	}
	return xml.Unmarshal(data, v) == nil
}

// firstSheetPath finds the XML file of the first sheet in the workbook.
func firstSheetPath(files map[string]*zip.File) string {
	var wb xlsxWorkbook
	var rels xlsxRels
	if readZipXML(files, "xl/workbook.xml", &wb) && len(wb.Sheets) > 0 &&
		readZipXML(files, "xl/_rels/workbook.xml.rels", &rels) {
		for _, r := range rels.Rels {
			if r.ID == wb.Sheets[0].RID {
				t := strings.TrimPrefix(r.Target, "/")
				if !strings.HasPrefix(t, "xl/") {
					t = path.Join("xl", t)
				}
				if _, ok := files[t]; ok {
					return t
				}
			}
		}
	}
	return "xl/worksheets/sheet1.xml"
}

// columnIndex turns a cell reference ("C12") into a zero-based column index (2).
func columnIndex(ref string) int {
	col := 0
	for _, ch := range ref {
		if ch >= 'A' && ch <= 'Z' {
			col = col*26 + int(ch-'A') + 1
		} else if ch >= 'a' && ch <= 'z' {
			col = col*26 + int(ch-'a') + 1
		} else {
			break
		}
	}
	return col - 1
}

// cleanCellText collapses line breaks and repeated spaces inside a cell
// (Excel stores "PART\nNUMBER" when a header cell wraps).
func cleanCellText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// formatNumber prints numbers without trailing zeros or float noise
// (1294.8980000001 -> "1294.898", 400 -> "400").
func formatNumber(v string) string {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return v
	}
	// 12 significant digits hide binary noise such as 0.30000000000000004.
	if r, err := strconv.ParseFloat(strconv.FormatFloat(f, 'g', 12, 64), 64); err == nil {
		f = r
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func readXLSX(filePath string) [][]string {
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return nil
	}
	defer zr.Close()

	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[strings.TrimPrefix(f.Name, "/")] = f
	}

	var shared []string
	var ss xlsxSharedStrings
	if readZipXML(files, "xl/sharedStrings.xml", &ss) {
		shared = make([]string, len(ss.Items))
		for i, it := range ss.Items {
			if len(it.Runs) > 0 {
				var b strings.Builder
				for _, r := range it.Runs {
					b.WriteString(r.T)
				}
				shared[i] = b.String()
			} else {
				shared[i] = it.T
			}
		}
	}

	var sheet xlsxSheet
	if !readZipXML(files, firstSheetPath(files), &sheet) {
		return nil
	}

	var rows [][]string
	for _, r := range sheet.Rows {
		var cols []string
		next := 0
		for _, c := range r.Cells {
			idx := next
			if c.Ref != "" {
				idx = columnIndex(c.Ref)
			}
			if idx < 0 || idx > 1000 {
				continue
			}
			for len(cols) <= idx {
				cols = append(cols, "")
			}
			var val string
			switch c.Type {
			case "s":
				if i, err := strconv.Atoi(strings.TrimSpace(c.Value)); err == nil && i >= 0 && i < len(shared) {
					val = shared[i]
				}
			case "inlineStr":
				if len(c.Inline.Runs) > 0 {
					for _, run := range c.Inline.Runs {
						val += run.T
					}
				} else {
					val = c.Inline.T
				}
			case "b":
				if c.Value == "1" {
					val = "TRUE"
				} else {
					val = "FALSE"
				}
			case "str", "e":
				val = c.Value
			default: // number (also the cached result of formulas)
				val = formatNumber(c.Value)
			}
			cols[idx] = cleanCellText(val)
			next = idx + 1
		}
		// Skip fully empty rows
		empty := true
		for _, v := range cols {
			if v != "" {
				empty = false
				break
			}
		}
		if !empty {
			rows = append(rows, cols)
		}
	}
	return rows
}
