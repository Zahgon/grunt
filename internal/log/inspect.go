package log

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/gruntjs/grunt-go/internal/jsdate"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// This file ports the parts of Node 24's util.inspect (lib/internal/util/inspect.js)
// that util.format reaches for values of the js value model, with colours off.

// inspectOptions are the util.inspect options util.format varies.
type inspectOptions struct {
	showHidden  bool
	depth       float64 // math.Inf(1) stands for depth: null
	compact     int
	breakLength int
}

// defaultInspectOptions are Node's inspectDefaultOptions, which util.format uses for
// %O and for arguments past the format string.
var defaultInspectOptions = inspectOptions{depth: 2, compact: 3, breakLength: 80}

const (
	maxArrayLength  = 100
	maxStringLength = 10000
	minLineWidth    = 16 // kMinLineWidth
	bufferMaxBytes  = 50 // buffer.INSPECT_MAX_BYTES
)

// The kinds of entries formatProperty formats.
const (
	kObjectType = iota
	kArrayType
	kArrayExtrasType
)

// Inspect is util.inspect(v) with Node 24's default options.
//
// The value model maps onto inspect's output as follows: arrays and objects as in
// Node (circular references as <ref *n> / [Circular *n]); js.Date as its ISO string
// or "Invalid Date"; []byte as a Buffer (<Buffer 61 62>); *js.RegExp as /source/flags;
// *js.Function as [Function: name]. Two cases differ from Node because the model lacks
// the information: an error prints as [Name: message] (Node prints err.stack, which
// has stack frames), and showHidden (%o) does not list a function's hidden length,
// name and prototype properties.
func Inspect(v any) string {
	return inspectWith(v, defaultInspectOptions)
}

func inspectWith(v any, opts inspectOptions) string {
	c := &inspector{opts: opts}
	return c.formatValue(js.Normalize(v), 0)
}

type inspector struct {
	opts           inspectOptions
	seen           []any
	circular       map[any]int
	indentationLvl int
	currentDepth   int
}

// property is one own property formatProperty prints.
type property struct {
	key    string
	value  any
	hidden bool // non-enumerable: printed as [key]
}

func (c *inspector) formatValue(v any, recurseTimes int) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string, float64, bool, js.UndefinedType:
		return c.formatPrimitive(v)
	case []byte:
		// Buffer's [util.inspect.custom] runs before the circular check.
		return formatBuffer(x)
	case *js.Array, *js.Object, *js.Error:
		for _, s := range c.seen {
			if s == v {
				return "[Circular *" + strconv.Itoa(c.circularIndex(v)) + "]"
			}
		}
	}
	return c.formatRaw(v, recurseTimes)
}

func (c *inspector) circularIndex(v any) int {
	if c.circular == nil {
		c.circular = map[any]int{}
	}
	if i, ok := c.circular[v]; ok {
		return i
	}
	i := len(c.circular) + 1
	c.circular[v] = i
	return i
}

func (c *inspector) formatPrimitive(v any) string {
	switch x := v.(type) {
	case string:
		u := jsstr.ToUTF16(x)
		trailer := ""
		if len(u) > maxStringLength {
			remaining := len(u) - maxStringLength
			u = u[:maxStringLength]
			trailer = "... " + strconv.Itoa(remaining) + " more character" + plural(remaining)
		}
		if len(u) > minLineWidth && len(u) > c.opts.breakLength-c.indentationLvl-4 {
			// Split after every line break: /(?<=\n)/.
			var parts []string
			start := 0
			for i, cu := range u {
				if cu == '\n' {
					parts = append(parts, strEscape(u[start:i+1]))
					start = i + 1
				}
			}
			if start < len(u) {
				parts = append(parts, strEscape(u[start:]))
			}
			return strings.Join(parts, " +\n"+strings.Repeat(" ", c.indentationLvl+2)) + trailer
		}
		return strEscape(u) + trailer
	case float64:
		return formatNumber(x)
	case bool:
		return strconv.FormatBool(x)
	}
	return "undefined"
}

// formatNumber is inspect's formatNumber without numeric separators: -0 keeps its sign.
func formatNumber(f float64) string {
	if f == 0 && math.Signbit(f) {
		return "-0"
	}
	return js.NumberToString(f)
}

func plural(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

// formatBuffer is Buffer.prototype[util.inspect.custom] for a buffer with no extra
// properties.
func formatBuffer(b []byte) string {
	const hex = "0123456789abcdef"
	n := min(len(b), bufferMaxBytes)
	parts := make([]string, n)
	for i := range n {
		parts[i] = string([]byte{hex[b[i]>>4], hex[b[i]&0xf]})
	}
	s := strings.Join(parts, " ")
	if remaining := len(b) - bufferMaxBytes; remaining > 0 {
		s += " ... " + strconv.Itoa(remaining) + " more byte" + plural(remaining)
	}
	return "<Buffer " + s + ">"
}

func (c *inspector) formatRaw(v any, recurseTimes int) string {
	var props []property
	var arr *js.Array
	base := ""
	braces := [2]string{"{", "}"}
	extrasType := kObjectType
	switch x := v.(type) {
	case *js.Array:
		if c.opts.showHidden {
			props = append(props, property{"length", float64(len(x.Items)), true})
		}
		braces = [2]string{"[", "]"}
		if len(x.Items) == 0 && len(props) == 0 {
			return "[]"
		}
		arr = x
		extrasType = kArrayExtrasType
	case *js.Object:
		for _, k := range x.Keys() {
			props = append(props, property{k, x.Get(k), false})
		}
		if len(props) == 0 {
			return "{}"
		}
	case *js.Function:
		if x.Name == "" {
			return "[Function (anonymous)]"
		}
		return "[Function: " + x.Name + "]"
	case *js.RegExp:
		base = "/" + x.Source + "/" + x.Flags
		if c.opts.showHidden {
			props = append(props, property{"lastIndex", 0.0, true})
		}
		if len(props) == 0 || float64(recurseTimes) > c.opts.depth {
			return base
		}
	case js.Date:
		iso, ok := jsdate.ISOString(x.Ms)
		if !ok {
			return "Invalid Date"
		}
		return iso
	case *js.Error:
		base = c.formatError(x)
		if c.opts.showHidden {
			// V8 errors own a non-enumerable stack accessor and message.
			props = append(props, property{"stack", getterSetter{}, true}, property{"message", x.Message, true})
		}
		if x.Props != nil {
			for _, k := range x.Props.Keys() {
				// removeDuplicateErrorKeys: name, message and stack are dropped when
				// their value already shows in the stack.
				if !c.opts.showHidden && (k == "name" || k == "message" || k == "stack") {
					if s, ok := x.Props.Get(k).(string); ok && strings.Contains(base, s) {
						continue
					}
				}
				props = append(props, property{k, x.Props.Get(k), false})
			}
		}
		if len(props) == 0 {
			return base
		}
	default:
		// A Go value outside the value model has no enumerable properties.
		return "{}"
	}

	if float64(recurseTimes) > c.opts.depth {
		return "[" + constructorName(v) + "]"
	}
	recurseTimes++
	c.seen = append(c.seen, v)
	c.currentDepth = recurseTimes
	var output []string
	if arr != nil {
		output = c.formatArray(arr, recurseTimes)
	}
	for _, p := range props {
		output = append(output, c.formatProperty(p, recurseTimes, extrasType))
	}
	if index, ok := c.circular[v]; ok {
		reference := "<ref *" + strconv.Itoa(index) + ">"
		if base == "" {
			base = reference
		} else {
			base = reference + " " + base
		}
	}
	c.seen = c.seen[:len(c.seen)-1]
	return c.reduceToSingleString(output, base, braces, extrasType, recurseTimes, arr)
}

// formatError renders an error without a stack trace: [Name: message], indented like
// Node indents a multi-line stack inside a nested value.
func (c *inspector) formatError(e *js.Error) string {
	name := e.Name
	if name == "" && e.Message == "" {
		name = "Error"
	}
	stack := "[" + js.ToString(&js.Error{Name: name, Message: e.Message}) + "]"
	if c.indentationLvl != 0 {
		stack = strings.ReplaceAll(stack, "\n", "\n"+strings.Repeat(" ", c.indentationLvl))
	}
	return stack
}

func (c *inspector) formatArray(a *js.Array, recurseTimes int) []string {
	n := min(maxArrayLength, len(a.Items))
	output := make([]string, 0, n+1)
	for i := range n {
		output = append(output, c.formatProperty(property{value: a.Items[i]}, recurseTimes, kArrayType))
	}
	if remaining := len(a.Items) - n; remaining > 0 {
		output = append(output, "... "+strconv.Itoa(remaining)+" more item"+plural(remaining))
	}
	return output
}

// getterSetter stands for an accessor property's value, which inspect prints as
// [Getter/Setter].
type getterSetter struct{}

func (c *inspector) formatProperty(p property, recurseTimes, kind int) string {
	var str string
	if _, ok := p.value.(getterSetter); ok {
		str = "[Getter/Setter]"
	} else {
		c.indentationLvl += 2
		str = c.formatValue(p.value, recurseTimes)
		c.indentationLvl -= 2
	}
	if kind == kArrayType {
		return str
	}
	var name string
	switch {
	case p.key == "__proto__":
		name = "['__proto__']"
	case p.hidden:
		name = "[" + p.key + "]"
	case isIdentifierKey(p.key):
		name = p.key
	default:
		name = strEscape(jsstr.ToUTF16(p.key))
	}
	return name + ": " + str
}

// isIdentifierKey is /^[a-zA-Z_][a-zA-Z_0-9]*$/.
func isIdentifierKey(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		ch := k[i]
		switch {
		case ch == '_', ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func (c *inspector) reduceToSingleString(output []string, base string, braces [2]string, extrasType, recurseTimes int, arr *js.Array) string {
	entries := len(output)
	if extrasType == kArrayExtrasType && entries > 6 {
		output = c.groupArrayElements(output, arr)
	}
	if c.currentDepth-recurseTimes < c.opts.compact && entries == len(output) {
		start := len(output) + c.indentationLvl + jsstr.Length(braces[0]) + jsstr.Length(base) + 10
		if c.isBelowBreakLength(output, start, base) {
			joined := strings.Join(output, ", ")
			if !strings.Contains(joined, "\n") {
				prefix := ""
				if base != "" {
					prefix = base + " "
				}
				return prefix + braces[0] + " " + joined + " " + braces[1]
			}
		}
	}
	indentation := "\n" + strings.Repeat(" ", c.indentationLvl)
	prefix := ""
	if base != "" {
		prefix = base + " "
	}
	return prefix + braces[0] + indentation + "  " + strings.Join(output, ","+indentation+"  ") + indentation + braces[1]
}

func (c *inspector) isBelowBreakLength(output []string, start int, base string) bool {
	total := len(output) + start
	if total+len(output) > c.opts.breakLength {
		return false
	}
	for _, o := range output {
		total += jsstr.Length(o)
		if total > c.opts.breakLength {
			return false
		}
	}
	return base == "" || !strings.Contains(base, "\n")
}

// groupArrayElements lays more than six array entries out in aligned columns.
func (c *inspector) groupArrayElements(output []string, arr *js.Array) []string {
	totalLength, maxLength := 0, 0
	outputLength := len(output)
	if maxArrayLength < len(output) {
		// Leave the "... n more items" entry out of the grouping.
		outputLength--
	}
	const separatorSpace = 2 // a comma and a space
	dataLen := make([]int, outputLength)
	for i := range outputLength {
		n := stringWidth(output[i])
		dataLen[i] = n
		totalLength += n + separatorSpace
		maxLength = max(maxLength, n)
	}
	actualMax := maxLength + separatorSpace
	if actualMax*3+c.indentationLvl >= c.opts.breakLength ||
		(float64(totalLength)/float64(actualMax) <= 5 && maxLength > 6) {
		return output
	}
	const approxCharHeights = 2.5
	averageBias := math.Sqrt(float64(actualMax) - float64(totalLength)/float64(len(output)))
	biasedMax := math.Max(float64(actualMax)-3-averageBias, 1)
	columns := min(
		int(math.Round(math.Sqrt(approxCharHeights*biasedMax*float64(outputLength))/biasedMax)),
		(c.opts.breakLength-c.indentationLvl)/actualMax,
		c.opts.compact*4,
		15,
	)
	if columns <= 1 {
		return output
	}
	maxLineLength := make([]int, 0, columns)
	for i := range columns {
		lineLength := 0
		for j := i; j < outputLength; j += columns {
			lineLength = max(lineLength, dataLen[j])
		}
		maxLineLength = append(maxLineLength, lineLength+separatorSpace)
	}
	// Numbers are right-aligned, anything else left-aligned. The check runs over
	// every output entry, so extra entries ([length], "more items") count as
	// non-numbers unless an array item sits at their index.
	numeric := arr != nil
	for i := 0; numeric && i < len(output); i++ {
		if i >= len(arr.Items) {
			numeric = false
		} else if _, ok := arr.Items[i].(float64); !ok {
			numeric = false
		}
	}
	var tmp []string
	for i := 0; i < outputLength; i += columns {
		maxIdx := min(i+columns, outputLength)
		var b strings.Builder
		j := i
		for ; j < maxIdx-1; j++ {
			padding := maxLineLength[j-i] + jsstr.Length(output[j]) - dataLen[j]
			if numeric {
				b.WriteString(padStart(output[j]+", ", padding))
			} else {
				b.WriteString(padEnd(output[j]+", ", padding))
			}
		}
		if numeric {
			padding := maxLineLength[j-i] + jsstr.Length(output[j]) - dataLen[j] - separatorSpace
			b.WriteString(padStart(output[j], padding))
		} else {
			b.WriteString(output[j])
		}
		tmp = append(tmp, b.String())
	}
	if maxArrayLength < len(output) {
		tmp = append(tmp, output[outputLength])
	}
	return tmp
}

// stringWidth approximates Node's getStringWidth (ICU's East Asian width): ASCII
// control characters, combining marks and format characters are zero wide, East
// Asian wide and fullwidth characters (and the common emoji blocks) are two, all
// else one.
func stringWidth(s string) int {
	w := 0
	for _, r := range s {
		switch {
		case r < 0x20:
		case r < 0x7f:
			w++
		case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		case isWide(r):
			w += 2
		default:
			w++
		}
	}
	return w
}

func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115f,
		r >= 0x2e80 && r <= 0x303e,
		r >= 0x3041 && r <= 0x33ff,
		r >= 0x3400 && r <= 0x4dbf,
		r >= 0x4e00 && r <= 0x9fff,
		r >= 0xa000 && r <= 0xa4cf,
		r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff,
		r >= 0xfe30 && r <= 0xfe4f,
		r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1f64f,
		r >= 0x1f900 && r <= 0x1f9ff,
		r >= 0x20000 && r <= 0x3fffd:
		return true
	}
	return false
}

// strEscape quotes a string the way inspect does: single quotes by default, double
// quotes when the string has a single quote but no double quote, backticks when it
// has both but no backtick or "${"; control characters, C1 controls, the backslash,
// the chosen quote (when it is the single quote) and lone surrogates are escaped.
func strEscape(u []uint16) string {
	quote := uint16('\'')
	if slices.Contains(u, '\'') {
		if !slices.Contains(u, '"') {
			quote = '"'
		} else if !slices.Contains(u, '`') && !containsDollarBrace(u) {
			quote = '`'
		}
	}
	out := make([]uint16, 0, len(u)+2)
	out = append(out, quote)
	for i := 0; i < len(u); i++ {
		point := u[i]
		switch {
		case (point == '\'' && quote == '\'') || point == '\\' || point < 0x20 || (point > 0x7e && point < 0xa0):
			out = append(out, jsstr.ToUTF16(escapeMeta(point))...)
		case point >= 0xd800 && point <= 0xdbff && i+1 < len(u) && u[i+1] >= 0xdc00 && u[i+1] <= 0xdfff:
			out = append(out, point, u[i+1])
			i++
		case point >= 0xd800 && point <= 0xdfff:
			out = append(out, jsstr.ToUTF16(`\u`+strconv.FormatUint(uint64(point), 16))...)
		default:
			out = append(out, point)
		}
	}
	out = append(out, quote)
	return jsstr.FromUTF16WellFormed(out)
}

// escapeMeta is inspect's meta table entry for an escaped code unit.
func escapeMeta(point uint16) string {
	switch point {
	case '\b':
		return `\b`
	case '\t':
		return `\t`
	case '\n':
		return `\n`
	case '\f':
		return `\f`
	case '\r':
		return `\r`
	case '\'':
		return `\'`
	case '\\':
		return `\\`
	}
	return `\x` + strings.ToUpper(zeroPadHex(point))
}

func zeroPadHex(point uint16) string {
	s := strconv.FormatUint(uint64(point), 16)
	if len(s) < 2 {
		s = "0" + s
	}
	return s
}

func containsDollarBrace(u []uint16) bool {
	for i := 0; i+1 < len(u); i++ {
		if u[i] == '$' && u[i+1] == '{' {
			return true
		}
	}
	return false
}

// constructorName is the constructor name V8 and util.inspect report for an object
// of the value model.
func constructorName(v any) string {
	switch x := v.(type) {
	case *js.Array:
		return "Array"
	case *js.Error:
		return errorConstructor(x)
	case *js.RegExp:
		return "RegExp"
	}
	return "Object"
}

// errorConstructor guesses an error's constructor from its name: the model keeps no
// constructor, and errors grunt sees are built with the constructor matching the name.
func errorConstructor(e *js.Error) string {
	if js.IsNativeErrorName(e.Name) || e.Name == "AggregateError" {
		return e.Name
	}
	return "Error"
}
