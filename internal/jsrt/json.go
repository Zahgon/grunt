package jsrt

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

func (r *Realm) setupJSON() {
	j := r.staticObject("JSON")
	r.method(j, "stringify", 3, func(_ any, args []any) (any, error) {
		s, ok, err := r.JSONStringify(js.Arg(args, 0), js.Arg(args, 1), js.Arg(args, 2))
		if err != nil || !ok {
			return js.Undefined, err
		}
		return s, nil
	})
	r.method(j, "parse", 2, func(_ any, args []any) (any, error) {
		text, err := r.toString(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		v, err := JSONParse(text)
		if err != nil {
			return nil, err
		}
		if reviver, ok := js.Arg(args, 1).(*js.Function); ok {
			root := js.NewObject()
			root.Set("", v)
			return r.internalize(root, "", reviver)
		}
		return v, nil
	})
}

// JSONStringify is JSON.stringify(value, replacer, space) for any value in the
// js value model, without a realm of the caller's: Date values use their toJSON,
// objects' own toJSON methods are honoured and cycles throw V8's TypeError. ok is
// false when the result is undefined.
func JSONStringify(v, replacer, space any) (s string, ok bool, err error) {
	return defaultRealm().JSONStringify(v, replacer, space)
}

// defaultRealm is a realm without a clock for realm-independent helpers.
func defaultRealm() *Realm { return NewRealm(Options{}) }

// JSONStringify is JSON.stringify in this realm; see the package-level function.
func (r *Realm) JSONStringify(v, replacer, space any) (string, bool, error) {
	st := &jsonState{r: r}
	switch x := replacer.(type) {
	case *js.Function:
		st.replacer = x
	case *js.Array:
		st.keys = []string{}
		for _, it := range x.Items {
			var k string
			switch y := it.(type) {
			case string:
				k = y
			case float64:
				k = js.NumberToString(y)
			default:
				continue
			}
			if !slices.Contains(st.keys, k) {
				st.keys = append(st.keys, k)
			}
		}
	}
	switch x := space.(type) {
	case float64:
		n := min(js.ToIntegerOrInfinity(x), 10)
		if n >= 1 {
			st.gap = strings.Repeat(" ", int(n))
		}
	case string:
		u := jsstr.ToUTF16(x)
		st.gap = jsstr.FromUTF16(u[:min(len(u), 10)])
	}
	holder := js.NewObject()
	holder.Set("", v)
	var b strings.Builder
	ok, err := st.property(&b, holder, "", v, "")
	return b.String(), ok, err
}

// jsonState is the state of one JSON.stringify call.
type jsonState struct {
	r        *Realm
	replacer *js.Function
	keys     []string
	gap      string
	// stack holds the objects being serialized and the key leading to each.
	stack []jsonFrame
}

type jsonFrame struct {
	key string
	obj any
}

// property is SerializeJSONProperty: it writes the value of key in holder (already
// read as v) and reports false when the value serializes to undefined.
func (st *jsonState) property(b *strings.Builder, holder any, key string, v any, indent string) (bool, error) {
	r := st.r
	if js.IsObject(v) {
		if m, _ := r.lookup(v, "toJSON"); m != nil {
			if f, ok := m.(*js.Function); ok {
				var err error
				if v, err = r.call(f, v, []any{key}); err != nil {
					return false, err
				}
			}
		}
	}
	if st.replacer != nil {
		var err error
		if v, err = r.call(st.replacer, holder, []any{key, v}); err != nil {
			return false, err
		}
	}
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(js.ToString(x))
	case string:
		quoteJSON(b, x)
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			b.WriteString("null")
		} else {
			b.WriteString(js.NumberToString(x))
		}
	case js.UndefinedType, *js.Function:
		return false, nil
	case *js.Array:
		return true, st.array(b, x, key, indent)
	default:
		return true, st.object(b, x, key, indent)
	}
	return true, nil
}

// enter pushes an object, failing with V8's circular-structure message when it is
// already being serialized.
func (st *jsonState) enter(key string, obj any) error {
	for i, f := range st.stack {
		if f.obj == obj {
			return js.NewTypeError(st.circularMessage(i, key))
		}
	}
	st.stack = append(st.stack, jsonFrame{key, obj})
	return nil
}

// circularMessage reproduces V8's CircularStructureMessageBuilder: the start of the
// cycle, up to two following links, an ellipsis, the last link and the closing key.
func (st *jsonState) circularMessage(start int, closingKey string) string {
	var b strings.Builder
	b.WriteString("Converting circular structure to JSON\n    --> starting at object with constructor '" + st.constructorName(st.stack[start].obj) + "'")
	line := func(f jsonFrame) {
		b.WriteString("\n    |     " + jsonKeyLabel(f.key) + " -> object with constructor '" + st.constructorName(f.obj) + "'")
	}
	i := start + 1
	prefixEnd := min(len(st.stack), i+2)
	for ; i < prefixEnd; i++ {
		line(st.stack[i])
	}
	if len(st.stack) > i+1 {
		b.WriteString("\n    |     ...")
	}
	for i = max(i, len(st.stack)-1); i < len(st.stack); i++ {
		line(st.stack[i])
	}
	b.WriteString("\n    --- " + jsonKeyLabel(closingKey) + " closes the circle")
	return b.String()
}

func jsonKeyLabel(key string) string {
	if _, ok := js.ArrayIndex(key); ok {
		return "index " + key
	}
	return "property '" + key + "'"
}

// constructorName is the name V8 reports for an object's constructor.
func (st *jsonState) constructorName(obj any) string {
	switch x := obj.(type) {
	case *js.Array:
		return "Array"
	case *js.Error:
		return x.Name
	}
	if c, _ := st.r.lookup(obj, "constructor"); c != nil {
		if f, ok := c.(*js.Function); ok && f.Name != "" {
			return f.Name
		}
	}
	return "Object"
}

func (st *jsonState) array(b *strings.Builder, a *js.Array, key, indent string) error {
	if err := st.enter(key, a); err != nil {
		return err
	}
	defer func() { st.stack = st.stack[:len(st.stack)-1] }()
	if len(a.Items) == 0 {
		b.WriteString("[]")
		return nil
	}
	inner := indent + st.gap
	b.WriteString("[")
	for i := 0; i < len(a.Items); i++ {
		if i > 0 {
			b.WriteString(",")
		}
		if st.gap != "" {
			b.WriteString("\n" + inner)
		}
		ok, err := st.property(b, a, strconv.Itoa(i), a.Items[i], inner)
		if err != nil {
			return err
		}
		if !ok {
			b.WriteString("null")
		}
	}
	if st.gap != "" {
		b.WriteString("\n" + indent)
	}
	b.WriteString("]")
	return nil
}

func (st *jsonState) object(b *strings.Builder, o any, key, indent string) error {
	if raw, ok := o.([]byte); ok {
		// Buffer.prototype.toJSON.
		data := make([]any, len(raw))
		for i, c := range raw {
			data[i] = float64(c)
		}
		o = js.ObjectOf("type", "Buffer", "data", js.NewArray(data...))
	}
	if err := st.enter(key, o); err != nil {
		return err
	}
	defer func() { st.stack = st.stack[:len(st.stack)-1] }()
	keys := st.keys
	if keys == nil {
		keys = st.r.ownKeys(o)
	}
	inner := indent + st.gap
	b.WriteString("{")
	wrote := false
	for _, k := range keys {
		v, _ := st.r.lookup(o, k)
		var part strings.Builder
		ok, err := st.property(&part, o, k, v, inner)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if wrote {
			b.WriteString(",")
		}
		wrote = true
		if st.gap != "" {
			b.WriteString("\n" + inner)
		}
		quoteJSON(b, k)
		b.WriteString(":")
		if st.gap != "" {
			b.WriteString(" ")
		}
		b.WriteString(part.String())
	}
	if wrote && st.gap != "" {
		b.WriteString("\n" + indent)
	}
	b.WriteString("}")
	return nil
}

// quoteJSON is QuoteJSONString: control characters and unpaired surrogates are
// escaped with lowercase hex.
func quoteJSON(b *strings.Builder, s string) {
	const hex = "0123456789abcdef"
	b.WriteByte('"')
	u := jsstr.ToUTF16(s)
	for i := 0; i < len(u); i++ {
		c := u[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			switch {
			case c < 0x20:
				b.WriteString(`\u00` + string(hex[c>>4]) + string(hex[c&15]))
			case c >= 0xD800 && c < 0xE000:
				cp, n := jsstr.CodePointAt(u, i)
				if n == 2 {
					b.WriteRune(cp)
					i++
					continue
				}
				b.WriteString(`\u` + string(hex[c>>12]) + string(hex[c>>8&15]) + string(hex[c>>4&15]) + string(hex[c&15]))
			default:
				b.WriteString(jsstr.FromUTF16(u[i : i+1]))
			}
		}
	}
	b.WriteByte('"')
}

// internalize is InternalizeJSONProperty: the reviver sees every value bottom-up
// and undefined results delete the property.
func (r *Realm) internalize(holder any, key string, reviver *js.Function) (any, error) {
	val, _ := r.lookup(holder, key)
	switch x := val.(type) {
	case *js.Array:
		for i := 0; i < len(x.Items); i++ {
			nv, err := r.internalize(x, strconv.Itoa(i), reviver)
			if err != nil {
				return nil, err
			}
			x.Items[i] = nv
		}
	case *js.Object:
		for _, k := range x.Keys() {
			nv, err := r.internalize(x, k, reviver)
			if err != nil {
				return nil, err
			}
			if isUndefined(nv) {
				x.Delete(k)
			} else {
				x.Set(k, nv)
			}
		}
	}
	return r.call(reviver, holder, []any{key, val})
}

// JSONParse is JSON.parse(text) without a reviver. Errors are SyntaxErrors with
// Node 24's messages, including the position, line and column or the source excerpt.
func JSONParse(text string) (any, error) {
	p := &jsonParser{src: jsstr.ToUTF16(text)}
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if p.pos < len(p.src) {
		return nil, p.failAt("Unexpected non-whitespace character after JSON")
	}
	return v, nil
}

// jsonParser follows V8's JsonParser, including where it reports each error.
type jsonParser struct {
	src []uint16
	pos int
}

func (p *jsonParser) skipSpace() {
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

// fail reports a message with V8's "in JSON at position N (line L column C)".
func (p *jsonParser) fail(msg string) error {
	return p.failAt(msg + " in JSON")
}

// failAt appends " at position N (line L column C)" to msg.
func (p *jsonParser) failAt(msg string) error {
	line, col := 1, 1
	for i := 0; i < p.pos && i < len(p.src); i++ {
		switch p.src[i] {
		case '\r':
			if i+1 < len(p.src) && p.src[i+1] == '\n' {
				i++
			}
			line, col = line+1, 1
		case '\n':
			line, col = line+1, 1
		default:
			col++
		}
	}
	return js.NewSyntaxError(msg + " at position " + strconv.Itoa(p.pos) + " (line " + strconv.Itoa(line) + " column " + strconv.Itoa(col) + ")")
}

// unexpected reports the token at the current position: end of input, or an
// unexpected character with a short excerpt of the source.
func (p *jsonParser) unexpected() error {
	if p.pos >= len(p.src) {
		return js.NewSyntaxError("Unexpected end of JSON input")
	}
	src := jsstr.FromUTF16(p.src)
	switch src {
	case "undefined", "NaN", "Infinity", "[object Object]":
		return js.NewSyntaxError(`"` + src + `" is not valid JSON`)
	}
	tok := jsstr.FromUTF16(p.src[p.pos : p.pos+1])
	const context = 10
	n := len(p.src)
	var excerpt string
	switch {
	case n < 2*context+1:
		excerpt = `"` + src + `"`
	case p.pos < context:
		excerpt = `"` + jsstr.FromUTF16(p.src[:p.pos+context]) + `"...`
	case p.pos >= n-context:
		excerpt = `..."` + jsstr.FromUTF16(p.src[p.pos-context:]) + `"`
	default:
		excerpt = `..."` + jsstr.FromUTF16(p.src[p.pos-context:p.pos+context]) + `"...`
	}
	return js.NewSyntaxError("Unexpected token '" + tok + "', " + excerpt + " is not valid JSON")
}

func (p *jsonParser) value() (any, error) {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return nil, p.unexpected()
	}
	switch c := p.src[p.pos]; {
	case c == '{':
		return p.object()
	case c == '[':
		return p.array()
	case c == '"':
		return p.str()
	case c == '-' || c >= '0' && c <= '9':
		return p.number()
	case c == 't':
		return true, p.literal("true")
	case c == 'f':
		return false, p.literal("false")
	case c == 'n':
		return nil, p.literal("null")
	}
	return nil, p.unexpected()
}

func (p *jsonParser) literal(word string) error {
	for i := 0; i < len(word); i++ {
		if p.pos >= len(p.src) || p.src[p.pos] != uint16(word[i]) {
			return p.unexpected()
		}
		p.pos++
	}
	return nil
}

func (p *jsonParser) object() (any, error) {
	p.pos++
	o := js.NewObject()
	p.skipSpace()
	if p.pos < len(p.src) && p.src[p.pos] == '}' {
		p.pos++
		return o, nil
	}
	first := true
	for {
		p.skipSpace()
		if p.pos >= len(p.src) || p.src[p.pos] != '"' {
			if first {
				return nil, p.fail("Expected property name or '}'")
			}
			return nil, p.fail("Expected double-quoted property name")
		}
		first = false
		k, err := p.str()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.pos >= len(p.src) || p.src[p.pos] != ':' {
			return nil, p.fail("Expected ':' after property name")
		}
		p.pos++
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		o.Set(k.(string), v)
		p.skipSpace()
		if p.pos < len(p.src) && p.src[p.pos] == ',' {
			p.pos++
			continue
		}
		if p.pos < len(p.src) && p.src[p.pos] == '}' {
			p.pos++
			return o, nil
		}
		return nil, p.fail("Expected ',' or '}' after property value")
	}
}

func (p *jsonParser) array() (any, error) {
	p.pos++
	a := js.NewArray()
	p.skipSpace()
	if p.pos < len(p.src) && p.src[p.pos] == ']' {
		p.pos++
		return a, nil
	}
	for {
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		a.Items = append(a.Items, v)
		p.skipSpace()
		if p.pos < len(p.src) && p.src[p.pos] == ',' {
			p.pos++
			continue
		}
		if p.pos < len(p.src) && p.src[p.pos] == ']' {
			p.pos++
			return a, nil
		}
		return nil, p.fail("Expected ',' or ']' after array element")
	}
}

func (p *jsonParser) str() (any, error) {
	p.pos++
	var out []uint16
	for {
		if p.pos >= len(p.src) {
			return nil, p.fail("Unterminated string")
		}
		c := p.src[p.pos]
		switch {
		case c == '"':
			p.pos++
			return jsstr.FromUTF16(out), nil
		case c < 0x20:
			return nil, p.fail("Bad control character in string literal")
		case c == '\\':
			p.pos++
			if p.pos >= len(p.src) {
				return nil, p.unexpected()
			}
			e := p.src[p.pos]
			switch e {
			case '"', '\\', '/':
				out = append(out, e)
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'u':
				v := uint16(0)
				for k := 0; k < 4; k++ {
					p.pos++
					if p.pos >= len(p.src) {
						return nil, p.unexpected()
					}
					d := -1
					if p.src[p.pos] < 0x80 {
						d = jsstr.HexDigitValue(rune(byte(p.src[p.pos])))
					}
					if d < 0 {
						return nil, p.fail("Bad Unicode escape")
					}
					v = v<<4 | uint16(d)
				}
				out = append(out, v)
			default:
				return nil, p.fail("Bad escaped character")
			}
			p.pos++
		default:
			out = append(out, c)
			p.pos++
		}
	}
}

func (p *jsonParser) number() (any, error) {
	start := p.pos
	digit := func() bool { return p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' }
	if p.src[p.pos] == '-' {
		p.pos++
		if !digit() {
			return nil, p.fail("No number after minus sign")
		}
	}
	if p.src[p.pos] == '0' {
		p.pos++
		if digit() {
			return nil, p.fail("Unexpected number")
		}
	} else {
		for digit() {
			p.pos++
		}
	}
	if p.pos < len(p.src) && p.src[p.pos] == '.' {
		p.pos++
		if !digit() {
			return nil, p.fail("Unterminated fractional number")
		}
		for digit() {
			p.pos++
		}
	}
	if p.pos < len(p.src) && (p.src[p.pos] == 'e' || p.src[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.src) && (p.src[p.pos] == '+' || p.src[p.pos] == '-') {
			p.pos++
		}
		if !digit() {
			return nil, p.fail("Exponent part is missing a number")
		}
		for digit() {
			p.pos++
		}
	}
	// The text was validated above, so ParseFloat can only report ErrRange, with
	// the correctly rounded ±Inf or ±0 that JSON.parse also produces.
	v, _ := strconv.ParseFloat(jsstr.FromUTF16(p.src[start:p.pos]), 64)
	return v, nil
}
