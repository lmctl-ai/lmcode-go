package stdlib

import lmcode "github.com/lmctl-ai/lmcode-go"

func RegisterFile(r *lmcode.Registry, a ...*Collections) error { return lmcode.RegisterFile(r, a...) }
