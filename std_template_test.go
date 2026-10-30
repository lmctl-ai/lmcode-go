package lmcode

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestTemplateRenderConformance(t *testing.T) {
	raw, e := os.ReadFile("std/template-cases.json")
	if e != nil {
		t.Fatal(e)
	}
	var cases []struct {
		Name, Template, Result, Error string
		Context                       json.RawMessage
	}
	if e = json.Unmarshal(raw, &cases); e != nil {
		t.Fatal(e)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			context, e := ParseJSON(string(c.Context))
			if e != nil {
				t.Fatal(e)
			}
			before := context.ToJSON()
			got, e := RenderTemplate(c.Template, context)
			if c.Error != "" {
				if e == nil || e.Error() != c.Error {
					t.Fatalf("error %v, want %s", e, c.Error)
				}
			} else if e != nil || got != c.Result {
				t.Fatalf("got %q (%v), want %q", got, e, c.Result)
			}
			if context.ToJSON() != before {
				t.Fatal("context mutated")
			}
		})
	}
}
func TestTemplateBytesAndLimits(t *testing.T) {
	c, _ := ParseJSON("{\"yes\":true,\"n\":9223372036854775807,\"f\":1.20e3}")
	got, e := RenderTemplate("\xff\x00<n>/<f>", c)
	if e != nil || got != "\xff\x009223372036854775807/1.20e3" {
		t.Fatalf("%q %v", got, e)
	}
	_, e = RenderTemplate(strings.Repeat("<if(yes)>", 257)+strings.Repeat("<endif>", 257), c)
	if e == nil {
		t.Fatal("expected depth limit")
	}
}
