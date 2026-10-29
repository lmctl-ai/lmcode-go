package main

import (
	_ "embed"
	"fmt"
	"github.com/lmctl-ai/lmcode-go"
	"os"
)

//go:embed schema.json
var schema []byte

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: rule-entries FILE.dslb")
		os.Exit(2)
	}
	bytecode, err := os.ReadFile(os.Args[1])
	lmcode.ExitIfError("load", err)
	registry := lmcode.NewRegistry()
	program, err := lmcode.Load(bytecode, schema, registry)
	lmcode.ExitIfError("load", err)
	for index, entry := range program.Entries() {
		if entry.Annotation != "increment" {
			continue
		}
		if len(entry.Params) != 1 || entry.Params[0] != "int" {
			panic("incorrect entry parameters")
		}
		value, err := program.Call(index, lmcode.NewContext(), registry, []lmcode.Value{lmcode.Int(41)})
		lmcode.ExitIfError("run", err)
		if value.Kind != lmcode.KindInt || value.Int != 42 {
			panic("incorrect entry result")
		}
		fmt.Println("PASS @increment implements Score: 41 -> 42")
		return
	}
	panic("missing @increment entry")
}
