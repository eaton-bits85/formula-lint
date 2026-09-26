package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: xlint <file.csv|file.xlsx>")
		os.Exit(2)
	}
	path := os.Args[1]

	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "xlint: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	var findings []Finding
	if strings.HasSuffix(strings.ToLower(path), ".xlsx") {
		info, statErr := f.Stat()
		if statErr != nil {
			fmt.Fprintf(os.Stderr, "xlint: %v\n", statErr)
			os.Exit(1)
		}
		findings, err = LintXLSX(f, info.Size())
	} else {
		findings, err = Lint(f)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "xlint: %v\n", err)
		os.Exit(1)
	}

	for _, fnd := range findings {
		fmt.Printf("%s:%d: %s: %s (%s)\n", path, fnd.Line, fnd.Cell, fnd.Message, fnd.Rule)
	}

	if len(findings) > 0 {
		os.Exit(1)
	}
}
