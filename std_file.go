package lmcode

import (
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf8"
)

func filePath(function, path string) (string, error) {
	if strings.IndexByte(path, 0) >= 0 || (runtime.GOOS == "windows" && !utf8.ValidString(path)) {
		return "", NewHostError(function + ": " + path + ": invalid path")
	}
	return path, nil
}

func fileFailure(function, path string, err error) error {
	reason := "file operation failed"
	if err != nil && err.Error() != "" {
		reason = err.Error()
	}
	return NewHostError(function + ": " + path + ": " + reason)
}

func openRegular(function, path string, flags int) (*os.File, error) {
	native, err := filePath(function, path)
	if err != nil {
		return nil, err
	}
	if info, statErr := os.Stat(native); statErr == nil {
		if !info.Mode().IsRegular() {
			return nil, fileFailure(function, path, os.ErrInvalid)
		}
	} else if !os.IsNotExist(statErr) || flags&os.O_CREATE == 0 {
		return nil, fileFailure(function, path, statErr)
	}
	f, err := os.OpenFile(native, flags, 0o666)
	if err != nil {
		return nil, fileFailure(function, path, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fileFailure(function, path, err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, fileFailure(function, path, os.ErrInvalid)
	}
	return f, nil
}

func readRegular(function, path string) (string, error) {
	f, err := openRegular(function, path, os.O_RDONLY)
	if err != nil {
		return "", err
	}
	data, readErr := io.ReadAll(f)
	closeErr := f.Close()
	if readErr != nil {
		return "", fileFailure(function, path, readErr)
	}
	if closeErr != nil {
		return "", fileFailure(function, path, closeErr)
	}
	return string(data), nil
}

func writeRegular(function, path, content string, flags int) error {
	f, err := openRegular(function, path, flags)
	if err != nil {
		return err
	}
	remaining := []byte(content)
	for len(remaining) > 0 {
		n, writeErr := f.Write(remaining)
		if writeErr != nil {
			_ = f.Close()
			return fileFailure(function, path, writeErr)
		}
		if n <= 0 {
			_ = f.Close()
			return fileFailure(function, path, io.ErrShortWrite)
		}
		remaining = remaining[n:]
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fileFailure(function, path, err)
	}
	if err := f.Close(); err != nil {
		return fileFailure(function, path, err)
	}
	return nil
}

// RegisterFile registers the five raw-byte regular-file helpers. An optional
// arena supplies ownership for file_lines; without one, a private arena is used.
func RegisterFile(r *Registry, arenas ...*Collections) error {
	arena := NewCollections()
	if len(arenas) > 0 && arenas[0] != nil {
		arena = arenas[0]
	}
	if err := r.registerBuiltinFunction("file_read", []string{"string"}, "string", func(a []Value) (Value, error) {
		data, err := readRegular("file_read", a[0].Str)
		if err != nil {
			return None(), err
		}
		return Str(data), nil
	}); err != nil {
		return err
	}
	if err := r.registerBuiltinFunction("file_write", []string{"string", "string"}, "void", func(a []Value) (Value, error) {
		if err := writeRegular("file_write", a[0].Str, a[1].Str, os.O_WRONLY|os.O_CREATE|os.O_TRUNC); err != nil {
			return None(), err
		}
		return None(), nil
	}); err != nil {
		return err
	}
	if err := r.registerBuiltinFunction("file_append", []string{"string", "string"}, "void", func(a []Value) (Value, error) {
		if err := writeRegular("file_append", a[0].Str, a[1].Str, os.O_WRONLY|os.O_CREATE|os.O_APPEND); err != nil {
			return None(), err
		}
		return None(), nil
	}); err != nil {
		return err
	}
	if err := r.registerBuiltinFunction("file_exists", []string{"string"}, "bool", func(a []Value) (Value, error) {
		path, err := filePath("file_exists", a[0].Str)
		if err != nil {
			return None(), err
		}
		info, statErr := os.Stat(path)
		if statErr != nil {
			if os.IsNotExist(statErr) || errors.Is(statErr, syscall.ENOTDIR) {
				return Bool(false), nil
			}
			return None(), fileFailure("file_exists", a[0].Str, statErr)
		}
		return Bool(info.Mode().IsRegular()), nil
	}); err != nil {
		return err
	}
	return r.registerBuiltinFunction("file_lines", []string{"string"}, "StrList", func(a []Value) (Value, error) {
		data, err := readRegular("file_lines", a[0].Str)
		if err != nil {
			return None(), err
		}
		value, err := arena.object("StrList", &StrList{values: strings.Split(data, "\n")})
		if err != nil {
			return None(), err
		}
		return value, nil
	})
}
