package lodash

import (
	"math"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsnum"
	"github.com/gruntjs/grunt-go/js"
)

// lodashParseInt is lodash's parseInt: the guard argument or a nullish radix mean
// radix 0, a truthy radix is converted with ToNumber, and the string (lodash's
// toString, leading white space removed) goes to the global parseInt.
func lodashParseInt(a []any) (any, error) {
	radix, guard := js.Arg(a, 1), js.Arg(a, 2)
	r := 0.0
	if !js.ToBoolean(guard) && js.ToBoolean(radix) {
		r = js.ToNumber(radix)
	}
	if math.IsNaN(r) {
		// radix || 0
		r = 0
	}
	return jsnum.ParseInt(lodashToString(js.Arg(a, 0)), int(int32(js.ToUint32(r)))), nil
}
