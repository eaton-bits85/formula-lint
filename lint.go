package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// Finding is one thing wrong with one cell.
type Finding struct {
	Line    int    // 1-indexed row in the source file
	Cell    string // A1-style reference, e.g. "D7"
	Rule    string
	Message string
}

const (
	maxFormulaLen   = 200 // formulas longer than this are hard to review by eye
	maxNestingDepth = 7   // matched to roughly what fits on one screen line
)

// Volatile functions force a full recalculation of the workbook every time
// anything changes, not just their own dependents. One or two are fine; a
// formula full of them is why the sheet takes ten seconds to respond to a
// keystroke.
var volatileFuncs = []string{
	"NOW(", "TODAY(", "RAND(", "RANDBETWEEN(", "OFFSET(", "INDIRECT(",
}

// Lint reads a CSV file and returns one Finding per problem found in cells
// whose content starts with "=". Plain values are ignored.
//
// The CSV is expected to contain formula text, not computed values -- most
// spreadsheet tools only export values by default, so producing this input
// usually means a small export script or a helper column using
// FORMULATEXT(). See README.md for details.
func Lint(r io.Reader) ([]Finding, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // rows don't have to be the same width

	var findings []Finding
	row := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading csv: %w", err)
		}
		row++
		for col, cell := range record {
			cell = strings.TrimSpace(cell)
			if !strings.HasPrefix(cell, "=") {
				continue
			}
			ref := cellRef(col, row)
			findings = append(findings, checkFormula(row, ref, cell)...)
		}
	}
	return findings, nil
}

func checkFormula(line int, ref, formula string) []Finding {
	var out []Finding
	upper := strings.ToUpper(formula)

	for _, fn := range volatileFuncs {
		if strings.Contains(upper, fn) {
			out = append(out, Finding{
				Line: line, Cell: ref, Rule: "volatile",
				Message: fmt.Sprintf("uses volatile function %s, recalculates on every change",
					strings.TrimSuffix(fn, "(")),
			})
		}
	}

	if len(formula) > maxFormulaLen {
		out = append(out, Finding{
			Line: line, Cell: ref, Rule: "long-formula",
			Message: fmt.Sprintf("formula is %d characters, consider splitting into helper cells", len(formula)),
		})
	}

	if depth := maxParenDepth(formula); depth > maxNestingDepth {
		out = append(out, Finding{
			Line: line, Cell: ref, Rule: "deep-nesting",
			Message: fmt.Sprintf("parentheses nested %d levels deep, hard to audit", depth),
		})
	}

	trimmed := strings.TrimSpace(formula)
	if strings.Contains(formula, "/0)") || strings.Contains(formula, "/0,") || strings.HasSuffix(trimmed, "/0") {
		out = append(out, Finding{
			Line: line, Cell: ref, Rule: "div-by-zero",
			Message: "divides by a literal zero",
		})
	}

	return out
}

func maxParenDepth(s string) int {
	depth, max := 0, 0
	for _, r := range s {
		switch r {
		case '(':
			depth++
			if depth > max {
				max = depth
			}
		case ')':
			depth--
		}
	}
	return max
}

// cellRef turns a zero-indexed row/column pair from encoding/csv into an
// A1-style reference like "C5". row is already 1-indexed on the way in.
func cellRef(col, row int) string {
	return colLetters(col) + fmt.Sprint(row)
}

func colLetters(col int) string {
	var s string
	for col >= 0 {
		s = string(rune('A'+col%26)) + s
		col = col/26 - 1
	}
	return s
}
