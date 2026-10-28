package lmcode_test

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"testing"

	lmcode "github.com/lmctl-ai/lmcode-go"
)

func TestAnnotatedSAMRuleRunsAsInlinedV5Code(t *testing.T) {
	schema, err := os.ReadFile("testdata/rule-methods/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	bytecode, err := os.ReadFile("testdata/rule-methods/program.dslb")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(bytecode, []byte("DSLB\x00\x05")) {
		t.Fatal("expected bytecode v5")
	}
	var captured []int64
	reg := lmcode.NewRegistryWithoutStd()
	if err := reg.Function("capture", []string{"int"}, "void", func(args []lmcode.Value) (lmcode.Value, error) {
		if args[0].Kind != lmcode.KindInt {
			t.Fatalf("capture received %v", args[0])
		}
		captured = append(captured, args[0].Int)
		return lmcode.None(), nil
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reg.Signatures(), []string{"fn capture(int) -> void"}) {
		t.Fatal(reg.Signatures())
	}
	program, err := lmcode.Load(bytecode, schema, reg)
	if err != nil {
		t.Fatal(err)
	}
	if err := program.Run(lmcode.NewContext(), reg); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(captured, []int64{42}) {
		t.Fatalf("capture: %v", captured)
	}
	_, err = lmcode.Load(bytecode, append(schema, ' '), reg)
	var mismatch *lmcode.Error
	if !errors.As(err, &mismatch) || mismatch.Code != lmcode.LOAD_SCHEMA_MISMATCH {
		t.Fatalf("changed schema must fail before execution: %v", err)
	}
}
