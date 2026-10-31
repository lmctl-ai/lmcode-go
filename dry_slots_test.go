package lmcode

import "testing"

func TestDryDummiesUseDeclaredKindsAndFreshCollections(t *testing.T) {
	r := NewRegistry()
	cases := []struct {
		declared string
		kind     Kind
		check    func(Value) bool
	}{
		{"bool", KindBool, func(v Value) bool { return v.Bool }},
		{"int", KindInt, func(v Value) bool { return v.Int == 0 }},
		{"integer", KindInt, func(v Value) bool { return v.Int == 0 }},
		{"float", KindFloat, func(v Value) bool { return v.Float == 0 }},
		{"number", KindFloat, func(v Value) bool { return v.Float == 0 }},
		{"string", KindStr, func(v Value) bool { return v.Str == "" }},
		{"void", KindVoid, func(Value) bool { return true }},
		{"Opaque", kindDryNull, func(v Value) bool { return v.isDryNull() && v.Type == "Opaque" }},
	}
	for _, row := range cases {
		v, err := r.dryDummy(row.declared)
		if err != nil || v.Kind != row.kind || !row.check(v) {
			t.Fatalf("dry dummy %s = %#v, %v", row.declared, v, err)
		}
	}
	listA, err := r.dryDummy("IntList")
	if err != nil {
		t.Fatal(err)
	}
	listB, err := r.dryDummy("IntList")
	if err != nil {
		t.Fatal(err)
	}
	a, b := listA.Obj.(*IntList), listB.Obj.(*IntList)
	a.values = append(a.values, 12)
	if a == b || len(b.values) != 0 {
		t.Fatal("dry list dummy was shared across calls")
	}
	mapA, err := r.dryDummy("IntMap")
	if err != nil {
		t.Fatal(err)
	}
	mapB, err := r.dryDummy("IntMap")
	if err != nil {
		t.Fatal(err)
	}
	ma, mb := mapA.Obj.(*IntMap), mapB.Obj.(*IntMap)
	ma.data.set("sentinel", 1)
	if ma == mb || mb.data.index["sentinel"] != 0 || len(mb.data.entries) != 0 {
		t.Fatal("dry map dummy was shared across calls")
	}
}

func TestDryRegistrySlotsAreTypedOptionalAndAtomic(t *testing.T) {
	r := NewRegistryWithoutStd()
	fn := Function(func([]Value) (Value, error) { return Int(1), nil })
	if err := r.Function("bad", nil, "int", fn, Dry(fn), Dry(fn)); err == nil {
		t.Fatal("multiple dry slots were accepted")
	}
	if r.FindFunction("bad") != nil {
		t.Fatal("rejected registration mutated the registry")
	}
	if err := r.Function("bad", nil, "int", fn, Dry(Function(nil))); err == nil {
		t.Fatal("nil dry function was accepted")
	}
	if r.FindFunction("bad") != nil {
		t.Fatal("nil dry slot mutated the registry")
	}
	if err := r.Function("bad", nil, "int", fn); err != nil {
		t.Fatal(err)
	}
	if err := r.Function("nil", nil, "void", nil); err == nil {
		t.Fatal("nil real callback was accepted")
	}
	if r.FindFunction("nil") != nil {
		t.Fatal("nil real callback mutated the registry")
	}
}

func TestCanonicalStdCallbacksHaveDeliberateDrySlots(t *testing.T) {
	r := NewRegistry()
	for signature := range r.stdSignatures {
		if fn := r.FunctionBySignature(signature); fn != nil {
			writable := signature == "fn file_write(string, string) -> void" || signature == "fn file_append(string, string) -> void"
			if writable && fn.DryFn != nil {
				t.Fatalf("external write unexpectedly has dry implementation: %s", signature)
			}
			if !writable && fn.DryFn == nil {
				t.Fatalf("standard function has no dry policy: %s", signature)
			}
			continue
		}
		method := r.MethodBySignature(signature)
		if method == nil || method.DryFn == nil {
			t.Fatalf("standard method has no dry policy: %s", signature)
		}
	}
}
