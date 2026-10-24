package lmcode

import (
	"fmt"
	"os"
)

// Standard error codes.
const (
	LOAD_BAD_BYTECODE   = "LOAD_BAD_BYTECODE"
	LOAD_SCHEMA_MISMATCH = "LOAD_SCHEMA_MISMATCH"
	LOAD_UNBOUND        = "LOAD_UNBOUND"
	RUN_ERROR           = "RUN_ERROR"
	RUN_HOST            = "RUN_HOST"
	APP_FAIL            = "APP_FAIL"
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

// ReportOK prints the standard one-line JSON result on stdout and returns 0.
// resultJSON must already be valid JSON text (e.g. "null", "42", "\"hello\"").
func ReportOK(resultJSON ...string) int {
	res := "null"
	if len(resultJSON) > 0 && resultJSON[0] != "" {
		res = resultJSON[0]
	}
	fmt.Printf("{\"ok\": true, \"result\": %s}\n", res)
	return 0
}

// ReportError prints the standard one-line JSON error on stdout and returns
// exit code 4 for APP_FAIL, or 3 for other errors.
func ReportError(phase string, err error) int {
	var e *Error
	if errVal, ok := err.(*Error); ok {
		e = errVal
	} else if err != nil {
		e = NewHostError(err.Error())
	} else {
		e = NewError("RUN_ERROR", "unknown error")
	}

	result := fmt.Sprintf("{\"ok\": false, \"phase\": %s, \"code\": %s, \"message\": %s",
		JSONEscape(phase), JSONEscape(e.Code), JSONEscape(e.Message))
	if e.HasLocation {
		result += fmt.Sprintf(", \"line\": %d", e.Line)
		if e.HasColumn {
			result += fmt.Sprintf(", \"column\": %d", e.Column)
		}
	}
	result += "}"
	fmt.Println(result)

	if e.Code == APP_FAIL {
		return 4
	}
	return 3
}

// ReportAppFailure prints an application failure line on stdout and returns 4.
func ReportAppFailure(message string) int {
	return ReportError("app", NewError(APP_FAIL, message))
}

// ExitIfError reports an error if non-nil and exits with the corresponding code.
func ExitIfError(phase string, err error) {
	if err != nil {
		code := ReportError(phase, err)
		os.Exit(code)
	}
}
