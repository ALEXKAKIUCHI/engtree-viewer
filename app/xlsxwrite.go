package app

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// Minimal .xlsx writer using only the standard library: one sheet, text
// and numbers, a bold first row. Enough for the summaries the program
// hands out; the reader in xlsx.go reads it back.

// xlsxCell is a cell value: a string, or an int or float64 written as a
// number.
type xlsxCell = any

func writeXLSX(sheetName string, rows [][]xlsxCell) ([]byte, error) {
	var sheet strings.Builder
	sheet.WriteString(xml.Header)
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r, row := range rows {
		fmt.Fprintf(&sheet, `<row r="%d">`, r+1)
		for c, v := range row {
			ref := xlsxColName(c) + strconv.Itoa(r+1)
			style := ""
			if r == 0 {
				style = ` s="1"`
			}
			switch n := v.(type) {
			case int:
				fmt.Fprintf(&sheet, `<c r="%s"%s><v>%d</v></c>`, ref, style, n)
			case float64:
				fmt.Fprintf(&sheet, `<c r="%s"%s><v>%s</v></c>`, ref, style, strconv.FormatFloat(n, 'f', -1, 64))
			case nil:
			default:
				var esc bytes.Buffer
				_ = xml.EscapeText(&esc, []byte(fmt.Sprint(v)))
				fmt.Fprintf(&sheet, `<c r="%s" t="inlineStr"%s><is><t xml:space="preserve">%s</t></is></c>`, ref, style, esc.String())
			}
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)

	var name bytes.Buffer
	_ = xml.EscapeText(&name, []byte(sheetName))
	files := []struct{ name, body string }{
		{"[Content_Types].xml", xml.Header + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
			`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
			`</Types>`},
		{"_rels/.rels", xml.Header + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
			`</Relationships>`},
		{"xl/workbook.xml", xml.Header + `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
			`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets><sheet name="` + name.String() + `" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", xml.Header + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
			`</Relationships>`},
		{"xl/styles.xml", xml.Header + `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
			`<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>` +
			`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
			`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
			`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
			`<cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
			`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs>` +
			`</styleSheet>`},
		{"xl/worksheets/sheet1.xml", sheet.String()},
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(f.body)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// xlsxColName turns 0, 1, ..., 25, 26 into A, B, ..., Z, AA.
func xlsxColName(i int) string {
	s := ""
	for i >= 0 {
		s = string(rune('A'+i%26)) + s
		i = i/26 - 1
	}
	return s
}
