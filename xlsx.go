package main

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// LintXLSX reads formula cells straight out of an .xlsx workbook and runs
// the same checks Lint applies to CSV input, so there's no more need for a
// FORMULATEXT() export step first.
//
// Only cells that carry an <f> element (a formula) are examined; computed
// <v> values are ignored, same as CSV cells that don't start with "=".
// Slave cells of a shared formula group (t="shared" with no formula body,
// just a reference to the master cell's index) are skipped, since checking
// them correctly would mean re-deriving the translated formula text rather
// than reading it.
func LintXLSX(r io.ReaderAt, size int64) ([]Finding, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("opening xlsx: %w", err)
	}

	sheets, err := xlsxSheets(zr)
	if err != nil {
		return nil, err
	}
	if len(sheets) == 0 {
		return nil, fmt.Errorf("opening xlsx: no worksheets found")
	}

	var findings []Finding
	for _, sheet := range sheets {
		f, err := zr.Open(sheet.path)
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", sheet.path, err)
		}
		findings, err = xlsxSheetFindings(sheet.name, f, findings)
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return findings, nil
}

type xlsxSheetRef struct {
	name string
	path string
}

// xlsxSheets returns the workbook's sheets in tab order, resolved from
// xl/workbook.xml and xl/_rels/workbook.xml.rels. If either file is missing
// or doesn't parse, it falls back to whatever xl/worksheets/sheetN.xml
// entries exist in the archive, ordered by N.
func xlsxSheets(zr *zip.Reader) ([]xlsxSheetRef, error) {
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}

	if wb, ok := files["xl/workbook.xml"]; ok {
		if rels, ok := files["xl/_rels/workbook.xml.rels"]; ok {
			if sheets, err := xlsxSheetsFromWorkbook(wb, rels); err == nil && len(sheets) > 0 {
				return sheets, nil
			}
		}
	}

	return xlsxSheetsFallback(files), nil
}

type xlsxWorkbookXML struct {
	Sheets []struct {
		Name  string     `xml:"name,attr"`
		Attrs []xml.Attr `xml:",any,attr"`
	} `xml:"sheets>sheet"`
}

type xlsxRelsXML struct {
	Relationships []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

func xlsxSheetsFromWorkbook(wbFile, relsFile *zip.File) ([]xlsxSheetRef, error) {
	wbData, err := readZipFile(wbFile)
	if err != nil {
		return nil, err
	}
	var wb xlsxWorkbookXML
	if err := xml.Unmarshal(wbData, &wb); err != nil {
		return nil, fmt.Errorf("parsing workbook.xml: %w", err)
	}

	relsData, err := readZipFile(relsFile)
	if err != nil {
		return nil, err
	}
	var rels xlsxRelsXML
	if err := xml.Unmarshal(relsData, &rels); err != nil {
		return nil, fmt.Errorf("parsing workbook.xml.rels: %w", err)
	}
	targets := map[string]string{}
	for _, rel := range rels.Relationships {
		targets[rel.ID] = rel.Target
	}

	var sheets []xlsxSheetRef
	for _, s := range wb.Sheets {
		var rID string
		for _, a := range s.Attrs {
			// The relationship id is written as r:id; the namespace prefix
			// is dropped by the decoder, but its local name stays "id" and
			// nothing else on <sheet> is called that.
			if a.Name.Local == "id" {
				rID = a.Value
				break
			}
		}
		target, ok := targets[rID]
		if !ok {
			continue
		}
		sheets = append(sheets, xlsxSheetRef{name: s.Name, path: xlsxResolveTarget(target)})
	}
	return sheets, nil
}

func xlsxResolveTarget(target string) string {
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(target, "/")
	}
	return "xl/" + target
}

func xlsxSheetsFallback(files map[string]*zip.File) []xlsxSheetRef {
	type numbered struct {
		n    int
		path string
	}
	var found []numbered
	for name := range files {
		if !strings.HasPrefix(name, "xl/worksheets/sheet") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		numPart := strings.TrimSuffix(strings.TrimPrefix(name, "xl/worksheets/sheet"), ".xml")
		n, err := strconv.Atoi(numPart)
		if err != nil {
			continue
		}
		found = append(found, numbered{n: n, path: name})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].n < found[j].n })

	sheets := make([]xlsxSheetRef, len(found))
	for i, f := range found {
		sheets[i] = xlsxSheetRef{name: fmt.Sprintf("Sheet%d", f.n), path: f.path}
	}
	return sheets
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", f.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", f.Name, err)
	}
	return data, nil
}

type xlsxSheetXML struct {
	Rows []struct {
		R     int `xml:"r,attr"`
		Cells []struct {
			Ref     string `xml:"r,attr"`
			Formula string `xml:"f"`
		} `xml:"c"`
	} `xml:"sheetData>row"`
}

func xlsxSheetFindings(sheetName string, r io.Reader, findings []Finding) ([]Finding, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading sheet %q: %w", sheetName, err)
	}

	var sheet xlsxSheetXML
	if err := xml.Unmarshal(data, &sheet); err != nil {
		return nil, fmt.Errorf("parsing sheet %q: %w", sheetName, err)
	}

	for _, row := range sheet.Rows {
		for _, cell := range row.Cells {
			formula := strings.TrimSpace(cell.Formula)
			if formula == "" {
				continue
			}
			ref := cell.Ref
			if sheetName != "" {
				ref = sheetName + "!" + ref
			}
			findings = append(findings, checkFormula(row.R, ref, "="+formula)...)
		}
	}
	return findings, nil
}
