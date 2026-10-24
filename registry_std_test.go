package lmcode_test

import (
	"reflect"
	"testing"

	lmcode "github.com/lmctl-ai/lmcode-go"
)

func TestRegistryStdDefaultOptOutAndSelfCheck(t *testing.T) {
	defaultReg := lmcode.NewRegistry()
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
