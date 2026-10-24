package stdlib_test

import (
	"bytes"
	"math"
	"testing"

	lmcode "github.com/lmctl-ai/lmcode-go"
	"github.com/lmctl-ai/lmcode-go/stdlib"
)

func TestTestingCatalog(t *testing.T) {
	var output bytes.Buffer
	r := lmcode.NewRegistry()
	if err := stdlib.RegisterTesting(r, &output); err != nil {
		t.Fatal(err)
	}
	printFn := r.FunctionBySignature("fn print(string) -> void")
	if printFn == nil {
		t.Fatal("missing print")
	}
	if _, err := printFn.Fn([]lmcode.Value{lmcode.Str("a\x00b")}); err != nil {
		t.Fatal(err)
	}
	if output.String() != "a\x00b\n" {
		t.Fatalf("print output = %q", output.String())
	}

	assertFn := r.FunctionBySignature("fn assert_close(float, float, float) -> void")
	if assertFn == nil {
		t.Fatal("missing assert_close")
	}
	if _, err := assertFn.Fn([]lmcode.Value{lmcode.Float(1), lmcode.Float(1.0001), lmcode.Float(.001)}); err != nil {
		t.Fatal(err)
	}
	_, err := assertFn.Fn([]lmcode.Value{lmcode.Float(math.NaN()), lmcode.Float(1), lmcode.Float(1)})
	if err == nil || err.Error() != "assert_close failed: nan vs 1 (tol 1)" {
		t.Fatalf("NaN error = %v", err)
	}
	_, err = assertFn.Fn([]lmcode.Value{lmcode.Float(1.25), lmcode.Float(1), lmcode.Float(.1)})
	if err == nil || err.Error() != "assert_close failed: 1.25 vs 1 (tol 0.1)" {
		t.Fatalf("close error = %v", err)
	}
}
