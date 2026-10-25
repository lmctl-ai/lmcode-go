package lmcode

import (
	"strconv"
	"strings"
)

const collectionObjectLimit = 65536

// Collections owns the object budget for one registry lifetime. Reset permits
// a host to reuse the registry for a new run after all old handles expire.
type Collections struct{ objects int }

// NewCollections creates an empty reusable collection arena.
func NewCollections() *Collections { return &Collections{} }

// Reset releases the arena's object budget for a subsequent run.
func (a *Collections) Reset() { a.objects = 0 }

// Size reports the number of collection objects allocated since the last reset.
func (a *Collections) Size() int { return a.objects }

func (a *Collections) object(typeName string, value any) (Value, error) {
	if a.objects >= collectionObjectLimit {
		return None(), NewHostError(typeName + " object limit exceeded (65536)")
	}
	a.objects++
	return Object(typeName, value), nil
}

type StrList struct{ values []string }
type IntList struct{ values []int64 }
type FloatList struct{ values []float64 }
type StrMap struct {
	keys      []string
	values    map[string]string
	positions map[string]int
	deleted   map[int]bool
	live      int
	dirty     bool
	fenwick   []int
}

// Values returns a snapshot of the list's current byte strings.
func (l *StrList) Values() []string { return append([]string(nil), l.values...) }

// Values returns a snapshot of the list's current integers.
func (l *IntList) Values() []int64 { return append([]int64(nil), l.values...) }

// Values returns a snapshot of the list's current floats.
func (l *FloatList) Values() []float64 { return append([]float64(nil), l.values...) }

// StrMapEntry is one key/value pair in insertion order.
type StrMapEntry struct{ Key, Value string }

// Entries returns a snapshot of the map in insertion order.
func (m *StrMap) Entries() []StrMapEntry {
	m.compact()
	entries := make([]StrMapEntry, 0, len(m.keys))
	for _, key := range m.keys {
		entries = append(entries, StrMapEntry{Key: key, Value: m.values[key]})
	}
	return entries
}

func (m *StrMap) compact() {
	if !m.dirty {
		return
	}
	keys := make([]string, 0, m.live)
	positions := make(map[string]int, m.live)
	for i, key := range m.keys {
		if m.deleted[i] {
			continue
		}
		positions[key] = len(keys)
		keys = append(keys, key)
	}
	m.keys, m.positions, m.deleted, m.dirty = keys, positions, make(map[int]bool), false
	m.fenwick = make([]int, len(keys)+1)
	for i := range keys {
		m.fenwickAdd(i+1, 1)
	}
}

func (m *StrMap) fenwickPrefix(index int) int {
	total := 0
	for index > 0 {
		total += m.fenwick[index]
		index -= index & -index
	}
	return total
}

func (m *StrMap) fenwickAdd(index, delta int) {
	for index < len(m.fenwick) {
		m.fenwick[index] += delta
		index += index & -index
	}
}
func (m *StrMap) fenwickSelect(target int) int {
	index, bit := 0, 1
	for bit<<1 < len(m.fenwick) {
		bit <<= 1
	}
	for bit > 0 {
		next := index + bit
		if next < len(m.fenwick) && m.fenwick[next] < target {
			index, target = next, target-m.fenwick[next]
		}
		bit >>= 1
	}
	return index
}

func collectionIndex(typeName, method string, index int64, length int) (int, error) {
	if index < 0 || index >= int64(length) {
		return 0, NewHostError(typeName + "." + method + ": index out of range: " + formatInt(index))
	}
	return int(index), nil
}

func formatInt(v int64) string { return strconv.FormatInt(v, 10) }

// RegisterCollections registers the byte-oriented list and map catalog.
// Each registration owns a 65536-object arena captured by its callbacks.
func RegisterCollections(r *Registry, arenas ...*Collections) error {
	if len(arenas) > 0 && arenas[0] != nil && r.stdCollections == nil {
		r.explicitCollections = true
	}
	arena := NewCollections()
	if len(arenas) > 0 && arenas[0] != nil {
		arena = arenas[0]
	}
	add := func(name string, params []string, returns string, fn Function) error {
		return r.registerBuiltinFunction(name, params, returns, fn)
	}

	if err := add("str_list", nil, "StrList", func([]Value) (Value, error) {
		return arena.object("StrList", &StrList{})
	}); err != nil {
		return err
	}
	if err := add("int_list", nil, "IntList", func([]Value) (Value, error) {
		return arena.object("IntList", &IntList{})
	}); err != nil {
		return err
	}
	if err := add("float_list", nil, "FloatList", func([]Value) (Value, error) {
		return arena.object("FloatList", &FloatList{})
	}); err != nil {
		return err
	}
	if err := add("str_map", nil, "StrMap", func([]Value) (Value, error) {
		return arena.object("StrMap", &StrMap{values: make(map[string]string), positions: make(map[string]int), deleted: make(map[int]bool), fenwick: []int{0}})
	}); err != nil {
		return err
	}
	if err := add("str_split", []string{"string", "string"}, "StrList", func(args []Value) (Value, error) {
		separator := args[1].Str
		if separator == "" {
			return None(), NewHostError("str_split: separator must not be empty")
		}
		parts := strings.Split(args[0].Str, separator)
		return arena.object("StrList", &StrList{values: parts})
	}); err != nil {
		return err
	}

	methodNames := []string{
		"len", "at", "add", "set", "contains", "index_of", "join", "clear",
		"len", "at", "add", "set", "contains", "index_of", "clear",
		"len", "at", "add", "set", "contains", "index_of", "clear",
		"len", "has", "get", "get_or", "set", "key_at", "remove", "clear",
	}
	methods := []struct {
		typeName string
		params   []string
		returns  string
		fn       Method
	}{
		{"StrList", nil, "int", func(self any, _ []Value) (Value, error) {
			return Int(int64(len(self.(*StrList).values))), nil
		}},
		{"StrList", []string{"int"}, "string", func(self any, a []Value) (Value, error) {
			i, err := collectionIndex("StrList", "at", a[0].Int, len(self.(*StrList).values))
			if err != nil {
				return None(), err
			}
			return Str(self.(*StrList).values[i]), nil
		}},
		{"StrList", []string{"string"}, "StrList", func(self any, a []Value) (Value, error) {
			self.(*StrList).values = append(self.(*StrList).values, a[0].Str)
			return Object("StrList", self), nil
		}},
		{"StrList", []string{"int", "string"}, "StrList", func(self any, a []Value) (Value, error) {
			l := self.(*StrList)
			i, err := collectionIndex("StrList", "set", a[0].Int, len(l.values))
			if err != nil {
				return None(), err
			}
			l.values[i] = a[1].Str
			return Object("StrList", self), nil
		}},
		{"StrList", []string{"string"}, "bool", func(self any, a []Value) (Value, error) {
			for _, v := range self.(*StrList).values {
				if v == a[0].Str {
					return Bool(true), nil
				}
			}
			return Bool(false), nil
		}},
		{"StrList", []string{"string"}, "int", func(self any, a []Value) (Value, error) {
			for i, v := range self.(*StrList).values {
				if v == a[0].Str {
					return Int(int64(i)), nil
				}
			}
			return Int(-1), nil
		}},
		{"StrList", []string{"string"}, "string", func(self any, a []Value) (Value, error) {
			return Str(strings.Join(self.(*StrList).values, a[0].Str)), nil
		}},
		{"StrList", nil, "StrList", func(self any, _ []Value) (Value, error) {
			self.(*StrList).values = self.(*StrList).values[:0]
			return Object("StrList", self), nil
		}},
		{"IntList", nil, "int", func(self any, _ []Value) (Value, error) {
			return Int(int64(len(self.(*IntList).values))), nil
		}},
		{"IntList", []string{"int"}, "int", func(self any, a []Value) (Value, error) {
			i, err := collectionIndex("IntList", "at", a[0].Int, len(self.(*IntList).values))
			if err != nil {
				return None(), err
			}
			return Int(self.(*IntList).values[i]), nil
		}},
		{"IntList", []string{"int"}, "IntList", func(self any, a []Value) (Value, error) {
			self.(*IntList).values = append(self.(*IntList).values, a[0].Int)
			return Object("IntList", self), nil
		}},
		{"IntList", []string{"int", "int"}, "IntList", func(self any, a []Value) (Value, error) {
			l := self.(*IntList)
			i, err := collectionIndex("IntList", "set", a[0].Int, len(l.values))
			if err != nil {
				return None(), err
			}
			l.values[i] = a[1].Int
			return Object("IntList", self), nil
		}},
		{"IntList", []string{"int"}, "bool", func(self any, a []Value) (Value, error) {
			for _, v := range self.(*IntList).values {
				if v == a[0].Int {
					return Bool(true), nil
				}
			}
			return Bool(false), nil
		}},
		{"IntList", []string{"int"}, "int", func(self any, a []Value) (Value, error) {
			for i, v := range self.(*IntList).values {
				if v == a[0].Int {
					return Int(int64(i)), nil
				}
			}
			return Int(-1), nil
		}},
		{"IntList", nil, "IntList", func(self any, _ []Value) (Value, error) {
			self.(*IntList).values = self.(*IntList).values[:0]
			return Object("IntList", self), nil
		}},
		{"FloatList", nil, "int", func(self any, _ []Value) (Value, error) {
			return Int(int64(len(self.(*FloatList).values))), nil
		}},
		{"FloatList", []string{"int"}, "float", func(self any, a []Value) (Value, error) {
			i, err := collectionIndex("FloatList", "at", a[0].Int, len(self.(*FloatList).values))
			if err != nil {
				return None(), err
			}
			return Float(self.(*FloatList).values[i]), nil
		}},
		{"FloatList", []string{"float"}, "FloatList", func(self any, a []Value) (Value, error) {
			self.(*FloatList).values = append(self.(*FloatList).values, a[0].Float)
			return Object("FloatList", self), nil
		}},
		{"FloatList", []string{"int", "float"}, "FloatList", func(self any, a []Value) (Value, error) {
			l := self.(*FloatList)
			i, err := collectionIndex("FloatList", "set", a[0].Int, len(l.values))
			if err != nil {
				return None(), err
			}
			l.values[i] = a[1].Float
			return Object("FloatList", self), nil
		}},
		{"FloatList", []string{"float"}, "bool", func(self any, a []Value) (Value, error) {
			for _, v := range self.(*FloatList).values {
				if v == a[0].Float {
					return Bool(true), nil
				}
			}
			return Bool(false), nil
		}},
		{"FloatList", []string{"float"}, "int", func(self any, a []Value) (Value, error) {
			for i, v := range self.(*FloatList).values {
				if v == a[0].Float {
					return Int(int64(i)), nil
				}
			}
			return Int(-1), nil
		}},
		{"FloatList", nil, "FloatList", func(self any, _ []Value) (Value, error) {
			self.(*FloatList).values = self.(*FloatList).values[:0]
			return Object("FloatList", self), nil
		}},
		{"StrMap", nil, "int", func(self any, _ []Value) (Value, error) {
			return Int(int64(self.(*StrMap).live)), nil
		}},
		{"StrMap", []string{"string"}, "bool", func(self any, a []Value) (Value, error) {
			_, ok := self.(*StrMap).values[a[0].Str]
			return Bool(ok), nil
		}},
		{"StrMap", []string{"string"}, "string", func(self any, a []Value) (Value, error) {
			m := self.(*StrMap)
			v, ok := m.values[a[0].Str]
			if !ok {
				return None(), NewHostError("StrMap.get: missing key")
			}
			return Str(v), nil
		}},
		{"StrMap", []string{"string", "string"}, "string", func(self any, a []Value) (Value, error) {
			if v, ok := self.(*StrMap).values[a[0].Str]; ok {
				return Str(v), nil
			}
			return Str(a[1].Str), nil
		}},
		{"StrMap", []string{"string", "string"}, "StrMap", func(self any, a []Value) (Value, error) {
			m := self.(*StrMap)
			if _, ok := m.positions[a[0].Str]; !ok {
				m.positions[a[0].Str] = len(m.keys)
				m.keys = append(m.keys, a[0].Str)
				index := len(m.keys)
				m.fenwick = append(m.fenwick, m.fenwickPrefix(index-1)-m.fenwickPrefix(index-(index&-index))+1)
				m.live++
			}
			m.values[a[0].Str] = a[1].Str
			return Object("StrMap", self), nil
		}},
		{"StrMap", []string{"int"}, "string", func(self any, a []Value) (Value, error) {
			m := self.(*StrMap)
			if a[0].Int < 0 || a[0].Int >= int64(m.live) {
				return None(), NewHostError("StrMap.key_at: index out of range: " + formatInt(a[0].Int))
			}
			return Str(m.keys[m.fenwickSelect(int(a[0].Int)+1)]), nil
		}},
		{"StrMap", []string{"string"}, "bool", func(self any, a []Value) (Value, error) {
			m := self.(*StrMap)
			i, ok := m.positions[a[0].Str]
			if !ok {
				return Bool(false), nil
			}
			delete(m.values, a[0].Str)
			delete(m.positions, a[0].Str)
			m.deleted[i], m.live, m.dirty = true, m.live-1, true
			m.fenwickAdd(i+1, -1)
			return Bool(true), nil
		}},
		{"StrMap", nil, "StrMap", func(self any, _ []Value) (Value, error) {
			m := self.(*StrMap)
			clear(m.values)
			m.keys = m.keys[:0]
			clear(m.positions)
			clear(m.deleted)
			m.live, m.dirty, m.fenwick = 0, false, []int{0}
			return Object("StrMap", self), nil
		}},
	}
	for i, e := range methods {
		if err := r.registerBuiltinMethod(e.typeName, methodNames[i], e.params, e.returns, e.fn); err != nil {
			return err
		}
	}
	return nil
}
