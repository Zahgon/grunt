package js

import (
	"math"
	"slices"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
)

// The property operations below are the ones every package needs on the data a
// value holds itself: object keys, the indexes and length of arrays, strings and
// Buffers, the name and length of functions, and the message, name, stack and extra
// properties of errors. What a value inherits from its prototype belongs to the
// template runtime (package jsrt), which models the builtin prototypes.

// MaxArrayLength is the largest array length, 2^32 - 1.
const MaxArrayLength = 1<<32 - 1

// MaxStringLength is V8's String::kMaxLength on 64-bit builds, in UTF-16 code
// units; a longer string is a RangeError "Invalid string length".
const MaxStringLength = 1<<29 - 24

// NewRangeError returns an Error with name "RangeError".
func NewRangeError(msg string) *Error { return &Error{Name: "RangeError", Message: msg} }

// InvalidArrayLength is the RangeError V8 throws for an array length that is not
// an integer from 0 to MaxArrayLength.
func InvalidArrayLength() *Error { return NewRangeError("Invalid array length") }

// InvalidStringLength is the RangeError V8 throws for a string longer than
// MaxStringLength.
func InvalidStringLength() *Error { return NewRangeError("Invalid string length") }

// NativeErrorNames are the names of the native error constructors, whose
// prototypes supply an error's name.
var NativeErrorNames = []string{"Error", "TypeError", "RangeError", "SyntaxError", "ReferenceError", "URIError", "EvalError"}

// IsNativeErrorName reports whether name is one of NativeErrorNames.
func IsNativeErrorName(name string) bool { return slices.Contains(NativeErrorNames, name) }

// IsObject reports whether v is an object (functions included) rather than a
// primitive.
func IsObject(v any) bool {
	switch v.(type) {
	case nil, UndefinedType, float64, string, bool:
		return false
	}
	return true
}

// SetIndex is a[i] = v: an index past the end grows the array, and the items in
// between read as undefined (arrays of the model have no holes).
func (a *Array) SetIndex(i int, v any) {
	if i >= len(a.Items) {
		a.SetLength(i + 1)
	}
	a.Items[i] = v
}

// SetLength is a.length = n for a valid length: the array is truncated, or
// extended with undefined.
func (a *Array) SetLength(n int) {
	if n <= len(a.Items) {
		a.Items = a.Items[:n]
		return
	}
	a.Items = append(a.Items, slices.Repeat([]any{Undefined}, n-len(a.Items))...)
}

// GetOwn returns v's own property key and whether v has it: an object's key; an
// index or "length" of an array, a string (UTF-16 code units) or a Buffer; "name"
// and "length" of a function; and "message", "stack", the extra properties and,
// unless it is a native error's (which the prototype supplies), "name" of an error.
// Other values have no own properties in the model.
func GetOwn(v any, key string) (any, bool) {
	switch x := v.(type) {
	case *Object:
		if x.Has(key) {
			return x.Get(key), true
		}
	case *Array:
		if key == "length" {
			return float64(len(x.Items)), true
		}
		if i, ok := ArrayIndex(key); ok && i < len(x.Items) {
			return x.Items[i], true
		}
	case string:
		if key == "length" {
			return float64(jsstr.Length(x)), true
		}
		if i, ok := ArrayIndex(key); ok {
			if u := jsstr.ToUTF16(x); i < len(u) {
				return jsstr.FromUTF16(u[i : i+1]), true
			}
		}
	case []byte:
		if key == "length" {
			return float64(len(x)), true
		}
		if i, ok := ArrayIndex(key); ok && i < len(x) {
			return float64(x[i]), true
		}
	case *Function:
		switch key {
		case "name":
			return x.Name, true
		case "length":
			return float64(x.Length), true
		}
	case *Error:
		switch key {
		case "message":
			return x.Message, true
		case "stack":
			return ToString(x), true
		case "name":
			if !IsNativeErrorName(x.Name) {
				return x.Name, true
			}
		}
		if x.Props != nil && x.Props.Has(key) {
			return x.Props.Get(key), true
		}
	}
	return nil, false
}

// HasOwn is Object.prototype.hasOwnProperty for the own properties GetOwn reads.
func HasOwn(v any, key string) bool {
	_, ok := GetOwn(v, key)
	return ok
}

// SetProperty is v[key] = value for the properties the model can hold: any key of
// an object; an index of an array (growing it) or its length, when value is a
// valid length; and the message, name or an extra property of an error. Other
// assignments, such as to a primitive or to a non-index key of an array, have no
// place in the model and are dropped.
func SetProperty(v any, key string, value any) {
	switch x := v.(type) {
	case *Object:
		x.Set(key, value)
	case *Array:
		if i, ok := ArrayIndex(key); ok {
			x.SetIndex(i, value)
		} else if n, ok := value.(float64); ok && key == "length" && n >= 0 && n == math.Trunc(n) && n <= MaxArrayLength {
			x.SetLength(int(n))
		}
	case *Error:
		switch key {
		case "message":
			x.Message = ToString(value)
		case "name":
			x.Name = ToString(value)
		default:
			if x.Props == nil {
				x.Props = NewObject()
			}
			x.Props.Set(key, value)
		}
	}
}

// DeleteProperty is delete v[key] for the properties SetProperty can create. A
// deleted array item reads undefined afterwards, as the model has no holes.
func DeleteProperty(v any, key string) {
	switch x := v.(type) {
	case *Object:
		x.Delete(key)
	case *Array:
		if i, ok := ArrayIndex(key); ok && i < len(x.Items) {
			x.Items[i] = Undefined
		}
	case *Error:
		if x.Props != nil {
			x.Props.Delete(key)
		}
	}
}

// OwnKeys is Object.keys(v) for the model: the keys of an object, the indexes of
// an array, a string or a Buffer, the extra properties of an error, and nothing
// for other values.
func OwnKeys(v any) []string {
	switch x := v.(type) {
	case *Object:
		return x.Keys()
	case *Array:
		return IndexKeys(len(x.Items))
	case string:
		return IndexKeys(jsstr.Length(x))
	case []byte:
		return IndexKeys(len(x))
	case *Error:
		if x.Props != nil {
			return x.Props.Keys()
		}
	}
	return nil
}

// RelativeIndex resolves a relative index as the slice-like built-ins do
// (Array.prototype.slice, String.prototype.slice, at, includes, ...): the
// integer part of rel, counted from the end when negative, clamped to [0, n].
// NaN counts as 0.
func RelativeIndex(rel float64, n int) int {
	switch {
	case math.IsNaN(rel):
		return 0
	case rel < 0:
		return int(math.Max(float64(n)+math.Trunc(rel), 0))
	}
	return int(math.Min(math.Trunc(rel), float64(n)))
}

// Flatten appends items to out, spreading nested arrays up to depth levels
// (FlattenIntoArray of Array.prototype.flat, and lodash's baseFlatten).
func Flatten(out, items []any, depth float64) []any {
	for _, v := range items {
		if a, ok := v.(*Array); ok && depth >= 1 {
			out = Flatten(out, a.Items, depth-1)
			continue
		}
		out = append(out, v)
	}
	return out
}
