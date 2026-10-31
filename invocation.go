package lmcode

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

type InvocationRegistration struct {
	File  string
	Index int
}
type invocationSlot struct {
	sink   *ResultSink
	result *JSON
}
type invocationFailure struct {
	phase string
	err   Error
	part  *int
}

// InvocationResults is one command's collector. Hosts serialize selection,
// completion and failure recording, and drain active work before WriteFrame.
// Do not copy a collector after use. Registrations are returned as owned copies.
type InvocationResults struct {
	builder       bool
	writing       bool
	registrations []InvocationRegistration
	slots         []invocationSlot
	failure       *invocationFailure
}

func invocationError(message string) error { return NewError(CLI_USAGE, "lmcode invocation: "+message) }
func invocationKeys(n *JSON, allowed ...string) error {
	if n == nil || n.kind != jsonObject {
		return invocationError("object required")
	}
	for _, m := range n.object {
		found := false
		for _, key := range allowed {
			if key == m.key {
				found = true
			}
		}
		if !found {
			return invocationError("unknown key " + m.key)
		}
	}
	return nil
}
func InvocationResultsFromMetadata(raw []byte, files []string) (*InvocationResults, error) {
	n, err := ParseJSON(string(raw))
	if err != nil {
		return nil, invocationError(err.Error())
	}
	n, err = copyResultJSON(n, "", map[*JSON]bool{})
	if err != nil {
		return nil, invocationError(err.Error())
	}
	if err = invocationKeys(n, "version", "mode", "parts"); err != nil {
		return nil, err
	}
	v, ok := jsonInteger(jsonMemberValue(n, "version"))
	if !ok || v != 1 {
		return nil, invocationError("unsupported version")
	}
	mode := jsonMemberValue(n, "mode")
	if mode == nil || mode.kind != jsonString || (mode.text != "single" && mode.text != "builder") {
		return nil, invocationError("invalid mode")
	}
	parts := jsonMemberValue(n, "parts")
	if parts == nil || parts.kind != jsonArray || len(parts.array) == 0 || mode.text == "single" && len(parts.array) != 1 {
		return nil, invocationError("invalid parts")
	}
	indices := map[string]int{}
	for i, p := range parts.array {
		if err = invocationKeys(p, "index", "file"); err != nil {
			return nil, err
		}
		index, ok := jsonInteger(jsonMemberValue(p, "index"))
		if !ok || index != int64(i) {
			return nil, invocationError("noncontiguous index")
		}
		file := jsonMemberValue(p, "file")
		if file == nil || file.kind != jsonString || strings.ContainsRune(file.text, 0) || !filepath.IsAbs(file.text) {
			return nil, invocationError("file must be absolute")
		}
		if _, exists := indices[file.text]; exists {
			return nil, invocationError("duplicate file")
		}
		indices[file.text] = i
	}
	if len(files) != len(indices) {
		return nil, invocationError("registration count mismatch")
	}
	r := &InvocationResults{builder: mode.text == "builder", slots: make([]invocationSlot, len(files))}
	seen := map[int]bool{}
	for _, file := range files {
		index, ok := indices[file]
		if !ok {
			return nil, invocationError("extra registration")
		}
		if seen[index] {
			return nil, invocationError("duplicate registration")
		}
		seen[index] = true
		r.registrations = append(r.registrations, InvocationRegistration{strings.Clone(file), index})
	}
	return r, nil
}

// InvocationResultsFromCommand reads the private environment descriptor when
// present. Introspection paths must not call this execution-context constructor.
func InvocationResultsFromCommand(files []string) (*InvocationResults, error) {
	if path, ok := os.LookupEnv("LMCODE_INVOCATION_FILE"); ok {
		return InvocationResultsFromManifest(files, &path)
	}
	return InvocationResultsFromManifest(files, nil)
}

// InvocationResultsFromManifest supports embedding without changing process env.
// nil means standalone argument-order slots; a present empty path is an error.
func InvocationResultsFromManifest(files []string, manifest *string) (*InvocationResults, error) {
	if manifest != nil {
		if !utf8.ValidString(*manifest) || strings.ContainsRune(*manifest, 0) || !filepath.IsAbs(*manifest) {
			return nil, invocationError("manifest path must be absolute")
		}
		raw, err := os.ReadFile(*manifest)
		if err != nil {
			return nil, invocationError("cannot read manifest")
		}
		return InvocationResultsFromMetadata(raw, files)
	}
	if len(files) == 0 {
		return nil, invocationError("empty parts")
	}
	r := &InvocationResults{builder: len(files) > 1, slots: make([]invocationSlot, len(files))}
	for i, file := range files {
		if file == "" || strings.ContainsRune(file, 0) {
			return nil, invocationError("invalid command file")
		}
		if !utf8.ValidString(file) {
			return nil, invocationError("file must be UTF-8")
		}
		r.registrations = append(r.registrations, InvocationRegistration{strings.Clone(file), i})
	}
	return r, nil
}
func (r *InvocationResults) Registrations() []InvocationRegistration {
	return append([]InvocationRegistration(nil), r.registrations...)
}
func (r *InvocationResults) slot(index int) (*invocationSlot, error) {
	if index < 0 || index >= len(r.slots) {
		return nil, NewHostError("lmcode invocation: invalid slot index")
	}
	return &r.slots[index], nil
}
func (r *InvocationResults) Select(index int, registry *Registry, contract ResultContract) (*ResultSink, error) {
	if r.writing {
		return nil, NewHostError("lmcode invocation: finalized")
	}
	s, err := r.slot(index)
	if err != nil {
		return nil, err
	}
	if s.sink == nil {
		if r.failure != nil {
			return nil, NewHostError("lmcode invocation: cannot start work after failure")
		}
		s.sink = NewResultSink(contract)
	}
	if err = s.sink.writable(); err != nil {
		return nil, err
	}
	if s.sink.contract != contract {
		return nil, NewHostError("lmcode invocation: result contract changed")
	}
	if err = registry.SetResultSink(s.sink); err != nil {
		return nil, err
	}
	return s.sink, nil
}
func (r *InvocationResults) Complete(index int) error {
	if r.writing {
		return NewHostError("lmcode invocation: finalized")
	}
	s, err := r.slot(index)
	if err != nil {
		return err
	}
	if s.sink == nil {
		return NewHostError("lmcode invocation: slot was not selected")
	}
	result, err := s.sink.seal()
	if err != nil {
		_ = r.Fail(&index, "run", err)
		return err
	}
	s.result = result
	return nil
}
func failureExit(phase string) (int, error) {
	switch phase {
	case "cli":
		return 2, nil
	case "load", "run":
		return 3, nil
	case "app":
		return 4, nil
	}
	return 0, NewHostError("lmcode invocation: invalid failure phase")
}
func invocationNativeError(err error) *Error {
	var e *Error
	if errors.As(err, &e) && e != nil {
		copy := *e
		return &copy
	}
	if err != nil {
		return NewHostError(err.Error())
	}
	return NewError(RUN_ERROR, "unknown error")
}
func failureJSON(phase string, e *Error, part *int) (string, error) {
	if _, err := failureExit(phase); err != nil {
		return "", err
	}
	if !utf8.ValidString(e.Code) || !utf8.ValidString(e.Message) {
		return "", resultInvalid("", "utf8")
	}
	if e.HasColumn && (!e.HasLocation || e.Line == 0 || e.Column == 0) {
		return "", NewHostError("lmcode invocation: column requires a positive line")
	}
	out := "{\"ok\":false,\"phase\":" + resultQuote(phase) + ",\"code\":" + resultQuote(e.Code) + ",\"message\":" + resultQuote(e.Message)
	if e.HasLocation {
		if e.Line == 0 {
			return "", NewHostError("lmcode invocation: line must be positive")
		}
		out += ",\"line\":" + strconv.FormatUint(uint64(e.Line), 10)
		if e.HasColumn {
			out += ",\"column\":" + strconv.Itoa(int(e.Column))
		}
	}
	if part != nil {
		out += ",\"part\":" + strconv.Itoa(*part)
	}
	return out + "}", nil
}
func (r *InvocationResults) Fail(index *int, phase string, err error) error {
	if r.writing {
		return NewHostError("lmcode invocation: finalized")
	}
	if r.failure != nil {
		return nil
	}
	if index != nil {
		if _, e := r.slot(*index); e != nil {
			return e
		}
	}
	native := invocationNativeError(err)
	if _, e := failureJSON(phase, native, nil); e != nil {
		return e
	}
	var part *int
	if r.builder && index != nil {
		copy := *index
		part = &copy
	}
	r.failure = &invocationFailure{phase: phase, err: *native, part: part}
	return nil
}
func emitInvocation(w io.Writer, payload string, code int) int {
	frame := "LMCODE_RETURN_BEGIN\n" + payload + "\nLMCODE_RETURN_END\n"
	n, err := io.WriteString(w, frame)
	if err == nil && n != len(frame) {
		err = io.ErrShortWrite
	}
	if err == nil {
		if buffered, ok := w.(interface{ Flush() error }); ok {
			err = buffered.Flush()
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lmcode invocation: cannot write envelope")
		return 4
	}
	return code
}

// FailureFrame is for errors before collector construction. Return immediately
// with its status; never append a second frame after output has begun.
func FailureFrame(w io.Writer, phase string, err error) int {
	payload, e := failureJSON(phase, invocationNativeError(err), nil)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 4
	}
	code, _ := failureExit(phase)
	return emitInvocation(w, payload, code)
}

// WriteFrame finalizes once, including on write/flush failure. The host has
// already drained active work and recorded global application validation errors.
func (r *InvocationResults) WriteFrame(w io.Writer) int {
	if r.writing {
		return 4
	}
	if r.failure == nil {
		for i := range r.slots {
			if r.slots[i].result == nil {
				_ = r.Fail(&i, "run", NewHostError("lmcode invocation: slot not completed"))
				break
			}
		}
	}
	payload := ""
	code := 0
	if r.failure != nil {
		payload, _ = failureJSON(r.failure.phase, &r.failure.err, r.failure.part)
		code, _ = failureExit(r.failure.phase)
	} else {
		payload = "{\"ok\":true,\"result\":"
		if r.builder {
			payload += "["
			for i, s := range r.slots {
				if i > 0 {
					payload += ","
				}
				payload += s.result.ToJSON()
			}
			payload += "],\"parts\":" + strconv.Itoa(len(r.slots))
		} else {
			payload += r.slots[0].result.ToJSON()
		}
		payload += "}"
	}
	r.writing = true
	return emitInvocation(w, payload, code)
}
