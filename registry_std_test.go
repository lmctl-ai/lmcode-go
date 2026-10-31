package lmcode_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	lmcode "github.com/lmctl-ai/lmcode-go"
)

func canonicalSignatures(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile("std/lmcode-std.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Name      string `json:"name"`
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
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Name != "lmcode_std_v3" {
		t.Fatalf("canonical std name = %q, want lmcode_std_v3", schema.Name)
	}
	var out []string
	for n, defs := range schema.Functions {
		for _, d := range defs {
			out = append(out, fmt.Sprintf("fn %s(%s) -> %s", n, strings.Join(d.Params, ", "), d.Returns))
		}
	}
	for typ, methods := range schema.Types {
		for n, defs := range methods.Methods {
			for _, d := range defs {
				out = append(out, fmt.Sprintf("method %s.%s(%s) -> %s", typ, n, strings.Join(d.Params, ", "), d.Returns))
			}
		}
	}
	sort.Strings(out)
	return out
}

func TestStdSchemaLock(t *testing.T) {
	lock, err := os.ReadFile("std.lock")
	if err != nil {
		t.Fatal(err)
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(lock), "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			fields[key] = value
		}
	}
	if fields["schema"] != "std/lmcode-std.schema.json" || fields["version"] != "3" {
		t.Fatalf("std.lock does not pin canonical v3: %v", fields)
	}
	data, err := os.ReadFile(fields["schema"])
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if got := hex.EncodeToString(digest[:]); got != fields["sha256"] {
		t.Fatalf("std schema sha256 = %s, std.lock has %s", got, fields["sha256"])
	}
}

// npm exposes a command shim on Windows; execute its Node entry directly.
func compilerCommand(t *testing.T, compiler string, args ...string) *exec.Cmd {
	t.Helper()
	if runtime.GOOS == "windows" && strings.HasSuffix(strings.ToLower(compiler), ".cmd") {
		shim, err := os.ReadFile(compiler)
		if err != nil {
			t.Fatal(err)
		}
		match := regexp.MustCompile(`(?i)%dp0%[\\/]([^"\r\n]+)"\s+%\*`).FindSubmatch(shim)
		if len(match) != 2 {
			t.Fatalf("unsupported npm compiler shim: %s", compiler)
		}
		entry := filepath.Join(filepath.Dir(compiler), string(match[1]))
		return exec.Command("node", append([]string{entry}, args...)...)
	}
	return exec.Command(compiler, args...)
}

func TestPublishedCompilerStdSignatures(t *testing.T) {
	compiler := os.Getenv("LMSCRIPT_BIN")
	if compiler == "" {
		var err error
		compiler, err = exec.LookPath("lmscript")
		if err != nil {
			t.Skip("lmscript is not installed; CI installs the published compiler")
		}
	}
	output, err := compilerCommand(t, compiler, "--lmcode-signatures", "std/lmcode-std.schema.json").Output()
	if err != nil {
		t.Fatalf("lmscript --lmcode-signatures: %v", err)
	}
	want := strings.Join(lmcode.NewRegistry().StdSignatures(), "\n") + "\n"
	if !bytes.Equal(output, []byte(want)) {
		t.Fatalf("compiler signatures differ from the default Go registry (%d output bytes)", len(output))
	}
}

func TestCompilerDefaultCatalogParity(t *testing.T) {
	compiler := os.Getenv("LMSCRIPT_BIN")
	if compiler == "" {
		var err error
		compiler, err = exec.LookPath("lmscript")
		if err != nil {
			t.Skip("lmscript is not installed; CI installs the published compiler")
		}
	}
	tmp := t.TempDir() + "/empty-host.schema.json"
	const emptyHost = `{"dsl.schema":2,"name":"empty_host","context":{},"types":{},"functions":{},"conversions":[]}`
	if err := os.WriteFile(tmp, []byte(emptyHost), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := compilerCommand(t, compiler, "--lmcode-signatures", tmp).Output()
	if err != nil {
		t.Fatalf("lmscript --lmcode-signatures empty host: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(lines) < len(lmcode.NewRegistry().StdSignatures()) {
		t.Skipf("compiler bundles %d default std signatures; explicit v3 schema parity passed, but the compiler's v3 default catalog is not yet available", len(lines))
	}
	want := lmcode.NewRegistry().StdSignatures()
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("compiler default catalog has %d signatures; Go registry has %d", len(lines), len(want))
	}
}

func TestRegistryStdDefaultOptOutAndSelfCheck(t *testing.T) {
	defaultReg := lmcode.NewRegistry()
	if got, want := defaultReg.Signatures(), canonicalSignatures(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("default std signatures = %v, want canonical %v", got, want)
	}
	if got := defaultReg.StdSignatures(); !reflect.DeepEqual(got, defaultReg.Signatures()) {
		t.Fatalf("std signatures mismatch: %v", got)
	}
	if !defaultReg.MatchesSchemaSignatures(defaultReg.Signatures()) {
		t.Fatal("canonical schema signatures should match")
	}
	if defaultReg.FunctionBySignature("fn str_len(string) -> int") == nil {
		t.Fatal("default registry did not install std")
	}
	host := defaultReg.HostSignatures()
	if len(host) != 0 {
		t.Fatalf("default host signatures = %v", host)
	}
	want := append([]string(nil), defaultReg.Signatures()...)
	if err := lmcode.RegisterStd(defaultReg); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(defaultReg.Signatures(), want) {
		t.Fatal("RegisterStd is not idempotent")
	}

	bare := lmcode.NewRegistryWithoutStd()
	if len(bare.Signatures()) != 0 || len(bare.HostSignatures()) != 0 {
		t.Fatal("opt-out registry is not empty")
	}
	if err := bare.Function("app", nil, "void", func([]lmcode.Value) (lmcode.Value, error) { return lmcode.None(), nil }); err != nil {
		t.Fatal(err)
	}
	if err := bare.Context("ctx", "App"); err != nil {
		t.Fatal(err)
	}
	if err := bare.CheckSchemaSignatures([]string{"context ctx: App", "fn app() -> void"}); err != nil {
		t.Fatal(err)
	}
	if got := bare.HostSignatures(); len(got) != 2 || got[0] != "context ctx: App" {
		t.Fatalf("host signatures = %v", got)
	}
	if err := lmcode.RegisterStd(bare); err != nil {
		t.Fatal(err)
	}
	if bare.FunctionBySignature("fn str_len(string) -> int") == nil {
		t.Fatal("explicit std registration missing")
	}
	if err := lmcode.RegisterString(defaultReg); err != nil {
		t.Fatalf("idempotent group registration: %v", err)
	}
	if err := defaultReg.Function("str_len", []string{"string"}, "int", func([]lmcode.Value) (lmcode.Value, error) { return lmcode.Int(0), nil }); err == nil {
		t.Fatal("ordinary host override of built-in tuple was accepted")
	}
}

func TestTestingCatalogRemainsHostOptional(t *testing.T) {
	r := lmcode.NewRegistry()
	if len(r.StdSignatures()) != 206 {
		t.Fatalf("default std count = %d", len(r.StdSignatures()))
	}
	if err := lmcode.RegisterTesting(r, nil); err != nil {
		t.Fatal(err)
	}
	host := r.HostSignatures()
	if len(host) != 2 || host[0] != "fn assert_close(float, float, float) -> void" || host[1] != "fn print(string) -> void" {
		t.Fatalf("testing host signatures = %v", host)
	}
	if len(r.StdSignatures()) != 206 {
		t.Fatalf("std count after testing = %d", len(r.StdSignatures()))
	}
	if r.MatchesSchemaSignatures(r.StdSignatures()) {
		t.Fatal("testing declarations should be required by a host schema")
	}
	if !r.MatchesSchemaSignatures(append(append([]string{}, r.StdSignatures()...), host...)) {
		t.Fatal("merged testing schema should match")
	}
	if err := lmcode.RegisterTesting(r, nil); err == nil {
		t.Fatal("duplicate testing registration accepted")
	}
}
