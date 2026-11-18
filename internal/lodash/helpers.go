package lodash

import (
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/jsrt/unicase"
	"github.com/gruntjs/grunt-go/js"
)

// entry is one function of the `_` object: its name, its declared parameter count
// (Function.prototype.length) and its implementation.
type entry struct {
	name   string
	length int
	call   func(args []any) (any, error)
}

// makeString is underscore.string's makeString: "" for null and undefined, the
// string form of anything else.
func makeString(v any) string {
	if js.IsNullish(v) {
		return ""
	}
	return js.ToString(v)
}

// toUpperCase and toLowerCase are String.prototype.toUpperCase and toLowerCase.
func toUpperCase(s string) string { return jsstr.FromUTF16(unicase.ToUpper(jsstr.ToUTF16(s))) }
func toLowerCase(s string) string { return jsstr.FromUTF16(unicase.ToLower(jsstr.ToUTF16(s))) }
func unitSlice(u []uint16, a, b int) string {
	return jsstr.FromUTF16(u[a:b])
}

// toInt32 is ToInt32, the conversion behind ~~value and value | 0.
func toInt32(v any) int {
	return int(js.ToInt32(js.ToNumber(v)))
}

// readError is the TypeError V8 throws when reading a property of null or undefined.
func readError(v any, key string) error {
	return js.NewTypeError("Cannot read properties of " + js.ToString(v) + " (reading '" + key + "')")
}

// notFunctionError is the TypeError V8 throws when calling a non-function, named
// by the expression the original source used.
func notFunctionError(expr string) error {
	return js.NewTypeError(expr + " is not a function")
}
