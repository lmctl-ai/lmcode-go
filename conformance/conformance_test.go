package conformance_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/lmctl-ai/lmcode-go"
	"github.com/lmctl-ai/lmcode-go/conformance"
)

// Verify the sync script's sha256sum listing natively, including on Windows without a bash dependency.
func TestVectorsLock(t *testing.T) {
	for _, suite := range []string{"vectors", "stdlib-vectors"} {
		t.Run(suite, func(t *testing.T) {
			lock, err := os.ReadFile(filepath.Join("..", suite+".lock"))
			if err != nil {
				t.Fatal(err)
			}
			var want string
			for _, line := range strings.Split(string(lock), "\n") {
				if strings.HasPrefix(line, "sha256=") {
					want = strings.TrimSpace(strings.TrimPrefix(line, "sha256="))
				}
			}
			if len(want) != 64 {
				t.Fatal("missing or invalid sha256 in lock")
			}
			var names []string
			err = filepath.WalkDir(suite, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !entry.IsDir() {
					relative, err := filepath.Rel(suite, path)
					if err != nil {
						return err
					}
					names = append(names, filepath.ToSlash(relative))
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			sort.Strings(names)
			var listing strings.Builder
			for _, name := range names {
				raw, err := os.ReadFile(filepath.Join(suite, filepath.FromSlash(name)))
				if err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256(raw)
				fmt.Fprintf(&listing, "%x  ./%s\n", sum, name)
			}
			sum := sha256.Sum256([]byte(listing.String()))
			if fmt.Sprintf("%x", sum) != want {
				t.Fatalf("%s differs from %s.lock: run scripts/sync-vectors.sh", suite, suite)
			}
		})
	}
}

func TestApproximateTraceMatcher(t *testing.T) {
	if !conformance.TraceLineMatches("float 1.0000000000005", "~ float 1.0") {
		t.Errorf("expected close float to match")
	}
	if conformance.TraceLineMatches("int 1.0", "~ float 1.0") {
		t.Errorf("expected prefix mismatch to fail")
	}
	if conformance.TraceLineMatches("float 1.00000000001", "~ float 1.0") {
		t.Errorf("expected drift to fail")
	}
	if conformance.TraceLineMatches("float nan", "~ float nan") {
		t.Errorf("expected NaN in approx matcher to fail")
	}
	if conformance.TraceLineMatches("float inf", "~ float 1.0") {
		t.Errorf("expected inf to fail")
	}
	if !conformance.TraceLineMatches("float 1", "float 1") {
		t.Errorf("expected exact match")
	}
}

func TestAllVectors(t *testing.T) {
	dir := "vectors"
	names, err := conformance.VectorNames(dir)
	if err != nil {
		t.Fatalf("VectorNames(%s): %v", dir, err)
	}
	if len(names) != 70 {
		t.Fatalf("expected 70 core vectors, got %d", len(names))
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			_, err := conformance.CheckVector(dir, name)
			if err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

func TestAllCatalogVectors(t *testing.T) {
	dir := "../conformance/stdlib-vectors"
	names, err := conformance.VectorNames(dir)
	if err != nil {
		t.Fatalf("VectorNames(%s): %v", dir, err)
	}
	if len(names) != 267 {
		t.Fatalf("expected 267 catalog vectors, got %d", len(names))
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			if _, err := conformance.CheckVector(dir, name); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

func TestStdlibVectors(t *testing.T) {
	names, err := conformance.VectorNames("stdlib-vectors")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 267 {
		t.Fatalf("expected 267 catalog vectors, got %d", len(names))
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			if _, err := conformance.CheckVector("stdlib-vectors", name); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFixtureBytes(t *testing.T) {
	stub := conformance.NewStub()
	decode := stub.Reg.FunctionBySignature("fn bytes(string) -> string")
	record := stub.Reg.MethodBySignature("method Trace.hex(string) -> void")
	if decode == nil || record == nil {
		t.Fatal("fixture adapters not registered")
	}
	for _, encoded := range []string{"", "ff00c3a9", "0123456789abcdef"} {
		value, err := decode.Fn([]lmcode.Value{lmcode.String(encoded)})
		if err != nil {
			t.Fatal(err)
		}
		if hex.EncodeToString([]byte(value.Str)) != encoded {
			t.Fatalf("byte fixture %q changed", encoded)
		}
		if _, err := record.Fn(stub, []lmcode.Value{value}); err != nil {
			t.Fatal(err)
		}
		if stub.Trace[len(stub.Trace)-1] != "hex "+encoded {
			t.Fatal("hex trace changed fixture bytes")
		}
	}
	for _, encoded := range []string{"f", "FF", "0g", " 0", "00\x00", "é"} {
		_, err := decode.Fn([]lmcode.Value{lmcode.String(encoded)})
		e, ok := err.(*lmcode.Error)
		if !ok || e.Code != lmcode.RUN_HOST || e.Message != "bytes: expected lowercase hexadecimal pairs" {
			t.Fatalf("bytes(%q): %v", encoded, err)
		}
	}
}

func TestWrongExpectationDetected(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"literals.dslb", "schema.json"} {
		raw, err := os.ReadFile(filepath.Join("vectors", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "literals.expect"), []byte("trace\nint 9999999\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := conformance.CheckVector(dir, "literals"); err == nil {
		t.Fatal("corrupted expectation was accepted")
	}
}
