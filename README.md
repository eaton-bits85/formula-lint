# formula-lint

A linter for spreadsheet formulas. It reads a `.xlsx` workbook directly, or a
CSV file where formula cells contain the formula text itself (starting with
`=`) rather than the computed value, and reports problems with a file, row,
and cell reference so you can jump straight to the offending cell.

## Why

Spreadsheets rot the same way code does, except nobody runs a linter on
them. Formulas get copy-pasted until half the sheet calls `NOW()` and
recalculates on every keystroke, a nested `IF` grows nine levels deep because
nobody wanted to refactor it, and one row still divides by a literal zero
from when a column was deleted. Nobody notices until the sheet times out or
returns `#DIV/0!` in a board deck.

`xlint` runs a handful of static checks over formula text and prints
findings you can act on before that happens.

## Checks

- `volatile` -- use of `NOW`, `TODAY`, `RAND`, `RANDBETWEEN`, `OFFSET`, or
  `INDIRECT`. These force Excel/Sheets to recalculate the whole workbook on
  every change, not just cells that depend on them.
- `long-formula` -- formula text over 200 characters. Usually a sign it
  should be split across a couple of helper cells.
- `deep-nesting` -- parentheses nested more than 7 levels deep.
- `div-by-zero` -- formula divides by a literal `0`.

## Input format

`xlint` picks the reader based on the file extension.

- `.xlsx` is read natively: the workbook's sheet XML is parsed straight out
  of the zip archive, no export step needed. Cells that are the slave half
  of a shared-formula group (the ones Excel writes without their own
  formula text, just a pointer to a master cell) are skipped rather than
  guessed at.
- `.csv` (or anything else) is read as CSV, where formula cells must
  contain the formula text itself (starting with `=`) rather than the
  computed value. Most "export to CSV" features write computed values, so
  producing this input usually means either:
  - wrapping the range you want to check in `=FORMULATEXT(A1)` in a scratch
    column in Google Sheets before exporting, or
  - reading the workbook with a small script (Python's `openpyxl` with
    `data_only=False`, for example) and writing formula-bearing cells
    straight into a CSV.

## Usage

```
$ go run . testdata/sample.csv
testdata/sample.csv:3: E3: uses volatile function NOW, recalculates on every change (volatile)
testdata/sample.csv:3: E3: uses volatile function TODAY, recalculates on every change (volatile)
testdata/sample.csv:4: E4: divides by a literal zero (div-by-zero)

$ go run . path/to/workbook.xlsx
path/to/workbook.xlsx:3: Sheet1!E3: uses volatile function NOW, recalculates on every change (volatile)
```

Exit code is `1` if any findings were reported, `0` otherwise, so it can be
wired into a CI step or a pre-commit hook once the sheet is checked into a
repo as CSV.

Build a binary with:

```
$ go build -o xlint .
$ ./xlint path/to/sheet.csv
```

## Status

Early skeleton. Four checks, two input formats (CSV and native `.xlsx`).
See the checks list above for what exists today.
