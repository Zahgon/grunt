package lodash

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsnum"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// This file is sprintf-js 1.1.3, which underscore.string 3.3.6 exposes (through
// util.deprecate) as sprintf and vsprintf. In Node the first call of either also
// prints a DeprecationWarning to stderr; that side effect is not reproduced here.

// placeholder is one parsed %-conversion of a format string.
type placeholder struct {
	paramNo   string   // explicit position (%2$s), or ""
	keys      []string // named argument path (%(a.b[0])s), or nil
	sign      bool     // '+' flag
	padChar   string   // "0", "'x" or ""
	align     bool     // '-' flag: pad on the right
	width     string   // digits, or ""
	precision string   // digits after '.', or ""
	typ       byte
}

// sprintfToken is literal text (ph nil) or a placeholder.
type sprintfToken struct {
	text string
	ph   *placeholder
}

// sprintf is sprintf-js's sprintf(format, ...args). argv[0] is the format, as in
// the original, whose implicit cursor starts at 1.
func sprintf(argv []any) (any, error) {
	tree, err := sprintfParse(js.Arg(argv, 0))
	if err != nil {
		return nil, err
	}
	return sprintfFormat(tree, argv)
}

// vsprintf is sprintf.apply(null, [fmt].concat(argv || [])).
func vsprintf(args []any) (any, error) {
	argv := []any{js.Arg(args, 0)}
	switch list := js.Arg(args, 1).(type) {
	case *js.Array:
		argv = append(argv, list.Items...)
	default:
		if js.ToBoolean(list) {
			argv = append(argv, list)
		}
	}
	return sprintf(argv)
}

// sprintfParse is sprintf_parse. A format that is not a string is consumed as its
// string form until the original calls _fmt.substring on it, which only strings
// have (sprintf-js's parse cache, keyed by the format's string form, is not
// modelled).
func sprintfParse(format any) ([]sprintfToken, error) {
	if !js.ToBoolean(format) {
		return nil, nil
	}
	s, isString := format.(string)
	if !isString {
		s = js.ToString(format)
	}
	var tree []sprintfToken
	argNames := 0
	for s != "" {
		var consumed int
		switch {
		case s[0] != '%':
			consumed = strings.IndexByte(s, '%')
			if consumed < 0 {
				consumed = len(s)
			}
			tree = append(tree, sprintfToken{text: s[:consumed]})
		case strings.HasPrefix(s, "%%"):
			consumed = 2
			tree = append(tree, sprintfToken{text: "%"})
		default:
			ph, n, ok := parsePlaceholder(s)
			if !ok {
				return nil, js.NewSyntaxError("[sprintf] unexpected placeholder")
			}
			if ph.keys != nil {
				argNames |= 1
			} else {
				argNames |= 2
			}
			if ph.keys != nil && len(ph.keys) == 0 {
				return nil, js.NewSyntaxError("[sprintf] failed to parse named argument key")
			}
			if argNames == 3 {
				return nil, js.NewError("[sprintf] mixing positional and named placeholders is not (yet) supported")
			}
			consumed = n
			tree = append(tree, sprintfToken{ph: ph})
		}
		if !isString {
			return nil, notFunctionError("_fmt.substring")
		}
		s = s[consumed:]
	}
	return tree, nil
}

// parsePlaceholder matches re.placeholder,
// /^\x25(?:([1-9]\d*)\$|\(([^)]+)\))?(\+)?(0|'[^$])?(-)?(\d+)?(?:\.(\d+))?([b-gijostTuvxX])/,
// at the start of s and returns the placeholder and its length. A named key that
// does not parse is returned as an empty, non-nil keys slice.
func parsePlaceholder(s string) (*placeholder, int, bool) {
	ph := &placeholder{}
	i := 1
	digitsAt := func(j int) int {
		k := j
		for k < len(s) && s[k] >= '0' && s[k] <= '9' {
			k++
		}
		return k
	}
	if i < len(s) && s[i] >= '1' && s[i] <= '9' {
		if k := digitsAt(i); k < len(s) && s[k] == '$' {
			ph.paramNo = s[i:k]
			i = k + 1
		}
	} else if i < len(s) && s[i] == '(' {
		if k := strings.IndexByte(s[i+1:], ')'); k > 0 {
			ph.keys = parseKeys(s[i+1 : i+1+k])
			i += k + 2
		}
	}
	if i < len(s) && s[i] == '+' {
		ph.sign = true
		i++
	}
	if i < len(s) && s[i] == '0' {
		ph.padChar = "0"
		i++
	} else if i+1 < len(s) && s[i] == '\'' && s[i+1] != '$' {
		// '[^$] takes one UTF-16 code unit; a padding character is read as a
		// whole character here, which differs only for astral characters.
		_, size := utf8.DecodeRuneInString(s[i+1:])
		ph.padChar = s[i : i+1+size]
		i += 1 + size
	}
	if i < len(s) && s[i] == '-' {
		ph.align = true
		i++
	}
	if k := digitsAt(i); k > i {
		ph.width = s[i:k]
		i = k
	}
	if i+1 < len(s) && s[i] == '.' && s[i+1] >= '0' && s[i+1] <= '9' {
		k := digitsAt(i + 1)
		ph.precision = s[i+1 : k]
		i = k
	}
	if i < len(s) && strings.IndexByte("bcdefgijostTuvxX", s[i]) >= 0 {
		ph.typ = s[i]
		return ph, i + 1, true
	}
	return nil, 0, false
}

// parseKeys splits a named-argument path: a key, then .key and [index] parts.
// An unparsable path yields an empty slice.
func parseKeys(field string) []string {
	keys := []string{}
	isKeyChar := func(c byte, first bool) bool {
		return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (!first && c >= '0' && c <= '9')
	}
	readKey := func(s string) int {
		if s == "" || !isKeyChar(s[0], true) {
			return 0
		}
		k := 1
		for k < len(s) && isKeyChar(s[k], false) {
			k++
		}
		return k
	}
	n := readKey(field)
	if n == 0 {
		return keys
	}
	keys = append(keys, field[:n])
	for field = field[n:]; field != ""; {
		switch {
		case field[0] == '.' && readKey(field[1:]) > 0:
			k := readKey(field[1:])
			keys = append(keys, field[1:1+k])
			field = field[1+k:]
		case field[0] == '[':
			k := 1
			for k < len(field) && field[k] >= '0' && field[k] <= '9' {
				k++
			}
			if k == 1 || k >= len(field) || field[k] != ']' {
				return []string{}
			}
			keys = append(keys, field[1:k])
			field = field[k+1:]
		default:
			return []string{}
		}
	}
	return keys
}

// sprintfFormat is sprintf_format.
func sprintfFormat(tree []sprintfToken, argv []any) (any, error) {
	cursor := 1
	var out strings.Builder
	for _, tok := range tree {
		if tok.ph == nil {
			out.WriteString(tok.text)
			continue
		}
		ph := tok.ph
		var a any
		switch {
		case ph.keys != nil:
			a = js.Arg(argv, cursor)
			for k, key := range ph.keys {
				if js.IsNullish(a) {
					prev := "undefined"
					if k > 0 {
						prev = ph.keys[k-1]
					}
					return nil, js.NewError(`[sprintf] Cannot access property "` + key + `" of undefined value "` + prev + `"`)
				}
				var err error
				if a, err = propertyOf(a, key); err != nil {
					return nil, err
				}
			}
		case ph.paramNo != "":
			n, err := strconv.Atoi(ph.paramNo)
			if err != nil {
				a = js.Undefined
			} else {
				a = js.Arg(argv, n)
			}
		default:
			a = js.Arg(argv, cursor)
			cursor++
		}
		if f, ok := a.(*js.Function); ok && ph.typ != 'T' && ph.typ != 'v' {
			var err error
			if a, err = f.Call(js.Undefined, nil); err != nil {
				return nil, err
			}
		}
		if strings.IndexByte("bcdiefguxX", ph.typ) >= 0 {
			if _, isNum := a.(float64); !isNum && math.IsNaN(js.ToNumber(toPrimitiveNumber(a))) {
				return nil, js.NewTypeError("[sprintf] expecting number but found " + typeTag(a))
			}
		}
		numeric := strings.IndexByte("diefg", ph.typ) >= 0
		positive := false
		if numeric {
			positive = !lessThan(a, 0.0) && !math.IsNaN(js.ToNumber(toPrimitiveNumber(a)))
		}
		v, err := convertPlaceholder(ph, a)
		if err != nil {
			return nil, err
		}
		if ph.typ == 'j' {
			out.WriteString(js.ToString(v))
			continue
		}
		sign := ""
		text := js.ToString(v)
		if numeric && (!positive || ph.sign) {
			sign = "-"
			if positive {
				sign = "+"
			}
			if text != "" && (text[0] == '+' || text[0] == '-') {
				text = text[1:]
			}
		}
		padChar := " "
		switch {
		case ph.padChar == "0":
			padChar = "0"
		case ph.padChar != "":
			padChar = ph.padChar[1:]
		}
		pad := ""
		if ph.width != "" {
			padLength := js.StringToNumber(ph.width) - float64(jsstr.Length(sign+text))
			if padLength > 0 {
				if math.IsInf(padLength, 1) {
					return nil, js.NewRangeError("Invalid count value: Infinity")
				}
				if padLength > js.MaxStringLength {
					return nil, js.InvalidStringLength()
				}
				if pad, err = stringRepeat(padChar, float64(padLength)); err != nil {
					return nil, err
				}
			}
		}
		switch {
		case ph.align:
			out.WriteString(sign + text + pad)
		case padChar == "0":
			out.WriteString(sign + pad + text)
		default:
			out.WriteString(pad + sign + text)
		}
	}
	return out.String(), nil
}

// convertPlaceholder applies the conversion of ph.typ to a, giving the value the
// original concatenates (a number or a string; undefined for %j of undefined).
func convertPlaceholder(ph *placeholder, a any) (any, error) {
	precision := func() float64 { return js.ToIntegerOrInfinity(js.StringToNumber(ph.precision)) }
	parseInt10 := func() float64 { return jsnum.ParseInt(js.ToString(a), 10) }
	clip := func(s string) string {
		if ph.precision == "" {
			return s
		}
		u := jsstr.ToUTF16(s)
		return jsstr.FromUTF16(u[:int(math.Min(precision(), float64(len(u))))])
	}
	switch ph.typ {
	case 'b':
		return jsnum.ToStringRadix(parseInt10(), 2), nil
	case 'c':
		return jsstr.FromUTF16([]uint16{uint16(js.ToUint32(parseInt10()))}), nil
	case 'd', 'i':
		return parseInt10(), nil
	case 'j':
		gap := 0
		if ph.width != "" {
			gap = int(jsnum.ParseInt(ph.width, 0))
		}
		s, ok, err := jsrt.JSONStringify(a, nil, float64(gap))
		if err != nil || !ok {
			return js.Undefined, err
		}
		return s, nil
	case 'e':
		x := jsnum.ParseFloat(js.ToString(a))
		if ph.precision != "" {
			return jsnum.ToExponential(x, precision(), true)
		}
		return jsnum.ToExponential(x, 0, false)
	case 'f':
		x := jsnum.ParseFloat(js.ToString(a))
		if ph.precision != "" {
			return jsnum.ToFixed(x, precision())
		}
		return x, nil
	case 'g':
		if ph.precision == "" {
			return jsnum.ParseFloat(js.ToString(a)), nil
		}
		// arg.toPrecision is called on the argument itself, so only numbers have it.
		x, ok := a.(float64)
		if !ok {
			if js.IsNullish(a) {
				return nil, readError(a, "toPrecision")
			}
			return nil, notFunctionError("arg.toPrecision")
		}
		s, err := jsnum.ToPrecision(x, precision())
		if err != nil {
			return nil, err
		}
		return js.NumberToString(js.StringToNumber(s)), nil
	case 'o':
		return jsnum.ToStringRadix(float64(js.ToUint32(parseInt10())), 8), nil
	case 's':
		return clip(js.ToString(a)), nil
	case 't':
		return clip(strconv.FormatBool(js.ToBoolean(a))), nil
	case 'T':
		return clip(typeTag(a)), nil
	case 'u':
		return float64(js.ToUint32(parseInt10())), nil
	case 'v':
		if js.IsNullish(a) {
			return nil, readError(a, "valueOf")
		}
		if d, ok := a.(js.Date); ok {
			a = d.Ms
		}
		if ph.precision == "" {
			return a, nil
		}
		s, ok := a.(string)
		if !ok {
			return nil, notFunctionError("arg.substring")
		}
		return clip(s), nil
	case 'x':
		return jsnum.ToStringRadix(float64(js.ToUint32(parseInt10())), 16), nil
	case 'X':
		return strings.ToUpper(jsnum.ToStringRadix(float64(js.ToUint32(parseInt10())), 16)), nil
	}
	panic("lodash: unknown sprintf conversion")
}
