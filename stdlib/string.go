package stdlib

import (
	"github.com/lmctl-ai/lmcode-go"
)

// StrLen returns the byte length of s, including NUL bytes. UTF-8 is not decoded.
func StrLen(s string) int64 {
	return int64(len(s))
}

// RegisterString registers the string catalog function (str_len) into the provided Registry.
func RegisterString(r *lmcode.Registry) error {
	return r.Function("str_len", []string{"string"}, "int", func(args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Int(StrLen(args[0].Str)), nil
	})
}
