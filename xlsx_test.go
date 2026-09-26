package main

import (
	"archive/zip"
	"bytes"
	"testing"
)

// buildXLSX assembles an in-memory zip archive from the given path -> XML
// content pairs, standing in for a real .xlsx file.
func buildXLSX(t *testing.T, files map[string]string) *bytes.Reader {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

const xlsxWorkbookOneSheet = `<?xml version="1.0"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
          xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="Budget" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`

const xlsxRelsOneSheet = `<?xml version="1.0"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`

func TestLintXLSXBasic(t *testing.T) {
	sheet := `<?xml version="1.0"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="2">
      <c r="B2"><v>10</v></c>
      <c r="D2"><f>B2*2</f><v>20</v></c>
    </row>
    <row r="3">
      <c r="D3"><f>IF(NOW()>TODAY(),D2*0.8,D2)</f><v>16</v></c>
    </row>
    <row r="4">
      <c r="D4"><f>D3/0</f><v>0</v></c>
    </row>
  </sheetData>
</worksheet>`

	zr := buildXLSX(t, map[string]string{
		"xl/workbook.xml":              xlsxWorkbookOneSheet,
		"xl/_rels/workbook.xml.rels":   xlsxRelsOneSheet,
		"xl/worksheets/sheet1.xml":     sheet,
	})

	findings, err := LintXLSX(zr, int64(zr.Len()))
	if err != nil {
		t.Fatalf("LintXLSX returned error: %v", err)
	}

	want := map[string]string{
		"Budget!D3": "volatile",
		"Budget!D4": "div-by-zero",
	}
	got := map[string]string{}
	for _, f := range findings {
		got[f.Cell] = f.Rule
	}
	for cell, rule := range want {
		if got[cell] != rule {
			t.Errorf("expected %s to have rule %s, findings: %+v", cell, rule, findings)
		}
	}

	// NOW and TODAY each fire their own volatile finding on D3.
	volatileCount := 0
	for _, f := range findings {
		if f.Cell == "Budget!D3" && f.Rule == "volatile" {
			volatileCount++
		}
	}
	if volatileCount != 2 {
		t.Errorf("expected 2 volatile findings on D3, got %d: %+v", volatileCount, findings)
	}

	if len(findings) != 3 {
		t.Errorf("got %d findings, want 3: %+v", len(findings), findings)
	}
}

func TestLintXLSXMultiSheet(t *testing.T) {
	workbook := `<?xml version="1.0"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
          xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="First" sheetId="1" r:id="rId1"/>
    <sheet name="Second" sheetId="2" r:id="rId2"/>
  </sheets>
</workbook>`

	rels := `<?xml version="1.0"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/>
</Relationships>`

	sheet1 := `<?xml version="1.0"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1"><c r="A1"><f>A2/0</f></c></row>
  </sheetData>
</worksheet>`

	sheet2 := `<?xml version="1.0"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1"><c r="A1"><f>RAND()</f></c></row>
  </sheetData>
</worksheet>`

	zr := buildXLSX(t, map[string]string{
		"xl/workbook.xml":            workbook,
		"xl/_rels/workbook.xml.rels": rels,
		"xl/worksheets/sheet1.xml":   sheet1,
		"xl/worksheets/sheet2.xml":   sheet2,
	})

	findings, err := LintXLSX(zr, int64(zr.Len()))
	if err != nil {
		t.Fatalf("LintXLSX returned error: %v", err)
	}

	cells := map[string]bool{}
	for _, f := range findings {
		cells[f.Cell] = true
	}
	if !cells["First!A1"] || !cells["Second!A1"] {
		t.Errorf("expected findings prefixed with sheet names, got %+v", findings)
	}
}

func TestLintXLSXSkipsSharedFormulaSlave(t *testing.T) {
	sheet := `<?xml version="1.0"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1">
      <c r="A1"><f t="shared" ref="A1:A2" si="0">A1/0</f></c>
      <c r="A2"><f t="shared" si="0"/></c>
    </row>
  </sheetData>
</worksheet>`

	zr := buildXLSX(t, map[string]string{
		"xl/workbook.xml":            xlsxWorkbookOneSheet,
		"xl/_rels/workbook.xml.rels": xlsxRelsOneSheet,
		"xl/worksheets/sheet1.xml":   sheet,
	})

	findings, err := LintXLSX(zr, int64(zr.Len()))
	if err != nil {
		t.Fatalf("LintXLSX returned error: %v", err)
	}
	if len(findings) != 1 || findings[0].Cell != "Budget!A1" {
		t.Errorf("expected a single finding on A1 only, got %+v", findings)
	}
}

func TestLintXLSXFallbackWithoutWorkbook(t *testing.T) {
	sheet := `<?xml version="1.0"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1"><c r="A1"><f>A1/0</f></c></row>
  </sheetData>
</worksheet>`

	zr := buildXLSX(t, map[string]string{
		"xl/worksheets/sheet1.xml": sheet,
	})

	findings, err := LintXLSX(zr, int64(zr.Len()))
	if err != nil {
		t.Fatalf("LintXLSX returned error: %v", err)
	}
	if len(findings) != 1 || findings[0].Cell != "Sheet1!A1" {
		t.Errorf("expected fallback sheet name Sheet1, got %+v", findings)
	}
}

func TestLintXLSXNoWorksheets(t *testing.T) {
	zr := buildXLSX(t, map[string]string{
		"xl/workbook.xml": xlsxWorkbookOneSheet,
	})

	if _, err := LintXLSX(zr, int64(zr.Len())); err == nil {
		t.Fatal("expected an error when no worksheets are present, got nil")
	}
}
