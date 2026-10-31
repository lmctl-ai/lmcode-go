// A canonical fixture host, not automatic VM framing. Sample adopters own their
// application's dispatch/validation policy and opt into framing separately.
package main

import (
	"fmt"
	"github.com/lmctl-ai/lmcode-go/conformance"
	"os"
	"path/filepath"
	"strconv"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: return-conformance FILE.dslb [REPEAT] [--app-fail]")
		os.Exit(2)
	}
	repeat := 1
	var err error
	if len(os.Args) > 2 {
		repeat, err = strconv.Atoi(os.Args[2])
		if err != nil || repeat < 1 {
			fmt.Fprintln(os.Stderr, "repeat must be positive")
			os.Exit(2)
		}
	}
	out, code, err := conformance.ExecuteReturnFixture(filepath.Dir(os.Args[1]), filepath.Base(os.Args[1]), repeat, len(os.Args) > 3 && os.Args[3] == "--app-fail")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if _, err = fmt.Fprint(os.Stdout, out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	os.Exit(code)
}
