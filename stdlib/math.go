package stdlib

import lmcode "github.com/lmctl-ai/lmcode-go"

func MathAbsI(x int64) int64                { return lmcode.StdMathAbsI(x) }
func MathMinI(a, b int64) int64             { return lmcode.StdMathMinI(a, b) }
func MathMaxI(a, b int64) int64             { return lmcode.StdMathMaxI(a, b) }
func MathFloorI(x int64) int64              { return lmcode.StdMathFloorI(x) }
func MathCeilI(x int64) int64               { return lmcode.StdMathCeilI(x) }
func MathRoundI(x int64) int64              { return lmcode.StdMathRoundI(x) }
func MathMinF(a, b float64) float64         { return lmcode.StdMathMinF(a, b) }
func MathMaxF(a, b float64) float64         { return lmcode.StdMathMaxF(a, b) }
func MathPowI(a, b int64) (int64, error)    { return lmcode.StdMathPowI(a, b) }
func MathSqrtI(x int64) (int64, error)      { return lmcode.StdMathSqrtI(x) }
func MathPowF(a, b float64) float64         { return lmcode.StdMathPowF(a, b) }
func RegisterMath(r *lmcode.Registry) error { return lmcode.RegisterStdMath(r) }
