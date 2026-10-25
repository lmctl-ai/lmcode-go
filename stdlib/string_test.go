package stdlib_test

import (
	"encoding/json"
	"fmt"
	"math"
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

func TestCatalogByteSemanticsAndEdges(t *testing.T) {
	if got := stdlib.StrLen("a\x00\xff"); got != 3 {
		t.Fatalf("byte length = %d", got)
	}
	if got := stdlib.StrSubstr("a\x00\xffz", 1, 2); got != "\x00\xff" {
		t.Fatalf("byte substring = %q", got)
	}
	if got := stdlib.StrSubstr("abc", -1<<63, 1<<63-1); got != "abc" || stdlib.StrSubstr("abc", 1<<63-1, 1<<63-1) != "" {
		t.Fatalf("extreme substring bounds: %q", got)
	}
	if got := stdlib.StrSubstr("é", 1, 1); got != "\xa9" {
		t.Fatalf("UTF-8 byte split = %q", got)
	}
	if got := stdlib.StrLower("A\xffÉ"); got != "a\xffÉ" {
		t.Fatalf("lower = %q", got)
	}
	if got := stdlib.StrUpper("a\xffé"); got != "A\xffé" {
		t.Fatalf("upper = %q", got)
	}
	if got := stdlib.StrTrim(" \t\x00x\r\n"); got != "\x00x" {
		t.Fatalf("trim = %q", got)
	}
	if !stdlib.StrStartsWith("abc", "") || !stdlib.StrEndsWith("abc", "") || !stdlib.StrContains("abc", "") {
		t.Fatal("empty matching semantics")
	}
	if stdlib.StrFind("a\x00b", "\x00") != 1 || stdlib.StrFind("abc", "z") != -1 {
		t.Fatal("byte find semantics")
	}
	if got, err := stdlib.StrReplace("a\x00a", "\x00", "X"); err != nil || got != "aXa" {
		t.Fatalf("replace = %q, %v", got, err)
	}
	if got, err := stdlib.StrRepeat("ab", 3); err != nil || got != "ababab" {
		t.Fatalf("repeat = %q, %v", got, err)
	}
	for _, tc := range []struct {
		s string
		n int64
	}{{"x", -1}, {"", -1}, {"x", -1 << 63}} {
		if _, err := stdlib.StrRepeat(tc.s, tc.n); err == nil || err.Error() != "str_repeat: count must be nonnegative and result at most 67108864 bytes" {
			t.Fatalf("repeat(%q,%d) did not return exact cap error", tc.s, tc.n)
		}
	}
	if got, err := stdlib.StrRepeat("", 1<<63-1); err != nil || got != "" {
		t.Fatalf("empty max repeat = %q, %v", got, err)
	}
	for _, s := range []string{"+12", "-9223372036854775808"} {
		if !stdlib.StrIsInt(s) {
			t.Errorf("integer %q rejected", s)
		}
	}
	for _, s := range []string{"9223372036854775808", "-9223372036854775809"} {
		if stdlib.StrIsInt(s) || stdlib.StrToInt(s) != 0 {
			t.Errorf("overflow integer %q accepted or converted", s)
		}
	}
	for _, s := range []string{" 1", "1 ", "9223372036854775808", "1\x00"} {
		if stdlib.StrIsInt(s) {
			t.Errorf("invalid integer %q accepted", s)
		}
	}
	for _, s := range []string{".5", "1.", "+1e-2", "4.9406564584124654e-324"} {
		if !stdlib.StrIsFloat(s) {
			t.Errorf("float %q rejected", s)
		}
	}
	for _, s := range []string{"1e99999", "1e-99999", "0x1p2", "nan", "inf", "-inf", "+NaN", "nan(payload)", "1,5", ".", "1e", "1e+", "1e-", "1..0", "0X1P2", "\t1", "1\n", "1\x00"} {
		if stdlib.StrIsFloat(s) {
			t.Errorf("invalid float %q accepted", s)
		}
	}
	for _, s := range []string{"0e99999", "0e-99999", "-0", "-0.0e-99999"} {
		if !stdlib.StrIsFloat(s) {
			t.Errorf("zero float %q rejected", s)
		}
	}
	for _, tc := range []struct {
		s    string
		want float64
	}{
		{"4.9406564584124654e-324", math.SmallestNonzeroFloat64},
		{"2.2250738585072014e-308", math.SmallestNonzeroFloat64 * (1 << 52)},
		{"1.7976931348623157e308", math.MaxFloat64},
		{"1.00000000000000011102230246251565404236316680908203125", 1},
		{"1.00000000000000011102230246251565404236316680908203126", math.Nextafter(1, 2)},
	} {
		if got := stdlib.StrToFloat(tc.s); got != tc.want {
			t.Errorf("float %q = %.17g, want %.17g", tc.s, got, tc.want)
		}
	}
	if math.Signbit(stdlib.StrToFloat("-0")) == false {
		t.Error("negative zero sign lost")
	}
	for _, tc := range []struct {
		text, pattern string
		want          bool
	}{
		{"a/b.txt", "a/*.txt", true}, {"a", "[!b]", true}, {"x", "[z-a]", false},
		{"a\x00b", "a?b", true}, {"?*[]\\", `\?\*\[\]\\`, true}, {"[", "[", true},
		{"]", `[\]]`, true}, {"-", `[a\-c]`, true}, {"\\", `[\\]`, true}, {"!", `\!`, true},
		{"]", "[]]", true}, {"a", "[!]]", true}, {"]", "[!]]", false},
		{"-", "[-a]", true}, {"-", "[a-]", true}, {"z", "[z-a]", false},
		{"[]", "[]", true}, {"[!", "[!", true}, {"\\", "\\", true},
	} {
		got, err := stdlib.StrMatch(tc.text, tc.pattern)
		if err != nil || got != tc.want {
			t.Errorf("match(%q,%q) = %v,%v", tc.text, tc.pattern, got, err)
		}
	}
	if _, err := stdlib.StrReplace(strings.Repeat("x", 65), "x", strings.Repeat("y", 1024*1024)); err == nil || err.Error() != "str_replace: result must be at most 67108864 bytes" {
		t.Fatal("replace cap not enforced")
	}
	megabyte := strings.Repeat("x", 1024*1024)
	if got, err := stdlib.StrReplace(strings.Repeat("x", 64), "x", megabyte); err != nil || len(got) != 64*1024*1024 {
		t.Fatalf("exact replacement cap = %d, %v", len(got), err)
	}
	if got, err := stdlib.StrReplace("aaaaa", "aa", "b"); err != nil || got != "bba" {
		t.Fatalf("nonoverlap replace = %q, %v", got, err)
	}
	if got, err := stdlib.StrReplace("aaa", "a", ""); err != nil || got != "" {
		t.Fatalf("deleting replace = %q, %v", got, err)
	}
	oversized := strings.Repeat("x", 64*1024*1024+1)
	for _, tc := range []struct{ from, to string }{{"z", "y"}, {"", "y"}} {
		if _, err := stdlib.StrReplace(oversized, tc.from, tc.to); err == nil || err.Error() != "str_replace: result must be at most 67108864 bytes" {
			t.Fatalf("oversized replace(%q) not rejected", tc.from)
		}
	}
	malformed := strings.Repeat("[", 20000)
	for i := 0; i < 2; i++ {
		if _, err := stdlib.StrMatch(malformed, malformed); err == nil || err.Error() != "str_match: pattern exceeds the 2^27 work limit" {
			t.Fatal("glob work cap not enforced")
		}
	}
}

func TestSignatureParityAgainstFragment(t *testing.T) {
	reg := lmcode.NewRegistryWithoutStd()
	if err := stdlib.RegisterString(reg); err != nil {
		t.Fatalf("RegisterString: %v", err)
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
	var expectedSigs []string
	for name, defs := range fragment.Functions {
		for _, def := range defs {
			expectedSigs = append(expectedSigs, fmt.Sprintf("fn %s(%s) -> %s", name, strings.Join(def.Params, ", "), def.Returns))
		}
	}
	sort.Strings(expectedSigs)
	if sigs := reg.Signatures(); !reflect.DeepEqual(sigs, expectedSigs) {
		t.Errorf("Registry.Signatures() = %v, want fragment signatures %v", sigs, expectedSigs)
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
	if len(rawLines) > 0 && rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}
	compilerSigs = append(compilerSigs, rawLines...)
	sort.Strings(compilerSigs)

	// lmscript 0.2.420 builds the std catalog into every schema, so the
	// compiler reports the full std surface; compare against a std registry.
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

	reg := lmcode.NewRegistryWithoutStd()
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
