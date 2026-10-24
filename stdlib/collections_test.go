package stdlib_test

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	lmcode "github.com/lmctl-ai/lmcode-go"
	"github.com/lmctl-ai/lmcode-go/stdlib"
)

func fragmentSignatures(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fragment struct {
		Functions map[string][]struct {
			Params  []string `json:"params"`
			Returns string   `json:"returns"`
		} `json:"functions"`
		Types map[string]struct {
			Methods map[string][]struct {
				Params  []string `json:"params"`
				Returns string   `json:"returns"`
			} `json:"methods"`
		} `json:"types"`
	}
	if err := json.Unmarshal(data, &fragment); err != nil {
		t.Fatal(err)
	}
	var out []string
	for name, defs := range fragment.Functions {
		for _, d := range defs {
			out = append(out, fmt.Sprintf("fn %s(%s) -> %s", name, strings.Join(d.Params, ", "), d.Returns))
		}
	}
	for typ, methods := range fragment.Types {
		for name, defs := range methods.Methods {
			for _, d := range defs {
				out = append(out, fmt.Sprintf("method %s.%s(%s) -> %s", typ, name, strings.Join(d.Params, ", "), d.Returns))
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestCollectionsFragmentParity(t *testing.T) {
	r := lmcode.NewRegistryWithoutStd()
	if err := stdlib.RegisterCollections(r); err != nil {
		t.Fatal(err)
	}
	want := fragmentSignatures(t, "collections.schema-fragment.json")
	if got := r.Signatures(); !equalStrings(got, want) {
		t.Fatalf("collection signatures = %v, want %v", got, want)
	}
	assertCompilerParity(t, r, "collections.schema-fragment.json")
}

func TestTestingFragmentParity(t *testing.T) {
	r := lmcode.NewRegistryWithoutStd()
	if err := stdlib.RegisterTesting(r, nil); err != nil {
		t.Fatal(err)
	}
	want := fragmentSignatures(t, "testing.schema-fragment.json")
	if got := r.Signatures(); !equalStrings(got, want) {
		t.Fatalf("testing signatures = %v, want %v", got, want)
	}
	assertCompilerParity(t, r, "testing.schema-fragment.json")
}

func assertCompilerParity(t *testing.T, registry *lmcode.Registry, fragmentPath string) {
	t.Helper()
	compiler := os.Getenv("LMSCRIPT_BIN")
	if compiler == "" {
		var err error
		compiler, err = exec.LookPath("lmscript")
		if err != nil {
			t.Skip("lmscript not found on PATH; compiler parity skipped")
		}
	}
	fragment, err := os.ReadFile(fragmentPath)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(fragment, &fields); err != nil {
		t.Fatal(err)
	}
	fields["dsl.schema"] = json.RawMessage("2")
	fields["name"] = json.RawMessage(`"catalog_parity"`)
	schema, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "schema.json")
	if err := os.WriteFile(path, schema, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(compiler, "--dsl-signatures", path).CombinedOutput()
	if err != nil {
		t.Fatalf("compiler failed (%v): %s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	sort.Strings(lines)
	if !equalStrings(lines, registry.Signatures()) {
		t.Fatalf("compiler signatures = %v, registry signatures = %v", lines, registry.Signatures())
	}
}

func equalStrings(a, b []string) bool {
	return len(a) == len(b) && func() bool {
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}()
}

func collectionFn(t *testing.T, r *lmcode.Registry, sig string, args ...lmcode.Value) lmcode.Value {
	t.Helper()
	e := r.FunctionBySignature(sig)
	if e == nil {
		t.Fatalf("missing %s", sig)
	}
	v, err := e.Fn(args)
	if err != nil {
		t.Fatalf("%s: %v", sig, err)
	}
	return v
}

func collectionMethod(t *testing.T, r *lmcode.Registry, sig string, self any, args ...lmcode.Value) (lmcode.Value, error) {
	t.Helper()
	e := r.MethodBySignature(sig)
	if e == nil {
		t.Fatalf("missing %s", sig)
	}
	return e.Fn(self, args)
}

func TestCollectionsListsAndMap(t *testing.T) {
	r := lmcode.NewRegistryWithoutStd()
	if err := stdlib.RegisterCollections(r); err != nil {
		t.Fatal(err)
	}
	list := collectionFn(t, r, "fn str_list() -> StrList").Obj
	strs, ok := list.(*stdlib.StrList)
	if !ok {
		t.Fatalf("str_list object type = %T", list)
	}
	collectionMethod(t, r, "method StrList.add(string) -> StrList", list, lmcode.Str("a"))
	collectionMethod(t, r, "method StrList.add(string) -> StrList", list, lmcode.Str("\x00b"))
	if got := strs.Values(); len(got) != 2 || got[0] != "a" || got[1] != "\x00b" {
		t.Fatalf("str_list values = %#v", got)
	}
	joined, err := collectionMethod(t, r, "method StrList.join(string) -> string", list, lmcode.Str(","))
	if err != nil || joined.Str != "a,\x00b" {
		t.Fatalf("join = %q, %v", joined.Str, err)
	}
	contains, err := collectionMethod(t, r, "method StrList.contains(string) -> bool", list, lmcode.Str("\x00b"))
	if err != nil || !contains.Bool {
		t.Fatalf("contains = %#v, %v", contains, err)
	}

	fl := collectionFn(t, r, "fn float_list() -> FloatList").Obj
	collectionMethod(t, r, "method FloatList.add(float) -> FloatList", fl, lmcode.Float(math.NaN()))
	contains, err = collectionMethod(t, r, "method FloatList.contains(float) -> bool", fl, lmcode.Float(math.NaN()))
	if err != nil || contains.Bool {
		t.Fatalf("NaN contains = %#v, %v", contains, err)
	}

	m := collectionFn(t, r, "fn str_map() -> StrMap").Obj
	strMap, ok := m.(*stdlib.StrMap)
	if !ok {
		t.Fatalf("str_map object type = %T", m)
	}
	collectionMethod(t, r, "method StrMap.set(string, string) -> StrMap", m, lmcode.Str("a"), lmcode.Str("1"))
	collectionMethod(t, r, "method StrMap.set(string, string) -> StrMap", m, lmcode.Str("b"), lmcode.Str("2"))
	collectionMethod(t, r, "method StrMap.set(string, string) -> StrMap", m, lmcode.Str("a"), lmcode.Str("3"))
	if got := strMap.Entries(); len(got) != 2 || got[0].Key != "a" || got[0].Value != "3" || got[1].Key != "b" {
		t.Fatalf("str_map entries = %#v", got)
	}
	key, err := collectionMethod(t, r, "method StrMap.key_at(int) -> string", m, lmcode.Int(0))
	if err != nil || key.Str != "a" {
		t.Fatalf("key order = %q, %v", key.Str, err)
	}
	removed, err := collectionMethod(t, r, "method StrMap.remove(string) -> bool", m, lmcode.Str("a"))
	if err != nil || !removed.Bool {
		t.Fatalf("remove = %#v, %v", removed, err)
	}
	missing, err := collectionMethod(t, r, "method StrMap.get(string) -> string", m, lmcode.Str("a"))
	if err == nil || err.Error() != "StrMap.get: missing key" || missing.Kind != lmcode.KindVoid {
		t.Fatalf("missing get = %#v, %v", missing, err)
	}
}

func TestCollectionsErrorsAndArenaLimit(t *testing.T) {
	r := lmcode.NewRegistryWithoutStd()
	arena := stdlib.NewCollections()
	if err := stdlib.RegisterCollections(r, arena); err != nil {
		t.Fatal(err)
	}
	list := collectionFn(t, r, "fn int_list() -> IntList").Obj
	_, err := collectionMethod(t, r, "method IntList.at(int) -> int", list, lmcode.Int(-1))
	if err == nil || err.Error() != "IntList.at: index out of range: -1" {
		t.Fatalf("at error = %v", err)
	}
	_, err = collectionFnErr(t, r, "fn str_split(string, string) -> StrList", lmcode.Str("a"), lmcode.Str(""))
	if err == nil || err.Error() != "str_split: separator must not be empty" {
		t.Fatalf("split error = %v", err)
	}
	for i := 0; i < 65535; i++ {
		collectionFn(t, r, "fn int_list() -> IntList")
	}
	if arena.Size() != 65536 {
		t.Fatalf("arena size = %d, want 65536", arena.Size())
	}
	_, err = collectionFnErr(t, r, "fn int_list() -> IntList")
	if err == nil || err.Error() != "IntList object limit exceeded (65536)" {
		t.Fatalf("limit error = %v", err)
	}
	arena.Reset()
	if arena.Size() != 0 {
		t.Fatalf("reset arena size = %d, want 0", arena.Size())
	}
	if _, err = collectionFnErr(t, r, "fn int_list() -> IntList"); err != nil {
		t.Fatalf("constructor after reset: %v", err)
	}
}

func TestStrMapBulkRemovalKeepsInsertionOrder(t *testing.T) {
	r := lmcode.NewRegistryWithoutStd()
	if err := stdlib.RegisterCollections(r); err != nil {
		t.Fatal(err)
	}
	mapValue := collectionFn(t, r, "fn str_map() -> StrMap").Obj
	set := r.MethodBySignature("method StrMap.set(string, string) -> StrMap")
	remove := r.MethodBySignature("method StrMap.remove(string) -> bool")
	keyAt := r.MethodBySignature("method StrMap.key_at(int) -> string")
	for i := 0; i < 10000; i++ {
		k := fmt.Sprintf("k%d", i)
		if _, err := set.Fn(mapValue, []lmcode.Value{lmcode.Str(k), lmcode.Str(k)}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10000; i += 2 {
		if _, err := remove.Fn(mapValue, []lmcode.Value{lmcode.Str(fmt.Sprintf("k%d", i))}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5000; i++ {
		got, err := keyAt.Fn(mapValue, []lmcode.Value{lmcode.Int(int64(i))})
		if err != nil || got.Str != fmt.Sprintf("k%d", i*2+1) {
			t.Fatalf("key_at(%d) = %q, %v", i, got.Str, err)
		}
	}
}

func collectionFnErr(t *testing.T, r *lmcode.Registry, sig string, args ...lmcode.Value) (lmcode.Value, error) {
	t.Helper()
	e := r.FunctionBySignature(sig)
	if e == nil {
		t.Fatalf("missing %s", sig)
	}
	return e.Fn(args)
}
