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

func TestMathIntegerEdges(t *testing.T) {
	if stdlib.MathAbsI(math.MinInt64) != math.MinInt64 {
		t.Fatal("abs MinInt64 must wrap")
	}
	if stdlib.MathMinI(4, -2) != -2 || stdlib.MathMaxI(4, -2) != 4 {
		t.Fatal("integer min/max")
	}
	if stdlib.MathFloorI(-3) != -3 || stdlib.MathCeilI(-3) != -3 || stdlib.MathRoundI(-3) != -3 {
		t.Fatal("integer identity")
	}
	if got, err := stdlib.MathPowI(-2, 63); err != nil || got != math.MinInt64 {
		t.Fatalf("pow wrap = %d, %v", got, err)
	}
	if got, err := stdlib.MathPowI(0, 0); err != nil || got != 1 {
		t.Fatalf("0^0 = %d, %v", got, err)
	}
	if _, err := stdlib.MathPowI(2, -1); err == nil || err.Error() != "math_pow: integer exponent must be nonnegative" {
		t.Fatalf("negative exponent error: %v", err)
	}
	if got, err := stdlib.MathSqrtI(math.MaxInt64); err != nil || got != 3037000499 {
		t.Fatalf("isqrt max = %d, %v", got, err)
	}
	if got, err := stdlib.MathSqrtI(9223372030926249000); err != nil || got != 3037000498 {
		t.Fatalf("isqrt near boundary = %d, %v", got, err)
	}
	if _, err := stdlib.MathSqrtI(-1); err == nil || err.Error() != "math_sqrt: integer argument must be nonnegative" {
		t.Fatalf("negative sqrt error: %v", err)
	}
}

func TestMathFloatEdges(t *testing.T) {
	negZero := math.Copysign(0, -1)
	if stdlib.MathMinF(4, -2) != -2 || stdlib.MathMinF(-2, 4) != -2 {
		t.Fatal("min operand order")
	}
	if stdlib.MathMaxF(4, -2) != 4 || stdlib.MathMaxF(-2, 4) != 4 {
		t.Fatal("max operand order")
	}
	if !math.Signbit(stdlib.MathMinF(negZero, 0)) || math.Signbit(stdlib.MathMinF(0, negZero)) {
		t.Fatal("min signed-zero tie")
	}
	if !math.Signbit(stdlib.MathMaxF(negZero, 0)) || math.Signbit(stdlib.MathMaxF(0, negZero)) {
		t.Fatal("max signed-zero tie")
	}
	if !math.IsNaN(stdlib.MathMinF(math.NaN(), 1)) || !math.IsNaN(stdlib.MathMaxF(1, math.NaN())) {
		t.Fatal("NaN min/max")
	}
	if math.Signbit(math.Abs(negZero)) || math.Abs(negZero) != 0 {
		t.Fatal("abs")
	}
	if stdlib.MathPowF(math.NaN(), 0) != 1 || stdlib.MathPowF(1, math.NaN()) != 1 || stdlib.MathPowF(-1, math.Inf(1)) != 1 {
		t.Fatal("pow special cases")
	}
	if !math.IsNaN(stdlib.MathPowF(math.NaN(), 2)) || !math.IsNaN(stdlib.MathPowF(2, math.NaN())) {
		t.Fatal("pow NaN")
	}
	if !math.Signbit(math.Sqrt(negZero)) {
		t.Fatal("sqrt signed zero")
	}
	if math.Round(0.5) != 1 || math.Round(-0.5) != -1 {
		t.Fatal("round ties away")
	}
}

func TestRegisterMath(t *testing.T) {
	r := lmcode.NewRegistryWithoutStd()
	if err := stdlib.RegisterMath(r); err != nil {
		t.Fatal(err)
	}
	for _, sig := range []string{
		"fn math_abs(int) -> int", "fn math_abs(float) -> float",
		"fn math_min(int, int) -> int", "fn math_min(float, float) -> float",
		"fn math_max(int, int) -> int", "fn math_max(float, float) -> float",
		"fn math_floor(int) -> int", "fn math_floor(float) -> float",
		"fn math_ceil(int) -> int", "fn math_ceil(float) -> float",
		"fn math_round(int) -> int", "fn math_round(float) -> float",
		"fn math_pow(int, int) -> int", "fn math_pow(float, float) -> float",
		"fn math_sqrt(int) -> int", "fn math_sqrt(float) -> float",
	} {
		if r.FunctionBySignature(sig) == nil {
			t.Errorf("missing %s", sig)
		}
	}
}

func TestMathCompilerSignatureParity(t *testing.T) {
	var fixture struct {
		Functions map[string][]struct {
			Params  []string `json:"params"`
			Returns string   `json:"returns"`
		} `json:"functions"`
	}
	raw, err := os.ReadFile("math.schema-fragment.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	var expected []string
	for name, defs := range fixture.Functions {
		for _, def := range defs {
			expected = append(expected, fmt.Sprintf("fn %s(%s) -> %s", name, strings.Join(def.Params, ", "), def.Returns))
		}
	}
	sort.Strings(expected)
	r := lmcode.NewRegistryWithoutStd()
	if err := stdlib.RegisterMath(r); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.Signatures(), expected) {
		t.Fatalf("registry signatures = %v, want %v", r.Signatures(), expected)
	}
	compiler, err := exec.LookPath("lmscript")
	if err != nil {
		t.Skip("lmscript not found on PATH; compiler parity skipped")
	}
	schema, err := filepath.Abs(filepath.Join("testdata", "math.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(compiler, "--dsl-signatures", schema).CombinedOutput()
	if err != nil {
		t.Fatalf("compiler failed (%v): %s", err, out)
	}
	lines := strings.Fields(string(out))
	// Signatures contain spaces, so split output by lines rather than fields.
	lines = strings.Split(strings.TrimSpace(string(out)), "\n")
	sort.Strings(lines)
	// lmscript 0.2.420 builds the std catalog into every schema, so the
	// compiler reports the full std surface; compare against a std registry.
	full := lmcode.NewRegistry()
	if err := stdlib.RegisterMath(full); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(lines, full.Signatures()) {
		t.Fatalf("compiler signatures = %v, want %v", lines, full.Signatures())
	}
}
