package main

import (
	"fmt"
	"os"

	"github.com/lmctl-ai/lmcode-go/conformance"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: conformance VECTOR_DIR")
		os.Exit(2)
	}

	if !conformance.TraceLineMatches("float 1.0000000000001", "~ float 1") ||
		conformance.TraceLineMatches("float 1.00000000001", "~ float 1") ||
		conformance.TraceLineMatches("float nan", "~ float 1") ||
		conformance.TraceLineMatches("double 1", "~ float 1") {
		fmt.Fprintln(os.Stderr, "approximate trace matcher self-test failed")
		os.Exit(2)
	}

	dir := os.Args[1]
	names, err := conformance.VectorNames(dir)
	if err != nil || len(names) == 0 {
		fmt.Fprintf(os.Stderr, "no vectors in %s\n", dir)
		os.Exit(2)
	}

	failed := 0
	for _, name := range names {
		kind, err := conformance.CheckVector(dir, name)
		if err == nil {
			fmt.Printf("ok - %s (%s)\n", name, kind)
		} else {
			failed++
			fmt.Printf("FAIL - %s\n", err)
		}
	}

	status := "PASS"
	if failed > 0 {
		status = "FAIL"
	}
	fmt.Printf("%s %d/%d vectors\n", status, len(names)-failed, len(names))
	if failed > 0 {
		os.Exit(1)
	}
}
