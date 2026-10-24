package stdlib

import (
	"bytes"
	lmcode "github.com/lmctl-ai/lmcode-go"
	"io"
	"os"
)

// RegisterTesting registers print and floating-point assertion helpers.
func RegisterTesting(r *lmcode.Registry, output io.Writer) error {
	if output == nil {
		output = os.Stdout
	}
	captured, isBuffer := output.(*bytes.Buffer)
	if !isBuffer {
		captured = &bytes.Buffer{}
	}
	if err := r.Function("print", []string{"string"}, "void", func(args []lmcode.Value) (lmcode.Value, error) {
		if _, err := io.WriteString(output, args[0].Str+"\n"); err != nil {
			return lmcode.None(), lmcode.NewHostError(err.Error())
		}
		if !isBuffer {
			_, _ = io.WriteString(captured, args[0].Str+"\n")
		}
		return lmcode.None(), nil
	}); err != nil {
		return err
	}
	if err := r.Function("printed", nil, "string", func([]lmcode.Value) (lmcode.Value, error) {
		return lmcode.Str(captured.String()), nil
	}); err != nil {
		return err
	}
	return r.Function("assert_close", []string{"float", "float", "float"}, "void", func(args []lmcode.Value) (lmcode.Value, error) {
		actual, expected, tolerance := args[0].Float, args[1].Float, args[2].Float
		if !(absFloat(actual-expected) <= tolerance) {
			return lmcode.None(), lmcode.NewHostError("assert_close failed: " + lmcode.FormatG15(actual) + " vs " + lmcode.FormatG15(expected) + " (tol " + lmcode.FormatG15(tolerance) + ")")
		}
		return lmcode.None(), nil
	})
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
