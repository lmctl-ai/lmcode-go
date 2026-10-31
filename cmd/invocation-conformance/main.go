// Canonical fixture adapter. Application adapters own their dispatch policy.
package main

import (
	"encoding/json"
	lmcode "github.com/lmctl-ai/lmcode-go"
	"os"
)

func run() int {
	if len(os.Args) < 4 {
		return lmcode.FailureFrame(os.Stdout, "cli", lmcode.NewError(lmcode.CLI_USAGE, "fixture arguments required"))
	}
	configBytes, err := os.ReadFile(os.Args[2])
	if err != nil {
		return lmcode.FailureFrame(os.Stdout, "cli", err)
	}
	var config struct {
		Execution []int
		AppFail   bool `json:"app_fail"`
	}
	if err = json.Unmarshal(configBytes, &config); err != nil {
		return lmcode.FailureFrame(os.Stdout, "cli", err)
	}
	results, err := lmcode.InvocationResultsFromCommand(os.Args[3:])
	if err != nil {
		return lmcode.FailureFrame(os.Stdout, "cli", err)
	}
	registry := lmcode.NewRegistry()
	context := lmcode.NewContext()
	counter := int64(0)
	_ = registry.Function("next_counter", nil, "int", func([]lmcode.Value) (lmcode.Value, error) { counter++; return lmcode.Int(counter), nil })
	_ = registry.Function("entry_fail", nil, "void", func([]lmcode.Value) (lmcode.Value, error) {
		return lmcode.None(), lmcode.NewHostError("native entry failure")
	})
	schema, err := os.ReadFile(os.Args[1])
	if err != nil {
		return lmcode.FailureFrame(os.Stdout, "cli", err)
	}
	contract, err := lmcode.ResultContractFromSchema(schema)
	if err != nil {
		return lmcode.FailureFrame(os.Stdout, "cli", err)
	}
	for _, index := range config.Execution {
		phase := "load"
		execute := func() error {
			file := ""
			for _, part := range results.Registrations() {
				if part.Index == index {
					file = part.File
					break
				}
			}
			if file == "" {
				return lmcode.NewError(lmcode.CLI_USAGE, "fixture index absent")
			}
			raw, e := os.ReadFile(file)
			if e != nil {
				return lmcode.NewError(lmcode.LOAD_BAD_BYTECODE, "cannot read fixture")
			}
			program, e := lmcode.Load(raw, schema, registry)
			if e != nil {
				return e
			}
			if _, e = results.Select(index, registry, contract); e != nil {
				return e
			}
			phase = "run"
			if e = program.Run(context, registry); e != nil {
				return e
			}
			for i := range program.Entries() {
				v, e := program.Call(i, context, registry, nil)
				if e != nil {
					return e
				}
				if v.Kind != lmcode.KindInt || v.Int != 7 {
					return lmcode.NewHostError("entry HALT result changed")
				}
			}
			return results.Complete(index)
		}
		if e := execute(); e != nil {
			_ = results.Fail(&index, phase, e)
			break
		}
	}
	if config.AppFail {
		_ = results.Fail(nil, "app", lmcode.NewError(lmcode.APP_FAIL, "native validation failed"))
	}
	return results.WriteFrame(os.Stdout)
}
func main() { os.Exit(run()) }
