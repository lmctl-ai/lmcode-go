package stdlib

import lmcode "github.com/lmctl-ai/lmcode-go"

type JSON = lmcode.JSON

func ParseJSON(source string) (*JSON, error) { return lmcode.ParseJSON(source) }
func RegisterJSON(r *lmcode.Registry, arena ...*Collections) error {
	return lmcode.RegisterJSON(r, arena...)
}
