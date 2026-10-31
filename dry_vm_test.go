package lmcode_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lmctl-ai/lmcode-go"
)

func dryInvocation(t *testing.T, dry bool) (*lmcode.InvocationResults, string) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "part.dslb")
	raw := []byte(fmt.Sprintf(`{"version":1,"mode":"single","dry":%t,"parts":[{"index":0,"file":%q}]}`, dry, file))
	result, err := lmcode.InvocationResultsFromMetadata(raw, []string{file})
	if err != nil {
		t.Fatal(err)
	}
	return result, file
}

func executeDryReturn(t *testing.T, reg *lmcode.Registry, results *lmcode.InvocationResults, file string, code []byte) string {
	t.Helper()
	if _, err := results.Select(0, reg, lmcode.ResultContract{}); err != nil {
		t.Fatal(err)
	}
	program, err := lmcode.Load(code, schemaBytes, reg)
	if err != nil {
		t.Fatal(err)
	}
	ctx := lmcode.NewContext()
	ctx.SetDry(results.Dry())
	if err = program.Run(ctx, reg); err != nil {
		t.Fatal(err)
	}
	if err = results.Complete(0); err != nil {
		t.Fatal(err)
	}
	var frame bytes.Buffer
	if status := results.WriteFrame(&frame); status != 0 {
		t.Fatalf("WriteFrame status %d: %s", status, frame.String())
	}
	return frame.String()
}

func TestDryDefaultsSkipRealAndKeepResultSinkActive(t *testing.T) {
	var realCalls int
	sentinel := filepath.Join(t.TempDir(), "real-callback-ran")
	reg := lmcode.NewRegistry()
	if err := reg.Function("effect_counter", nil, "int", func([]lmcode.Value) (lmcode.Value, error) {
		realCalls++
		if err := os.WriteFile(sentinel, []byte("ran"), 0600); err != nil {
			return lmcode.None(), err
		}
		return lmcode.Int(99), nil
	}); err != nil {
		t.Fatal(err)
	}
	code := newAsm().fn("fn effect_counter() -> int", 0).fn("fn lmcode_return_set(int) -> void", 1).halt().file(schemaBytes)
	results, file := dryInvocation(t, true)
	frame := executeDryReturn(t, reg, results, file, code)
	if realCalls != 0 {
		t.Fatalf("dry invocation ran real callback %d times", realCalls)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("dry invocation wrote real-callback sentinel: %v", err)
	}
	if frame != "LMCODE_RETURN_BEGIN\n{\"ok\":true,\"result\":0,\"dry\":true}\nLMCODE_RETURN_END\n" {
		t.Fatalf("dry dummy/result envelope = %q", frame)
	}
}

func TestDryOverrideRunsOnceAndRealModeIgnoresIt(t *testing.T) {
	var realCalls, previewCalls int
	preview := lmcode.Function(func([]lmcode.Value) (lmcode.Value, error) {
		previewCalls++
		return lmcode.Int(7), nil
	})
	reg := lmcode.NewRegistry()
	if err := reg.Function("read_counter", nil, "int", func([]lmcode.Value) (lmcode.Value, error) {
		realCalls++
		return lmcode.Int(9), nil
	}, lmcode.Dry(preview)); err != nil {
		t.Fatal(err)
	}
	code := newAsm().fn("fn read_counter() -> int", 0).fn("fn lmcode_return_set(int) -> void", 1).halt().file(schemaBytes)
	results, file := dryInvocation(t, true)
	frame := executeDryReturn(t, reg, results, file, code)
	if realCalls != 0 || previewCalls != 1 || !strings.Contains(frame, `"result":7,"dry":true`) {
		t.Fatalf("dry dispatch counts/result: real=%d preview=%d frame=%s", realCalls, previewCalls, frame)
	}

	results, file = dryInvocation(t, false)
	frame = executeDryReturn(t, reg, results, file, code)
	if realCalls != 1 || previewCalls != 1 || frame != "LMCODE_RETURN_BEGIN\n{\"ok\":true,\"result\":9}\nLMCODE_RETURN_END\n" {
		t.Fatalf("real dispatch counts/result: real=%d preview=%d frame=%s", realCalls, previewCalls, frame)
	}
}

func TestDryRealHelperOptsIntoARealRead(t *testing.T) {
	var calls int
	read := lmcode.Function(func([]lmcode.Value) (lmcode.Value, error) {
		calls++
		return lmcode.Int(13), nil
	})
	reg := lmcode.NewRegistry()
	if err := reg.Function("read_fixture", nil, "int", read, lmcode.Dry(read)); err != nil {
		t.Fatal(err)
	}
	code := newAsm().fn("fn read_fixture() -> int", 0).fn("fn lmcode_return_set(int) -> void", 1).halt().file(schemaBytes)
	results, file := dryInvocation(t, true)
	frame := executeDryReturn(t, reg, results, file, code)
	if calls != 1 || !strings.Contains(frame, `"result":13,"dry":true`) {
		t.Fatalf("dry(real) read calls=%d frame=%s", calls, frame)
	}
}

func TestDryStandardLibraryPoliciesAndDiagnostics(t *testing.T) {
	reg := lmcode.NewRegistry()
	for _, signature := range []string{
		"fn str_len(string) -> int",
		"fn file_read(string) -> string",
		"fn file_exists(string) -> bool",
		"fn file_lines(string) -> StrList",
		"fn lmcode_return_set(int) -> void",
		"fn lmcode_return_put_null(string) -> void",
	} {
		entry := reg.FunctionBySignature(signature)
		if entry == nil || entry.DryFn == nil {
			t.Fatalf("required dry-safe standard slot missing: %s", signature)
		}
	}
	for _, signature := range []string{"fn file_write(string, string) -> void", "fn file_append(string, string) -> void"} {
		entry := reg.FunctionBySignature(signature)
		if entry == nil || entry.DryFn != nil {
			t.Fatalf("effectful standard slot must default to dummy: %s", signature)
		}
	}

	var output bytes.Buffer
	if err := lmcode.RegisterTesting(reg, &output); err != nil {
		t.Fatal(err)
	}
	if err := reg.SetStdOutput(&output); err != nil {
		t.Fatal(err)
	}
	code := newAsm().pushStr("dry diagnostic").fn("fn print(string) -> void", 1).halt().file(schemaBytes)
	results, file := dryInvocation(t, true)
	frame := executeDryReturn(t, reg, results, file, code)
	if output.String() != "dry diagnostic\n" || !strings.Contains(frame, `"result":null,"dry":true`) {
		t.Fatalf("dry print duplicated/suppressed output: %q %s", output.String(), frame)
	}
}

func TestDryCollectionCallbacksAndFileReadRunWithoutWrites(t *testing.T) {
	reg := lmcode.NewRegistry()
	listCode := newAsm().fn("fn int_list() -> IntList", 0).pushInt(4).
		method("method IntList.add(int) -> IntList", 1).
		fn("fn lmcode_return_set(IntList) -> void", 1).halt().file(schemaBytes)
	results, file := dryInvocation(t, true)
	frame := executeDryReturn(t, reg, results, file, listCode)
	if !strings.Contains(frame, `"result":[4],"dry":true`) {
		t.Fatalf("pure collection path did not run in dry mode: %s", frame)
	}

	input := filepath.Join(t.TempDir(), "read.txt")
	if err := os.WriteFile(input, []byte("fixture value"), 0600); err != nil {
		t.Fatal(err)
	}
	readCode := newAsm().pushStr(input).fn("fn file_read(string) -> string", 1).
		fn("fn lmcode_return_set(string) -> void", 1).halt().file(schemaBytes)
	results, file = dryInvocation(t, true)
	frame = executeDryReturn(t, reg, results, file, readCode)
	if !strings.Contains(frame, `"result":"fixture value","dry":true`) {
		t.Fatalf("file_read dry(real) result = %s", frame)
	}

	output := filepath.Join(t.TempDir(), "must-not-exist")
	writeCode := newAsm().pushStr(output).pushStr("sentinel").fn("fn file_write(string, string) -> void", 2).halt().file(schemaBytes)
	results, file = dryInvocation(t, true)
	frame = executeDryReturn(t, reg, results, file, writeCode)
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("dry file_write created output: %v", err)
	}
	if !strings.Contains(frame, `"result":null,"dry":true`) {
		t.Fatalf("dry file_write frame = %s", frame)
	}
}

func TestDryMethodsSkipRealCallbacksAndCanPreview(t *testing.T) {
	type receiver struct{}
	var realCalls, previewCalls int
	reg := lmcode.NewRegistry()
	if err := reg.Context("receiver", "Thing"); err != nil {
		t.Fatal(err)
	}
	preview := lmcode.Method(func(self any, _ []lmcode.Value) (lmcode.Value, error) {
		previewCalls++
		return lmcode.Int(6), nil
	})
	if err := reg.Method("Thing", "inspect", nil, "int", func(self any, _ []lmcode.Value) (lmcode.Value, error) {
		realCalls++
		return lmcode.Int(8), nil
	}, lmcode.Dry(preview)); err != nil {
		t.Fatal(err)
	}
	code := newAsm().ctx("receiver", "Thing").method("method Thing.inspect() -> int", 0).
		fn("fn lmcode_return_set(int) -> void", 1).halt().file(schemaBytes)
	results, file := dryInvocation(t, true)
	_, err := runDryMethod(t, reg, results, file, code, &receiver{})
	if err != nil {
		t.Fatal(err)
	}
	if realCalls != 0 || previewCalls != 1 {
		t.Fatalf("dry method calls: real=%d preview=%d", realCalls, previewCalls)
	}

	// A dry-null receiver is safe only when the method has no explicit dry slot;
	// the SDK must not dereference it while supplying the declared result dummy.
	reg = lmcode.NewRegistry()
	if err := reg.Function("opaque", nil, "Opaque", func([]lmcode.Value) (lmcode.Value, error) {
		t.Fatal("opaque real callback ran in dry mode")
		return lmcode.None(), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Method("Opaque", "value", nil, "int", func(any, []lmcode.Value) (lmcode.Value, error) {
		t.Fatal("method real callback received a dry null")
		return lmcode.None(), nil
	}); err != nil {
		t.Fatal(err)
	}
	code = newAsm().fn("fn opaque() -> Opaque", 0).method("method Opaque.value() -> int", 0).
		fn("fn lmcode_return_set(int) -> void", 1).halt().file(schemaBytes)
	results, file = dryInvocation(t, true)
	frame := executeDryReturn(t, reg, results, file, code)
	if !strings.Contains(frame, `"result":0,"dry":true`) {
		t.Fatalf("dry-null method dummy frame = %s", frame)
	}
}

func runDryMethod(t *testing.T, reg *lmcode.Registry, results *lmcode.InvocationResults, file string, code []byte, receiver any) (string, error) {
	t.Helper()
	if _, err := results.Select(0, reg, lmcode.ResultContract{}); err != nil {
		return "", err
	}
	program, err := lmcode.Load(code, schemaBytes, reg)
	if err != nil {
		return "", err
	}
	ctx := lmcode.NewContext()
	ctx.SetDry(results.Dry())
	ctx.Set("receiver", receiver)
	if err = program.Run(ctx, reg); err != nil {
		return "", err
	}
	if err = results.Complete(0); err != nil {
		return "", err
	}
	var out bytes.Buffer
	if code := results.WriteFrame(&out); code != 0 {
		return out.String(), fmt.Errorf("WriteFrame status %d", code)
	}
	return out.String(), nil
}
