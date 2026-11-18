package log

import (
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsnum"
	"github.com/gruntjs/grunt-go/js"
)

// Format is Node 24's util.format(...args) for the value model.
//
// A string first argument is a format string: %s (String(), or inspect at depth 0
// for arrays, objects and errors; -0 keeps its sign), %d (Number()), %i (parseInt),
// %f (parseFloat), %j (JSON.stringify, "[Circular]" for a cycle), %o (inspect with
// showHidden and depth 4), %O (inspect), %c (consumes an argument, prints nothing) and
// %% (a literal %, only while arguments remain). Arguments left over are appended,
// separated by spaces, strings as they are and anything else through Inspect.
//
// %s of a *js.Function prints js.ToString's "function name() { [native code] }"
// where Node prints the function's source text, which the model does not keep.
func Format(args ...any) string {
	normalized := make([]any, len(args))
	for i, a := range args {
		normalized[i] = js.Normalize(a)
	}
	args = normalized
	var b strings.Builder
	a := 0
	join := ""
	if first, ok := firstString(args); ok {
		if len(args) == 1 {
			return first
		}
		lastPos := 0
		for i := 0; i < len(first)-1; i++ {
			if first[i] != '%' {
				continue
			}
			i++
			next := first[i]
			if a+1 == len(args) {
				if next == '%' {
					b.WriteString(first[lastPos:i])
					lastPos = i + 1
				}
				continue
			}
			var tempStr string
			switch next {
			case 's':
				a++
				tempStr = formatS(args[a])
			case 'j':
				a++
				tempStr = formatJ(args[a])
			case 'd':
				a++
				tempStr = formatNumber(js.ToNumber(args[a]))
			case 'O':
				a++
				tempStr = inspectWith(args[a], defaultInspectOptions)
			case 'o':
				a++
				opts := defaultInspectOptions
				opts.showHidden = true
				opts.depth = 4
				tempStr = inspectWith(args[a], opts)
			case 'i':
				a++
				tempStr = formatNumber(jsnum.ParseInt(js.ToString(args[a]), 0))
			case 'f':
				a++
				tempStr = formatNumber(jsnum.ParseFloat(js.ToString(args[a])))
			case 'c':
				a++
				tempStr = ""
			case '%':
				b.WriteString(first[lastPos:i])
				lastPos = i + 1
				continue
			default:
				// Not a placeholder: the text stays as it is.
				continue
			}
			if lastPos != i-1 {
				b.WriteString(first[lastPos : i-1])
			}
			b.WriteString(tempStr)
			lastPos = i + 1
		}
		if lastPos != 0 {
			a++
			join = " "
			if lastPos < len(first) {
				b.WriteString(first[lastPos:])
			}
		}
	}
	for ; a < len(args); a++ {
		b.WriteString(join)
		if s, ok := args[a].(string); ok {
			b.WriteString(s)
		} else {
			b.WriteString(Inspect(args[a]))
		}
		join = " "
	}
	return b.String()
}

func firstString(args []any) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	s, ok := args[0].(string)
	return s, ok
}

// formatS is the %s conversion. Objects whose toString is a built-in one (arrays,
// plain objects, dates, regular expressions, errors) are inspected at depth 0; a
// Buffer, whose toString is not one of inspect's built-ins, and primitives go
// through String().
func formatS(v any) string {
	switch x := v.(type) {
	case float64:
		return formatNumber(x)
	case *js.Array, *js.Object, *js.Error, *js.RegExp, js.Date:
		opts := defaultInspectOptions
		opts.depth = 0
		return inspectWith(v, opts)
	}
	return js.ToString(v)
}

// formatJ is the %j conversion (tryStringify).
func formatJ(v any) string {
	s, ok, err := stringifyJSON(v)
	switch {
	case err != nil:
		// A cycle is the only error JSON.stringify raises for the value model, and
		// util.format turns exactly that one into "[Circular]".
		return "[Circular]"
	case !ok:
		return "undefined"
	}
	return s
}
