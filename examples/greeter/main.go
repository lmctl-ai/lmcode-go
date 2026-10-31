package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/lmctl-ai/lmcode-go"
)

type Greeter struct {
	last string
}

func main() {
	reg := lmcode.NewRegistry()
	_ = reg.Context("g", "Greeter")
	_ = reg.Method("Greeter", "say", []string{"string"}, "void", func(self any, a []lmcode.Value) (lmcode.Value, error) {
		self.(*Greeter).last = a[0].Str
		return lmcode.None(), nil
	})
	_ = reg.Function("now", nil, "string", func(a []lmcode.Value) (lmcode.Value, error) {
		return lmcode.String("2026-10-24"), nil
	})

	if len(os.Args) > 1 && (os.Args[1] == "--lmcode-signatures" || os.Args[1] == "--host-signatures") {
		signatures := reg.Signatures()
		if os.Args[1] == "--host-signatures" {
			signatures = reg.HostSignatures()
		}
		for _, s := range signatures {
			fmt.Println(s)
		}
		return
	}
	if len(os.Args) < 3 || strings.HasPrefix(os.Args[1], "--") {
		fmt.Fprintf(os.Stderr, "usage: greeter BYTECODE SCHEMA\n")
		os.Exit(2)
	}

	bc, err := lmcode.ReadFile(os.Args[1])
	lmcode.ExitIfError("load", err)
	schema, err := lmcode.ReadFile(os.Args[2])
	lmcode.ExitIfError("load", err)

	program, err := lmcode.Load(bc, schema, reg)
	lmcode.ExitIfError("load", err)

	greeter := &Greeter{}
	ctx := lmcode.NewContext()
	ctx.Set("g", greeter)

	err = program.Run(ctx, reg)
	lmcode.ExitIfError("run", err)

	fmt.Println(greeter.last)
}
