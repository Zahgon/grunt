package log

import (
	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/js"
)

// stringifyJSON is JSON.stringify(v) with no replacer or indentation. ok is false
// when the result is undefined (v is undefined or a function). A cycle fails with
// V8's TypeError message.
func stringifyJSON(v any) (s string, ok bool, err error) {
	return jsrt.JSONStringify(js.Normalize(v), nil, js.Undefined)
}
