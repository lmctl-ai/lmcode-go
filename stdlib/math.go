package stdlib

import (
	"math"

	"github.com/lmctl-ai/lmcode-go"
)

// MathAbsI returns the absolute value with the VM's two's-complement
// overflow rule: abs(MinInt64) is MinInt64.
func MathAbsI(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func MathMinI(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
func MathMaxI(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func MathFloorI(x int64) int64 { return x }
func MathCeilI(x int64) int64  { return x }
func MathRoundI(x int64) int64 { return x }

func MathMinF(a, b float64) float64 {
	if math.IsNaN(a) {
		return a
	}
	if math.IsNaN(b) {
		return b
	}
	if b < a {
		return b
	}
	return a // ties, including signed zero, select the first argument
}

func MathMaxF(a, b float64) float64 {
	if math.IsNaN(a) {
		return a
	}
	if math.IsNaN(b) {
		return b
	}
	if b > a {
		return b
	}
	return a // ties, including signed zero, select the first argument
}

func MathPowI(base, exponent int64) (int64, error) {
	if exponent < 0 {
		return 0, lmcode.NewHostError("math_pow: integer exponent must be nonnegative")
	}
	result, factor, n := int64(1), base, uint64(exponent)
	for n != 0 {
		if n&1 != 0 {
			result *= factor
		}
		n >>= 1
		if n != 0 {
			factor *= factor
		}
	}
	return result, nil
}

func MathSqrtI(x int64) (int64, error) {
	if x < 0 {
		return 0, lmcode.NewHostError("math_sqrt: integer argument must be nonnegative")
	}
	// Binary search with division avoids overflowing mid*mid.
	n := uint64(x)
	lo, hi := uint64(0), uint64(3037000500)
	for lo <= hi {
		mid := lo + (hi-lo)/2
		if mid == 0 || mid <= n/mid {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return int64(hi), nil
}

func MathPowF(base, exponent float64) float64 {
	if exponent == 0 || base == 1 {
		return 1
	}
	if (base == 1 || base == -1) && math.IsInf(exponent, 0) {
		return 1
	}
	if math.IsNaN(base) || math.IsNaN(exponent) {
		return math.NaN()
	}
	return math.Pow(base, exponent)
}

func RegisterMath(r *lmcode.Registry) error {
	entries := []struct {
		n, t string
		p    []string
		f    lmcode.Function
	}{
		{"math_abs", "int", []string{"int"}, func(a []lmcode.Value) (lmcode.Value, error) { return lmcode.Int(MathAbsI(a[0].Int)), nil }},
		{"math_abs", "float", []string{"float"}, func(a []lmcode.Value) (lmcode.Value, error) { return lmcode.Float(math.Abs(a[0].Float)), nil }},
		{"math_min", "int", []string{"int", "int"}, func(a []lmcode.Value) (lmcode.Value, error) { return lmcode.Int(MathMinI(a[0].Int, a[1].Int)), nil }},
		{"math_min", "float", []string{"float", "float"}, func(a []lmcode.Value) (lmcode.Value, error) {
			return lmcode.Float(MathMinF(a[0].Float, a[1].Float)), nil
		}},
		{"math_max", "int", []string{"int", "int"}, func(a []lmcode.Value) (lmcode.Value, error) { return lmcode.Int(MathMaxI(a[0].Int, a[1].Int)), nil }},
		{"math_max", "float", []string{"float", "float"}, func(a []lmcode.Value) (lmcode.Value, error) {
			return lmcode.Float(MathMaxF(a[0].Float, a[1].Float)), nil
		}},
		{"math_floor", "int", []string{"int"}, func(a []lmcode.Value) (lmcode.Value, error) { return lmcode.Int(MathFloorI(a[0].Int)), nil }},
		{"math_floor", "float", []string{"float"}, func(a []lmcode.Value) (lmcode.Value, error) { return lmcode.Float(math.Floor(a[0].Float)), nil }},
		{"math_ceil", "int", []string{"int"}, func(a []lmcode.Value) (lmcode.Value, error) { return lmcode.Int(MathCeilI(a[0].Int)), nil }},
		{"math_ceil", "float", []string{"float"}, func(a []lmcode.Value) (lmcode.Value, error) { return lmcode.Float(math.Ceil(a[0].Float)), nil }},
		{"math_round", "int", []string{"int"}, func(a []lmcode.Value) (lmcode.Value, error) { return lmcode.Int(MathRoundI(a[0].Int)), nil }},
		{"math_round", "float", []string{"float"}, func(a []lmcode.Value) (lmcode.Value, error) { return lmcode.Float(math.Round(a[0].Float)), nil }},
		{"math_pow", "int", []string{"int", "int"}, func(a []lmcode.Value) (lmcode.Value, error) {
			v, e := MathPowI(a[0].Int, a[1].Int)
			if e != nil {
				return lmcode.None(), e
			}
			return lmcode.Int(v), nil
		}},
		{"math_pow", "float", []string{"float", "float"}, func(a []lmcode.Value) (lmcode.Value, error) {
			return lmcode.Float(MathPowF(a[0].Float, a[1].Float)), nil
		}},
		{"math_sqrt", "int", []string{"int"}, func(a []lmcode.Value) (lmcode.Value, error) {
			v, e := MathSqrtI(a[0].Int)
			if e != nil {
				return lmcode.None(), e
			}
			return lmcode.Int(v), nil
		}},
		{"math_sqrt", "float", []string{"float"}, func(a []lmcode.Value) (lmcode.Value, error) { return lmcode.Float(math.Sqrt(a[0].Float)), nil }},
	}
	for _, e := range entries {
		if err := r.Function(e.n, e.p, e.t, e.f); err != nil {
			return err
		}
	}
	return nil
}
