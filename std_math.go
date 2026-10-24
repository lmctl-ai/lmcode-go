package lmcode

import (
	"math"
)

// StdMathAbsI returns the absolute value with the VM's two's-complement
// overflow rule: abs(MinInt64) is MinInt64.
func StdMathAbsI(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func StdMathMinI(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
func StdMathMaxI(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
func StdMathFloorI(x int64) int64 { return x }
func StdMathCeilI(x int64) int64  { return x }
func StdMathRoundI(x int64) int64 { return x }

func StdMathMinF(a, b float64) float64 {
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

func StdMathMaxF(a, b float64) float64 {
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

func StdMathPowI(base, exponent int64) (int64, error) {
	if exponent < 0 {
		return 0, NewHostError("math_pow: integer exponent must be nonnegative")
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

func StdMathSqrtI(x int64) (int64, error) {
	if x < 0 {
		return 0, NewHostError("math_sqrt: integer argument must be nonnegative")
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

func StdMathPowF(base, exponent float64) float64 {
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

func RegisterStdMath(r *Registry) error {
	entries := []struct {
		n, t string
		p    []string
		f    Function
	}{
		{"math_abs", "int", []string{"int"}, func(a []Value) (Value, error) { return Int(StdMathAbsI(a[0].Int)), nil }},
		{"math_abs", "float", []string{"float"}, func(a []Value) (Value, error) { return Float(math.Abs(a[0].Float)), nil }},
		{"math_min", "int", []string{"int", "int"}, func(a []Value) (Value, error) { return Int(StdMathMinI(a[0].Int, a[1].Int)), nil }},
		{"math_min", "float", []string{"float", "float"}, func(a []Value) (Value, error) {
			return Float(StdMathMinF(a[0].Float, a[1].Float)), nil
		}},
		{"math_max", "int", []string{"int", "int"}, func(a []Value) (Value, error) { return Int(StdMathMaxI(a[0].Int, a[1].Int)), nil }},
		{"math_max", "float", []string{"float", "float"}, func(a []Value) (Value, error) {
			return Float(StdMathMaxF(a[0].Float, a[1].Float)), nil
		}},
		{"math_floor", "int", []string{"int"}, func(a []Value) (Value, error) { return Int(StdMathFloorI(a[0].Int)), nil }},
		{"math_floor", "float", []string{"float"}, func(a []Value) (Value, error) { return Float(math.Floor(a[0].Float)), nil }},
		{"math_ceil", "int", []string{"int"}, func(a []Value) (Value, error) { return Int(StdMathCeilI(a[0].Int)), nil }},
		{"math_ceil", "float", []string{"float"}, func(a []Value) (Value, error) { return Float(math.Ceil(a[0].Float)), nil }},
		{"math_round", "int", []string{"int"}, func(a []Value) (Value, error) { return Int(StdMathRoundI(a[0].Int)), nil }},
		{"math_round", "float", []string{"float"}, func(a []Value) (Value, error) { return Float(math.Round(a[0].Float)), nil }},
		{"math_pow", "int", []string{"int", "int"}, func(a []Value) (Value, error) {
			v, e := StdMathPowI(a[0].Int, a[1].Int)
			if e != nil {
				return None(), e
			}
			return Int(v), nil
		}},
		{"math_pow", "float", []string{"float", "float"}, func(a []Value) (Value, error) {
			return Float(StdMathPowF(a[0].Float, a[1].Float)), nil
		}},
		{"math_sqrt", "int", []string{"int"}, func(a []Value) (Value, error) {
			v, e := StdMathSqrtI(a[0].Int)
			if e != nil {
				return None(), e
			}
			return Int(v), nil
		}},
		{"math_sqrt", "float", []string{"float"}, func(a []Value) (Value, error) { return Float(math.Sqrt(a[0].Float)), nil }},
	}
	for _, e := range entries {
		if err := r.Function(e.n, e.p, e.t, e.f); err != nil {
			return err
		}
	}
	return nil
}
