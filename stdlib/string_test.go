package stdlib_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/lmctl-ai/lmcode-go"
	"github.com/lmctl-ai/lmcode-go/stdlib"
)

func TestStrLen(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"", 0},
		{"abc", 3},
		{"a\x00b", 3},
		{"é", 2},
		{"日本", 6},
		{"\xff\xfe", 2},
	}

	for _, tc := range tests {
		got := stdlib.StrLen(tc.input)
		if got != tc.want {
			t.Errorf("StrLen(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestSignatureParityAgainstFragment(t *testing.T) {
	reg := lmcode.NewRegistry()
	if err := stdlib.RegisterString(reg); err != nil {
		t.Fatalf("RegisterString: %v", err)
	}

	sigs := reg.Signatures()
	expectedSigs := []string{"fn str_len(string) -> int"}
	if !reflect.DeepEqual(sigs, expectedSigs) {
		t.Errorf("Registry.Signatures() = %v, want %v", sigs, expectedSigs)
	}

	fragmentBytes, err := os.ReadFile("string.schema-fragment.json")
	if err != nil {
		t.Fatalf("ReadFile(string.schema-fragment.json): %v", err)
	}
	var fragment struct {
		Functions map[string][]struct {
			Params  []string `json:"params"`
			Returns string   `json:"returns"`
		} `json:"functions"`
	}
	if err := json.Unmarshal(fragmentBytes, &fragment); err != nil {
		t.Fatalf("Unmarshal fragment: %v", err)
	}

	fnList, ok := fragment.Functions["str_len"]
	if !ok || len(fnList) != 1 {
		t.Fatalf("expected str_len in fragment, got %v", fragment.Functions)
	}
	if !reflect.DeepEqual(fnList[0].Params, []string{"string"}) || fnList[0].Returns != "int" {
		t.Errorf("unexpected str_len definition in fragment: %v", fnList[0])
	}

	fixtureBytes, err := os.ReadFile(filepath.Join("testdata", "string.schema.json"))
	if err != nil {
		t.Fatalf("ReadFile(testdata/string.schema.json): %v", err)
	}
	var fixture struct {
		Functions map[string][]struct {
			Params  []string `json:"params"`
			Returns string   `json:"returns"`
		} `json:"functions"`
	}
	if err := json.Unmarshal(fixtureBytes, &fixture); err != nil {
		t.Fatalf("Unmarshal fixture: %v", err)
	}
	if !reflect.DeepEqual(fixture.Functions, fragment.Functions) {
		t.Errorf("fixture functions %v do not match fragment %v", fixture.Functions, fragment.Functions)
	}
}

func TestCompilerSignaturesParity(t *testing.T) {
	compiler := os.Getenv("LMSCRIPT_BIN")
	if compiler == "" {
		var err error
		compiler, err = exec.LookPath("lmscript")
		if err != nil {
			t.Skip("lmscript not found on PATH; compiler parity skipped")
		}
	}

	fixturePath, err := filepath.Abs(filepath.Join("testdata", "string.schema.json"))
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}

	cmd := exec.Command(compiler, "--dsl-signatures", fixturePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compiler failed (%v): %s", err, string(out))
	}

	rawLines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	var compilerSigs []string
	for _, l := range rawLines {
		l = strings.TrimSpace(l)
		if l != "" {
			compilerSigs = append(compilerSigs, l)
		}
	}
	sort.Strings(compilerSigs)

	reg := lmcode.NewRegistry()
	if err := stdlib.RegisterString(reg); err != nil {
		t.Fatalf("RegisterString: %v", err)
	}
	regSigs := reg.Signatures()
	sort.Strings(regSigs)

	if !reflect.DeepEqual(compilerSigs, regSigs) {
		t.Errorf("compiler signatures %v != registry signatures %v", compilerSigs, regSigs)
	}
}

func TestStrLenEndToEnd(t *testing.T) {
	bytecodePath := filepath.Join("testdata", "strlen.dslb")
	schemaPath := filepath.Join("testdata", "strlen.schema.json")

	bytecode, err := os.ReadFile(bytecodePath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", bytecodePath, err)
	}
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", schemaPath, err)
	}

	reg := lmcode.NewRegistry()
	if err := stdlib.RegisterString(reg); err != nil {
		t.Fatalf("RegisterString: %v", err)
	}

	var traces []int64
	err = reg.Function("trace_int", []string{"int"}, "void", func(args []lmcode.Value) (lmcode.Value, error) {
		traces = append(traces, args[0].Int)
		return lmcode.None(), nil
	})
	if err != nil {
		t.Fatalf("Function(trace_int): %v", err)
	}

	prog, err := lmcode.Load(bytecode, schema, reg)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	ctx := lmcode.NewContext()
	if err := prog.Run(ctx, reg); err != nil {
		t.Fatalf("Run: %v", err)
	}

	wantTraces := []int64{
		0,  // ""
		5,  // "hello"
		2,  // "é"
		6,  // "日本"
		13, // "hello, world!"
	}
	if !reflect.DeepEqual(traces, wantTraces) {
		t.Errorf("traces = %v, want %v", traces, wantTraces)
	}
}
