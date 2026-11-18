package yaml

import (
	"github.com/gruntjs/grunt-go/js"
)

// ownKeys returns Object.keys(v) and a getter when v is an object (common.isObject).
// Arrays and Buffers have their indexes as keys; dates and regular expressions have
// none. ok is false for primitives, null and undefined.
func ownKeys(v any) (keys []string, get func(string) any, ok bool) {
	switch v.(type) {
	case *js.Object, *js.Array, []byte:
		return js.OwnKeys(v), func(k string) any {
			val, _ := js.GetOwn(v, k) // k is one of v's own keys
			return val
		}, true
	case js.Date, *js.RegExp:
		return nil, nil, true
	}
	return nil, nil, false
}
