// rest-serve serves a compiled program's REST routes over stdin/stdout (the opt-in `rest` std module; no network).
//
//	rest-serve PROGRAM.dslb HOST.schema.json [--max-body-bytes N]
//
// stdout carries only HTTP responses; handler failures go to stderr. The host registers no print function.
// Exit status: 0 after serving, 2 for usage errors, 3 for load or run errors.
package main

import (
	"fmt"
	"os"
	"strconv"

	lmcode "github.com/lmctl-ai/lmcode-go"
	"github.com/lmctl-ai/lmcode-go/rest"
)

func main() {
	args := os.Args[1:]
	var maxBody int64
	if n := len(args); n >= 4 && args[n-2] == "--max-body-bytes" {
		v, err := strconv.ParseInt(args[n-1], 10, 64)
		if err != nil || v <= 0 {
			fmt.Fprintln(os.Stderr, "rest-serve: --max-body-bytes needs a positive integer")
			os.Exit(2)
		}
		maxBody, args = v, args[:n-2]
	}
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: rest-serve PROGRAM.dslb HOST.schema.json [--max-body-bytes N]")
		os.Exit(2)
	}
	fail := func(err error) {
		fmt.Fprintf(os.Stderr, "rest-serve: %v\n", err)
		os.Exit(3)
	}
	bytecode, err := os.ReadFile(args[0])
	if err != nil {
		fail(err)
	}
	schema, err := os.ReadFile(args[1])
	if err != nil {
		fail(err)
	}
	registry := lmcode.NewRegistry()
	controller, err := rest.Register(registry, rest.Options{MaxBodyBytes: maxBody})
	if err != nil {
		fail(err)
	}
	program, err := lmcode.Load(bytecode, schema, registry)
	if err != nil {
		fail(err)
	}
	if err := controller.Run(program, lmcode.NewContext()); err != nil {
		fail(err)
	}
}
