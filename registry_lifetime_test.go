package lmcode

import "testing"

func TestDefaultCollectionsResetAndRejectNestedRuns(t *testing.T) {
	r := NewRegistry()
	if err := r.beginRun(); err != nil {
		t.Fatal(err)
	}
	if err := r.beginRun(); err == nil || err.Error() != "registry run already active" {
		t.Fatalf("nested run error = %v", err)
	}
	constructor := r.FunctionBySignature("fn int_list() -> IntList")
	if constructor == nil {
		t.Fatal("missing default collection constructor")
	}
	for i := 0; i < collectionObjectLimit; i++ {
		if _, err := constructor.Fn(nil); err != nil {
			t.Fatalf("allocation %d: %v", i, err)
		}
	}
	r.endRun()
	if r.stdCollections.Size() != 0 {
		t.Fatalf("arena after run = %d", r.stdCollections.Size())
	}
	if err := r.beginRun(); err != nil {
		t.Fatal(err)
	}
	if _, err := constructor.Fn(nil); err != nil {
		t.Fatalf("allocation after reset: %v", err)
	}
	r.endRun()
}
