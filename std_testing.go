package lmcode

import (
	"io"
	"os"
)

// RegisterTesting registers print and floating-point assertion helpers.
func RegisterTesting(r *Registry, output io.Writer) error {
	if output == nil {
		output = os.Stdout
	}
	if err := r.registerBuiltinFunction("print", []string{"string"}, "void", func(args []Value) (Value, error) {
		if _, err := io.WriteString(output, args[0].Str+"\n"); err != nil {
			return None(), NewHostError(err.Error())
		}
		return None(), nil
	}); err != nil {
		return err
	}
	return r.registerBuiltinFunction("assert_close", []string{"float", "float", "float"}, "void", func(args []Value) (Value, error) {
		actual, expected, tolerance := args[0].Float, args[1].Float, args[2].Float
		if !(absFloat(actual-expected) <= tolerance) {
			return None(), NewHostError("assert_close failed: " + FormatG15(actual) + " vs " + FormatG15(expected) + " (tol " + FormatG15(tolerance) + ")")
		}
		return None(), nil
	})
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
