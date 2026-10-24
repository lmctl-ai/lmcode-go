package stdlib

import (
	lmcode "github.com/lmctl-ai/lmcode-go"
	"io"
)

func RegisterTesting(r *lmcode.Registry, output io.Writer) error {
	return lmcode.RegisterTesting(r, output)
}
