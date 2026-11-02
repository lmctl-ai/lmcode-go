package lmcode_test

import (
	"strings"
	"testing"

	"github.com/lmctl-ai/lmcode-go"
)

// Repro: a host method/function whose schema return type is an enum (wire
// form: the constant's name as a string) cannot have a dry callback. The
// real callback returns lmcode.String("OK") and works; the dry callback
// returning the same value fails with "dry callback for ErrorCode returned
// string", and with no dry slot the dry-null dummy cannot be compared with
// an enum constant. lmcloud hosts hit this on CloudError.code() -> ErrorCode.
func enumRegistry(t *testing.T, declare bool, dryValue string, withSlot bool) *lmcode.Registry {
	t.Helper()
	reg := lmcode.NewRegistry()
	if declare {
		if err := reg.Enum("ErrorCode", []string{"OK", "NOT_FOUND"}); err != nil {
			t.Fatal(err)
		}
	}
	real := lmcode.Function(func([]lmcode.Value) (lmcode.Value, error) { return lmcode.String("NOT_FOUND"), nil })
	var slots []lmcode.DrySlot[lmcode.Function]
	if withSlot {
		slots = append(slots, lmcode.Dry[lmcode.Function](func([]lmcode.Value) (lmcode.Value, error) { return lmcode.String(dryValue), nil }))
	}
	if err := reg.Function("current_code", nil, "ErrorCode", real, slots...); err != nil {
		t.Fatal(err)
	}
	return reg
}

func runEnumProgram(t *testing.T, reg *lmcode.Registry) (string, error) {
	t.Helper()
	code := newAsm().fn("fn current_code() -> ErrorCode", 0).fn("fn lmcode_return_set(string) -> void", 1).halt().file(schemaBytes)
	results, _ := dryInvocation(t, true)
	if _, err := results.Select(0, reg, lmcode.ResultContract{}); err != nil {
		t.Fatal(err)
	}
	program, err := lmcode.Load(code, schemaBytes, reg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := lmcode.NewContext()
	ctx.SetDry(true)
	if err := program.Run(ctx, reg); err != nil {
		return "", err
	}
	if err := results.Complete(0); err != nil {
		t.Fatal(err)
	}
	var frame strings.Builder
	results.WriteFrame(&frame)
	return frame.String(), nil
}

func TestDryEnumStringResultForDeclaredEnum(t *testing.T) {
	frame, err := runEnumProgram(t, enumRegistry(t, true, "OK", true))
	if err != nil {
		t.Fatalf("dry callback returning an enum constant string was rejected: %v", err)
	}
	if !strings.Contains(frame, `"result":"OK"`) || !strings.Contains(frame, `"dry":true`) {
		t.Fatalf("frame = %q", frame)
	}
}

func TestDryEnumRejectsNonMember(t *testing.T) {
	if _, err := runEnumProgram(t, enumRegistry(t, true, "UNAVAILABLE", true)); err == nil || !strings.Contains(err.Error(), "ErrorCode") {
		t.Fatalf("non-member enum string must be rejected, got %v", err)
	}
}

func TestDryEnumUndeclaredStaysStrict(t *testing.T) {
	if _, err := runEnumProgram(t, enumRegistry(t, false, "OK", true)); err == nil || !strings.Contains(err.Error(), "dry callback for ErrorCode returned string") {
		t.Fatalf("undeclared type must keep the object check, got %v", err)
	}
}

func TestDryEnumDummyIsFirstMember(t *testing.T) {
	frame, err := runEnumProgram(t, enumRegistry(t, true, "", false))
	if err != nil || !strings.Contains(frame, `"result":"OK"`) {
		t.Fatalf("no-slot dry dummy for a declared enum = %q, %v", frame, err)
	}
}

func TestEnumDeclarationValidation(t *testing.T) {
	reg := lmcode.NewRegistry()
	for _, c := range []struct {
		name   string
		values []string
	}{{"", []string{"A"}}, {"E", nil}, {"E", []string{"A", "A"}}, {"E", []string{"a-b"}}, {"string", []string{"A"}}} {
		if err := reg.Enum(c.name, c.values); err == nil {
			t.Fatalf("Enum(%q, %v) accepted", c.name, c.values)
		}
	}
	if err := reg.Enum("E", []string{"A", "B"}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Enum("E", []string{"A"}); err == nil {
		t.Fatal("redeclaration accepted")
	}
}
