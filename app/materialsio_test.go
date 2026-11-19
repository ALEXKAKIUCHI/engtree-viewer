package app

import (
	"reflect"
	"testing"
)

func TestMaterialsRoundTrip(t *testing.T) {
	l := testMaterials()
	l.Materials[0].Offcuts = []Offcut{{Length: 1000, Width: 800, Quantity: 2}, {Length: 600, Width: 500.5, Quantity: 1}}
	l.Materials[0].MinOffcutLength = 250
	l.Materials[2].Offcuts = []Offcut{{Length: 2500, Quantity: 3}}
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	xlsx, err := writeXLSX("Materiais", materialRows(l))
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"cadastro.xlsx": xlsx, "cadastro.csv": materialsCSV(materialRows(l))} {
		res, err := importMaterials(name, data, MaterialList{MinOffcutLength: 300})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(res.Problems) != 0 || len(res.Missing) != 0 || res.Added != 3 || res.Updated != 0 {
			t.Fatalf("%s: %+v", name, res)
		}
		if !reflect.DeepEqual(res.List.Materials, l.Materials) {
			t.Errorf("%s: read back differs:\n got  %+v\n want %+v", name, res.List.Materials, l.Materials)
		}
		if res.List.MinOffcutLength != 300 {
			t.Errorf("%s: the general defaults of the current list must stay", name)
		}
	}
}

func TestMaterialsImportOwnHeadersAndMerge(t *testing.T) {
	// A sheet made by hand: other header names, other order, a title row,
	// Portuguese words for type and grain, a row to update and errors.
	csv := "Cadastro de chapas\n" +
		"Cód. MP;Descrição;Esp.;Comp.;Larg.;Tipo;Veio;Retalhos\n" +
		"100234;MDF BRANCO 15 NOVO;15;2750;1850;Chapa;não;\n" +
		"200100;COMPENSADO 18;18;2200;1600;chapa;Sim;1200x800x2\n" +
		"300200;TUBO 30X30;1,5;6000;;Tubo;;\"2500x2; 1200\"\n" +
		"400300;ERRADO;15;;1830;chapa;;\n" +
		"500400;ROLO;1;100;100;rolo;;\n" +
		";;;;;;;\n"
	res, err := importMaterials("chapas.csv", []byte(csv), testMaterials())
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 || res.Added != 2 {
		t.Errorf("updated %d added %d: %+v", res.Updated, res.Added, res.Problems)
	}
	want := []ImportProblem{{Row: 6, Code: "400300", Reason: "no_size"}, {Row: 7, Code: "500400", Reason: "bad_type", Value: "rolo"}}
	if !reflect.DeepEqual(res.Problems, want) {
		t.Errorf("problems: %q", res.Problems)
	}
	mdf, _ := res.List.Find("100234")
	if mdf.Description != "MDF BRANCO 15 NOVO" || mdf.Width != 1850 || mdf.AltCode != "" {
		t.Errorf("updated material: %+v", mdf)
	}
	if ply, _ := res.List.Find("200100"); !ply.Grain || len(ply.Offcuts) != 1 || ply.Offcuts[0].Quantity != 2 {
		t.Errorf("plywood: %+v", ply)
	}
	tube, _ := res.List.Find("300200")
	if tube.Kind != KindBar || tube.Thickness != 1.5 || len(tube.Offcuts) != 2 || tube.Offcuts[1] != (Offcut{Length: 1200, Quantity: 1}) {
		t.Errorf("tube: %+v", tube)
	}
	if _, ok := res.List.Find("TB2020"); !ok {
		t.Error("materials not in the sheet must stay")
	}
}

func TestMaterialsImportRepeatedCodeAndMissingColumns(t *testing.T) {
	csv := "Código;Código alternativo;Comprimento;Largura\nA1;;2750;1830\nB2;A1;2750;1830\n"
	res, err := importMaterials("x.csv", []byte(csv), MaterialList{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) != 1 || res.Problems[0] != (ImportProblem{Code: "A1", Reason: "repeated_code", Value: "A1 / B2"}) {
		t.Errorf("problems: %q", res.Problems)
	}
	res, _ = importMaterials("x.csv", []byte("Nome;Valor\nA;1\n"), MaterialList{})
	if len(res.Missing) == 0 {
		t.Error("missing columns not reported")
	}
	if rows := materialRows(MaterialList{}); len(rows) != 1 {
		t.Errorf("template should be the header only: %v", rows)
	}
}
