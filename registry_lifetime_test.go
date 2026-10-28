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

func TestProgramUsesBoundRegistryAcrossAlternateRunArgument(t *testing.T) {
	a, b := NewRegistry(), NewRegistry()
	constructor := a.FunctionBySignature("fn int_list() -> IntList")
	if _, err := constructor.Fn(nil); err != nil {
		t.Fatal(err)
	}
	p := &Program{region: region{code: []byte{OpHalt}}, registry: a}
	if err := p.Run(NewContext(), b); err != nil {
		t.Fatal(err)
	}
	if a.stdCollections.Size() != 0 {
		t.Fatalf("bound arena after alternate run = %d", a.stdCollections.Size())
	}
	if b.stdCollections.Size() != 0 {
		t.Fatalf("alternate arena changed = %d", b.stdCollections.Size())
	}
	if err := a.beginRun(); err != nil {
		t.Fatal(err)
	}
	if err := p.Run(NewContext(), b); err == nil || err.Error() != "registry run already active" {
		t.Fatalf("bound nested error = %v", err)
	}
	a.endRun()
}

func TestExplicitCollectionsSurviveRegisterStd(t *testing.T) {
	r := NewRegistryWithoutStd()
	arena := NewCollections()
	if err := RegisterCollections(r, arena); err != nil {
		t.Fatal(err)
	}
	constructor := r.FunctionBySignature("fn int_list() -> IntList")
	if _, err := constructor.Fn(nil); err != nil {
		t.Fatal(err)
	}
	if err := RegisterStd(r); err != nil {
		t.Fatal(err)
	}
	if arena.Size() != 1 {
		t.Fatalf("explicit arena size after RegisterStd = %d", arena.Size())
	}
}

func TestBareRegistryKeepsLegacyRunReuse(t *testing.T) {
	r := NewRegistryWithoutStd()
	if err := r.beginRun(); err != nil {
		t.Fatal(err)
	}
	if err := r.beginRun(); err != nil {
		t.Fatalf("bare nested run: %v", err)
	}
	r.endRun()
	r.endRun()
}
