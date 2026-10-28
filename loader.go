package lmcode

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"strings"
)

// Format constants.
const (
	Magic          = "DSLB"
	Version uint16 = 6
)

// Local represents a declared local variable slot in the bytecode.
type Local struct {
	Type   string
	Hidden bool
}

// Line records a line table mapping from bytecode PC to source line and column.
type Line struct {
	PC     uint32
	Line   uint32
	Column uint16
}

type importEntry struct {
	fn         *FunctionEntry
	method     *MethodEntry
	typeName   string
	methodName string
}

type loopInfo struct {
	counter uint16
	end     uint16
	next    int
}

// Entry describes a host-callable rule, in source order.
type Entry struct {
	Name, Annotation, Iface string
	Params                  []string
	Returns                 string
	region                  region
}
type region struct {
	locals   []Local
	maxStack uint16
	code     []byte
	lines    []Line
	loops    map[int]loopInfo
}

// Program shares pools and a bound registry across independent code regions.
type Program struct {
	consts  []string
	imports []importEntry
	context []string
	region
	entries  []Entry
	registry *Registry
}

// Entries returns metadata copies; modifying them cannot change a loaded program.
func (p *Program) Entries() []Entry {
	out := append([]Entry(nil), p.entries...)
	for i := range out {
		out[i].Params = append([]string(nil), out[i].Params...)
	}
	return out
}

// ConstantCount returns the number of constant pool entries.
func (p *Program) ConstantCount() int { return len(p.consts) }

// CodeSize returns the number of bytecode bytes.
func (p *Program) CodeSize() int { return len(p.code) }

// ImportCount returns the number of import table entries.
func (p *Program) ImportCount() int { return len(p.imports) }

// LocalCount returns the number of declared local slots.
func (p *Program) LocalCount() int { return len(p.locals) }

// Locals returns the declared locals table.
func (p *Program) Locals() []Local { return p.locals }

// MaxStack returns the compiler's measured operand stack depth hint.
func (p *Program) MaxStack() uint16 { return p.maxStack }

// Lines returns the source line table entries.
func (p *Program) Lines() []Line { return p.lines }

// LineOf returns the source position recorded for pc, or nil if none.
func (p *Program) LineOf(pc int) *Line {
	var best *Line
	for i := range p.lines {
		if int(p.lines[i].PC) <= pc {
			best = &p.lines[i]
		} else {
			break
		}
	}
	return best
}

// ReadFile reads a file completely into a byte slice.
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, NewError(LOAD_BAD_BYTECODE, "cannot read "+path)
	}
	return data, nil
}

// SHA256 returns the 32-byte SHA-256 digest of data.
func SHA256(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// Hex returns the hexadecimal string encoding of data.
func Hex(data []byte) string {
	return hex.EncodeToString(data)
}

type reader struct {
	b   []byte
	pos int
}

func newReader(b []byte) *reader {
	return &reader{b: b, pos: 0}
}

func (r *reader) need(n int) error {
	if r.pos+n > len(r.b) {
		return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("bytecode is truncated: wanted %d byte(s) at offset %d, file has %d", n, r.pos, len(r.b)))
	}
	return nil
}

func (r *reader) u8() (byte, error) {
	if err := r.need(1); err != nil {
		return 0, err
	}
	v := r.b[r.pos]
	r.pos++
	return v, nil
}

func (r *reader) u16() (uint16, error) {
	if err := r.need(2); err != nil {
		return 0, err
	}
	v := binary.BigEndian.Uint16(r.b[r.pos : r.pos+2])
	r.pos += 2
	return v, nil
}

func (r *reader) u32() (uint32, error) {
	if err := r.need(4); err != nil {
		return 0, err
	}
	v := binary.BigEndian.Uint32(r.b[r.pos : r.pos+4])
	r.pos += 4
	return v, nil
}

func (r *reader) i32() (int32, error) {
	v, err := r.u32()
	return int32(v), err
}

func (r *reader) i64() (int64, error) {
	if err := r.need(8); err != nil {
		return 0, err
	}
	v := binary.BigEndian.Uint64(r.b[r.pos : r.pos+8])
	r.pos += 8
	return int64(v), nil
}

func (r *reader) f64() (float64, error) {
	if err := r.need(8); err != nil {
		return 0, err
	}
	bits := binary.BigEndian.Uint64(r.b[r.pos : r.pos+8])
	r.pos += 8
	return math.Float64frombits(bits), nil
}

func (r *reader) bytes(n int) ([]byte, error) {
	if err := r.need(n); err != nil {
		return nil, err
	}
	s := r.b[r.pos : r.pos+n]
	r.pos += n
	return s, nil
}

func (r *reader) done() bool {
	return r.pos == len(r.b)
}

// Load parses, binds and verifies bytecode against schemaBytes and the registry.
func Load(bytecode, schemaBytes []byte, reg *Registry) (*Program, error) {
	return LoadProgram(bytecode, schemaBytes, reg)
}

// LoadProgram parses, binds and verifies bytecode against schemaBytes and the registry.
func LoadProgram(bytecode, schemaBytes []byte, reg *Registry) (*Program, error) {
	if len(bytecode) < 4 || string(bytecode[:4]) != Magic {
		return nil, NewError(LOAD_BAD_BYTECODE, "not a DSL bytecode file: bad magic (expected 'DSLB')")
	}

	r := newReader(bytecode)
	if _, err := r.bytes(4); err != nil {
		return nil, err
	}

	version, err := r.u16()
	if err != nil {
		return nil, err
	}
	if version != Version {
		return nil, NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("unsupported bytecode version %d (this runtime reads version %d)", version, Version))
	}

	hash, err := r.bytes(32)
	if err != nil {
		return nil, err
	}
	expectedHash := sha256.Sum256(schemaBytes)
	if !bytes.Equal(hash, expectedHash[:]) {
		return nil, NewError(LOAD_SCHEMA_MISMATCH, fmt.Sprintf("schema mismatch: bytecode was compiled against schema sha256 %s, the given schema has sha256 %s", hex.EncodeToString(hash), hex.EncodeToString(expectedHash[:])))
	}

	nconst, err := r.u32()
	if err != nil {
		return nil, err
	}

	p := &Program{registry: reg}

	for i := uint32(0); i < nconst; i++ {
		tag, err := r.u8()
		if err != nil {
			return nil, err
		}
		if tag != 1 {
			return nil, NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("constant %d has unknown tag %d (1 = string)", i, tag))
		}
		length, err := r.u32()
		if err != nil {
			return nil, err
		}
		strBytes, err := r.bytes(int(length))
		if err != nil {
			return nil, err
		}
		p.consts = append(p.consts, string(strBytes))
	}

	cidx := func(i uint32, what string) (string, error) {
		if int(i) >= len(p.consts) {
			return "", NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("%s: constant index %d out of range (%d constants)", what, i, len(p.consts)))
		}
		return p.consts[i], nil
	}

	nimports, err := r.u16()
	if err != nil {
		return nil, err
	}
	for i := uint16(0); i < nimports; i++ {
		cIndex, err := r.u32()
		if err != nil {
			return nil, err
		}
		canonical, err := cidx(cIndex, "import")
		if err != nil {
			return nil, err
		}

		var imp importEntry
		if strings.HasPrefix(canonical, "fn ") {
			imp.fn = reg.FunctionBySignature(canonical)
		} else if strings.HasPrefix(canonical, "method ") {
			imp.method = reg.MethodBySignature(canonical)
			dot := strings.IndexByte(canonical, '.')
			par := strings.IndexByte(canonical, '(')
			if dot != -1 && par != -1 && dot < par {
				imp.typeName = canonical[7:dot]
				imp.methodName = canonical[dot+1 : par]
			}
		}

		if imp.fn == nil && imp.method == nil {
			have := reg.DeclaredFor(canonical)
			suffix := ""
			if have != "" {
				suffix = " (the host declares " + have + ")"
			}
			return nil, NewError(LOAD_UNBOUND, fmt.Sprintf("import '%s' is not provided by the host%s", canonical, suffix))
		}
		p.imports = append(p.imports, imp)
	}

	ncontext, err := r.u16()
	if err != nil {
		return nil, err
	}
	for i := uint16(0); i < ncontext; i++ {
		nameIdx, err := r.u32()
		if err != nil {
			return nil, err
		}
		typeIdx, err := r.u32()
		if err != nil {
			return nil, err
		}
		name, err := cidx(nameIdx, "context name")
		if err != nil {
			return nil, err
		}
		typeName, err := cidx(typeIdx, "context type")
		if err != nil {
			return nil, err
		}

		declared, ok := reg.ContextType(name)
		if !ok {
			return nil, NewError(LOAD_UNBOUND, fmt.Sprintf("context variable '%s' is not declared by the host", name))
		}
		if declared != typeName {
			return nil, NewError(LOAD_UNBOUND, fmt.Sprintf("context variable '%s' is declared as %s by the bytecode, the host provides %s", name, typeName, declared))
		}
		p.context = append(p.context, name)
	}

	readRegion := func(r *reader) (region, error) {
		g := region{loops: make(map[int]loopInfo)}
		nlocals, err := r.u16()
		if err != nil {
			return g, err
		}
		for i := uint16(0); i < nlocals; i++ {
			typeIdx, err := r.u32()
			if err != nil {
				return g, err
			}
			typeName, err := cidx(typeIdx, "local type")
			if err != nil {
				return g, err
			}
			hidden, err := r.u8()
			if err != nil {
				return g, err
			}
			if hidden > 1 {
				return g, NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("local %d hidden flag must be 0 or 1", i))
			}
			g.locals = append(g.locals, Local{Type: typeName, Hidden: hidden == 1})
		}

		g.maxStack, err = r.u16()
		if err != nil {
			return g, err
		}

		codeLen, err := r.u32()
		if err != nil {
			return g, err
		}
		codeBytes, err := r.bytes(int(codeLen))
		if err != nil {
			return g, err
		}
		g.code = codeBytes

		nlines, err := r.u32()
		if err != nil {
			return g, err
		}
		for i := uint32(0); i < nlines; i++ {
			pc, err := r.u32()
			if err != nil {
				return g, err
			}
			line, err := r.u32()
			if err != nil {
				return g, err
			}
			col, err := r.u16()
			if err != nil {
				return g, err
			}
			if i > 0 && pc < g.lines[i-1].PC {
				return g, NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("line table is not sorted by pc at entry %d", i))
			}
			g.lines = append(g.lines, Line{PC: pc, Line: line, Column: col})
		}

		return g, nil
	}
	p.region, err = readRegion(r)
	if err != nil {
		return nil, err
	}
	nentries, err := r.u16()
	if err != nil {
		return nil, err
	}
	for i := uint16(0); i < nentries; i++ {
		nameIndex, e := r.u32()
		if e != nil {
			return nil, e
		}
		name, e := cidx(nameIndex, "entry name")
		if e != nil {
			return nil, e
		}
		readEntry := func() (Entry, error) {
			entry := Entry{Name: name}
			annotationIndex, e := r.u32()
			if e != nil {
				return entry, e
			}
			entry.Annotation, e = cidx(annotationIndex, "entry annotation")
			if e != nil {
				return entry, e
			}
			ifaceIndex, e := r.u32()
			if e != nil {
				return entry, e
			}
			entry.Iface, e = cidx(ifaceIndex, "entry interface")
			if e != nil {
				return entry, e
			}
			var ok bool
			entry.Params, entry.Returns, ok = parseInterface(entry.Iface)
			if !ok {
				return entry, NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("interface '%s' is not an interface signature", entry.Iface))
			}
			nparams, e := r.u16()
			if e != nil {
				return entry, e
			}
			entry.region, e = readRegion(r)
			if e != nil {
				return entry, e
			}
			if int(nparams) != len(entry.Params) {
				return entry, NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("declares %d parameter(s); %s has %d", nparams, entry.Iface, len(entry.Params)))
			}
			if int(nparams) > len(entry.region.locals) {
				return entry, NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("nparams %d exceeds nlocals %d", nparams, len(entry.region.locals)))
			}
			for j, ty := range entry.Params {
				local := entry.region.locals[j]
				if local.Hidden {
					return entry, NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("parameter %d is hidden", j+1))
				}
				if local.Type != ty {
					return entry, NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("parameter %d has type %s; the interface expects %s", j+1, local.Type, ty))
				}
			}
			return entry, nil
		}
		entry, e := readEntry()
		if e != nil {
			return nil, entryError(name, e)
		}
		view := *p
		view.region = entry.region
		if e = view.verify(&entry.Returns); e != nil {
			return nil, entryError(name, e)
		}
		entry.region = view.region
		p.entries = append(p.entries, entry)
	}
	if !r.done() {
		return nil, NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("bytecode has %d trailing byte(s) after the entries", len(bytecode)-r.pos))
	}
	if err := p.verify(nil); err != nil {
		return nil, err
	}

	return p, nil
}

type verifiedInstr struct {
	pc     int
	next   int
	target int
	op     byte
	depth  int
	owner  int
}

type openLoop struct {
	op      byte
	pc      int
	next    int
	exit    int
	counter uint16
	end     uint16
	tested  bool
	parent  int
	test    int
}

type loopRegion struct {
	exit    int
	parent  int
	test    int
	op      byte
	counter uint16
	end     uint16
}

type jumpEdge struct {
	from   int
	target int
}

func (p *Program) verify(returns *string) error {
	var insns []verifiedInstr
	var open []openLoop
	var ordinary []jumpEdge
	var whileExits []jumpEdge
	regions := make(map[int]loopRegion)
	var usedLoopSlots []uint16

	r := newReader(p.code)

	checkIndex := func(i, n int, what string) error {
		if i >= n {
			return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("%s index %d out of range (%d entries)", what, i, n))
		}
		return nil
	}

	checkLocalIntHidden := func(slot uint16, role string) error {
		if err := checkIndex(int(slot), len(p.locals), role); err != nil {
			return err
		}
		if p.locals[slot].Type != "int" || !p.locals[slot].Hidden {
			return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("%s slot %d must be a hidden int local", role, slot))
		}
		return nil
	}

	halted := false
	const noOwner = -1

	for !r.done() {
		pc := r.pos
		depth := len(open)
		owner := noOwner
		if len(open) > 0 {
			owner = open[len(open)-1].pc
		}

		op, err := r.u8()
		if err != nil {
			return err
		}
		insns = append(insns, verifiedInstr{
			pc:    pc,
			op:    op,
			depth: depth,
			owner: owner,
		})
		halted = false

		switch op {
		case OpPushInt:
			if _, err := r.i64(); err != nil {
				return err
			}
		case OpPushFloat:
			if _, err := r.f64(); err != nil {
				return err
			}
		case OpPushStr:
			idx, err := r.u32()
			if err != nil {
				return err
			}
			if int(idx) >= len(p.consts) {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("constant index %d out of range (%d constants)", idx, len(p.consts)))
			}
		case OpPushBool:
			v, err := r.u8()
			if err != nil {
				return err
			}
			if v > 1 {
				return NewError(LOAD_BAD_BYTECODE, "PUSH_BOOL operand must be 0 or 1")
			}
		case OpLoadLocal, OpStoreLocal:
			slot, err := r.u16()
			if err != nil {
				return err
			}
			if err := checkIndex(int(slot), len(p.locals), "local slot"); err != nil {
				return err
			}
		case OpLoadCtx:
			idx, err := r.u16()
			if err != nil {
				return err
			}
			if err := checkIndex(int(idx), len(p.context), "context"); err != nil {
				return err
			}
		case OpCallFn:
			idx, err := r.u16()
			if err != nil {
				return err
			}
			argc, err := r.u8()
			if err != nil {
				return err
			}
			if err := checkIndex(int(idx), len(p.imports), "import"); err != nil {
				return err
			}
			imp := p.imports[idx]
			if imp.fn == nil {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("CALL_FN at offset %d names import %d, which is a method", pc, idx))
			}
			if len(imp.fn.Sig.Params) != int(argc) {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("CALL_FN at offset %d passes %d argument(s), the import takes %d", pc, argc, len(imp.fn.Sig.Params)))
			}
		case OpCallMethod:
			idx, err := r.u16()
			if err != nil {
				return err
			}
			argc, err := r.u8()
			if err != nil {
				return err
			}
			if err := checkIndex(int(idx), len(p.imports), "import"); err != nil {
				return err
			}
			imp := p.imports[idx]
			if imp.method == nil {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("CALL_METHOD at offset %d names import %d, which is a function", pc, idx))
			}
			if len(imp.method.Sig.Params) != int(argc) {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("CALL_METHOD at offset %d passes %d argument(s), the import takes %d", pc, argc, len(imp.method.Sig.Params)))
			}
		case OpEq, OpNe, OpNot, OpDup, OpPop:
			// no operands
		case OpAddI, OpSubI, OpMulI, OpDivI, OpModI, OpAddF, OpSubF, OpMulF, OpDivF, OpNegI, OpNegF:
			// no operands
		case OpLtI, OpLeI, OpGtI, OpGeI, OpLtF, OpLeF, OpGtF, OpGeF, OpI2F, OpConcat, OpI2S, OpF2S, OpB2S:
			// no operands
		case OpMath:
			fn, err := r.u8()
			if err != nil {
				return err
			}
			if fn > MathClampI {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("unknown MATH function %d at offset %d", fn, pc))
			}
		case OpJump, OpJumpIfFalse, OpJumpIfTrue:
			rel, err := r.i32()
			if err != nil {
				return err
			}
			if rel < 0 {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("jump at offset %d goes backwards (only paired LOOP_BACK may jump backward)", pc))
			}
			target := r.pos + int(rel)
			insns[len(insns)-1].target = target
			ordinary = append(ordinary, jumpEdge{from: pc, target: target})
		case OpForRange:
			counter, err := r.u16()
			if err != nil {
				return err
			}
			end, err := r.u16()
			if err != nil {
				return err
			}
			rel, err := r.i32()
			if err != nil {
				return err
			}
			if err := checkLocalIntHidden(counter, "FOR_RANGE counter"); err != nil {
				return err
			}
			if err := checkLocalIntHidden(end, "FOR_RANGE end"); err != nil {
				return err
			}
			if counter == end {
				return NewError(LOAD_BAD_BYTECODE, "FOR_RANGE counter and end slots must be distinct")
			}
			for _, used := range usedLoopSlots {
				if counter == used || end == used {
					culprit := counter
					if end == used {
						culprit = end
					}
					return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("FOR_RANGE slot %d is already owned by another range loop", culprit))
				}
			}
			usedLoopSlots = append(usedLoopSlots, counter, end)
			if rel < 0 {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("FOR_RANGE at offset %d exits backwards", pc))
			}
			exit := r.pos + int(rel)
			open = append(open, openLoop{
				op:      OpForRange,
				pc:      pc,
				next:    r.pos,
				exit:    exit,
				counter: counter,
				end:     end,
				parent:  owner,
			})
			insns[len(insns)-1].target = exit
		case OpForNext:
			encodedEnd, err := r.u16()
			if err != nil {
				return err
			}
			rel, err := r.i32()
			if err != nil {
				return err
			}
			if len(open) == 0 || open[len(open)-1].op != OpForRange {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("FOR_NEXT at offset %d has no matching FOR_RANGE", pc))
			}
			loop := open[len(open)-1]
			open = open[:len(open)-1]
			if encodedEnd != loop.end {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("FOR_NEXT at offset %d names end slot %d, expected %d", pc, encodedEnd, loop.end))
			}
			if r.pos+int(rel) != loop.next {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("FOR_NEXT at offset %d must return to its FOR_RANGE body", pc))
			}
			if loop.exit != r.pos {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("FOR_RANGE at offset %d must exit immediately after its FOR_NEXT", loop.pc))
			}
			p.loops[pc] = loopInfo{
				counter: loop.counter,
				end:     loop.end,
				next:    loop.next,
			}
			target := r.pos + int(rel)
			insns[len(insns)-1].target = target
			regions[loop.pc] = loopRegion{
				exit:    loop.exit,
				parent:  loop.parent,
				test:    0,
				op:      OpForRange,
				counter: loop.counter,
				end:     loop.end,
			}
		case OpWhileBegin:
			open = append(open, openLoop{
				op:     OpWhileBegin,
				pc:     pc,
				next:   r.pos,
				parent: owner,
			})
		case OpWhileTest:
			rel, err := r.i32()
			if err != nil {
				return err
			}
			if rel < 0 {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("WHILE_TEST at offset %d exits backwards", pc))
			}
			if len(open) == 0 || open[len(open)-1].op != OpWhileBegin {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("WHILE_TEST at offset %d has no matching WHILE_BEGIN", pc))
			}
			cur := &open[len(open)-1]
			if cur.tested {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("WHILE_BEGIN at offset %d has more than one WHILE_TEST", cur.pc))
			}
			cur.test = pc
			cur.tested = true
			cur.exit = r.pos + int(rel)
			insns[len(insns)-1].target = cur.exit
			whileExits = append(whileExits, jumpEdge{from: pc, target: cur.exit})
		case OpLoopBack:
			rel, err := r.i32()
			if err != nil {
				return err
			}
			if len(open) == 0 || open[len(open)-1].op != OpWhileBegin {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("LOOP_BACK at offset %d has no matching WHILE_BEGIN", pc))
			}
			loop := open[len(open)-1]
			open = open[:len(open)-1]
			if !loop.tested {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("WHILE_BEGIN at offset %d has no WHILE_TEST", loop.pc))
			}
			if r.pos+int(rel) != loop.pc {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("LOOP_BACK at offset %d does not target its paired WHILE_BEGIN", pc))
			}
			if loop.exit != r.pos {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("WHILE_TEST at offset %d must exit immediately after its LOOP_BACK", loop.pc))
			}
			regions[loop.pc] = loopRegion{
				exit:   loop.exit,
				parent: loop.parent,
				test:   loop.test,
				op:     OpWhileBegin,
			}
			insns[len(insns)-1].target = r.pos + int(rel)
		case OpHalt:
			if depth != 0 {
				return NewError(LOAD_BAD_BYTECODE, "HALT inside loop")
			}
			halted = true
		default:
			return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("unknown opcode %d at offset %d", op, pc))
		}

		insns[len(insns)-1].next = r.pos
	}

	if len(open) > 0 {
		top := open[len(open)-1]
		if top.op == OpForRange {
			return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("unclosed FOR_RANGE at offset %d", top.pc))
		}
		return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("unclosed WHILE_BEGIN at offset %d", top.pc))
	}
	if !halted {
		return NewError(LOAD_BAD_BYTECODE, "code does not end with HALT")
	}

	findInsn := func(target int) *verifiedInstr {
		for i := range insns {
			if insns[i].pc == target {
				return &insns[i]
			}
		}
		return nil
	}

	for _, j := range ordinary {
		from := findInsn(j.from)
		to := findInsn(j.target)
		if to == nil {
			return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("jump at offset %d targets %d, not an instruction start", j.from, j.target))
		}
		if from == nil {
			return NewError(LOAD_BAD_BYTECODE, "internal verifier error: missing jump instruction")
		}
		loop, inLoop := regions[from.owner]
		if inLoop && loop.op == OpWhileBegin && from.pc < loop.test && to.pc > loop.test {
			return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("jump at offset %d skips its WHILE_TEST", from.pc))
		}
		if from.owner == to.owner {
			continue
		}
		if !inLoop || j.target != loop.exit || to.owner != loop.parent {
			return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("jump at offset %d crosses a loop boundary (only a break to the innermost loop exit is allowed)", j.from))
		}
	}

	for _, in := range insns {
		if in.op == OpStoreLocal {
			slot := binary.BigEndian.Uint16(p.code[in.pc+1 : in.pc+3])
			for _, l := range p.loops {
				if slot == l.counter || slot == l.end {
					return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("STORE_LOCAL at offset %d writes a read-only range slot %d", in.pc, slot))
				}
			}
		}
	}

	for _, in := range insns {
		if in.op == OpForRange {
			rel := int32(binary.BigEndian.Uint32(p.code[in.pc+5 : in.pc+9]))
			target := in.pc + 9 + int(rel)
			to := findInsn(target)
			if to == nil || to.owner != in.owner {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("FOR_RANGE at offset %d exits to an invalid loop region", in.pc))
			}
		}
	}

	for _, j := range whileExits {
		from := findInsn(j.from)
		to := findInsn(j.target)
		if to == nil || from == nil || from.owner == noOwner {
			return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("WHILE_TEST at offset %d exits to an invalid loop region", j.from))
		}
		loop, inLoop := regions[from.owner]
		if !inLoop || loop.op != OpWhileBegin || loop.exit != j.target || to.owner != loop.parent {
			return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("WHILE_TEST at offset %d exits to an invalid loop region", j.from))
		}
	}

	if err := p.verifyStack(insns, returns); err != nil {
		return err
	}

	for _, l := range p.lines {
		if int(l.PC) >= len(p.code) {
			return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("line table entry points at pc %d, beyond the code", l.PC))
		}
	}

	return nil
}

func (p *Program) verifyStack(insns []verifiedInstr, returns *string) error {
	hasLoops := false
	for _, in := range insns {
		if in.op == OpForRange || in.op == OpWhileBegin {
			hasLoops = true
			break
		}
	}
	if !hasLoops && returns == nil {
		return nil
	}

	at := make(map[int]int)
	for i, in := range insns {
		at[in.pc] = i
	}

	incoming := make([]int64, len(insns))
	for i := range incoming {
		incoming[i] = -1
	}

	var work []int
	if len(insns) == 0 {
		return NewError(LOAD_BAD_BYTECODE, "code is empty")
	}

	incoming[0] = 0
	work = append(work, 0)

	var mergeErr error
	merge := func(pc int, depth int64) {
		if mergeErr != nil {
			return
		}
		foundIdx, ok := at[pc]
		if !ok {
			mergeErr = NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("control flow targets %d, not an instruction start", pc))
			return
		}
		old := incoming[foundIdx]
		if old < 0 {
			incoming[foundIdx] = depth
			work = append(work, foundIdx)
		} else if old != depth {
			mergeErr = NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("inconsistent operand stack depth at control-flow join %d: %d versus %d", pc, old, depth))
		}
	}

	mathArity := func(fn byte) (int, error) {
		switch fn {
		case MathSinF, MathCosF, MathSqrtF, MathAbsF, MathFloorF, MathEaseF, MathEaseInF, MathEaseOutF, MathIntF, MathAbsI:
			return 1, nil
		case MathPowF, MathMinF, MathMaxF, MathMinI, MathMaxI:
			return 2, nil
		case MathClampF, MathLerpF, MathClampI:
			return 3, nil
		default:
			return 0, NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("unknown MATH function %d", fn))
		}
	}

	for len(work) > 0 {
		ix := work[len(work)-1]
		work = work[:len(work)-1]
		in := insns[ix]

		op := p.code[in.pc]
		pop := 0
		push := 0
		fallthroughFlag := true
		branch := false
		conditional := false

		switch op {
		case OpPushInt, OpPushFloat, OpPushStr, OpPushBool, OpLoadLocal, OpLoadCtx:
			push = 1
		case OpStoreLocal, OpPop:
			pop = 1
		case OpCallFn:
			idx := binary.BigEndian.Uint16(p.code[in.pc+1 : in.pc+3])
			argc := p.code[in.pc+3]
			pop = int(argc)
			if p.imports[idx].fn.Sig.Returns != "void" {
				push = 1
			}
		case OpCallMethod:
			idx := binary.BigEndian.Uint16(p.code[in.pc+1 : in.pc+3])
			argc := p.code[in.pc+3]
			pop = int(argc) + 1
			if p.imports[idx].method.Sig.Returns != "void" {
				push = 1
			}
		case OpEq, OpNe, OpAddI, OpSubI, OpMulI, OpDivI, OpModI, OpAddF, OpSubF, OpMulF, OpDivF:
			pop = 2
			push = 1
		case OpLtI, OpLeI, OpGtI, OpGeI, OpLtF, OpLeF, OpGtF, OpGeF, OpConcat:
			pop = 2
			push = 1
		case OpNot, OpNegI, OpNegF, OpI2F, OpI2S, OpF2S, OpB2S:
			pop = 1
			push = 1
		case OpMath:
			fn := p.code[in.pc+1]
			arity, err := mathArity(fn)
			if err != nil {
				return err
			}
			pop = arity
			push = 1
		case OpDup:
			pop = 1
			push = 2
		case OpJump:
			branch = true
			fallthroughFlag = false
		case OpJumpIfFalse, OpJumpIfTrue:
			pop = 1
			branch = true
			conditional = true
		case OpForRange:
			pop = 2
			branch = true
			conditional = true
		case OpForNext:
			branch = true
			conditional = true
		case OpWhileBegin:
			// no pop/push
		case OpWhileTest:
			pop = 1
			branch = true
			conditional = true
		case OpLoopBack:
			branch = true
			fallthroughFlag = false
		case OpHalt:
			fallthroughFlag = false
		default:
			return NewError(LOAD_BAD_BYTECODE, "unknown opcode during stack verification")
		}

		depth := incoming[ix]
		if depth < int64(pop) {
			return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("operand stack underflow at offset %d", in.pc))
		}
		after := depth - int64(pop) + int64(push)
		if op == OpHalt && returns != nil {
			want := int64(1)
			if *returns == "void" {
				want = 0
			}
			if after != want {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("HALT leaves %d values, interface returns %s", after, *returns))
			}
		}

		if branch {
			merge(in.target, after)
			if mergeErr != nil {
				return mergeErr
			}
		}
		if conditional && (op == OpJumpIfFalse || op == OpJumpIfTrue || op == OpForRange || op == OpForNext || op == OpWhileTest) {
			merge(in.next, after)
			if mergeErr != nil {
				return mergeErr
			}
		} else if fallthroughFlag {
			if ix+1 < len(insns) {
				merge(in.next, after)
				if mergeErr != nil {
					return mergeErr
				}
			} else {
				return NewError(LOAD_BAD_BYTECODE, "control flow falls past the end of code")
			}
		}
	}

	for _, in := range insns {
		if in.op == OpWhileTest && incoming[at[in.pc]] >= 0 {
			ownerIdx := at[in.owner]
			if incoming[at[in.pc]] != incoming[ownerIdx]+1 {
				return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("WHILE_TEST must leave the loop-head stack depth at offset %d", in.pc))
			}
		}
	}

	return nil
}

func entryError(name string, err error) error {
	sep := ": "
	if strings.HasPrefix(err.Error(), "declares ") || strings.HasPrefix(err.Error(), "parameter ") {
		sep = " "
	}
	return NewError(LOAD_BAD_BYTECODE, fmt.Sprintf("entry '%s'%s%s", name, sep, err.Error()))
}
func parseInterface(text string) ([]string, string, bool) {
	ident := func(s string) bool {
		if len(s) == 0 {
			return false
		}
		for i, c := range []byte(s) {
			if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9') {
				return false
			}
		}
		return true
	}
	tail, ok := strings.CutPrefix(text, "interface ")
	if !ok {
		return nil, "", false
	}
	name, tail, ok := strings.Cut(tail, "(")
	if !ok || !ident(name) {
		return nil, "", false
	}
	args, returns, ok := strings.Cut(tail, ") -> ")
	if !ok || !ident(returns) {
		return nil, "", false
	}
	var params []string
	if args != "" {
		params = strings.Split(args, ", ")
	}
	for _, p := range params {
		if !ident(p) || p == "void" {
			return nil, "", false
		}
	}
	return params, returns, true
}
