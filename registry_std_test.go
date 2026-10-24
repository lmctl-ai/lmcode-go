package lmcode_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	lmcode "github.com/lmctl-ai/lmcode-go"
)

func canonicalSignatures(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("std/lmcode-std.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Functions map[string][]struct {
			Params  []string `json:"params"`
			Returns string   `json:"returns"`
		} `json:"functions"`
		Types map[string]struct {
			Methods map[string][]struct {
				Params  []string `json:"params"`
				Returns string   `json:"returns"`
			} `json:"methods"`
		} `json:"types"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	var out []string
	for n, defs := range schema.Functions {
		for _, d := range defs {
			out = append(out, fmt.Sprintf("fn %s(%s) -> %s", n, strings.Join(d.Params, ", "), d.Returns))
		}
	}
	for typ, methods := range schema.Types {
		for n, defs := range methods.Methods {
			for _, d := range defs {
				out = append(out, fmt.Sprintf("method %s.%s(%s) -> %s", typ, n, strings.Join(d.Params, ", "), d.Returns))
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestRegistryStdDefaultOptOutAndSelfCheck(t *testing.T) {
	defaultReg := lmcode.NewRegistry()
	if got, want := defaultReg.Signatures(), canonicalSignatures(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("default std signatures = %v, want canonical %v", got, want)
	}
	if defaultReg.FunctionBySignature("fn str_len(string) -> int") == nil {
		t.Fatal("default registry did not install std")
	}
	host := defaultReg.HostSignatures()
	if len(host) != 0 {
		t.Fatalf("default host signatures = %v", host)
	}
	want := append([]string(nil), defaultReg.Signatures()...)
	if err := lmcode.RegisterStd(defaultReg); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(defaultReg.Signatures(), want) {
		t.Fatal("RegisterStd is not idempotent")
	}

	bare := lmcode.NewRegistryWithoutStd()
	if len(bare.Signatures()) != 0 || len(bare.HostSignatures()) != 0 {
		t.Fatal("opt-out registry is not empty")
	}
	if err := bare.Function("app", nil, "void", func([]lmcode.Value) (lmcode.Value, error) { return lmcode.None(), nil }); err != nil {
		t.Fatal(err)
	}
	if err := bare.CheckSchemaSignatures([]string{"fn app() -> void"}); err != nil {
		t.Fatal(err)
	}
	if err := lmcode.RegisterStd(bare); err != nil {
		t.Fatal(err)
	}
	if bare.FunctionBySignature("fn str_len(string) -> int") == nil {
		t.Fatal("explicit std registration missing")
	}
}
