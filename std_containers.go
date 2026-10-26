package lmcode

import (
	"container/heap"
	"math"
)

// orderedMap uses byte-exact keys and keeps overwrite position stable.
type orderedMapEntry[T any] struct {
	key   string
	value T
}

type orderedMap[T any] struct {
	entries []orderedMapEntry[T]
	index   map[string]int
}

func newOrderedMap[T any]() orderedMap[T] {
	return orderedMap[T]{index: make(map[string]int)}
}

func (m *orderedMap[T]) set(key string, value T) {
	if m.index == nil {
		m.index = make(map[string]int)
	}
	if i, ok := m.index[key]; ok {
		m.entries[i].value = value
		return
	}
	m.index[key] = len(m.entries)
	m.entries = append(m.entries, orderedMapEntry[T]{key, value})
}

func (m *orderedMap[T]) remove(key string) bool {
	i, ok := m.index[key]
	if !ok {
		return false
	}
	delete(m.index, key)
	copy(m.entries[i:], m.entries[i+1:])
	var zero orderedMapEntry[T]
	m.entries[len(m.entries)-1] = zero
	m.entries = m.entries[:len(m.entries)-1]
	for j := i; j < len(m.entries); j++ {
		m.index[m.entries[j].key] = j
	}
	return true
}

func (m *orderedMap[T]) clear() {
	clear(m.index)
	for i := range m.entries {
		var zero orderedMapEntry[T]
		m.entries[i] = zero
	}
	m.entries = m.entries[:0]
}

// IntMap is an insertion-ordered byte-keyed map of int64 values.
type IntMap struct{ data orderedMap[int64] }

// FloatMap is an insertion-ordered byte-keyed map of binary64 values.
type FloatMap struct{ data orderedMap[float64] }

type IntMapEntry struct {
	Key   string
	Value int64
}

type FloatMapEntry struct {
	Key   string
	Value float64
}

func (m *IntMap) Entries() []IntMapEntry {
	out := make([]IntMapEntry, len(m.data.entries))
	for i, entry := range m.data.entries {
		out[i] = IntMapEntry{entry.key, entry.value}
	}
	return out
}

func (m *FloatMap) Entries() []FloatMapEntry {
	out := make([]FloatMapEntry, len(m.data.entries))
	for i, entry := range m.data.entries {
		out[i] = FloatMapEntry{entry.key, entry.value}
	}
	return out
}

type stackData[T any] struct{ items []T }

type queueData[T any] struct {
	items []T
	head  int
}

func (q *queueData[T]) len() int { return len(q.items) - q.head }

func (q *queueData[T]) push(value T) {
	if q.head == len(q.items) {
		q.items = q.items[:0]
		q.head = 0
	}
	q.items = append(q.items, value)
}

func (q *queueData[T]) pop() T {
	value := q.items[q.head]
	var zero T
	q.items[q.head] = zero
	q.head++
	if q.head == len(q.items) {
		q.items = q.items[:0]
		q.head = 0
	} else if q.head >= 1024 && q.head*2 >= len(q.items) {
		remaining := copy(q.items, q.items[q.head:])
		q.items = q.items[:remaining]
		q.head = 0
	}
	return value
}

func (q *queueData[T]) at(index int) T { return q.items[q.head+index] }

type priorityItem[T any] struct {
	value    T
	priority float64
	sequence int64
}

type priorityHeap[T any] []priorityItem[T]

func (h priorityHeap[T]) Len() int { return len(h) }
func (h priorityHeap[T]) Less(i, j int) bool {
	if h[i].priority == h[j].priority {
		return h[i].sequence < h[j].sequence
	}
	return h[i].priority < h[j].priority
}
func (h priorityHeap[T]) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *priorityHeap[T]) Push(value any) {
	*h = append(*h, value.(priorityItem[T]))
}
func (h *priorityHeap[T]) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	var zero priorityItem[T]
	old[last] = zero
	*h = old[:last]
	return value
}

type priorityQueueData[T any] struct {
	items        priorityHeap[T]
	nextSequence int64
}

func (q *priorityQueueData[T]) push(value T, priority float64, typeName string) error {
	if math.IsNaN(priority) || math.IsInf(priority, 0) {
		return NewHostError(typeName + ".push: priority must be finite")
	}
	if q.nextSequence == int64(1<<63-1) {
		return NewHostError(typeName + ".push: insertion sequence exhausted")
	}
	sequence := q.nextSequence
	heap.Push(&q.items, priorityItem[T]{value, priority, sequence})
	q.nextSequence = sequence + 1
	return nil
}

func (q *priorityQueueData[T]) pop() T  { return heap.Pop(&q.items).(priorityItem[T]).value }
func (q *priorityQueueData[T]) peek() T { return q.items[0].value }
func (q *priorityQueueData[T]) clear() {
	for i := range q.items {
		var zero priorityItem[T]
		q.items[i] = zero
	}
	q.items = nil
	q.nextSequence = 0
}
func (q *priorityQueueData[T]) values() []T {
	items := append(priorityHeap[T](nil), q.items...)
	out := make([]T, 0, len(items))
	for len(items) > 0 {
		out = append(out, heap.Pop(&items).(priorityItem[T]).value)
	}
	return out
}

type IntStack struct{ data stackData[int64] }
type FloatStack struct{ data stackData[float64] }
type StrStack struct{ data stackData[string] }
type IntQueue struct{ data queueData[int64] }
type FloatQueue struct{ data queueData[float64] }
type StrQueue struct{ data queueData[string] }
type IntPQ struct{ data priorityQueueData[int64] }
type FloatPQ struct{ data priorityQueueData[float64] }
type StrPQ struct{ data priorityQueueData[string] }

func (s *IntStack) Values() []int64     { return append([]int64(nil), s.data.items...) }
func (s *FloatStack) Values() []float64 { return append([]float64(nil), s.data.items...) }
func (s *StrStack) Values() []string    { return append([]string(nil), s.data.items...) }
func (q *IntQueue) Values() []int64     { return append([]int64(nil), q.data.items[q.data.head:]...) }
func (q *FloatQueue) Values() []float64 { return append([]float64(nil), q.data.items[q.data.head:]...) }
func (q *StrQueue) Values() []string    { return append([]string(nil), q.data.items[q.data.head:]...) }
func (q *IntPQ) Values() []int64        { return q.data.values() }
func (q *FloatPQ) Values() []float64    { return q.data.values() }
func (q *StrPQ) Values() []string       { return q.data.values() }

func groupCollections(r *Registry, arenas []*Collections) *Collections {
	if len(arenas) > 0 && arenas[0] != nil {
		return arenas[0]
	}
	if r.stdCollections != nil {
		return r.stdCollections
	}
	return NewCollections()
}

type mapBinding[T any] struct {
	typeName  string
	valueType string
	newMap    func() any
	data      func(any) *orderedMap[T]
	from      func(Value) T
	to        func(T) Value
}

func registerMap[T any](r *Registry, arena *Collections, b mapBinding[T]) error {
	if err := r.registerBuiltinFunction(b.typeNameToName(), nil, b.typeName, func([]Value) (Value, error) {
		return arena.object(b.typeName, b.newMap())
	}); err != nil {
		return err
	}
	data := func(self any) *orderedMap[T] { return b.data(self) }
	typeName := b.typeName
	methods := []struct {
		name    string
		params  []string
		returns string
		fn      Method
	}{
		{"len", nil, "int", func(self any, _ []Value) (Value, error) { return Int(int64(len(data(self).entries))), nil }},
		{"has", []string{"string"}, "bool", func(self any, a []Value) (Value, error) { _, ok := data(self).index[a[0].Str]; return Bool(ok), nil }},
		{"get", []string{"string"}, b.valueType, func(self any, a []Value) (Value, error) {
			m := data(self)
			i, ok := m.index[a[0].Str]
			if !ok {
				return None(), NewHostError(typeName + ".get: missing key")
			}
			return b.to(m.entries[i].value), nil
		}},
		{"get_or", []string{"string", b.valueType}, b.valueType, func(self any, a []Value) (Value, error) {
			m := data(self)
			i, ok := m.index[a[0].Str]
			if !ok {
				return a[1], nil
			}
			return b.to(m.entries[i].value), nil
		}},
		{"set", []string{"string", b.valueType}, typeName, func(self any, a []Value) (Value, error) {
			data(self).set(a[0].Str, b.from(a[1]))
			return Object(typeName, self), nil
		}},
		{"key_at", []string{"int"}, "string", func(self any, a []Value) (Value, error) {
			m := data(self)
			i, err := collectionIndex(typeName, "key_at", a[0].Int, len(m.entries))
			if err != nil {
				return None(), err
			}
			return String(m.entries[i].key), nil
		}},
		{"remove", []string{"string"}, "bool", func(self any, a []Value) (Value, error) { return Bool(data(self).remove(a[0].Str)), nil }},
		{"clear", nil, typeName, func(self any, _ []Value) (Value, error) { data(self).clear(); return Object(typeName, self), nil }},
		{"keys", nil, "StrList", func(self any, _ []Value) (Value, error) {
			keys := make([]string, len(data(self).entries))
			for i, entry := range data(self).entries {
				keys[i] = entry.key
			}
			return arena.object("StrList", &StrList{values: keys})
		}},
		{"values", nil, mapListType(typeName), func(self any, _ []Value) (Value, error) {
			entries := data(self).entries
			switch typeName {
			case "IntMap":
				values := make([]int64, len(entries))
				for i, entry := range entries {
					values[i] = b.to(entry.value).Int
				}
				return arena.object("IntList", &IntList{values: values})
			default:
				values := make([]float64, len(entries))
				for i, entry := range entries {
					values[i] = b.to(entry.value).Float
				}
				return arena.object("FloatList", &FloatList{values: values})
			}
		}},
	}
	for _, method := range methods {
		if err := r.registerBuiltinMethod(typeName, method.name, method.params, method.returns, method.fn); err != nil {
			return err
		}
	}
	return nil
}

func mapListType(typeName string) string {
	if typeName == "IntMap" {
		return "IntList"
	}
	return "FloatList"
}

func (b mapBinding[T]) typeNameToName() string {
	if b.typeName == "IntMap" {
		return "int_map"
	}
	return "float_map"
}

func registerStack[T any](r *Registry, arena *Collections, typeName, constructor, elementType string,
	newObject func() any, items func(any) *[]T, from func(Value) T, to func(T) Value) error {
	if err := r.registerBuiltinFunction(constructor, nil, typeName, func([]Value) (Value, error) { return arena.object(typeName, newObject()) }); err != nil {
		return err
	}
	methods := []struct {
		name    string
		params  []string
		returns string
		fn      Method
	}{
		{"push", []string{elementType}, typeName, func(self any, a []Value) (Value, error) {
			*items(self) = append(*items(self), from(a[0]))
			return Object(typeName, self), nil
		}},
		{"pop", nil, elementType, func(self any, _ []Value) (Value, error) {
			x := items(self)
			if len(*x) == 0 {
				return None(), NewHostError(typeName + ".pop: empty")
			}
			i := len(*x) - 1
			v := (*x)[i]
			var zero T
			(*x)[i] = zero
			*x = (*x)[:i]
			return to(v), nil
		}},
		{"peek", nil, elementType, func(self any, _ []Value) (Value, error) {
			x := *items(self)
			if len(x) == 0 {
				return None(), NewHostError(typeName + ".peek: empty")
			}
			return to(x[len(x)-1]), nil
		}},
		{"len", nil, "int", func(self any, _ []Value) (Value, error) { return Int(int64(len(*items(self)))), nil }},
		{"empty", nil, "bool", func(self any, _ []Value) (Value, error) { return Bool(len(*items(self)) == 0), nil }},
		{"clear", nil, typeName, func(self any, _ []Value) (Value, error) {
			x := items(self)
			var zero T
			for i := range *x {
				(*x)[i] = zero
			}
			*x = (*x)[:0]
			return Object(typeName, self), nil
		}},
		{"at", []string{"int"}, elementType, func(self any, a []Value) (Value, error) {
			x := *items(self)
			i, err := collectionIndex(typeName, "at", a[0].Int, len(x))
			if err != nil {
				return None(), err
			}
			return to(x[len(x)-1-i]), nil
		}},
	}
	for _, method := range methods {
		if err := r.registerBuiltinMethod(typeName, method.name, method.params, method.returns, method.fn); err != nil {
			return err
		}
	}
	return nil
}

func registerQueue[T any](r *Registry, arena *Collections, typeName, constructor, elementType string,
	newObject func() any, queue func(any) *queueData[T], from func(Value) T, to func(T) Value) error {
	if err := r.registerBuiltinFunction(constructor, nil, typeName, func([]Value) (Value, error) { return arena.object(typeName, newObject()) }); err != nil {
		return err
	}
	methods := []struct {
		name    string
		params  []string
		returns string
		fn      Method
	}{
		{"push", []string{elementType}, typeName, func(self any, a []Value) (Value, error) {
			queue(self).push(from(a[0]))
			return Object(typeName, self), nil
		}},
		{"pop", nil, elementType, func(self any, _ []Value) (Value, error) {
			q := queue(self)
			if q.len() == 0 {
				return None(), NewHostError(typeName + ".pop: empty")
			}
			return to(q.pop()), nil
		}},
		{"peek", nil, elementType, func(self any, _ []Value) (Value, error) {
			q := queue(self)
			if q.len() == 0 {
				return None(), NewHostError(typeName + ".peek: empty")
			}
			return to(q.items[q.head]), nil
		}},
		{"len", nil, "int", func(self any, _ []Value) (Value, error) { return Int(int64(queue(self).len())), nil }},
		{"empty", nil, "bool", func(self any, _ []Value) (Value, error) { return Bool(queue(self).len() == 0), nil }},
		{"clear", nil, typeName, func(self any, _ []Value) (Value, error) {
			q := queue(self)
			var zero T
			for i := q.head; i < len(q.items); i++ {
				q.items[i] = zero
			}
			q.items = nil
			q.head = 0
			return Object(typeName, self), nil
		}},
		{"at", []string{"int"}, elementType, func(self any, a []Value) (Value, error) {
			q := queue(self)
			i, err := collectionIndex(typeName, "at", a[0].Int, q.len())
			if err != nil {
				return None(), err
			}
			return to(q.at(i)), nil
		}},
	}
	for _, method := range methods {
		if err := r.registerBuiltinMethod(typeName, method.name, method.params, method.returns, method.fn); err != nil {
			return err
		}
	}
	return nil
}

func registerPQ[T any](r *Registry, arena *Collections, typeName, constructor, elementType string,
	newObject func() any, queue func(any) *priorityQueueData[T], from func(Value) T, to func(T) Value) error {
	if err := r.registerBuiltinFunction(constructor, nil, typeName, func([]Value) (Value, error) { return arena.object(typeName, newObject()) }); err != nil {
		return err
	}
	methods := []struct {
		name    string
		params  []string
		returns string
		fn      Method
	}{
		{"push", []string{elementType, "float"}, typeName, func(self any, a []Value) (Value, error) {
			q := queue(self)
			if err := q.push(from(a[0]), a[1].Float, typeName); err != nil {
				return None(), err
			}
			return Object(typeName, self), nil
		}},
		{"pop", nil, elementType, func(self any, _ []Value) (Value, error) {
			q := queue(self)
			if len(q.items) == 0 {
				return None(), NewHostError(typeName + ".pop: empty")
			}
			return to(heap.Pop(&q.items).(priorityItem[T]).value), nil
		}},
		{"peek", nil, elementType, func(self any, _ []Value) (Value, error) {
			q := queue(self)
			if len(q.items) == 0 {
				return None(), NewHostError(typeName + ".peek: empty")
			}
			return to(q.items[0].value), nil
		}},
		{"len", nil, "int", func(self any, _ []Value) (Value, error) { return Int(int64(len(queue(self).items))), nil }},
		{"empty", nil, "bool", func(self any, _ []Value) (Value, error) { return Bool(len(queue(self).items) == 0), nil }},
		{"clear", nil, typeName, func(self any, _ []Value) (Value, error) { queue(self).clear(); return Object(typeName, self), nil }},
	}
	for _, method := range methods {
		if err := r.registerBuiltinMethod(typeName, method.name, method.params, method.returns, method.fn); err != nil {
			return err
		}
	}
	return nil
}

// RegisterContainers installs maps, stacks, queues, and priority queues. When
// called by RegisterStd, all constructors use the registry's shared run arena.
func RegisterContainers(r *Registry, arenas ...*Collections) error {
	a := groupCollections(r, arenas)
	if err := registerMap(r, a, mapBinding[int64]{"IntMap", "int", func() any { return &IntMap{data: newOrderedMap[int64]()} }, func(x any) *orderedMap[int64] { return &x.(*IntMap).data }, func(v Value) int64 { return v.Int }, Int}); err != nil {
		return err
	}
	if err := registerMap(r, a, mapBinding[float64]{"FloatMap", "float", func() any { return &FloatMap{data: newOrderedMap[float64]()} }, func(x any) *orderedMap[float64] { return &x.(*FloatMap).data }, func(v Value) float64 { return v.Float }, Float}); err != nil {
		return err
	}
	if err := registerStack(r, a, "IntStack", "int_stack", "int", func() any { return &IntStack{} }, func(x any) *[]int64 { return &x.(*IntStack).data.items }, func(v Value) int64 { return v.Int }, Int); err != nil {
		return err
	}
	if err := registerStack(r, a, "FloatStack", "float_stack", "float", func() any { return &FloatStack{} }, func(x any) *[]float64 { return &x.(*FloatStack).data.items }, func(v Value) float64 { return v.Float }, Float); err != nil {
		return err
	}
	if err := registerStack(r, a, "StrStack", "str_stack", "string", func() any { return &StrStack{} }, func(x any) *[]string { return &x.(*StrStack).data.items }, func(v Value) string { return v.Str }, String); err != nil {
		return err
	}
	if err := registerQueue(r, a, "IntQueue", "int_queue", "int", func() any { return &IntQueue{} }, func(x any) *queueData[int64] { return &x.(*IntQueue).data }, func(v Value) int64 { return v.Int }, Int); err != nil {
		return err
	}
	if err := registerQueue(r, a, "FloatQueue", "float_queue", "float", func() any { return &FloatQueue{} }, func(x any) *queueData[float64] { return &x.(*FloatQueue).data }, func(v Value) float64 { return v.Float }, Float); err != nil {
		return err
	}
	if err := registerQueue(r, a, "StrQueue", "str_queue", "string", func() any { return &StrQueue{} }, func(x any) *queueData[string] { return &x.(*StrQueue).data }, func(v Value) string { return v.Str }, String); err != nil {
		return err
	}
	if err := registerPQ(r, a, "IntPQ", "int_pq", "int", func() any { return &IntPQ{} }, func(x any) *priorityQueueData[int64] { return &x.(*IntPQ).data }, func(v Value) int64 { return v.Int }, Int); err != nil {
		return err
	}
	if err := registerPQ(r, a, "FloatPQ", "float_pq", "float", func() any { return &FloatPQ{} }, func(x any) *priorityQueueData[float64] { return &x.(*FloatPQ).data }, func(v Value) float64 { return v.Float }, Float); err != nil {
		return err
	}
	if err := registerPQ(r, a, "StrPQ", "str_pq", "string", func() any { return &StrPQ{} }, func(x any) *priorityQueueData[string] { return &x.(*StrPQ).data }, func(v Value) string { return v.Str }, String); err != nil {
		return err
	}
	return nil
}
