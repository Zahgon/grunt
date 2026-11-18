package lodash

import (
	"math"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/lodash/jsregex"
	"github.com/gruntjs/grunt-go/js"
)

// maxSafeInteger is Number.MAX_SAFE_INTEGER, lodash's MAX_SAFE_INTEGER.
const maxSafeInteger = 9007199254740991

// Character ranges and regular expressions of lodash 4.18.1, composed exactly as
// lodash composes them so that jsregex matches the same words and symbols.
const (
	rsAstralRange         = `\ud800-\udfff`
	rsComboMarksRange     = `\u0300-\u036f`
	reComboHalfMarksRange = `\ufe20-\ufe2f`
	rsComboSymbolsRange   = `\u20d0-\u20ff`
	rsComboRange          = rsComboMarksRange + reComboHalfMarksRange + rsComboSymbolsRange
	rsDingbatRange        = `\u2700-\u27bf`
	rsLowerRange          = `a-z\xdf-\xf6\xf8-\xff`
	rsMathOpRange         = `\xac\xb1\xd7\xf7`
	rsNonCharRange        = `\x00-\x2f\x3a-\x40\x5b-\x60\x7b-\xbf`
	rsPunctuationRange    = `\u2000-\u206f`
	rsSpaceRange          = ` \t\x0b\f\xa0\ufeff\n\r\u2028\u2029\u1680\u180e\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u202f\u205f\u3000`
	rsUpperRange          = `A-Z\xc0-\xd6\xd8-\xde`
	rsVarRange            = `\ufe0e\ufe0f`
	rsBreakRange          = rsMathOpRange + rsNonCharRange + rsPunctuationRange + rsSpaceRange

	rsApos      = "['\u2019]"
	rsAstral    = `[` + rsAstralRange + `]`
	rsBreak     = `[` + rsBreakRange + `]`
	rsCombo     = `[` + rsComboRange + `]`
	rsDigits    = `\d+`
	rsDingbat   = `[` + rsDingbatRange + `]`
	rsLower     = `[` + rsLowerRange + `]`
	rsMisc      = `[^` + rsAstralRange + rsBreakRange + rsDigits + rsDingbatRange + rsLowerRange + rsUpperRange + `]`
	rsFitz      = `\ud83c[\udffb-\udfff]`
	rsModifier  = `(?:` + rsCombo + `|` + rsFitz + `)`
	rsNonAstral = `[^` + rsAstralRange + `]`
	rsRegional  = `(?:\ud83c[\udde6-\uddff]){2}`
	rsSurrPair  = `[\ud800-\udbff][\udc00-\udfff]`
	rsUpper     = `[` + rsUpperRange + `]`
	rsZWJ       = `\u200d`

	rsMiscLower     = `(?:` + rsLower + `|` + rsMisc + `)`
	rsMiscUpper     = `(?:` + rsUpper + `|` + rsMisc + `)`
	rsOptContrLower = `(?:` + rsApos + `(?:d|ll|m|re|s|t|ve))?`
	rsOptContrUpper = `(?:` + rsApos + `(?:D|LL|M|RE|S|T|VE))?`
	reOptMod        = rsModifier + `?`
	rsOptVar        = `[` + rsVarRange + `]?`
	rsOptJoin       = `(?:` + rsZWJ + `(?:` + rsNonAstral + `|` + rsRegional + `|` + rsSurrPair + `)` + rsOptVar + reOptMod + `)*`
	rsOrdLower      = `\d*(?:1st|2nd|3rd|(?![123])\dth)(?=\b|[A-Z_])`
	rsOrdUpper      = `\d*(?:1ST|2ND|3RD|(?![123])\dTH)(?=\b|[a-z_])`
	rsSeq           = rsOptVar + reOptMod + rsOptJoin
	rsEmoji         = `(?:` + rsDingbat + `|` + rsRegional + `|` + rsSurrPair + `)` + rsSeq
	rsSymbol        = `(?:` + rsNonAstral + rsCombo + `?|` + rsCombo + `|` + rsRegional + `|` + rsSurrPair + `|` + rsAstral + `)`
)

var (
	reAsciiWord = jsregex.MustCompile(`[^\x00-\x2f\x3a-\x40\x5b-\x60\x7b-\x7f]+`, "g")
	// reUnicode matches one user-perceived symbol (stringToArray, stringSize).
	reUnicode = jsregex.MustCompile(rsFitz+`(?=`+rsFitz+`)|`+rsSymbol+rsSeq, "g")
	// reUnicodeWord matches the words of compound or non-ASCII strings.
	reUnicodeWord = jsregex.MustCompile(strings.Join([]string{
		rsUpper + `?` + rsLower + `+` + rsOptContrLower + `(?=` + rsBreak + `|` + rsUpper + `|$)`,
		rsMiscUpper + `+` + rsOptContrUpper + `(?=` + rsBreak + `|` + rsUpper + rsMiscLower + `|$)`,
		rsUpper + `?` + rsMiscLower + `+` + rsOptContrLower,
		rsUpper + `+` + rsOptContrUpper,
		rsOrdUpper,
		rsOrdLower,
		rsDigits,
		rsEmoji,
	}, "|"), "g")
	reHasUnicode     = jsregex.MustCompile(`[`+rsZWJ+rsAstralRange+rsComboRange+rsVarRange+`]`, "")
	reHasUnicodeWord = jsregex.MustCompile(`[a-z][A-Z]|[A-Z]{2}[a-z]|[0-9][a-zA-Z]|[a-zA-Z][0-9]|[^a-zA-Z0-9 ]`, "")
)

// deburredLetters is lodash's table of Latin-1 Supplement and Latin Extended-A
// letters with their basic Latin equivalents.
var deburredLetters = map[rune]string{
	'\xc0': "A", '\xc1': "A", '\xc2': "A", '\xc3': "A", '\xc4': "A", '\xc5': "A",
	'\xe0': "a", '\xe1': "a", '\xe2': "a", '\xe3': "a", '\xe4': "a", '\xe5': "a",
	'\xc7': "C", '\xe7': "c",
	'\xd0': "D", '\xf0': "d",
	'\xc8': "E", '\xc9': "E", '\xca': "E", '\xcb': "E",
	'\xe8': "e", '\xe9': "e", '\xea': "e", '\xeb': "e",
	'\xcc': "I", '\xcd': "I", '\xce': "I", '\xcf': "I",
	'\xec': "i", '\xed': "i", '\xee': "i", '\xef': "i",
	'\xd1': "N", '\xf1': "n",
	'\xd2': "O", '\xd3': "O", '\xd4': "O", '\xd5': "O", '\xd6': "O", '\xd8': "O",
	'\xf2': "o", '\xf3': "o", '\xf4': "o", '\xf5': "o", '\xf6': "o", '\xf8': "o",
	'\xd9': "U", '\xda': "U", '\xdb': "U", '\xdc': "U",
	'\xf9': "u", '\xfa': "u", '\xfb': "u", '\xfc': "u",
	'\xdd': "Y", '\xfd': "y", '\xff': "y",
	'\xc6': "Ae", '\xe6': "ae",
	'\xde': "Th", '\xfe': "th",
	'\xdf':   "ss",
	'\u0100': "A", '\u0102': "A", '\u0104': "A",
	'\u0101': "a", '\u0103': "a", '\u0105': "a",
	'\u0106': "C", '\u0108': "C", '\u010a': "C", '\u010c': "C",
	'\u0107': "c", '\u0109': "c", '\u010b': "c", '\u010d': "c",
	'\u010e': "D", '\u0110': "D", '\u010f': "d", '\u0111': "d",
	'\u0112': "E", '\u0114': "E", '\u0116': "E", '\u0118': "E", '\u011a': "E",
	'\u0113': "e", '\u0115': "e", '\u0117': "e", '\u0119': "e", '\u011b': "e",
	'\u011c': "G", '\u011e': "G", '\u0120': "G", '\u0122': "G",
	'\u011d': "g", '\u011f': "g", '\u0121': "g", '\u0123': "g",
	'\u0124': "H", '\u0126': "H", '\u0125': "h", '\u0127': "h",
	'\u0128': "I", '\u012a': "I", '\u012c': "I", '\u012e': "I", '\u0130': "I",
	'\u0129': "i", '\u012b': "i", '\u012d': "i", '\u012f': "i", '\u0131': "i",
	'\u0134': "J", '\u0135': "j",
	'\u0136': "K", '\u0137': "k", '\u0138': "k",
	'\u0139': "L", '\u013b': "L", '\u013d': "L", '\u013f': "L", '\u0141': "L",
	'\u013a': "l", '\u013c': "l", '\u013e': "l", '\u0140': "l", '\u0142': "l",
	'\u0143': "N", '\u0145': "N", '\u0147': "N", '\u014a': "N",
	'\u0144': "n", '\u0146': "n", '\u0148': "n", '\u014b': "n",
	'\u014c': "O", '\u014e': "O", '\u0150': "O",
	'\u014d': "o", '\u014f': "o", '\u0151': "o",
	'\u0154': "R", '\u0156': "R", '\u0158': "R",
	'\u0155': "r", '\u0157': "r", '\u0159': "r",
	'\u015a': "S", '\u015c': "S", '\u015e': "S", '\u0160': "S",
	'\u015b': "s", '\u015d': "s", '\u015f': "s", '\u0161': "s",
	'\u0162': "T", '\u0164': "T", '\u0166': "T",
	'\u0163': "t", '\u0165': "t", '\u0167': "t",
	'\u0168': "U", '\u016a': "U", '\u016c': "U", '\u016e': "U", '\u0170': "U", '\u0172': "U",
	'\u0169': "u", '\u016b': "u", '\u016d': "u", '\u016f': "u", '\u0171': "u", '\u0173': "u",
	'\u0174': "W", '\u0175': "w",
	'\u0176': "Y", '\u0177': "y", '\u0178': "Y",
	'\u0179': "Z", '\u017b': "Z", '\u017d': "Z",
	'\u017a': "z", '\u017c': "z", '\u017e': "z",
	'\u0132': "IJ", '\u0133': "ij",
	'\u0152': "Oe", '\u0153': "oe",
	'\u0149': "'n", '\u017f': "s",
}

// lodashStringEntries are lodash's string functions on `_` that underscore.string
// does not replace.
func lodashStringEntries() []entry {
	return []entry{
		{"camelCase", 1, compounder(func(result, word string, index int) string {
			word = toLowerCase(word)
			if index > 0 {
				word = capitalizeWord(word)
			}
			return result + word
		})},
		{"deburr", 1, func(a []any) (any, error) { return deburr(lodashToString(js.Arg(a, 0))), nil }},
		{"escape", 1, func(a []any) (any, error) { return escapeHTML(lodashToString(js.Arg(a, 0))), nil }},
		{"kebabCase", 1, compounder(joinWords("-", toLowerCase))},
		{"lowerCase", 1, compounder(joinWords(" ", toLowerCase))},
		{"lowerFirst", 1, func(a []any) (any, error) { return caseFirst(lodashToString(js.Arg(a, 0)), toLowerCase), nil }},
		{"padEnd", 3, func(a []any) (any, error) { return padSide(a, false) }},
		{"padStart", 3, func(a []any) (any, error) { return padSide(a, true) }},
		{"parseInt", 3, lodashParseInt},
		{"replace", 0, lodashReplace},
		{"snakeCase", 1, compounder(joinWords("_", toLowerCase))},
		{"split", 3, lodashSplit},
		{"startCase", 1, compounder(joinWords(" ", func(word string) string { return caseFirst(word, toUpperCase) }))},
		{"toInteger", 1, func(a []any) (any, error) { return toInteger(js.Arg(a, 0)), nil }},
		{"toLower", 1, func(a []any) (any, error) { return toLowerCase(lodashToString(js.Arg(a, 0))), nil }},
		{"toString", 1, func(a []any) (any, error) { return lodashToString(js.Arg(a, 0)), nil }},
		{"toUpper", 1, func(a []any) (any, error) { return toUpperCase(lodashToString(js.Arg(a, 0))), nil }},
		{"trimEnd", 3, func(a []any) (any, error) { return trimSide(a, false), nil }},
		{"trimStart", 3, func(a []any) (any, error) { return trimSide(a, true), nil }},
		{"unescape", 1, func(a []any) (any, error) { return unescapeHTML(lodashToString(js.Arg(a, 0))), nil }},
		{"upperCase", 1, compounder(joinWords(" ", toUpperCase))},
		{"upperFirst", 1, func(a []any) (any, error) { return caseFirst(lodashToString(js.Arg(a, 0)), toUpperCase), nil }},
	}
}

// compounder is createCompounder: the words of the deburred string, apostrophes
// removed, folded into a result with callback.
func compounder(callback func(result, word string, index int) string) func([]any) (any, error) {
	return func(a []any) (any, error) {
		// .replace(reApos, '') drops both apostrophes of rsApos.
		s := aposRemover.Replace(deburr(lodashToString(js.Arg(a, 0))))
		result := ""
		for i, word := range words(s) {
			result = callback(result, word, i)
		}
		return result, nil
	}
}

var aposRemover = strings.NewReplacer("'", "", "\u2019", "")

// joinWords is the compounder callback that joins words with sep after convert.
func joinWords(sep string, convert func(string) string) func(result, word string, index int) string {
	return func(result, word string, index int) string {
		if index > 0 {
			result += sep
		}
		return result + convert(word)
	}
}

// words is lodash's internal words without a pattern: the Unicode word regexp for
// strings that need it (compounds, digits next to letters, non-ASCII), the ASCII one
// otherwise.
func words(s string) []string {
	if reHasUnicodeWord.Test(s) {
		return reUnicodeWord.MatchAll(s)
	}
	return reAsciiWord.MatchAll(s)
}

// capitalizeWord is lodash's capitalize, which camelCase uses internally (the `_`
// method of that name is underscore.string's): lowercase, then upperFirst.
func capitalizeWord(s string) string {
	return caseFirst(toLowerCase(s), toUpperCase)
}

// deburr is lodash's deburr: Latin-1 Supplement and Latin Extended-A letters
// become basic Latin and combining diacritical marks are removed.
func deburr(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case isLatinLetter(r):
			// deburrLetter is basePropertyOf(deburredLetters): a letter the table lacks
			// would read undefined; the table covers the whole reLatin range.
			b.WriteString(deburredLetters[r])
		case isComboMark(r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isLatinLetter matches reLatin, /[\xc0-\xd6\xd8-\xf6\xf8-\xff\u0100-\u017f]/.
func isLatinLetter(r rune) bool {
	return (r >= 0xc0 && r <= 0x17f) && r != 0xd7 && r != 0xf7
}

// isComboMark matches reComboMark, lodash's combining-mark ranges.
func isComboMark(r rune) bool {
	return (r >= 0x300 && r <= 0x36f) || (r >= 0xfe20 && r <= 0xfe2f) || (r >= 0x20d0 && r <= 0x20ff)
}

// escapeHTML is lodash's escape: &, <, >, " and ' become HTML entities.
func escapeHTML(s string) string {
	return htmlEscaper.Replace(s)
}

var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")

// unescapeHTML is lodash's unescape: only the five entities escape produces are
// decoded, left to right without rescanning.
func unescapeHTML(s string) string {
	return htmlUnescaper.Replace(s)
}

var htmlUnescaper = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'")

// hasUnicode reports whether s has a zero-width joiner, an astral character, a
// combining mark or a variation selector, which make lodash split it by symbol.
func hasUnicode(s string) bool { return reHasUnicode.Test(s) }

// stringToArray splits s into symbols: grapheme-like clusters from reUnicode when
// hasUnicode, otherwise code units.
func stringToArray(s string) []string {
	if hasUnicode(s) {
		return reUnicode.MatchAll(s)
	}
	u := jsstr.ToUTF16(s)
	out := make([]string, len(u))
	for i := range u {
		out[i] = unitSlice(u, i, i+1)
	}
	return out
}

// stringSize is lodash's stringSize: the number of symbols of s.
func stringSize(s string) int {
	if hasUnicode(s) {
		return len(reUnicode.MatchAll(s))
	}
	return jsstr.Length(s)
}

// caseFirst is createCaseFirst: convert applied to the first symbol of s.
func caseFirst(s string, convert func(string) string) string {
	if hasUnicode(s) {
		symbols := stringToArray(s)
		return convert(symbols[0]) + strings.Join(symbols[1:], "")
	}
	u := jsstr.ToUTF16(s)
	if len(u) == 0 {
		return ""
	}
	return convert(unitSlice(u, 0, 1)) + unitSlice(u, 1, len(u))
}

// padSide is padStart (start) or padEnd: string, length and chars as lodash reads
// them, padding to length symbols.
func padSide(a []any, start bool) (any, error) {
	s := lodashToString(js.Arg(a, 0))
	length := toInteger(js.Arg(a, 1))
	if length == 0 {
		return s, nil
	}
	size := float64(stringSize(s))
	if size >= length {
		return s, nil
	}
	padding, err := createPadding(length-size, js.Arg(a, 2))
	if err != nil {
		return nil, err
	}
	if start {
		return padding + s, nil
	}
	return s + padding, nil
}

// createPadding is lodash's createPadding: chars (default " ") repeated and cut to
// length symbols.
func createPadding(length float64, chars any) (string, error) {
	c := " "
	if _, undef := chars.(js.UndefinedType); !undef {
		c = baseToString(chars)
	}
	charsLength := jsstr.Length(c)
	if charsLength < 2 {
		if charsLength == 0 {
			return c, nil
		}
		return baseRepeat(c, length)
	}
	result, err := baseRepeat(c, math.Ceil(length/float64(stringSize(c))))
	if err != nil {
		return "", err
	}
	if hasUnicode(c) {
		symbols := stringToArray(result)
		return strings.Join(symbols[:prefixLen(len(symbols), length)], ""), nil
	}
	u := jsstr.ToUTF16(result)
	return unitSlice(u, 0, prefixLen(len(u), length)), nil
}

// prefixLen is min(n, length) for a slice end given as a (possibly huge) number.
func prefixLen(n int, length float64) int {
	if float64(n) <= length {
		return n
	}
	return int(length)
}

// baseRepeat is lodash's baseRepeat: s repeated n times, "" outside 1..2^53-1. A
// result longer than V8's string limit is the RangeError V8 throws while doubling.
func baseRepeat(s string, n float64) (string, error) {
	if s == "" || n < 1 || n > maxSafeInteger {
		return "", nil
	}
	return stringRepeat(s, n)
}

// trimSide is trimStart (start) or trimEnd. Without chars (or with the guard
// argument) it trims ECMAScript white space; otherwise it trims the symbols of
// baseToString(chars), so a null chars argument trims the letters of "null".
func trimSide(a []any, start bool) string {
	s := lodashToString(js.Arg(a, 0))
	chars, guard := js.Arg(a, 1), js.Arg(a, 2)
	if _, undef := chars.(js.UndefinedType); s != "" && (js.ToBoolean(guard) || undef) {
		if start {
			return strings.TrimLeftFunc(s, js.IsSpace)
		}
		return strings.TrimRightFunc(s, js.IsSpace)
	}
	if s == "" {
		return s
	}
	set := baseToString(chars)
	if set == "" {
		return s
	}
	symbols, trim := stringToArray(s), stringToArray(set)
	inSet := func(sym string) bool {
		for _, t := range trim {
			if t == sym {
				return true
			}
		}
		return false
	}
	if start {
		i := 0
		for i < len(symbols) && inSet(symbols[i]) {
			i++
		}
		return strings.Join(symbols[i:], "")
	}
	end := len(symbols)
	for end > 0 && inSet(symbols[end-1]) {
		end--
	}
	return strings.Join(symbols[:end], "")
}

// lodashReplace is lodash's replace: with fewer than three arguments the string
// itself, otherwise String.prototype.replace(pattern, replacement).
func lodashReplace(a []any) (any, error) {
	s := lodashToString(js.Arg(a, 0))
	if len(a) < 3 {
		return s, nil
	}
	return stringReplace(s, a[1], a[2])
}

// stringReplace is String.prototype.replace: a RegExp pattern replaces its matches,
// anything else is converted to a string and replaced once.
func stringReplace(s string, pattern, replacement any) (any, error) {
	if re, ok := pattern.(*js.RegExp); ok {
		compiled, err := regexpOf(re)
		if err != nil {
			return nil, err
		}
		return compiled.Replace(s, replacement)
	}
	return jsregex.ReplaceString(s, js.ToString(pattern), replacement)
}

// lodashSplit is lodash's split.
func lodashSplit(a []any) (any, error) {
	str, separator, limit := js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2)
	// The guard makes split usable as an iteratee of _.map.
	if _, isNum := limit.(float64); js.ToBoolean(limit) && !isNum && isIterateeCall(str, separator, limit) {
		separator, limit = js.Undefined, js.Undefined
	}
	lim := uint32(js.MaxArrayLength)
	if _, undef := limit.(js.UndefinedType); !undef {
		lim = js.ToUint32(js.ToNumber(limit))
	}
	if lim == 0 {
		return js.NewArray(), nil
	}
	s := lodashToString(str)
	if _, isRe := separator.(*js.RegExp); s != "" && !js.IsNullish(separator) && !isRe {
		separator = baseToString(separator)
		if separator == "" && hasUnicode(s) {
			symbols := stringToArray(s)
			return stringArray(symbols[:min(uint32(len(symbols)), lim)]), nil
		}
	}
	return stringSplit(s, separator, lim)
}

// stringSplit is String.prototype.split(separator, limit): a RegExp splits by its
// matches, undefined gives the whole string, anything else splits by its string form.
func stringSplit(s string, separator any, limit uint32) (*js.Array, error) {
	switch sep := separator.(type) {
	case *js.RegExp:
		re, err := regexpOf(sep)
		if err != nil {
			return nil, err
		}
		return &js.Array{Items: re.Split(s, limit)}, nil
	case js.UndefinedType:
		if limit == 0 {
			return js.NewArray(), nil
		}
		return js.NewArray(s), nil
	}
	return stringArray(jsstr.Split(s, js.ToString(separator), limit)), nil
}

func stringArray(items []string) *js.Array {
	out := make([]any, len(items))
	for i, it := range items {
		out[i] = it
	}
	return &js.Array{Items: out}
}

// isIterateeCall is lodash's isIterateeCall: whether (value, index, object) look
// like the arguments an iteratee receives from _.map and friends.
func isIterateeCall(value, index, object any) bool {
	if !js.IsObject(object) {
		return false
	}
	switch i := index.(type) {
	case float64:
		length, ok := arrayLikeLength(object)
		if !ok || !(i > -1 && i == math.Trunc(i) && i < length) {
			return false
		}
		return js.SameValueZero(jsrt.GetProperty(object, js.NumberToString(i)), value)
	case string:
		if !jsrt.HasProperty(object, i) {
			return false
		}
		return js.SameValueZero(jsrt.GetProperty(object, i), value)
	}
	return false
}

// arrayLikeLength returns the length of an array-like value (lodash isArrayLike):
// an array, or a non-function object whose length is a valid array length.
func arrayLikeLength(v any) (float64, bool) {
	switch x := v.(type) {
	case *js.Array:
		return float64(len(x.Items)), true
	case *js.Object:
		n, ok := x.Get("length").(float64)
		if ok && n >= 0 && n == math.Trunc(n) && n <= maxSafeInteger {
			return n, true
		}
	}
	return 0, false
}
