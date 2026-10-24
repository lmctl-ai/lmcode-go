package lmcode

import (
	"fmt"
	"slices"
	"sort"
	"strings"
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
	functions map[string]*FunctionEntry // canonical signature -> entry
	methods   map[string]*MethodEntry   // canonical signature -> entry
	contexts  map[string]string         // name -> schema type name
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		functions: make(map[string]*FunctionEntry),
		methods:   make(map[string]*MethodEntry),
		contexts:  make(map[string]string),
	}
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
