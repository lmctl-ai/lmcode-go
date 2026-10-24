package stdlib

import lmcode "github.com/lmctl-ai/lmcode-go"

type Collections = lmcode.Collections
type StrList = lmcode.StrList
type IntList = lmcode.IntList
type FloatList = lmcode.FloatList
type StrMap = lmcode.StrMap
type StrMapEntry = lmcode.StrMapEntry

func NewCollections() *Collections { return lmcode.NewCollections() }
func RegisterCollections(r *lmcode.Registry, a ...*Collections) error {
	return lmcode.RegisterCollections(r, a...)
}
