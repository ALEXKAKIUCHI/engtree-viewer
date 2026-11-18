package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A BOM in the layout of the Facta exports: the material code is in the
// "Kardex" column. Three levels: product > drawer (x2) > parts.
const factaBOM = `Nº do item;Nº da peça;Descrição;QTD.;Kardex;Comprimento;Largura
1;49815P107;FRENTE 2;1;100234;800;730
2;49815-GAV;GAVETA;2;;;
2.1;49815P121;LATERAL GAVETA;2;100234;427;183
2.2;49815P139;FUNDO GAVETA;1;MDF6BR;500;400
3;49815P200;TUBO 20X20;4;TB2020;1200;
4;49815P300;PARAFUSO;8;;;
5;49815P400;TAMPO;1;999999;900;600
6;49815P500;PORTA GRANDE;1;100234;3000;700
`

func testMaterials() MaterialList {
	return MaterialList{
		MinOffcutLength: 300, MinOffcutWidth: 300,
		Materials: []Material{
			{Code: "100234", AltCode: "MDF15BR", Description: "MDF BRANCO 15", Kind: KindSheet, Thickness: 15, Length: 2750, Width: 1840},
			{Code: "MDF6BR", Description: "MDF BRANCO 6", Kind: KindSheet, Thickness: 6, Length: 2750, Width: 1840, Grain: true},
			{Code: "TB2020", Description: "TUBO 20X20", Kind: KindBar, Thickness: 1.2, Length: 6000},
		},
	}
}

func bomFromCSV(t *testing.T, csv string) BomResult {
	t.Helper()
	p := filepath.Join(t.TempDir(), "BOM - 49815 - BALCAO.csv")
	if err := os.WriteFile(p, []byte(csv), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	return buildBom(Product{Code: "49815", BomFile: p}, cfg)
}

func TestCutLinesFromKardexColumn(t *testing.T) {
	bom := bomFromCSV(t, factaBOM)
	res := cutLines(bom, defaultCutColumns(), testMaterials(), 3)
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings: %v", res.Warnings)
	}
	type row struct {
		code, mat string
		l, w      float64
		qty       int
		kind      string
		warn      string
	}
	var got []row
	for _, l := range res.Lines {
		got = append(got, row{l.Code, l.Material, l.Length, l.Width, l.Qty, l.Kind, strings.Join(l.Warnings, ",")})
	}
	want := []row{
		{"49815P107", "100234", 800, 730, 3, KindSheet, ""},          // 1 × 3 products
		{"49815P121", "100234", 427, 183, 12, KindSheet, ""},         // 2 per drawer × 2 drawers × 3
		{"49815P139", "MDF6BR", 500, 400, 6, KindSheet, ""},          // 1 × 2 × 3
		{"49815P200", "TB2020", 1200, 0, 12, KindBar, ""},            // bar: no width needed
		{"49815P400", "999999", 900, 600, 3, "", "unknown_material"}, // not in the list
		{"49815P500", "100234", 3000, 700, 3, KindSheet, "too_big"},  // longer than the sheet
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lines:\n got  %v\n want %v", got, want)
	}
	// The assembly (GAVETA) and the screw (no material) are not cut.
	for _, l := range res.Lines {
		if l.Code == "49815-GAV" || l.Code == "49815P300" {
			t.Errorf("%s should not be a cut line", l.Code)
		}
	}
	if !res.Lines[2].Grain || res.Lines[0].Grain {
		t.Error("grain should follow the material")
	}
}

func TestCutLinesAlternativeCodeAndCase(t *testing.T) {
	bom := bomFromCSV(t, "Item;Código;Descrição;Qtd;Kardex;Comp;Larg\n1;P1;LATERAL;1; mdf15br ;500;400\n")
	res := cutLines(bom, defaultCutColumns(), testMaterials(), 1)
	if len(res.Lines) != 1 || len(res.Lines[0].Warnings) != 0 || res.Lines[0].Thickness != 15 {
		t.Fatalf("alternative code not matched: %+v", res.Lines)
	}
}

func TestCutLinesUnitsAndBadQuantities(t *testing.T) {
	bom := bomFromCSV(t, "Item;Código;Descrição;Qtd;Kardex;Comprimento;Largura\n1;P1;A;1,5;100234;50;40,5\n2;P2;B;x;100234;;30\n")
	cc := defaultCutColumns()
	cc.Unit = "cm"
	res := cutLines(bom, cc, testMaterials(), 1)
	if len(res.Lines) != 2 {
		t.Fatalf("%+v", res.Lines)
	}
	a, b := res.Lines[0], res.Lines[1]
	if a.Length != 500 || a.Width != 405 || a.Qty != 2 || strings.Join(a.Warnings, ",") != "bad_qty" {
		t.Errorf("A: %+v", a)
	}
	if b.Qty != 1 || strings.Join(b.Warnings, ",") != "bad_qty,no_length" {
		t.Errorf("B: %+v", b)
	}
}

func TestCutLinesWithoutMaterialColumn(t *testing.T) {
	bom := bomFromCSV(t, "Item;Código;Descrição;Qtd\n1;P1;A;1\n")
	res := cutLines(bom, defaultCutColumns(), testMaterials(), 1)
	if len(res.Lines) != 0 || strings.Join(res.Warnings, ",") != "no_material_column" {
		t.Errorf("%+v", res)
	}
}

func TestMaterialListValidate(t *testing.T) {
	l := testMaterials()
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	l.Materials = append(l.Materials, Material{Code: "mdf15br", Kind: KindSheet, Length: 1, Width: 1})
	err := l.Validate()
	if err == nil || !strings.Contains(err.Error(), "used by materials 100234 and mdf15br") {
		t.Errorf("repeated code accepted: %v", err)
	}
	bad := MaterialList{Materials: []Material{{Code: "X", Kind: "roll"}, {Kind: KindBar}}}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "kind must be") || !strings.Contains(err.Error(), "no code") {
		t.Errorf("bad materials accepted: %v", err)
	}
}

func TestMaterialsSaveAndLoad(t *testing.T) {
	old := dataDir
	dataDir = t.TempDir()
	defer func() { dataDir = old }()
	if got := loadMaterials(); len(got.Materials) != 0 {
		t.Fatalf("empty list expected, got %+v", got)
	}
	want := testMaterials()
	want.Materials[2].Offcuts = []Offcut{{Length: 2500, Width: 99, Quantity: 2}, {Length: 0, Quantity: 1}}
	if err := saveMaterials(want); err != nil {
		t.Fatal(err)
	}
	got := loadMaterials()
	if len(got.Materials) != 3 || got.MinOffcutWidth != 300 {
		t.Fatalf("%+v", got)
	}
	tube, _ := got.Find("tb2020")
	if len(tube.Offcuts) != 1 || tube.Offcuts[0].Width != 0 {
		t.Errorf("bar offcuts not cleaned: %+v", tube.Offcuts)
	}
	if l, w := got.MinOffcut(tube); l != 300 || w != 0 {
		t.Errorf("min offcut of a bar: %v x %v", l, w)
	}
}
