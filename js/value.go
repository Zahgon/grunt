// Package js models the JavaScript values that grunt's configuration, templates and
// file helpers pass around, with the conversions the original applies to them.
package js

import (
	"math"
	"strconv"
	"strings"
)

// UndefinedType is the type of Undefined.
type UndefinedType struct{}

// Undefined is JavaScript's undefined. JavaScript null is Go nil.
var Undefined = UndefinedType{}

// Array is a JavaScript array. It is a reference: copies of the pointer share items.
type Array struct {
	Items []any
}

// NewArray returns an array holding items.
func NewArray(items ...any) *Array {
	return &Array{Items: items}
}

// Date is a JavaScript Date: milliseconds since the epoch, NaN when invalid.
type Date struct {
	Ms float64
}

// Function is a callable JavaScript value.
type Function struct {
	Name string
	// Length is the declared parameter count (Function.prototype.length).
	Length int
	Call   func(this any, args []any) (any, error)
}

// Error is a thrown JavaScript error.
type Error struct {
	Name    string
	Message string
	// Orig is the error this one wraps (grunt's origError).
	Orig error
	// Props holds extra own properties, such as js-yaml's mark.
	Props *Object
}

// Error returns the error's message.
func (e *Error) Error() string { return e.Message }

// Unwrap returns the error e wraps (grunt's origError), if any.
func (e *Error) Unwrap() error { return e.Orig }

// Arg returns args[i], or Undefined when the call passed fewer arguments: how a
// JavaScript function reads a missing parameter.
func Arg(args []any, i int) any {
	if i < len(args) {
		return args[i]
	}
	return Undefined
}

// NewError returns an Error with name "Error".
func NewError(msg string) *Error { return &Error{Name: "Error", Message: msg} }

// NewTypeError returns an Error with name "TypeError".
func NewTypeError(msg string) *Error { return &Error{Name: "TypeError", Message: msg} }

// NewReferenceError returns an Error with name "ReferenceError".
func NewReferenceError(msg string) *Error { return &Error{Name: "ReferenceError", Message: msg} }

// NewSyntaxError returns an Error with name "SyntaxError".
func NewSyntaxError(msg string) *Error { return &Error{Name: "SyntaxError", Message: msg} }

// Normalize converts Go literals a gruntfile may write (int, int64, float32, []any,
// map[string]any) into the value model. Maps become objects with sorted keys, since a
// Go map has no order; gruntfiles that care about key order use *Object.
func Normalize(v any) any {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int8:
		return float64(x)
	case int16:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case uint:
		return float64(x)
	case uint8:
		return float64(x)
	case uint16:
		return float64(x)
	case uint32:
		return float64(x)
	case uint64:
		return float64(x)
	case float32:
		return float64(x)
	case []any:
		items := make([]any, len(x))
		for i, it := range x {
			items[i] = Normalize(it)
		}
		return &Array{Items: items}
	case []string:
		items := make([]any, len(x))
		for i, it := range x {
			items[i] = it
		}
		return &Array{Items: items}
	case map[string]any:
		o := NewObject()
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, k := range keys {
			o.Set(k, Normalize(x[k]))
		}
		return o
	}
	return v
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// TypeOf is the typeof operator.
func TypeOf(v any) string {
	switch v.(type) {
	case UndefinedType:
		return "undefined"
	case nil:
		return "object"
	case float64:
		return "number"
	case string:
		return "string"
	case bool:
		return "boolean"
	case *Function:
		return "function"
	}
	return "object"
}

// ClassOf is the tag Object.prototype.toString reports for v, as in
// "[object Array]": "Undefined", "Null", "Number", "String", "Boolean", "Array",
// "Function", "Date", "RegExp", "Error", "Uint8Array" for a Buffer, and "Object"
// for everything else.
func ClassOf(v any) string {
	switch v.(type) {
	case UndefinedType:
		return "Undefined"
	case nil:
		return "Null"
	case float64:
		return "Number"
	case string:
		return "String"
	case bool:
		return "Boolean"
	case *Array:
		return "Array"
	case *Function:
		return "Function"
	case Date:
		return "Date"
	case *RegExp:
		return "RegExp"
	case *Error:
		return "Error"
	case []byte:
		return "Uint8Array"
	}
	return "Object"
}

// IsNullish reports whether v is null or undefined.
func IsNullish(v any) bool {
	if v == nil {
		return true
	}
	_, ok := v.(UndefinedType)
	return ok
}

// ToBoolean is the ToBoolean abstract operation.
func ToBoolean(v any) bool {
	switch x := v.(type) {
	case UndefinedType, nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0 && !math.IsNaN(x)
	case string:
		return x != ""
	}
	return true
}

// NumberToString is Number::toString with radix 10.
func NumberToString(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case v == 0:
		return "0"
	case v < 0:
		return "-" + NumberToString(-v)
	case math.IsInf(v, 1):
		return "Infinity"
	}
	mantissa, exponent, _ := strings.Cut(strconv.FormatFloat(v, 'e', -1, 64), "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	k := len(digits)
	x, _ := strconv.Atoi(exponent)
	n := x + 1
	switch {
	case k <= n && n <= 21:
		return digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return "0." + strings.Repeat("0", -n) + digits
	}
	sign := "+"
	if n-1 < 0 {
		sign = "-"
	}
	magnitude := strconv.Itoa(max(n-1, 1-n))
	if k == 1 {
		return digits + "e" + sign + magnitude
	}
	return digits[:1] + "." + digits[1:] + "e" + sign + magnitude
}

// RegExp is a JavaScript regular expression literal: its source and flags. Matching is
// done by the packages that need it; this type only carries the value.
type RegExp struct {
	Source string
	Flags  string
}
