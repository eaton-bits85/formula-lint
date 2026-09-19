# formula-lint

A linter for spreadsheet formulas. It reads a CSV file where formula cells
contain the formula text itself (starting with `=`) rather than the computed
value, and reports problems with a file, row, and cell reference so you can
jump straight to the offending cell.

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

Most "export to CSV" features in Excel and Google Sheets write computed
values, not formulas, so you'll need formula text in the CSV to lint
anything. Two easy ways to get that:

- In Google Sheets, wrap the range you want to check in `=FORMULATEXT(A1)`
  in a scratch column, then export that.
- Read the workbook with a small script (Python's `openpyxl` with
  `data_only=False`, for example) and write formula-bearing cells straight
  into a CSV.

A native `.xlsx` reader is on the roadmap so this step goes away; see below.

## Usage

```
$ go run . testdata/sample.csv
testdata/sample.csv:3: E3: uses volatile function NOW, recalculates on every change (volatile)
testdata/sample.csv:3: E3: uses volatile function TODAY, recalculates on every change (volatile)
testdata/sample.csv:4: E4: divides by a literal zero (div-by-zero)
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

Early skeleton. Four checks, one input format. See the checks list above
for what exists today.
