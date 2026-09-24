package main

import (
	"strings"
	"testing"
)

func TestCheckFormulaVolatile(t *testing.T) {
	cases := []struct {
		name    string
		formula string
		want    []string // expected functions, without trailing "("
	}{
		{"now", "=NOW()", []string{"NOW"}},
		{"lowercase", "=now()", []string{"NOW"}},
		{"two volatiles", "=IF(NOW()>TODAY(),1,0)", []string{"NOW", "TODAY"}},
		{"none", "=SUM(A1:A2)", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings := checkFormula(1, "A1", c.formula)
			var got []string
			for _, f := range findings {
				if f.Rule != "volatile" {
					continue
				}
				fn := strings.TrimPrefix(f.Message, "uses volatile function ")
				fn = strings.SplitN(fn, ",", 2)[0]
				got = append(got, fn)
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestCheckFormulaLongFormula(t *testing.T) {
	short := "=A1+A2"
	long := "=" + strings.Repeat("A1+", 100)

	if findings := checkFormula(1, "A1", short); ruleFired(findings, "long-formula") {
		t.Errorf("short formula should not trigger long-formula, got %v", findings)
	}
	if findings := checkFormula(1, "A1", long); !ruleFired(findings, "long-formula") {
		t.Errorf("formula of length %d should trigger long-formula", len(long))
	}
}

func TestCheckFormulaDeepNesting(t *testing.T) {
	shallow := "=IF(A1>0,IF(A2>0,1,0),0)"
	deep := "=" + strings.Repeat("IF(A1>0,", 8) + "1" + strings.Repeat(",0)", 8)

	if findings := checkFormula(1, "A1", shallow); ruleFired(findings, "deep-nesting") {
		t.Errorf("shallow formula should not trigger deep-nesting, got %v", findings)
	}
	if findings := checkFormula(1, "A1", deep); !ruleFired(findings, "deep-nesting") {
		t.Errorf("deeply nested formula should trigger deep-nesting")
	}
}

func TestCheckFormulaDivByZero(t *testing.T) {
	cases := []struct {
		name    string
		formula string
		want    bool
	}{
		{"trailing div zero", "=A1/0", true},
		{"div zero before close paren", "=SUM(A1/0)", true},
		{"div zero before comma", "=IF(A1/0,1,2)", true},
		{"div by cell", "=A1/B1", false},
		{"div by ten", "=A1/10", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings := checkFormula(1, "A1", c.formula)
			if got := ruleFired(findings, "div-by-zero"); got != c.want {
				t.Errorf("formula %q: got div-by-zero=%v, want %v", c.formula, got, c.want)
			}
		})
	}
}

func TestMaxParenDepth(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"", 0},
		{"A1+A2", 0},
		{"(A1)", 1},
		{"((A1))", 2},
		{"(A1)+(A2)", 1},
		{"(A(B(C)))", 3},
	}

	for _, c := range cases {
		if got := maxParenDepth(c.s); got != c.want {
			t.Errorf("maxParenDepth(%q) = %d, want %d", c.s, got, c.want)
		}
	}
}

func TestCellRef(t *testing.T) {
	cases := []struct {
		col, row int
		want     string
	}{
		{0, 1, "A1"},
		{1, 1, "B1"},
		{25, 5, "Z5"},
		{26, 1, "AA1"},
		{27, 1, "AB1"},
		{51, 1, "AZ1"},
		{52, 1, "BA1"},
	}

	for _, c := range cases {
		if got := cellRef(c.col, c.row); got != c.want {
			t.Errorf("cellRef(%d, %d) = %q, want %q", c.col, c.row, got, c.want)
		}
	}
}

func TestLint(t *testing.T) {
	csv := "Item,Price,Total\n" +
		"Widget,10,=B2*2\n" +
		"Bad,5,=B3/0\n" +
		"Plain,1,3\n"

	findings, err := Lint(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(findings), findings)
	}
	f := findings[0]
	if f.Line != 3 || f.Cell != "C3" || f.Rule != "div-by-zero" {
		t.Errorf("unexpected finding: %+v", f)
	}
}

func TestLintIgnoresNonFormulaCells(t *testing.T) {
	csv := "A,B\n1,2\nhello,world\n"

	findings, err := Lint(strings.NewReader(csv))
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("got %d findings, want 0: %v", len(findings), findings)
	}
}

func TestLintInvalidCSV(t *testing.T) {
	// Unbalanced quotes make encoding/csv return an error.
	csv := "A,B\n\"unterminated,2\n"

	if _, err := Lint(strings.NewReader(csv)); err == nil {
		t.Fatal("expected an error for malformed CSV, got nil")
	}
}

func ruleFired(findings []Finding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}
