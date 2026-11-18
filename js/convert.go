package js

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gruntjs/grunt-go/internal/jsdate"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
)

// ToString is the ToString abstract operation, extended to the objects grunt
// stringifies: arrays join their items with commas (null and undefined as ""),
// plain objects give "[object Object]", dates use Date.prototype.toString (local
// time), errors give "Name: message".
func ToString(v any) string {
	return toString(v, nil)
}

func toString(v any, seen []*Array) string {
	switch x := v.(type) {
	case UndefinedType:
		return "undefined"
	case nil:
		return "null"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return NumberToString(x)
	case string:
		return x
	case *Array:
		// Array.prototype.join skips an array already being joined (cycle detection).
		for _, s := range seen {
			if s == x {
				return ""
			}
		}
		seen = append(seen, x)
		parts := make([]string, len(x.Items))
		for i, it := range x.Items {
			if !IsNullish(it) {
				parts[i] = toString(it, seen)
			}
		}
		return strings.Join(parts, ",")
	case Date:
		return DateToString(x)
	case *RegExp:
		return "/" + x.Source + "/" + x.Flags
	case *Error:
		switch {
		case x.Name == "":
			return x.Message
		case x.Message == "":
			return x.Name
		}
		return x.Name + ": " + x.Message
	case []byte:
		// Buffer.prototype.toString decodes UTF-8.
		return DecodeUTF8(x)
	case *Function:
		return "function " + x.Name + "() { [native code] }"
	}
	return "[object Object]"
}

// ToNumber is the ToNumber abstract operation.
func ToNumber(v any) float64 {
	switch x := v.(type) {
	case UndefinedType:
		return math.NaN()
	case nil:
		return 0
	case bool:
		if x {
			return 1
		}
		return 0
	case float64:
		return x
	case string:
		return StringToNumber(x)
	case Date:
		return x.Ms
	case *Array:
		return StringToNumber(ToString(x))
	case []byte:
		// Number(buffer) converts the buffer's UTF-8 text.
		return StringToNumber(ToString(x))
	}
	return math.NaN()
}

// IsSpace reports whether r is ECMAScript WhiteSpace or LineTerminator, the
// characters String.prototype.trim removes.
func IsSpace(r rune) bool {
	return jsstr.IsWhiteSpace(r) || jsstr.IsLineTerminator(r)
}

// ToIntegerOrInfinity is the ToIntegerOrInfinity abstract operation on a number:
// NaN is 0, infinities stay, anything else loses its fraction (and -0 its sign).
func ToIntegerOrInfinity(n float64) float64 {
	if math.IsNaN(n) {
		return 0
	}
	if math.IsInf(n, 0) {
		return n
	}
	return math.Trunc(n) + 0 // + 0 turns -0 into +0
}

// ToUint32 is the ToUint32 abstract operation on a number.
func ToUint32(n float64) uint32 {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	n = math.Mod(math.Trunc(n), 4294967296)
	if n < 0 {
		n += 4294967296
	}
	return uint32(n)
}

// ToInt32 is the ToInt32 abstract operation on a number, the conversion behind
// value | 0.
func ToInt32(n float64) int32 { return int32(ToUint32(n)) }

// StringToNumber is ToNumber applied to a string: the trimmed text must be a whole
// StringNumericLiteral; the empty string is 0 and anything else is NaN.
func StringToNumber(s string) float64 {
	s = strings.TrimFunc(s, IsSpace)
	if s == "" {
		return 0
	}
	if len(s) > 2 && s[0] == '0' {
		base := 0
		switch s[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			return radixValue(s[2:], base)
		}
	}
	body := s
	if body[0] == '+' || body[0] == '-' {
		body = body[1:]
	}
	if body == "Infinity" {
		if s[0] == '-' {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}
	if !isDecimalLiteral(body) {
		return math.NaN()
	}
	// The literal was validated above, so ParseFloat can only report ErrRange, and
	// then it still returns the correctly rounded ±Inf or 0, which is ToNumber's result.
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// isDecimalLiteral reports whether s is an unsigned StrUnsignedDecimalLiteral
// without "Infinity": digits with an optional fraction and exponent.
func isDecimalLiteral(s string) bool {
	i, digits := 0, 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start {
			return false
		}
	}
	return i == len(s)
}

func radixValue(s string, base int) float64 {
	if s == "" {
		return math.NaN()
	}
	v := 0.0
	for _, c := range s {
		d := jsstr.DigitValue(c)
		if d < 0 || d >= base {
			return math.NaN()
		}
		v = v*float64(base) + float64(d)
	}
	return v
}

// StrictEquals is the === operator.
func StrictEquals(a, b any) bool {
	switch x := a.(type) {
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case UndefinedType:
		_, ok := b.(UndefinedType)
		return ok
	case nil:
		return b == nil
	case Date:
		// Dates are objects; two Date values are equal only as the same object, which
		// this value type cannot express, so distinct Date values are never ===.
		return false
	case []byte:
		y, ok := b.([]byte)
		return ok && len(x) > 0 && len(y) > 0 && &x[0] == &y[0]
	}
	return a == b
}

// SameValueZero is the equality used by Array.prototype.includes and lodash's uniq.
func SameValueZero(a, b any) bool {
	x, ok1 := a.(float64)
	y, ok2 := b.(float64)
	if ok1 && ok2 && math.IsNaN(x) && math.IsNaN(y) {
		return true
	}
	return StrictEquals(a, b)
}

// DateToString is Date.prototype.toString: the date in the local time zone, as in
// "Tue Jan 02 2024 10:00:00 GMT+0000 (Coordinated Universal Time)", or
// "Invalid Date".
func DateToString(d Date) string { return jsdate.ToString(d.Ms) }

// Time returns the date as a time.Time in the local time zone, and false for an
// invalid date.
func (d Date) Time() (time.Time, bool) {
	if math.IsNaN(d.Ms) {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(d.Ms)), true
}

// DecodeUTF8 decodes bytes as the WHATWG UTF-8 decoder does, which is what Node's
// Buffer and file reading use: each maximal invalid subsequence becomes one U+FFFD. Go's []rune
// conversion would instead replace every invalid byte separately.
func DecodeUTF8(b []byte) string {
	var sb strings.Builder
	for i := 0; i < len(b); {
		c := b[i]
		if c < 0x80 {
			sb.WriteByte(c)
			i++
			continue
		}
		var need int
		lower, upper := byte(0x80), byte(0xBF)
		switch {
		case 0xC2 <= c && c <= 0xDF:
			need = 1
		case 0xE0 <= c && c <= 0xEF:
			need = 2
			if c == 0xE0 {
				lower = 0xA0
			} else if c == 0xED {
				upper = 0x9F
			}
		case 0xF0 <= c && c <= 0xF4:
			need = 3
			if c == 0xF0 {
				lower = 0x90
			} else if c == 0xF4 {
				upper = 0x8F
			}
		default:
			sb.WriteRune(utf8.RuneError)
			i++
			continue
		}
		j := i + 1
		for k := 0; k < need; k++ {
			if j >= len(b) || b[j] < lower || b[j] > upper {
				break
			}
			lower, upper = 0x80, 0xBF
			j++
		}
		if j-i == need+1 {
			sb.Write(b[i:j])
		} else {
			sb.WriteRune(utf8.RuneError)
		}
		i = j
	}
	return sb.String()
}
