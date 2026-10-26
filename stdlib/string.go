package stdlib

import lmcode "github.com/lmctl-ai/lmcode-go"

func StrBase64Encode(s string) string          { return lmcode.StrBase64Encode(s) }
func StrBase64Decode(s string) (string, error) { return lmcode.StrBase64Decode(s) }
func StrHexEncode(s string) string             { return lmcode.StrHexEncode(s) }
func StrHexDecode(s string) (string, error)    { return lmcode.StrHexDecode(s) }

func StrLen(s string) int64                         { return lmcode.StrLen(s) }
func StrStartsWith(s, p string) bool                { return lmcode.StrStartsWith(s, p) }
func StrEndsWith(s, p string) bool                  { return lmcode.StrEndsWith(s, p) }
func StrContains(s, n string) bool                  { return lmcode.StrContains(s, n) }
func StrFind(s, n string) int64                     { return lmcode.StrFind(s, n) }
func StrSubstr(s string, start, count int64) string { return lmcode.StrSubstr(s, start, count) }
func StrLower(s string) string                      { return lmcode.StrLower(s) }
func StrUpper(s string) string                      { return lmcode.StrUpper(s) }
func StrTrim(s string) string                       { return lmcode.StrTrim(s) }
func StrRepeat(s string, n int64) (string, error)   { return lmcode.StrRepeat(s, n) }
func StrReplace(s, from, to string) (string, error) { return lmcode.StrReplace(s, from, to) }
func StrIsInt(s string) bool                        { return lmcode.StrIsInt(s) }
func StrToInt(s string) int64                       { return lmcode.StrToInt(s) }
func StrIsFloat(s string) bool                      { return lmcode.StrIsFloat(s) }
func StrToFloat(s string) float64                   { return lmcode.StrToFloat(s) }
func StrMatch(text, pattern string) (bool, error)   { return lmcode.StrMatch(text, pattern) }
func RegisterString(r *lmcode.Registry) error       { return lmcode.RegisterString(r) }
