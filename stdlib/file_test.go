package stdlib_test

import (
	"os"
	"path/filepath"
	"testing"

	lmcode "github.com/lmctl-ai/lmcode-go"
	"github.com/lmctl-ai/lmcode-go/stdlib"
)

func TestFileCatalogBytesAndLines(t *testing.T) {
	r := lmcode.NewRegistryWithoutStd()
	arena := stdlib.NewCollections()
	if err := stdlib.RegisterFile(r, arena); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "bytes.bin")
	write := r.FunctionBySignature("fn file_write(string, string) -> void")
	if _, err := write.Fn([]lmcode.Value{lmcode.Str(path), lmcode.Str("a\x00\xff\n")}); err != nil {
		t.Fatal(err)
	}
	appendFn := r.FunctionBySignature("fn file_append(string, string) -> void")
	if _, err := appendFn.Fn([]lmcode.Value{lmcode.Str(path), lmcode.Str("b")}); err != nil {
		t.Fatal(err)
	}
	read := r.FunctionBySignature("fn file_read(string) -> string")
	value, err := read.Fn([]lmcode.Value{lmcode.Str(path)})
	if err != nil || value.Str != "a\x00\xff\nb" {
		t.Fatalf("read = %q, %v", value.Str, err)
	}
	exists := r.FunctionBySignature("fn file_exists(string) -> bool")
	value, err = exists.Fn([]lmcode.Value{lmcode.Str(path)})
	if err != nil || !value.Bool {
		t.Fatalf("exists = %#v, %v", value, err)
	}
	lines := r.FunctionBySignature("fn file_lines(string) -> StrList")
	value, err = lines.Fn([]lmcode.Value{lmcode.Str(path)})
	if err != nil {
		t.Fatal(err)
	}
	if got := value.Obj.(*stdlib.StrList).Values(); len(got) != 2 || got[0] != "a\x00\xff" || got[1] != "b" {
		t.Fatalf("lines = %#v", got)
	}
	if arena.Size() != 1 {
		t.Fatalf("arena size = %d", arena.Size())
	}
}

func TestFileCatalogFailures(t *testing.T) {
	r := lmcode.NewRegistryWithoutStd()
	if err := stdlib.RegisterFile(r); err != nil {
		t.Fatal(err)
	}
	missing := r.FunctionBySignature("fn file_read(string) -> string")
	_, err := missing.Fn([]lmcode.Value{lmcode.Str("missing.bin")})
	if err == nil || len(err.Error()) <= len("file_read: missing.bin: ") || err.Error()[:len("file_read: missing.bin: ")] != "file_read: missing.bin: " {
		t.Fatalf("missing error = %v", err)
	}
	exists := r.FunctionBySignature("fn file_exists(string) -> bool")
	dir := t.TempDir()
	v, err := exists.Fn([]lmcode.Value{lmcode.Str(dir)})
	if err != nil || v.Bool {
		t.Fatalf("directory exists = %#v, %v", v, err)
	}
	bad := r.FunctionBySignature("fn file_write(string, string) -> void")
	_, err = bad.Fn([]lmcode.Value{lmcode.Str("bad\x00path"), lmcode.Str("x")})
	if err == nil || len(err.Error()) < len("file_write: bad") || err.Error()[:len("file_write: bad")] != "file_write: bad" {
		t.Fatalf("NUL path error = %v", err)
	}
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err = exists.Fn([]lmcode.Value{lmcode.Str(regular + "/child")})
	if err != nil || v.Bool {
		t.Fatalf("non-directory parent exists = %#v, %v", v, err)
	}
	_ = os.RemoveAll(dir)
}
