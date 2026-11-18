package yaml

import (
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsdate"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// The scalar types of lib/js-yaml/type. Their resolvers test data.length or index
// characters, so a value that is not a string (see yamlType) fails them, except where
// the original first converts it to a string; each such case is noted.

// nullType is lib/js-yaml/type/null.js.
var nullType = &yamlType{
	tag:  "tag:yaml.org,2002:null",
	kind: kindScalar,
	resolve: func(data any) bool {
		if data == nil {
			return true
		}
		switch data {
		case "~", "null", "Null", "NULL":
			return true
		}
		return false
	},
	construct: func(any) (any, error) { return nil, nil },
}

// boolType is lib/js-yaml/type/bool.js.
var boolType = &yamlType{
	tag:  "tag:yaml.org,2002:bool",
	kind: kindScalar,
	resolve: func(data any) bool {
		switch data {
		case "true", "True", "TRUE", "false", "False", "FALSE":
			return true
		}
		return false
	},
	construct: func(data any) (any, error) {
		switch data {
		case "true", "True", "TRUE":
			return true, nil
		}
		return false, nil
	},
}

// intType is lib/js-yaml/type/int.js: decimal, 0b binary, 0x hexadecimal, 0-prefixed
// octal and base-60 ("1:30") integers, with "_" separators.
var intType = &yamlType{
	tag:  "tag:yaml.org,2002:int",
	kind: kindScalar,
	resolve: func(data any) bool {
		s, ok := data.(string)
		return ok && resolveYAMLInteger(s)
	},
	construct: func(data any) (any, error) { return constructYAMLInteger(data.(string)), nil },
}

// charAt is data[i] for an ASCII test: 0 past the end, where JavaScript yields
// undefined. Non-ASCII bytes never pass the tests, as non-ASCII code units do not.
func charAt(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return 0
}

func isHexDigit(c byte) bool { return jsstr.HexDigitValue(rune(c)) >= 0 }

func isOctDigit(c byte) bool { return '0' <= c && c <= '7' }

func isDecDigit(c byte) bool { return '0' <= c && c <= '9' }

var base60Tail = regexp.MustCompile(`^(:[0-5]?[0-9])+$`)

func resolveYAMLInteger(data string) bool {
	n := len(data)
	if n == 0 {
		return false
	}
	index := 0
	ch := data[0]
	if ch == '-' || ch == '+' {
		index++
		ch = charAt(data, index)
	}

	// digits scans data[index:] for digits accepted by ok, skipping "_"; it needs
	// one digit and must not end on "_".
	digits := func(ok func(byte) bool) bool {
		hasDigits := false
		for ; index < n; index++ {
			ch = data[index]
			if ch == '_' {
				continue
			}
			if !ok(ch) {
				return false
			}
			hasDigits = true
		}
		return hasDigits && ch != '_'
	}

	if ch == '0' {
		if index+1 == n {
			return true
		}
		index++
		ch = data[index]
		switch ch {
		case 'b':
			index++
			return digits(func(c byte) bool { return c == '0' || c == '1' })
		case 'x':
			index++
			return digits(isHexDigit)
		}
		return digits(isOctDigit)
	}

	// Base 10 (except 0) or base 60; the value must not start with "_".
	if ch == '_' {
		return false
	}
	hasDigits := false
	for ; index < n; index++ {
		ch = data[index]
		if ch == '_' {
			continue
		}
		if ch == ':' {
			break
		}
		if !isDecDigit(ch) {
			return false
		}
		hasDigits = true
	}
	if !hasDigits || ch == '_' {
		return false
	}
	if ch != ':' {
		return true
	}
	return base60Tail.MatchString(data[index:])
}

func constructYAMLInteger(data string) float64 {
	value := strings.ReplaceAll(data, "_", "")
	sign := 1.0
	if value[0] == '-' || value[0] == '+' {
		if value[0] == '-' {
			sign = -1
		}
		value = value[1:]
	}
	// "-0" is 0, not -0: this test comes before the sign is applied.
	if value == "0" {
		return 0
	}
	if value[0] == '0' {
		switch charAt(value, 1) {
		case 'b':
			return sign * parseIntDigits(value[2:], 2)
		case 'x':
			return sign * parseIntDigits(value[2:], 16)
		}
		return sign * parseIntDigits(value, 8)
	}
	if strings.Contains(value, ":") {
		return sign * sexagesimal(value, func(part string) float64 { return parseIntDigits(part, 10) })
	}
	return sign * parseIntDigits(value, 10)
}

// sexagesimal adds up base-60 parts, least significant last, in the float arithmetic
// of int.js and float.js.
func sexagesimal(value string, parse func(string) float64) float64 {
	parts := strings.Split(value, ":")
	result, base := 0.0, 1.0
	for i := len(parts) - 1; i >= 0; i-- {
		// float64() keeps the product from being fused into an FMA.
		result += float64(parse(parts[i]) * base)
		base *= 60
	}
	return result
}

// parseIntDigits is parseInt(digits, radix) for a string of valid digits: V8 rounds
// the exact value to the nearest double, ties to even, as big.Float does.
func parseIntDigits(digits string, radix int) float64 {
	if radix == 10 {
		f, err := strconv.ParseFloat(digits, 64)
		if err == nil || isRangeErr(err) {
			return f
		}
		return math.NaN()
	}
	n, ok := new(big.Int).SetString(digits, radix)
	if !ok {
		return math.NaN()
	}
	f, _ := new(big.Float).SetInt(n).Float64()
	return f
}

// isRangeErr reports whether err is strconv's out-of-range error, for which
// ParseFloat still returns the value JavaScript produces (±Infinity or 0).
func isRangeErr(err error) bool {
	ne, ok := err.(*strconv.NumError)
	return ok && ne.Err == strconv.ErrRange
}

var yamlFloatPattern = regexp.MustCompile(`^(?:` +
	// 2.5e4, 2.5 and integers
	`[-+]?(?:0|[1-9][0-9_]*)(?:\.[0-9_]*)?(?:[eE][-+]?[0-9]+)?` +
	// .2e4, .2 (special case, seems not from spec)
	`|\.[0-9_]+(?:[eE][-+]?[0-9]+)?` +
	// 20:59
	`|[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*` +
	// .inf
	`|[-+]?\.(?:inf|Inf|INF)` +
	// .nan
	`|\.(?:nan|NaN|NAN))$`)

// floatType is lib/js-yaml/type/float.js.
var floatType = &yamlType{
	tag:  "tag:yaml.org,2002:float",
	kind: kindScalar,
	resolve: func(data any) bool {
		if data == nil {
			return false
		}
		// The pattern is tested against String(data); only for a string can the
		// "ends with _" check fail.
		if !yamlFloatPattern.MatchString(js.ToString(data)) {
			return false
		}
		s, ok := data.(string)
		return !ok || s[len(s)-1] != '_'
	},
	construct: func(data any) (any, error) {
		s, ok := data.(string)
		if !ok {
			// float.js calls data.replace, which only strings have.
			return nil, js.NewTypeError("data.replace is not a function")
		}
		return constructYAMLFloat(s), nil
	},
}

func constructYAMLFloat(data string) float64 {
	value := strings.ToLower(strings.ReplaceAll(data, "_", ""))
	sign := 1.0
	if value[0] == '-' {
		sign = -1
	}
	if value[0] == '-' || value[0] == '+' {
		value = value[1:]
	}
	switch {
	case value == ".inf":
		return sign * math.Inf(1)
	case value == ".nan":
		return math.NaN()
	case strings.Contains(value, ":"):
		return sign * sexagesimal(value, parseFloatPrefix)
	}
	return sign * parseFloatPrefix(value)
}

// parseFloatPrefix is parseFloat for the unsigned decimal literals the float pattern
// admits: it reads the longest digits[.digits][e[+-]digits] prefix.
func parseFloatPrefix(s string) float64 {
	i, digits := 0, 0
	for i < len(s) && isDecDigit(s[i]) {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isDecDigit(s[i]) {
			i++
			digits++
		}
	}
	if digits == 0 {
		return math.NaN()
	}
	if i < len(s) && s[i] == 'e' {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		if j < len(s) && isDecDigit(s[j]) {
			for j < len(s) && isDecDigit(s[j]) {
				j++
			}
			i = j
		}
	}
	f, err := strconv.ParseFloat(s[:i], 64)
	if err != nil && !isRangeErr(err) {
		return math.NaN()
	}
	return f
}

var (
	yamlDateRegexp      = regexp.MustCompile(`^([0-9][0-9][0-9][0-9])-([0-9][0-9])-([0-9][0-9])$`)
	yamlTimestampRegexp = regexp.MustCompile(`^([0-9][0-9][0-9][0-9])` + // [1] year
		`-([0-9][0-9]?)` + // [2] month
		`-([0-9][0-9]?)` + // [3] day
		`(?:[Tt]|[ \t]+)` +
		`([0-9][0-9]?)` + // [4] hour
		`:([0-9][0-9])` + // [5] minute
		`:([0-9][0-9])` + // [6] second
		`(?:\.([0-9]*))?` + // [7] fraction
		`(?:[ \t]*(Z|([-+])([0-9][0-9]?)` + // [8] tz [9] tz_sign [10] tz_hour
		`(?::([0-9][0-9]))?))?$`) // [11] tz_minute
)

// timestampType is lib/js-yaml/type/timestamp.js. The patterns are tested against
// String(data), but no number, boolean or Date string starts with "dddd-", so only
// strings can match.
var timestampType = &yamlType{
	tag:  "tag:yaml.org,2002:timestamp",
	kind: kindScalar,
	resolve: func(data any) bool {
		s, ok := data.(string)
		return ok && (yamlDateRegexp.MatchString(s) || yamlTimestampRegexp.MatchString(s))
	},
	construct: func(data any) (any, error) { return constructYAMLTimestamp(data.(string)), nil },
}

func constructYAMLTimestamp(data string) js.Date {
	match := yamlDateRegexp.FindStringSubmatch(data)
	if match == nil {
		match = yamlTimestampRegexp.FindStringSubmatch(data)
	}
	num := func(i int) int64 {
		n, _ := strconv.ParseInt(match[i], 10, 64) // the patterns admit only digits
		return n
	}
	year, month, day := num(1), num(2)-1, num(3) // JS months start at 0
	if len(match) < 5 || match[4] == "" {
		return js.Date{Ms: dateUTC(year, month, day, 0, 0, 0, 0)}
	}
	var fraction int64
	if f := match[7]; f != "" {
		f = (f + "00")[:3] // milliseconds
		fraction, _ = strconv.ParseInt(f, 10, 64)
	}
	ms := dateUTC(year, month, day, num(4), num(5), num(6), fraction)
	if match[9] != "" {
		var tzMinute int64
		if match[11] != "" {
			tzMinute = num(11)
		}
		delta := float64((num(10)*60 + tzMinute) * 60000)
		if match[9] == "-" {
			delta = -delta
		}
		ms = jsdate.TimeClip(ms - delta)
	}
	return js.Date{Ms: ms}
}

// dateUTC is Date.UTC for the integral fields of a timestamp.
func dateUTC(year, month, day, hour, minute, second, ms int64) float64 {
	return jsdate.UTC(float64(year), float64(month), float64(day), float64(hour), float64(minute), float64(second), float64(ms))
}

// mergeType is lib/js-yaml/type/merge.js: the "<<" key.
var mergeType = &yamlType{
	tag:       tagMerge,
	kind:      kindScalar,
	resolve:   func(data any) bool { return data == nil || data == "<<" },
	construct: identity,
}

func identity(data any) (any, error) { return data, nil }

// strType is lib/js-yaml/type/str.js. It returns non-null data unchanged, even when
// that is a number resolved from a plain scalar.
var strType = &yamlType{
	tag:     "tag:yaml.org,2002:str",
	kind:    kindScalar,
	resolve: func(any) bool { return true },
	construct: func(data any) (any, error) {
		if data == nil {
			return "", nil
		}
		return data, nil
	},
}

// base64Map is BASE64_MAP of binary.js: indexes 64, 65 and 66 are padding, CR and LF.
const base64Map = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=\n\r"

// binaryType is lib/js-yaml/type/binary.js; it constructs a Node Buffer, here []byte.
var binaryType = &yamlType{
	tag:  "tag:yaml.org,2002:binary",
	kind: kindScalar,
	resolve: func(data any) bool {
		if data == nil {
			return false
		}
		s, ok := data.(string)
		if !ok {
			// data.length is undefined, so the loop never runs and 0 bits remain.
			return true
		}
		bitlen := 0
		for i := 0; i < len(s); i++ {
			code := strings.IndexByte(base64Map, s[i])
			if code > 64 {
				continue // CR and LF
			}
			if code < 0 {
				return false
			}
			// "=" counts as six bits too.
			bitlen += 6
		}
		return bitlen%8 == 0
	},
	construct: func(data any) (any, error) {
		s, ok := data.(string)
		if !ok {
			return nil, js.NewTypeError("data.replace is not a function")
		}
		return constructYAMLBinary(s), nil
	},
}

func constructYAMLBinary(data string) []byte {
	input := strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '=' {
			return -1
		}
		return r
	}, data)
	var bits uint32
	result := []byte{}
	for i := 0; i < len(input); i++ {
		if i%4 == 0 && i != 0 {
			result = append(result, byte(bits>>16), byte(bits>>8), byte(bits))
		}
		bits = bits<<6 | uint32(strings.IndexByte(base64Map, input[i]))
	}
	switch len(input) % 4 * 6 {
	case 0:
		result = append(result, byte(bits>>16), byte(bits>>8), byte(bits))
	case 18:
		result = append(result, byte(bits>>10), byte(bits>>2))
	case 12:
		result = append(result, byte(bits>>4))
	}
	return result
}
