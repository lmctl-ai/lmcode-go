package lmcode

import (
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
)

// Signature describes the parameter type names and return type name of a function or method.
type Signature struct {
	Params  []string
	Returns string
}

// Text formats the signature as "(T1, T2) -> R".
func (s Signature) Text() string {
	return "(" + strings.Join(s.Params, ", ") + ") -> " + s.Returns
}

// Function is a host function callable from the DSL.
type Function func(args []Value) (Value, error)

// Method is a host method callable on a host object from the DSL.
type Method func(self any, args []Value) (Value, error)

// FunctionEntry stores a registered function, its signature, and its schema name.
type FunctionEntry struct {
	Name string
	Sig  Signature
	Fn   Function
}

// MethodEntry stores a registered method, its signature, schema type, and method name.
type MethodEntry struct {
	Type string
	Name string
	Sig  Signature
	Fn   Method
}

// Context holds live host objects for one execution of a Program.
type Context struct {
	objects map[string]any
}

// NewContext creates an empty Context.
func NewContext() *Context {
	return &Context{objects: make(map[string]any)}
}

// Set binds a named context variable to a host object.
func (c *Context) Set(name string, obj any) {
	if c.objects == nil {
		c.objects = make(map[string]any)
	}
	c.objects[name] = obj
}

// Get returns the host object bound to name, or nil.
func (c *Context) Get(name string) any {
	if c.objects == nil {
		return nil
	}
	return c.objects[name]
}

// Registry declares the schema's types, context objects, methods, and functions.
type Registry struct {
	functions           map[string]*FunctionEntry // canonical signature -> entry
	methods             map[string]*MethodEntry   // canonical signature -> entry
	contexts            map[string]string         // name -> schema type name
	builtinFunctions    map[string]bool
	builtinMethods      map[string]bool
	stdSignatures       map[string]bool
	stdInstalled        bool
	stdCollections      *Collections
	explicitCollections bool
	runState            *registryRunState
}

type registryRunState struct{ mu sync.Mutex }

// NewRegistry creates a Registry with the canonical standard catalog enabled.
func NewRegistry() *Registry {
	r := &Registry{
		functions:        make(map[string]*FunctionEntry),
		methods:          make(map[string]*MethodEntry),
		contexts:         make(map[string]string),
		builtinFunctions: make(map[string]bool),
		builtinMethods:   make(map[string]bool),
		stdSignatures:    make(map[string]bool),
		runState:         &registryRunState{},
	}
	_ = RegisterStd(r)
	return r
}

// NewRegistryWithoutStd creates an empty registry for legacy or custom hosts.
func NewRegistryWithoutStd() *Registry {
	return &Registry{functions: make(map[string]*FunctionEntry), methods: make(map[string]*MethodEntry), contexts: make(map[string]string), builtinFunctions: make(map[string]bool), builtinMethods: make(map[string]bool), stdSignatures: make(map[string]bool), runState: &registryRunState{}}
}

// Context declares a context variable name and its schema type.
func (r *Registry) Context(name, typeName string) error {
	if _, exists := r.contexts[name]; exists {
		return NewError(LOAD_UNBOUND, fmt.Sprintf("registry: context '%s' is declared twice", name))
	}
	r.contexts[name] = typeName
	return nil
}

// Function registers a host function. Duplicate parameter tuples for the same name are refused;
// distinct parameter tuples are overloads.
func (r *Registry) Function(name string, params []string, returns string, fn Function) error {
	sigParams := make([]string, len(params))
	copy(sigParams, params)
	sig := Signature{Params: sigParams, Returns: returns}

	for _, entry := range r.functions {
		if entry.Name == name && slices.Equal(entry.Sig.Params, sig.Params) {
			return NewError(LOAD_UNBOUND, fmt.Sprintf("registry: function '%s%s' is registered twice", name, sig.Text()))
		}
	}

	canonical := "fn " + name + sig.Text()
	r.functions[canonical] = &FunctionEntry{
		Name: name,
		Sig:  sig,
		Fn:   fn,
	}
	return nil
}

// Method registers a host method on typeName. Duplicate parameter tuples for the same type and name are refused.
func (r *Registry) Method(typeName, name string, params []string, returns string, fn Method) error {
	sigParams := make([]string, len(params))
	copy(sigParams, params)
	sig := Signature{Params: sigParams, Returns: returns}

	for _, entry := range r.methods {
		if entry.Type == typeName && entry.Name == name && slices.Equal(entry.Sig.Params, sig.Params) {
			return NewError(LOAD_UNBOUND, fmt.Sprintf("registry: method '%s.%s%s' is registered twice", typeName, name, sig.Text()))
		}
	}

	canonical := "method " + typeName + "." + name + sig.Text()
	r.methods[canonical] = &MethodEntry{
		Type: typeName,
		Name: name,
		Sig:  sig,
		Fn:   fn,
	}
	return nil
}

func (r *Registry) registerBuiltinFunction(name string, params []string, returns string, fn Function) error {
	canonical := "fn " + name + Signature{Params: params, Returns: returns}.Text()
	if r.builtinFunctions[canonical] {
		return nil
	}
	if err := r.Function(name, params, returns, fn); err != nil {
		return err
	}
	r.builtinFunctions[canonical], r.stdSignatures[canonical] = true, true
	return nil
}

func (r *Registry) registerBuiltinMethod(typ, name string, params []string, returns string, fn Method) error {
	canonical := "method " + typ + "." + name + Signature{Params: params, Returns: returns}.Text()
	if r.builtinMethods[canonical] {
		return nil
	}
	if err := r.Method(typ, name, params, returns, fn); err != nil {
		return err
	}
	r.builtinMethods[canonical], r.stdSignatures[canonical] = true, true
	return nil
}

// RegisterStd installs the canonical built-in standard catalog once.
func RegisterStd(r *Registry) error {
	if r.stdInstalled {
		return nil
	}
	beforeF, beforeM := map[string]bool{}, map[string]bool{}
	for k := range r.functions {
		beforeF[k] = true
	}
	for k := range r.methods {
		beforeM[k] = true
	}
	if err := RegisterString(r); err != nil {
		return err
	}
	if err := RegisterStdMath(r); err != nil {
		return err
	}
	if !r.explicitCollections && r.stdCollections == nil {
		r.stdCollections = NewCollections()
	}
	if r.stdCollections != nil {
		if err := RegisterCollections(r, r.stdCollections); err != nil {
			return err
		}
	} else if err := RegisterCollections(r); err != nil {
		return err
	}
	if err := RegisterTesting(r, nil); err != nil {
		return err
	}
	for k := range r.functions {
		if !beforeF[k] {
			r.builtinFunctions[k] = true
			r.stdSignatures[k] = true
		}
	}
	for k := range r.methods {
		if !beforeM[k] {
			r.builtinMethods[k] = true
			r.stdSignatures[k] = true
		}
	}
	r.stdInstalled = true
	return nil
}

func (r *Registry) beginRun() error {
	if !r.stdInstalled {
		return nil
	}
	if r.runState == nil {
		r.runState = &registryRunState{}
	}
	if !r.runState.mu.TryLock() {
		return NewHostError("registry run already active")
	}
	if r.stdCollections != nil {
		r.stdCollections.Reset()
	}
	return nil
}

func (r *Registry) endRun() {
	if !r.stdInstalled {
		return
	}
	if r.stdCollections != nil {
		r.stdCollections.Reset()
	}
	if r.runState != nil {
		r.runState.mu.Unlock()
	}
}

// HostSignatures returns declarations excluding canonical built-in signatures.
func (r *Registry) HostSignatures() []string {
	var out []string
	for _, s := range r.Signatures() {
		if !r.stdSignatures[s] {
			out = append(out, s)
		}
	}
	return out
}

// StdSignatures returns the installed canonical standard declarations.
func (r *Registry) StdSignatures() []string {
	out := make([]string, 0, len(r.stdSignatures))
	for s := range r.stdSignatures {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// SetStdOutput replaces the built-in print sink. Nil restores stdout.
func (r *Registry) SetStdOutput(output io.Writer) error {
	if !r.stdInstalled {
		return NewError(LOAD_UNBOUND, "standard library is disabled")
	}
	if output == nil {
		output = os.Stdout
	}
	e := r.FunctionBySignature("fn print(string) -> void")
	if e == nil {
		return NewError(LOAD_UNBOUND, "standard print is not registered")
	}
	e.Fn = func(args []Value) (Value, error) {
		if _, err := io.WriteString(output, args[0].Str+"\n"); err != nil {
			return None(), NewHostError(err.Error())
		}
		return None(), nil
	}
	return nil
}

// CheckSchemaSignatures checks the host declarations plus enabled built-ins against a schema signature set.
func (r *Registry) CheckSchemaSignatures(schema []string) error {
	want := map[string]bool{}
	for _, s := range schema {
		want[s] = true
	}
	for s := range r.stdSignatures {
		want[s] = true
	}
	got := map[string]bool{}
	for _, s := range r.Signatures() {
		got[s] = true
	}
	var missing, extra []string
	for s := range want {
		if !got[s] {
			missing = append(missing, s)
		}
	}
	for s := range got {
		if !want[s] {
			extra = append(extra, s)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return NewError(LOAD_UNBOUND, fmt.Sprintf("schema signatures differ: missing %s; extra %s", strings.Join(missing, ", "), strings.Join(extra, ", ")))
}

// MatchesSchemaSignatures reports whether schema declarations plus installed std match the registry.
func (r *Registry) MatchesSchemaSignatures(schema []string) bool {
	return r.CheckSchemaSignatures(schema) == nil
}

// Signatures returns sorted canonical strings matching `lmscript --dsl-signatures SCHEMA`.
func (r *Registry) Signatures() []string {
	var out []string
	for name, typeName := range r.contexts {
		out = append(out, fmt.Sprintf("context %s: %s", name, typeName))
	}
	for key := range r.functions {
		out = append(out, key)
	}
	for key := range r.methods {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// ContextType returns the declared schema type for a context variable, or false if undeclared.
func (r *Registry) ContextType(name string) (string, bool) {
	t, ok := r.contexts[name]
	return t, ok
}

// FunctionBySignature returns the function registered with the given canonical signature.
func (r *Registry) FunctionBySignature(canonical string) *FunctionEntry {
	return r.functions[canonical]
}

// MethodBySignature returns the method registered with the given canonical signature.
func (r *Registry) MethodBySignature(canonical string) *MethodEntry {
	return r.methods[canonical]
}

// FindFunction finds a function entry by name only if unambiguous; returns nil if missing or overloaded.
func (r *Registry) FindFunction(name string) *FunctionEntry {
	var found *FunctionEntry
	for _, entry := range r.functions {
		if entry.Name == name {
			if found != nil {
				return nil
			}
			found = entry
		}
	}
	return found
}

// FindMethod finds a method entry by type and name only if unambiguous; returns nil if missing or overloaded.
func (r *Registry) FindMethod(typeName, name string) *MethodEntry {
	var found *MethodEntry
	for _, entry := range r.methods {
		if entry.Type == typeName && entry.Name == name {
			if found != nil {
				return nil
			}
			found = entry
		}
	}
	return found
}

// DeclaredFor returns a formatted list of signatures the host declares for a given canonical signature's name,
// or an empty string if unknown.
func (r *Registry) DeclaredFor(canonical string) string {
	sp := strings.IndexByte(canonical, ' ')
	par := strings.IndexByte(canonical, '(')
	if sp == -1 || par == -1 || par < sp {
		return ""
	}
	name := canonical[sp+1 : par]

	if strings.HasPrefix(canonical, "fn ") {
		var have []string
		for _, f := range r.functions {
			if f.Name == name {
				have = append(have, "fn "+name+f.Sig.Text())
			}
		}
		sort.Strings(have)
		return strings.Join(have, ", ")
	}

	dot := strings.IndexByte(name, '.')
	if dot == -1 {
		return ""
	}
	typeName := name[:dot]
	methodName := name[dot+1:]

	var have []string
	for _, m := range r.methods {
		if m.Type == typeName && m.Name == methodName {
			have = append(have, "method "+name+m.Sig.Text())
		}
	}
	sort.Strings(have)
	return strings.Join(have, ", ")
}
