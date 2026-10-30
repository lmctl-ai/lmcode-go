package lmcode

import (
	"regexp"
	"strconv"
	"strings"
)

const templatePath = "[A-Za-z_][A-Za-z0-9_]*(?:[.][A-Za-z_][A-Za-z0-9_]*|\\[[0-9]+\\])*"

var templateHead = regexp.MustCompile("^<(" + templatePath + ")(?::\\{([A-Za-z_][A-Za-z0-9_]*)\\||>)")
var templateIf = regexp.MustCompile("^<if\\((" + templatePath + ")\\)>")
var templatePart = regexp.MustCompile("[A-Za-z_][A-Za-z0-9_]*|\\[[0-9]+\\]")
var templateClose = regexp.MustCompile("^};[ ]*separator[ ]*=[ ]*(\"(?:[^\"\\\\]|\\\\.)*\")>")

type templateNode struct {
	text, path, item, separator string
	body, otherwise             []templateNode
	conditional                 bool
}

func parseTemplate(source string, pos *int, mode string, depth int) ([]templateNode, string, error) {
	if depth > 256 {
		return nil, "", NewHostError("template_render: maximum nesting exceeded")
	}
	nodes := []templateNode{}
	var literal strings.Builder
	flush := func() {
		if literal.Len() > 0 {
			nodes = append(nodes, templateNode{text: literal.String()})
			literal.Reset()
		}
	}
	for *pos < len(source) {
		rest := source[*pos:]
		if mode == "loop" && strings.HasPrefix(rest, "}>") {
			flush()
			*pos += 2
			return nodes, "", nil
		}
		if mode == "loop" && strings.HasPrefix(rest, "};") {
			m := templateClose.FindStringSubmatch(rest)
			if m == nil {
				return nil, "", NewHostError("template_render: invalid loop separator")
			}
			sep, e := ParseJSON(m[1])
			if e != nil || sep.kind != jsonString {
				return nil, "", NewHostError("template_render: invalid loop separator")
			}
			flush()
			*pos += len(m[0])
			return nodes, sep.text, nil
		}
		if strings.HasPrefix(rest, "<<") {
			literal.WriteByte('<')
			*pos += 2
			continue
		}
		if strings.HasPrefix(rest, "<else>") || strings.HasPrefix(rest, "<endif>") {
			tag := "endif"
			size := 7
			if strings.HasPrefix(rest, "<else>") {
				tag = "else"
				size = 6
			}
			if (mode != "if" && mode != "else") || (mode == "else" && tag == "else") {
				return nil, "", NewHostError("template_render: unexpected conditional delimiter")
			}
			flush()
			*pos += size
			return nodes, tag, nil
		}
		if m := templateIf.FindStringSubmatch(rest); m != nil {
			flush()
			*pos += len(m[0])
			n := templateNode{path: m[1], conditional: true}
			body, end, e := parseTemplate(source, pos, "if", depth+1)
			if e != nil {
				return nil, "", e
			}
			n.body = body
			if end == "else" {
				other, _, e := parseTemplate(source, pos, "else", depth+1)
				if e != nil {
					return nil, "", e
				}
				n.otherwise = other
			}
			nodes = append(nodes, n)
			continue
		}
		m := templateHead.FindStringSubmatch(rest)
		if m == nil {
			literal.WriteByte(source[*pos])
			*pos++
			continue
		}
		flush()
		*pos += len(m[0])
		n := templateNode{path: m[1], item: m[2]}
		if n.item != "" {
			if n.item == "index" {
				return nil, "", NewHostError("template_render: index is reserved")
			}
			body, sep, e := parseTemplate(source, pos, "loop", depth+1)
			if e != nil {
				return nil, "", e
			}
			n.body = body
			n.separator = sep
		}
		nodes = append(nodes, n)
	}
	if mode != "" {
		return nil, "", NewHostError("template_render: unterminated block")
	}
	flush()
	return nodes, "", nil
}
func templateLookup(context *JSON, locals map[string]*JSON, path string) (*JSON, error) {
	parts := templatePart.FindAllString(path, -1)
	n := locals[parts[0]]
	start := 0
	if n != nil {
		start = 1
	} else {
		n = context
	}
	for _, key := range parts[start:] {
		var next *JSON
		if strings.HasPrefix(key, "[") {
			digits := key[1 : len(key)-1]
			i, e := strconv.Atoi(digits)
			if n.kind == jsonArray && e == nil && i >= 0 && i < len(n.array) && strconv.Itoa(i) == digits {
				next = n.array[i]
			}
		} else if n.kind == jsonObject {
			for _, m := range n.object {
				if m.key == key {
					next = m.value
					break
				}
			}
		}
		if next == nil {
			return nil, NewHostError("template_render: missing variable " + path)
		}
		n = next
	}
	return n, nil
}
func evaluateTemplate(nodes []templateNode, context *JSON, locals map[string]*JSON, out *strings.Builder) error {
	for _, n := range nodes {
		if n.path == "" {
			out.WriteString(n.text)
			continue
		}
		value, e := templateLookup(context, locals, n.path)
		if e != nil {
			return e
		}
		if n.conditional {
			if value.kind != jsonBool {
				return NewHostError("template_render: condition requires a bool: " + n.path)
			}
			body := n.otherwise
			if value.boolean {
				body = n.body
			}
			if e := evaluateTemplate(body, context, locals, out); e != nil {
				return e
			}
			continue
		}
		if n.item == "" {
			if value.kind == jsonString {
				out.WriteString(value.text)
			} else {
				out.WriteString(value.ToJSON())
			}
			continue
		}
		if value.kind != jsonArray {
			return NewHostError("template_render: loop requires a list: " + n.path)
		}
		scope := make(map[string]*JSON, len(locals)+2)
		for k, v := range locals {
			scope[k] = v
		}
		for i, item := range value.array {
			if i > 0 {
				out.WriteString(n.separator)
			}
			scope[n.item] = item
			scope["index"] = &JSON{kind: jsonInt, integer: int64(i), raw: strconv.Itoa(i)}
			if e := evaluateTemplate(n.body, context, scope, out); e != nil {
				return e
			}
		}
	}
	return nil
}

// RenderTemplate renders arbitrary bytes with a nested JSON context. Paths use
// <user.name> and <items[0]>. Loops use <items:{item|<index>: <item>}; separator=", ">.
// Strings are inserted verbatim; other values use compact JSON. << escapes <.
// Insertions are opaque; no format-specific escaping is applied.
func RenderTemplate(source string, context *JSON) (string, error) {
	if context == nil || context.kind != jsonObject {
		return "", NewHostError("template_render: context must be an object")
	}
	pos := 0
	nodes, _, err := parseTemplate(source, &pos, "", 0)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if err = evaluateTemplate(nodes, context, map[string]*JSON{}, &out); err != nil {
		return "", err
	}
	return out.String(), nil
}
