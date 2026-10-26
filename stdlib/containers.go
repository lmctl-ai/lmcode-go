package stdlib

import lmcode "github.com/lmctl-ai/lmcode-go"

type IntMap = lmcode.IntMap
type FloatMap = lmcode.FloatMap
type IntMapEntry = lmcode.IntMapEntry
type FloatMapEntry = lmcode.FloatMapEntry
type IntStack = lmcode.IntStack
type FloatStack = lmcode.FloatStack
type StrStack = lmcode.StrStack
type IntQueue = lmcode.IntQueue
type FloatQueue = lmcode.FloatQueue
type StrQueue = lmcode.StrQueue
type IntPQ = lmcode.IntPQ
type FloatPQ = lmcode.FloatPQ
type StrPQ = lmcode.StrPQ

func RegisterContainers(r *lmcode.Registry, arena ...*Collections) error {
	return lmcode.RegisterContainers(r, arena...)
}
