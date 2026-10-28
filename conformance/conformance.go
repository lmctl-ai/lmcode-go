package conformance

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/lmctl-ai/lmcode-go"
	"github.com/lmctl-ai/lmcode-go/stdlib"
)

// Thing is a host object used in conformance tests.
type Thing struct {
	ID int64
}

// IntList is a host collection object used in conformance tests.
type IntList struct {
	Values []int64
}

// Stub implements the host stub described in conformance/vectors/manifest.json.
type Stub struct {
	Trace    []string
	Root     *Thing
	Variable *Thing
	Xs       *IntList
	Reg      *lmcode.Registry
	Output   bytes.Buffer
	TempDir  string
}

// NewStub creates a new Stub with the standard conformance methods and functions registered.
func NewStub(stdMode ...bool) *Stub {
	useStd := len(stdMode) > 0 && stdMode[0]
	s := &Stub{
		Root:     &Thing{ID: 0},
		Variable: &Thing{ID: 0},
		Xs:       &IntList{Values: []int64{0, 1, 2, 3}},
		Reg: func() *lmcode.Registry {
			if useStd {
				return lmcode.NewRegistry()
			}
			return lmcode.NewRegistryWithoutStd()
		}(),
	}
	if useStd {
		dir, err := os.MkdirTemp("", "lmcode-conformance-")
		if err != nil {
			panic(err)
		}
		s.TempDir = dir
	}
	s.initRegistry(useStd)
	return s
}

func (s *Stub) initRegistry(useStd bool) {
	// The catalog and fixture adapters are ordinary host callbacks, shared by both vector suites.
	if useStd {
		if len(s.Reg.StdSignatures()) != 181 {
			panic(fmt.Sprintf("default std signature count = %d, want 181", len(s.Reg.StdSignatures())))
		}
		if err := stdlib.RegisterTesting(s.Reg, &s.Output); err != nil {
			panic(err)
		}
		if err := s.Reg.SetStdOutput(&s.Output); err != nil {
			panic(err)
		}
		_ = s.Reg.Function("printed", nil, "string", func([]lmcode.Value) (lmcode.Value, error) { return lmcode.String(s.Output.String()), nil })
		_ = s.Reg.Function("test_temp_dir", nil, "string", func([]lmcode.Value) (lmcode.Value, error) { return lmcode.String(s.TempDir), nil })
		_ = s.Reg.Function("test_long_path", nil, "string", func([]lmcode.Value) (lmcode.Value, error) {
			parts := []string{s.TempDir}
			for i := 0; i < 12; i++ {
				parts = append(parts, "segment-0123456789abcdef")
			}
			dir := filepath.Join(parts...)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return lmcode.None(), lmcode.NewHostError(err.Error())
			}
			return lmcode.String(filepath.Join(dir, "résumé-文件-😀.bin")), nil
		})
	} else {
		if err := stdlib.RegisterString(s.Reg); err != nil {
			panic(err)
		}
		if err := stdlib.RegisterMath(s.Reg); err != nil {
			panic(err)
		}
	}
	_ = s.Reg.Function("bytes", []string{"string"}, "string", func(args []lmcode.Value) (lmcode.Value, error) {
		raw, err := fixtureBytes(args[0].Str)
		if err != nil {
			return lmcode.None(), err
		}
		return lmcode.String(raw), nil
	})
	_ = s.Reg.Method("Trace", "hex", []string{"string"}, "void", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		s.Trace = append(s.Trace, "hex "+hex.EncodeToString([]byte(args[0].Str)))
		return lmcode.None(), nil
	})
	_ = s.Reg.Context("t", "Trace")
	_ = s.Reg.Context("root", "Thing")
	_ = s.Reg.Context("v", "Var")
	if !useStd {
		_ = s.Reg.Context("xs", "IntList")
	}

	if !useStd {
		_ = s.Reg.Method("IntList", "len", nil, "int", func(self any, args []lmcode.Value) (lmcode.Value, error) {
			return lmcode.Int(int64(len(self.(*IntList).Values))), nil
		})
		_ = s.Reg.Method("IntList", "at", []string{"int"}, "int", func(self any, args []lmcode.Value) (lmcode.Value, error) {
			idx := args[0].Int
			list := self.(*IntList).Values
			if idx < 0 || int(idx) >= len(list) {
				return lmcode.None(), lmcode.NewHostError("IntList index out of range")
			}
			return lmcode.Int(list[idx]), nil
		})
	}

	_ = s.Reg.Method("Trace", "int", []string{"int"}, "void", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		s.Trace = append(s.Trace, fmt.Sprintf("int %d", args[0].Int))
		return lmcode.None(), nil
	})
	_ = s.Reg.Method("Trace", "float", []string{"float"}, "void", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		s.Trace = append(s.Trace, fmt.Sprintf("float %s", lmcode.FormatG15(args[0].Float)))
		return lmcode.None(), nil
	})
	_ = s.Reg.Method("Trace", "str", []string{"string"}, "void", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		s.Trace = append(s.Trace, fmt.Sprintf("str %s", args[0].Str))
		return lmcode.None(), nil
	})
	_ = s.Reg.Method("Trace", "bool", []string{"bool"}, "void", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		bStr := "false"
		if args[0].Bool {
			bStr = "true"
		}
		s.Trace = append(s.Trace, fmt.Sprintf("bool %s", bStr))
		return lmcode.None(), nil
	})
	_ = s.Reg.Method("Trace", "thing", []string{"Thing"}, "void", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		s.Trace = append(s.Trace, fmt.Sprintf("thing %d", args[0].Obj.(*Thing).ID))
		return lmcode.None(), nil
	})
	_ = s.Reg.Method("Thing", "len", nil, "int", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		n := self.(*Thing).ID
		s.Trace = append(s.Trace, fmt.Sprintf("len %d", n))
		return lmcode.Int(n), nil
	})
	_ = s.Reg.Method("Thing", "at", []string{"int"}, "Thing", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		id := args[0].Int
		s.Trace = append(s.Trace, fmt.Sprintf("at %d", id))
		return lmcode.Object("Thing", &Thing{ID: id}), nil
	})
	_ = s.Reg.Method("Thing", "id", nil, "int", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Int(self.(*Thing).ID), nil
	})
	_ = s.Reg.Method("Thing", "next", nil, "Thing", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Object("Thing", &Thing{ID: self.(*Thing).ID + 1}), nil
	})
	_ = s.Reg.Method("Thing", "name", nil, "string", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.String(fmt.Sprintf("thing-%d", self.(*Thing).ID)), nil
	})
	_ = s.Reg.Method("Thing", "scaled", []string{"float"}, "float", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Float(float64(self.(*Thing).ID) * args[0].Float), nil
	})
	_ = s.Reg.Method("Trace", "int", []string{"float"}, "void", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		s.Trace = append(s.Trace, fmt.Sprintf("float %s", lmcode.FormatG15(args[0].Float)))
		return lmcode.None(), nil
	})
	_ = s.Reg.Method("Trace", "status", nil, "Status", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.String("OPTIMAL"), nil
	})
	_ = s.Reg.Method("Thing", "plus", []string{"Thing"}, "Thing", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Object("Thing", &Thing{ID: self.(*Thing).ID + args[0].Obj.(*Thing).ID}), nil
	})
	_ = s.Reg.Method("Thing", "minus", []string{"Thing"}, "Thing", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Object("Thing", &Thing{ID: self.(*Thing).ID - args[0].Obj.(*Thing).ID}), nil
	})
	_ = s.Reg.Method("Thing", "times", []string{"int"}, "Thing", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Object("Thing", &Thing{ID: self.(*Thing).ID * args[0].Int}), nil
	})
	_ = s.Reg.Method("Thing", "neg", nil, "Thing", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Object("Thing", &Thing{ID: -self.(*Thing).ID}), nil
	})
	_ = s.Reg.Method("Thing", "rminus", []string{"int"}, "Thing", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Object("Thing", &Thing{ID: args[0].Int - self.(*Thing).ID}), nil
	})
	_ = s.Reg.Method("Thing", "set", []string{"int"}, "Thing", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		self.(*Thing).ID = args[0].Int
		return lmcode.Object("Thing", self), nil
	})
	_ = s.Reg.Method("Var", "id", nil, "int", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Int(self.(*Thing).ID), nil
	})
	_ = s.Reg.Method("Var", "expr", nil, "Thing", func(self any, args []lmcode.Value) (lmcode.Value, error) {
		n := self.(*Thing).ID
		s.Trace = append(s.Trace, fmt.Sprintf("expr %d", n))
		return lmcode.Object("Thing", &Thing{ID: n}), nil
	})

	_ = s.Reg.Function("var", []string{"int"}, "Var", func(args []lmcode.Value) (lmcode.Value, error) {
		id := args[0].Int
		s.Trace = append(s.Trace, fmt.Sprintf("var %d", id))
		return lmcode.Object("Var", &Thing{ID: id}), nil
	})
	_ = s.Reg.Function("twice", []string{"float"}, "float", func(args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Float(args[0].Float * 2.0), nil
	})
	_ = s.Reg.Function("thing", []string{"int"}, "Thing", func(args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Object("Thing", &Thing{ID: args[0].Int}), nil
	})
	_ = s.Reg.Function("twice", []string{"int"}, "int", func(args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Int(args[0].Int * 2), nil
	})
	_ = s.Reg.Function("bound", []string{"int"}, "int", func(args []lmcode.Value) (lmcode.Value, error) {
		v := args[0].Int
		s.Trace = append(s.Trace, fmt.Sprintf("bound %d", v))
		return lmcode.Int(v), nil
	})
	_ = s.Reg.Function("half", []string{"float"}, "float", func(args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.Float(args[0].Float / 2.0), nil
	})
	_ = s.Reg.Function("yes", nil, "bool", func(args []lmcode.Value) (lmcode.Value, error) {
		s.Trace = append(s.Trace, "yes")
		return lmcode.Bool(true), nil
	})
	_ = s.Reg.Function("fail", []string{"string"}, "void", func(args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.None(), lmcode.NewHostError(args[0].Str)
	})
	_ = s.Reg.Function("bad_int", nil, "int", func(args []lmcode.Value) (lmcode.Value, error) {
		return lmcode.String("x"), nil
	})
}

func (s *Stub) Cleanup() {
	if s.TempDir != "" {
		_ = os.RemoveAll(s.TempDir)
		s.TempDir = ""
	}
}

// fixtureBytes accepts only lowercase hex pairs, matching the language-neutral fixture host.
func fixtureBytes(encoded string) (string, error) {
	for i := 0; i < len(encoded); i++ {
		c := encoded[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", lmcode.NewHostError("bytes: expected lowercase hexadecimal pairs")
		}
	}
	raw, err := hex.DecodeString(encoded)
	if err != nil {
		return "", lmcode.NewHostError("bytes: expected lowercase hexadecimal pairs")
	}
	return string(raw), nil
}

// TraceLineMatches checks whether an actual trace line matches the expected line,
// honoring '~ ' tolerance prefix for floating point numbers.
func TraceLineMatches(actual, expected string) bool {
	if !strings.HasPrefix(expected, "~ ") {
		return actual == expected
	}
	body := expected[2:]
	split := strings.LastIndexByte(body, ' ')
	if split == -1 || split+1 == len(body) {
		return false
	}
	prefix := body[:split+1]
	if !strings.HasPrefix(actual, prefix) {
		return false
	}
	expectedVal, err1 := strconv.ParseFloat(body[split+1:], 64)
	actualVal, err2 := strconv.ParseFloat(actual[len(prefix):], 64)
	if err1 != nil || err2 != nil {
		return false
	}
	if math.IsNaN(expectedVal) || math.IsInf(expectedVal, 0) || math.IsNaN(actualVal) || math.IsInf(actualVal, 0) {
		return false
	}
	tolerance := 1e-12 * math.Max(math.Abs(expectedVal), 1e-300)
	return math.Abs(actualVal-expectedVal) <= tolerance
}

func equalStrings(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range actual {
		if actual[i] != expected[i] {
			return false
		}
	}
	return true
}
func tracesMatch(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range actual {
		if !TraceLineMatches(actual[i], expected[i]) {
			return false
		}
	}
	return true
}

// Expectation describes expected output parsed from an .expect file.
type Expectation struct {
	Kind  string
	Lines []string
}

func entryArg(raw json.RawMessage, typ string) (lmcode.Value, error) {
	var v any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&v); err != nil {
		return lmcode.None(), err
	}
	switch x := v.(type) {
	case bool:
		return lmcode.Bool(x), nil
	case string:
		return lmcode.String(x), nil
	case json.Number:
		if typ == "float" {
			n, err := x.Float64()
			return lmcode.Float(n), err
		}
		n, err := x.Int64()
		return lmcode.Int(n), err
	case map[string]any:
		if id, ok := x["id"].(json.Number); ok && x["type"] == "Thing" {
			n, err := id.Int64()
			return lmcode.Object("Thing", &Thing{ID: n}), err
		}
	}
	return lmcode.None(), fmt.Errorf("unsupported entry argument for %s", typ)
}

func valueText(v lmcode.Value) string {
	switch v.Kind {
	case lmcode.KindInt:
		return fmt.Sprintf("int %d", v.Int)
	case lmcode.KindFloat:
		return "float " + lmcode.FormatG15(v.Float)
	case lmcode.KindStr:
		return "string " + v.Str
	case lmcode.KindBool:
		if v.Bool {
			return "bool true"
		}
		return "bool false"
	case lmcode.KindObj:
		if t, ok := v.Obj.(*Thing); ok {
			return fmt.Sprintf("thing %d", t.ID)
		}
		return "thing ?"
	default:
		return "void"
	}
}

func runEntryVector(p *lmcode.Program, stub *Stub, entryName, argsJSON string) (string, []string, error) {
	idx := -1
	var entry lmcode.Entry
	for i, e := range p.Entries() {
		if e.Name == entryName {
			idx, entry = i, e
			break
		}
	}
	if idx < 0 {
		return "", nil, fmt.Errorf("entry %q is not present", entryName)
	}
	var raws []json.RawMessage
	if err := json.Unmarshal([]byte(argsJSON), &raws); err != nil {
		return "", nil, err
	}
	ctx := lmcode.NewContext()
	ctx.Set("t", stub)
	ctx.Set("root", stub.Root)
	ctx.Set("v", stub.Variable)
	if stub.Xs != nil {
		ctx.Set("xs", stub.Xs)
	}
	if err := p.Run(ctx, stub.Reg); err != nil {
		return "", nil, err
	}
	args := make([]lmcode.Value, len(raws))
	for i, raw := range raws {
		var err error
		typ := ""
		if i < len(entry.Params) {
			typ = entry.Params[i]
		}
		args[i], err = entryArg(raw, typ)
		if err != nil {
			return "", nil, err
		}
	}
	result, err := p.Call(idx, ctx, stub.Reg, args)
	if err != nil {
		return "", nil, err
	}
	return valueText(result), append([]string(nil), stub.Trace...), nil
}

// ReadExpect reads and unescapes an .expect file.
func ReadExpect(path string) (*Expectation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rawLines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(rawLines) > 0 && rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}
	if len(rawLines) == 0 {
		return &Expectation{}, nil
	}
	kind := rawLines[0]
	var lines []string
	for _, l := range rawLines[1:] {
		lines = append(lines, strings.ReplaceAll(l, "\\n", "\n"))
	}
	return &Expectation{Kind: kind, Lines: lines}, nil
}

// SchemaFor finds the schema path for a vector from manifest.json, or defaults to schema.json.
func SchemaFor(dir, name string) string {
	manifestPath := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err == nil {
		var manifest struct {
			Vectors []struct {
				Name   string `json:"name"`
				Schema string `json:"schema"`
			} `json:"vectors"`
		}
		if err := json.Unmarshal(data, &manifest); err == nil {
			for _, item := range manifest.Vectors {
				if item.Name == name && item.Schema != "" {
					return filepath.Join(dir, item.Schema)
				}
			}
		}
	}
	return filepath.Join(dir, "schema.json")
}

// VectorNames returns sorted names of all vectors in dir.
func VectorNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasSuffix(n, ".expect") {
			names = append(names, strings.TrimSuffix(n, ".expect"))
		}
	}
	sort.Strings(names)
	return names, nil
}

// CheckVector runs one conformance vector and returns an error description if it fails.
func CheckVector(dir, name string) (string, error) {
	expect, err := ReadExpect(filepath.Join(dir, name+".expect"))
	if err != nil {
		return "", err
	}
	bytecode, err := os.ReadFile(filepath.Join(dir, name+".dslb"))
	if err != nil {
		return "", err
	}
	schema, err := os.ReadFile(SchemaFor(dir, name))
	if err != nil {
		return "", err
	}

	stdMode := bytes.Contains(schema, []byte(`"str_len"`)) || bytes.Contains(schema, []byte(`"math_abs"`)) || bytes.Contains(schema, []byte(`"str_list"`)) || bytes.Contains(schema, []byte(`"file_read"`)) || bytes.Contains(schema, []byte(`"print"`))
	stub := NewStub(stdMode)
	defer stub.Cleanup()
	var runErr error
	var loadErr error
	loaded := false
	ran := false
	var entryLines []string

	p, err := lmcode.Load(bytecode, schema, stub.Reg)
	if err != nil {
		loadErr = err
	} else {
		loaded = true
		if expect.Kind == "entries" {
			for _, e := range p.Entries() {
				entryLines = append(entryLines, fmt.Sprintf("%s %s %s", e.Name, e.Annotation, e.Iface))
			}
			ran = true
		} else if strings.HasPrefix(expect.Kind, "call") {
			if len(expect.Lines) == 0 {
				return "", fmt.Errorf("missing call selector")
			}
			selector := expect.Lines[0]
			fields := strings.SplitN(selector, " ", 2)
			argsJSON := "[]"
			if len(fields) == 2 {
				argsJSON = fields[1]
			}
			result, trace, err := runEntryVector(p, stub, fields[0], argsJSON)
			if err != nil {
				runErr = err
			} else {
				entryLines = append([]string{"result " + result}, trace...)
				ran = true
			}
		} else {
			ctx := lmcode.NewContext()
			ctx.Set("t", stub)
			ctx.Set("root", stub.Root)
			ctx.Set("v", stub.Variable)
			if !stdMode {
				ctx.Set("xs", stub.Xs)
			}
			runErr = p.Run(ctx, stub.Reg)
			if runErr == nil {
				ran = true
			}
		}
	}

	if os.Getenv("LMCODE_TRACE") != "" {
		for _, line := range stub.Trace {
			fmt.Printf("TRACE\t%s\t%s\n", name, line)
		}
		if e, yes := runErr.(*lmcode.Error); yes {
			message := e.Message
			if e.HasColumn {
				message = strings.TrimPrefix(message, fmt.Sprintf("dsl line %d:%d: ", e.Line, e.Column))
			} else if e.HasLocation {
				message = strings.TrimPrefix(message, fmt.Sprintf("dsl line %d: ", e.Line))
			}
			fmt.Printf("ERROR\t%s\t%s\t%s\n", name, e.Code, message)
		}
	}
	ok := false
	switch expect.Kind {
	case "trace":
		if ran && len(stub.Trace) == len(expect.Lines) {
			ok = true
			for i := range expect.Lines {
				if !TraceLineMatches(stub.Trace[i], expect.Lines[i]) {
					ok = false
					break
				}
			}
		}
	case "entries":
		ok = ran && len(entryLines) == len(expect.Lines) && equalStrings(entryLines, expect.Lines)
	case "call":
		ok = ran && len(expect.Lines) >= 2 && len(entryLines) == len(expect.Lines)-1 && tracesMatch(entryLines, expect.Lines[1:])
	case "call_error":
		if loaded && !ran && runErr != nil && len(expect.Lines) == 3 {
			if e, yes := runErr.(*lmcode.Error); yes {
				ok = e.Code == expect.Lines[1] && strings.Contains(e.Message, expect.Lines[2])
			}
		}
	case "call_error_at":
		if loaded && !ran && runErr != nil && len(expect.Lines) >= 5 {
			if e, yes := runErr.(*lmcode.Error); yes {
				line, _ := strconv.ParseUint(expect.Lines[2], 10, 32)
				col, _ := strconv.ParseUint(expect.Lines[3], 10, 16)
				ok = e.Code == expect.Lines[1] && e.HasLocation == (line != 0) && (!e.HasLocation || e.Line == uint32(line)) && e.HasColumn == (col != 0) && (!e.HasColumn || e.Column == uint16(col)) && strings.Contains(e.Message, expect.Lines[4])
			}
		}
	case "runtime_error":
		if loaded && !ran && runErr != nil && len(expect.Lines) > 0 {
			ok = strings.Contains(runErr.Error(), expect.Lines[0])
		}
	case "runtime_error_file":
		if loaded && !ran && runErr != nil && len(expect.Lines) >= 3 {
			if lmErr, isLm := runErr.(*lmcode.Error); isLm && lmErr.Code == expect.Lines[0] {
				message := lmErr.Message
				if lmErr.HasColumn {
					message = strings.TrimPrefix(message, fmt.Sprintf("dsl line %d:%d: ", lmErr.Line, lmErr.Column))
				} else if lmErr.HasLocation {
					message = strings.TrimPrefix(message, fmt.Sprintf("dsl line %d: ", lmErr.Line))
				}
				ok = strings.Contains(message, expect.Lines[1]+": ") && strings.Contains(message, expect.Lines[2])
				if ok {
					ok = strings.TrimSpace(message[strings.LastIndexByte(message, ':')+1:]) != ""
				}
			}
		}
	case "runtime_error_at":
		if loaded && !ran && runErr != nil && len(expect.Lines) >= 4 {
			var code string
			var hasLoc, hasCol bool
			var line uint32
			var col uint16
			var msg string
			if lmErr, isLm := runErr.(*lmcode.Error); isLm {
				code = lmErr.Code
				hasLoc = lmErr.HasLocation
				hasCol = lmErr.HasColumn
				line = lmErr.Line
				col = lmErr.Column
				msg = lmErr.Message
			}

			var expectedLine uint32
			var expectedCol uint16
			if l, err := strconv.ParseUint(expect.Lines[1], 10, 32); err == nil {
				expectedLine = uint32(l)
			}
			if c, err := strconv.ParseUint(expect.Lines[2], 10, 16); err == nil {
				expectedCol = uint16(c)
			}

			codeMatch := code == expect.Lines[0]
			lineMatch := (expectedLine == 0 && !hasLoc) || (expectedLine != 0 && hasLoc && line == expectedLine)
			colMatch := (expectedCol == 0 && !hasCol) || (expectedCol != 0 && hasCol && col == expectedCol)
			msgMatch := strings.Contains(msg, expect.Lines[3])

			ok = codeMatch && lineMatch && colMatch && msgMatch
		}
	case "runtime_error_exact":
		if loaded && !ran && runErr != nil && len(expect.Lines) >= 2 {
			if lmErr, isLm := runErr.(*lmcode.Error); isLm {
				msg := lmErr.Message
				if lmErr.HasColumn {
					msg = strings.TrimPrefix(msg, fmt.Sprintf("dsl line %d:%d: ", lmErr.Line, lmErr.Column))
				} else if lmErr.HasLocation {
					msg = strings.TrimPrefix(msg, fmt.Sprintf("dsl line %d: ", lmErr.Line))
				}
				ok = lmErr.Code == expect.Lines[0] && msg == expect.Lines[1]
			}
		}
	case "load_error":
		if !loaded && loadErr != nil && len(expect.Lines) > 0 {
			ok = strings.Contains(loadErr.Error(), expect.Lines[0])
		}
	default:
		return "", fmt.Errorf("unknown expectation kind '%s'", expect.Kind)
	}

	if ok {
		return expect.Kind, nil
	}

	errMsg := ""
	if loadErr != nil {
		errMsg = "load_error: " + loadErr.Error()
	} else if runErr != nil {
		errMsg = "run_error: " + runErr.Error()
	}
	return "", fmt.Errorf("%s: expected %s %v, got err=%s, trace=%v", name, expect.Kind, expect.Lines, errMsg, stub.Trace)
}
