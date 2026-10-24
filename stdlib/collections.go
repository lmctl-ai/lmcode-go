package stdlib

import (
	"strconv"
	"strings"

	lmcode "github.com/lmctl-ai/lmcode-go"
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

func (a *Collections) object(typeName string, value any) (lmcode.Value, error) {
	if a.objects >= collectionObjectLimit {
		return lmcode.None(), lmcode.NewHostError(typeName + " object limit exceeded (65536)")
	}
	a.objects++
	return lmcode.Object(typeName, value), nil
}

type StrList struct{ values []string }
type IntList struct{ values []int64 }
type FloatList struct{ values []float64 }
type StrMap struct {
	keys   []string
	values map[string]string
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
	entries := make([]StrMapEntry, 0, len(m.keys))
	for _, key := range m.keys {
		entries = append(entries, StrMapEntry{Key: key, Value: m.values[key]})
	}
	return entries
}

func collectionIndex(typeName, method string, index int64, length int) (int, error) {
	if index < 0 || index >= int64(length) {
		return 0, lmcode.NewHostError(typeName + "." + method + ": index out of range: " + formatInt(index))
	}
	return int(index), nil
}

func formatInt(v int64) string { return strconv.FormatInt(v, 10) }

// RegisterCollections registers the byte-oriented list and map catalog.
// Each registration owns a 65536-object arena captured by its callbacks.
func RegisterCollections(r *lmcode.Registry, arenas ...*Collections) error {
	arena := NewCollections()
	if len(arenas) > 0 && arenas[0] != nil {
		arena = arenas[0]
	}
	add := func(name string, params []string, returns string, fn lmcode.Function) error {
		return r.Function(name, params, returns, fn)
	}

	if err := add("str_list", nil, "StrList", func([]lmcode.Value) (lmcode.Value, error) {
		return arena.object("StrList", &StrList{})
	}); err != nil {
		return err
	}
	if err := add("int_list", nil, "IntList", func([]lmcode.Value) (lmcode.Value, error) {
		return arena.object("IntList", &IntList{})
	}); err != nil {
		return err
	}
	if err := add("float_list", nil, "FloatList", func([]lmcode.Value) (lmcode.Value, error) {
		return arena.object("FloatList", &FloatList{})
	}); err != nil {
		return err
	}
	if err := add("str_map", nil, "StrMap", func([]lmcode.Value) (lmcode.Value, error) {
		return arena.object("StrMap", &StrMap{values: make(map[string]string)})
	}); err != nil {
		return err
	}
	if err := add("str_split", []string{"string", "string"}, "StrList", func(args []lmcode.Value) (lmcode.Value, error) {
		separator := args[1].Str
		if separator == "" {
			return lmcode.None(), lmcode.NewHostError("str_split: separator must not be empty")
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
		fn       lmcode.Method
	}{
		{"StrList", nil, "int", func(self any, _ []lmcode.Value) (lmcode.Value, error) {
			return lmcode.Int(int64(len(self.(*StrList).values))), nil
		}},
		{"StrList", []string{"int"}, "string", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			i, err := collectionIndex("StrList", "at", a[0].Int, len(self.(*StrList).values))
			if err != nil {
				return lmcode.None(), err
			}
			return lmcode.Str(self.(*StrList).values[i]), nil
		}},
		{"StrList", []string{"string"}, "StrList", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			self.(*StrList).values = append(self.(*StrList).values, a[0].Str)
			return lmcode.Object("StrList", self), nil
		}},
		{"StrList", []string{"int", "string"}, "StrList", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			l := self.(*StrList)
			i, err := collectionIndex("StrList", "set", a[0].Int, len(l.values))
			if err != nil {
				return lmcode.None(), err
			}
			l.values[i] = a[1].Str
			return lmcode.Object("StrList", self), nil
		}},
		{"StrList", []string{"string"}, "bool", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			for _, v := range self.(*StrList).values {
				if v == a[0].Str {
					return lmcode.Bool(true), nil
				}
			}
			return lmcode.Bool(false), nil
		}},
		{"StrList", []string{"string"}, "int", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			for i, v := range self.(*StrList).values {
				if v == a[0].Str {
					return lmcode.Int(int64(i)), nil
				}
			}
			return lmcode.Int(-1), nil
		}},
		{"StrList", []string{"string"}, "string", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			return lmcode.Str(strings.Join(self.(*StrList).values, a[0].Str)), nil
		}},
		{"StrList", nil, "StrList", func(self any, _ []lmcode.Value) (lmcode.Value, error) {
			self.(*StrList).values = self.(*StrList).values[:0]
			return lmcode.Object("StrList", self), nil
		}},
		{"IntList", nil, "int", func(self any, _ []lmcode.Value) (lmcode.Value, error) {
			return lmcode.Int(int64(len(self.(*IntList).values))), nil
		}},
		{"IntList", []string{"int"}, "int", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			i, err := collectionIndex("IntList", "at", a[0].Int, len(self.(*IntList).values))
			if err != nil {
				return lmcode.None(), err
			}
			return lmcode.Int(self.(*IntList).values[i]), nil
		}},
		{"IntList", []string{"int"}, "IntList", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			self.(*IntList).values = append(self.(*IntList).values, a[0].Int)
			return lmcode.Object("IntList", self), nil
		}},
		{"IntList", []string{"int", "int"}, "IntList", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			l := self.(*IntList)
			i, err := collectionIndex("IntList", "set", a[0].Int, len(l.values))
			if err != nil {
				return lmcode.None(), err
			}
			l.values[i] = a[1].Int
			return lmcode.Object("IntList", self), nil
		}},
		{"IntList", []string{"int"}, "bool", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			for _, v := range self.(*IntList).values {
				if v == a[0].Int {
					return lmcode.Bool(true), nil
				}
			}
			return lmcode.Bool(false), nil
		}},
		{"IntList", []string{"int"}, "int", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			for i, v := range self.(*IntList).values {
				if v == a[0].Int {
					return lmcode.Int(int64(i)), nil
				}
			}
			return lmcode.Int(-1), nil
		}},
		{"IntList", nil, "IntList", func(self any, _ []lmcode.Value) (lmcode.Value, error) {
			self.(*IntList).values = self.(*IntList).values[:0]
			return lmcode.Object("IntList", self), nil
		}},
		{"FloatList", nil, "int", func(self any, _ []lmcode.Value) (lmcode.Value, error) {
			return lmcode.Int(int64(len(self.(*FloatList).values))), nil
		}},
		{"FloatList", []string{"int"}, "float", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			i, err := collectionIndex("FloatList", "at", a[0].Int, len(self.(*FloatList).values))
			if err != nil {
				return lmcode.None(), err
			}
			return lmcode.Float(self.(*FloatList).values[i]), nil
		}},
		{"FloatList", []string{"float"}, "FloatList", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			self.(*FloatList).values = append(self.(*FloatList).values, a[0].Float)
			return lmcode.Object("FloatList", self), nil
		}},
		{"FloatList", []string{"int", "float"}, "FloatList", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			l := self.(*FloatList)
			i, err := collectionIndex("FloatList", "set", a[0].Int, len(l.values))
			if err != nil {
				return lmcode.None(), err
			}
			l.values[i] = a[1].Float
			return lmcode.Object("FloatList", self), nil
		}},
		{"FloatList", []string{"float"}, "bool", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			for _, v := range self.(*FloatList).values {
				if v == a[0].Float {
					return lmcode.Bool(true), nil
				}
			}
			return lmcode.Bool(false), nil
		}},
		{"FloatList", []string{"float"}, "int", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			for i, v := range self.(*FloatList).values {
				if v == a[0].Float {
					return lmcode.Int(int64(i)), nil
				}
			}
			return lmcode.Int(-1), nil
		}},
		{"FloatList", nil, "FloatList", func(self any, _ []lmcode.Value) (lmcode.Value, error) {
			self.(*FloatList).values = self.(*FloatList).values[:0]
			return lmcode.Object("FloatList", self), nil
		}},
		{"StrMap", nil, "int", func(self any, _ []lmcode.Value) (lmcode.Value, error) {
			return lmcode.Int(int64(len(self.(*StrMap).keys))), nil
		}},
		{"StrMap", []string{"string"}, "bool", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			_, ok := self.(*StrMap).values[a[0].Str]
			return lmcode.Bool(ok), nil
		}},
		{"StrMap", []string{"string"}, "string", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			m := self.(*StrMap)
			v, ok := m.values[a[0].Str]
			if !ok {
				return lmcode.None(), lmcode.NewHostError("StrMap.get: missing key")
			}
			return lmcode.Str(v), nil
		}},
		{"StrMap", []string{"string", "string"}, "string", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			if v, ok := self.(*StrMap).values[a[0].Str]; ok {
				return lmcode.Str(v), nil
			}
			return lmcode.Str(a[1].Str), nil
		}},
		{"StrMap", []string{"string", "string"}, "StrMap", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			m := self.(*StrMap)
			if _, ok := m.values[a[0].Str]; !ok {
				m.keys = append(m.keys, a[0].Str)
			}
			m.values[a[0].Str] = a[1].Str
			return lmcode.Object("StrMap", self), nil
		}},
		{"StrMap", []string{"int"}, "string", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			m := self.(*StrMap)
			i, err := collectionIndex("StrMap", "key_at", a[0].Int, len(m.keys))
			if err != nil {
				return lmcode.None(), err
			}
			return lmcode.Str(m.keys[i]), nil
		}},
		{"StrMap", []string{"string"}, "bool", func(self any, a []lmcode.Value) (lmcode.Value, error) {
			m := self.(*StrMap)
			if _, ok := m.values[a[0].Str]; !ok {
				return lmcode.Bool(false), nil
			}
			delete(m.values, a[0].Str)
			for i, k := range m.keys {
				if k == a[0].Str {
					m.keys = append(m.keys[:i], m.keys[i+1:]...)
					break
				}
			}
			return lmcode.Bool(true), nil
		}},
		{"StrMap", nil, "StrMap", func(self any, _ []lmcode.Value) (lmcode.Value, error) {
			m := self.(*StrMap)
			clear(m.values)
			m.keys = m.keys[:0]
			return lmcode.Object("StrMap", self), nil
		}},
	}
	for i, e := range methods {
		if err := r.Method(e.typeName, methodNames[i], e.params, e.returns, e.fn); err != nil {
			return err
		}
	}
	return nil
}
