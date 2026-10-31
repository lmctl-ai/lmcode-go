package lmcode

import (
	"fmt"
	"os"
)

// Standard error codes.
const (
	CLI_USAGE            = "CLI_USAGE"
	LOAD_BAD_SCHEMA      = "LOAD_BAD_SCHEMA"
	LOAD_BAD_BYTECODE    = "LOAD_BAD_BYTECODE"
	LOAD_SCHEMA_MISMATCH = "LOAD_SCHEMA_MISMATCH"
	LOAD_UNBOUND         = "LOAD_UNBOUND"
	RUN_ERROR            = "RUN_ERROR"
	RUN_HOST             = "RUN_HOST"
	APP_FAIL             = "APP_FAIL"
)

// Error represents a load, verify or run-time failure in lmcode.
type Error struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	HasLocation bool   `json:"-"`
	HasColumn   bool   `json:"-"`
	Line        uint32 `json:"line,omitempty"`
	Column      uint16 `json:"column,omitempty"`
}

func (e *Error) Error() string {
	return e.Message
}

// NewError creates a new Error with the given code and message.
func NewError(code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// NewHostError creates a new RUN_HOST error.
func NewHostError(message string) *Error {
	return &Error{
		Code:    RUN_HOST,
		Message: message,
	}
}

// Locate sets the source location of the error if not already set and line > 0.
func (e *Error) Locate(line uint32, column uint16) {
	if e.HasLocation || line == 0 {
		return
	}
	e.HasLocation = true
	e.HasColumn = column != 0
	e.Line = line
	e.Column = column
	if e.HasColumn {
		e.Message = fmt.Sprintf("dsl line %d:%d: %s", line, column, e.Message)
	} else {
		e.Message = fmt.Sprintf("dsl line %d: %s", line, e.Message)
	}
}

// JSONEscape escapes a string as a JSON string literal with quotes.
func JSONEscape(s string) string {
	out := `"`
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			out += `\"`
		case '\\':
			out += `\\`
		case '\n':
			out += `\n`
		case '\r':
			out += `\r`
		case '\t':
			out += `\t`
		default:
			if c < 0x20 {
				out += fmt.Sprintf(`\u%04x`, c)
			} else {
				out += string(c)
			}
		}
	}
	out += `"`
	return out
}

// ExitIfError emits a pre-collector failure envelope and terminates the host.
func ExitIfError(phase string, err error) {
	if err != nil {
		os.Exit(FailureFrame(os.Stdout, phase, err))
	}
}
