package lmcode

import (
	"bytes"
	"io"
	"os"
)

// RegisterTesting registers print and floating-point assertion helpers.
func RegisterTesting(r *Registry, output io.Writer) error {
	if output == nil {
		output = os.Stdout
	}
	captured, isBuffer := output.(*bytes.Buffer)
	if !isBuffer {
		captured = &bytes.Buffer{}
	}
	if err := r.Function("print", []string{"string"}, "void", func(args []Value) (Value, error) {
		if _, err := io.WriteString(output, args[0].Str+"\n"); err != nil {
			return None(), NewHostError(err.Error())
		}
		if !isBuffer {
			_, _ = io.WriteString(captured, args[0].Str+"\n")
		}
		return None(), nil
	}); err != nil {
		return err
	}
	if err := r.Function("printed", nil, "string", func([]Value) (Value, error) {
		return Str(captured.String()), nil
	}); err != nil {
		return err
	}
	return r.Function("assert_close", []string{"float", "float", "float"}, "void", func(args []Value) (Value, error) {
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
