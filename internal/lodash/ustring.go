package lodash

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsnum"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/lodash/jsregex"
	"github.com/gruntjs/grunt-go/js"
)

// ustringEntries are underscore.string 3.3.6's exports as _.mixin adds them to
// grunt.util._, aliases included. underscore.string's exports() also lists
// VERSION, which _.mixin skips because it is not a function, and exports itself,
// which lands on _ as _.exports but only makes sense called with `this` bound to
// the underscore.string object, so it is not provided. The lengths are the
// declared parameter counts of the original functions; sprintf and vsprintf are
// wrapped by util.deprecate, whose wrapper declares none.
//
// The originals index strings by UTF-16 code units, and so do these; strings are
// jsstr's WTF-8 form, so the lone halves of a split surrogate pair (chars('😀'), a
// mapChars callback, %c of a surrogate) survive as in JavaScript.
func ustringEntries() []entry {
	str1 := func(f func(s string) any) func([]any) (any, error) {
		return func(a []any) (any, error) { return f(makeString(js.Arg(a, 0))), nil }
	}
	return []entry{
		{"isBlank", 1, str1(func(s string) any { return isBlank(s) })},
		{"stripTags", 1, func(a []any) (any, error) { return reStripTags.Replace(makeString(js.Arg(a, 0)), "") }},
		{"capitalize", 2, func(a []any) (any, error) { return capitalize(js.Arg(a, 0), js.ToBoolean(js.Arg(a, 1))), nil }},
		{"decapitalize", 1, str1(func(s string) any { return decapitalize(s) })},
		{"chop", 2, chop},
		{"trim", 2, func(a []any) (any, error) { return trimChars(js.Arg(a, 0), js.Arg(a, 1), true, true) }},
		{"clean", 1, func(a []any) (any, error) { return clean(js.Arg(a, 0)) }},
		{"cleanDiacritics", 1, str1(func(s string) any { return cleanDiacritics(s) })},
		{"count", 2, count},
		{"chars", 1, str1(func(s string) any { return chars(s) })},
		{"swapCase", 1, str1(func(s string) any { return swapCase(s) })},
		{"escapeHTML", 1, str1(func(s string) any { return usEscapeHTML(s) })},
		{"unescapeHTML", 1, str1(func(s string) any { return usUnescapeHTML(s) })},
		{"splice", 4, func(a []any) (any, error) { return splice(js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2), js.Arg(a, 3)), nil }},
		{"insert", 3, func(a []any) (any, error) { return splice(js.Arg(a, 0), js.Arg(a, 1), 0.0, js.Arg(a, 2)), nil }},
		{"replaceAll", 4, replaceAll},
		{"lines", 1, lines},
		{"dedent", 2, dedent},
		{"startsWith", 3, startsWith},
		{"endsWith", 3, endsWith},
		{"pred", 1, str1(func(s string) any { return adjacent(s, -1) })},
		{"succ", 1, str1(func(s string) any { return adjacent(s, 1) })},
		{"titleize", 1, str1(func(s string) any { return titleize(s) })},
		{"camelize", 2, func(a []any) (any, error) { return camelize(js.Arg(a, 0), js.Arg(a, 1) == true) }},
		{"underscored", 1, func(a []any) (any, error) { return underscored(js.Arg(a, 0)) }},
		{"dasherize", 1, func(a []any) (any, error) { return dasherize(js.Arg(a, 0)) }},
		{"classify", 1, func(a []any) (any, error) { return classify(js.Arg(a, 0)) }},
		{"humanize", 1, func(a []any) (any, error) { return humanize(js.Arg(a, 0)) }},
		{"ltrim", 2, func(a []any) (any, error) { return trimChars(js.Arg(a, 0), js.Arg(a, 1), true, false) }},
		{"rtrim", 2, func(a []any) (any, error) { return trimChars(js.Arg(a, 0), js.Arg(a, 1), false, true) }},
		{"truncate", 3, truncate},
		{"prune", 3, prune},
		{"words", 2, usWords},
		{"pad", 4, func(a []any) (any, error) { return pad(js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2), js.Arg(a, 3)) }},
		{"lpad", 3, func(a []any) (any, error) { return pad(js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2), js.Undefined) }},
		{"rpad", 3, func(a []any) (any, error) { return pad(js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2), "right") }},
		{"lrpad", 3, func(a []any) (any, error) { return pad(js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2), "both") }},
		{"sprintf", 0, sprintf},
		{"vsprintf", 0, vsprintf},
		{"toNumber", 2, func(a []any) (any, error) { return toNumberPrecision(js.Arg(a, 0), js.Arg(a, 1)), nil }},
		{"numberFormat", 4, numberFormat},
		{"strRight", 2, func(a []any) (any, error) { return strSide(a, false, false), nil }},
		{"strRightBack", 2, func(a []any) (any, error) { return strSide(a, false, true), nil }},
		{"strLeft", 2, func(a []any) (any, error) { return strSide(a, true, false), nil }},
		{"strLeftBack", 2, func(a []any) (any, error) { return strSide(a, true, true), nil }},
		{"toSentence", 4, func(a []any) (any, error) { return toSentence(js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2), js.Arg(a, 3)) }},
		{"toSentenceSerial", 3, func(a []any) (any, error) { return toSentence(js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2), true) }},
		{"slugify", 1, func(a []any) (any, error) { return slugify(js.Arg(a, 0)) }},
		{"surround", 2, func(a []any) (any, error) { return surround(js.Arg(a, 0), js.Arg(a, 1)), nil }},
		{"quote", 2, quote},
		{"unquote", 2, unquote},
		{"repeat", 3, repeat},
		{"naturalCmp", 2, func(a []any) (any, error) { return naturalCmp(js.Arg(a, 0), js.Arg(a, 1)), nil }},
		{"levenshtein", 2, func(a []any) (any, error) {
			return levenshtein(makeString(js.Arg(a, 0)), makeString(js.Arg(a, 1))), nil
		}},
		{"toBoolean", 3, toBoolean},
		{"escapeRegExp", 1, str1(func(s string) any { return escapeRegExp(s) })},
		{"strip", 2, func(a []any) (any, error) { return trimChars(js.Arg(a, 0), js.Arg(a, 1), true, true) }},
		{"lstrip", 2, func(a []any) (any, error) { return trimChars(js.Arg(a, 0), js.Arg(a, 1), true, false) }},
		{"rstrip", 2, func(a []any) (any, error) { return trimChars(js.Arg(a, 0), js.Arg(a, 1), false, true) }},
		{"center", 3, func(a []any) (any, error) { return pad(js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2), "both") }},
		{"rjust", 3, func(a []any) (any, error) { return pad(js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2), js.Undefined) }},
		{"ljust", 3, func(a []any) (any, error) { return pad(js.Arg(a, 0), js.Arg(a, 1), js.Arg(a, 2), "right") }},
		{"q", 2, quote},
		{"toBool", 3, toBoolean},
		{"camelcase", 2, func(a []any) (any, error) { return camelize(js.Arg(a, 0), js.Arg(a, 1) == true) }},
		{"mapChars", 2, mapChars},
	}
}

// Patterns underscore.string uses as regular expression literals.
var (
	reStripTags     = jsregex.MustCompile(`<\/?[^>]+>`, "g")
	reBlank         = jsregex.MustCompile(`^\s*$`, "")
	reMultiSpace    = jsregex.MustCompile(`\s\s+`, "g")
	reAnyChar       = jsregex.MustCompile(`.`, "g")
	reNonSpace      = jsregex.MustCompile(`\S`, "g")
	reEntity        = jsregex.MustCompile(`\&([^;]{1,10});`, "g")
	reHexEntity     = jsregex.MustCompile(`^#x([\da-fA-F]+)$`, "")
	reDecEntity     = jsregex.MustCompile(`^#(\d+)$`, "")
	reLineBreak     = jsregex.MustCompile(`\r\n?|\n`, "")
	reIndent        = jsregex.MustCompile(`^[\s\\t]*`, "gm")
	reTitleStart    = jsregex.MustCompile(`(?:^|\s|-)\S`, "g")
	reCamelBreak    = jsregex.MustCompile(`[-_\s]+(.)?`, "g")
	reLowerUpper    = jsregex.MustCompile(`([a-z\d])([A-Z]+)`, "g")
	reDashSpace     = jsregex.MustCompile(`[-\s]+`, "g")
	reUpper         = jsregex.MustCompile(`([A-Z])`, "g")
	reSeparators    = jsregex.MustCompile(`[-_\s]+`, "g")
	reNonWord       = jsregex.MustCompile(`[\W_]`, "g")
	reSpace         = jsregex.MustCompile(`\s`, "g")
	reIDSuffix      = jsregex.MustCompile(`_id$`, "")
	reUnderscore    = jsregex.MustCompile(`_`, "g")
	rePruneTemplate = jsregex.MustCompile(`.(?=\W*\w*$)`, "g")
	reTwoWord       = jsregex.MustCompile(`\w\w`, "")
	reLastWord      = jsregex.MustCompile(`\s*\S+$`, "")
	reSpaces        = jsregex.MustCompile(`\s+`, "")
	reThousands     = jsregex.MustCompile(`(\d)(?=(?:\d{3})+$)`, "g")
	reNaturalToken  = jsregex.MustCompile(`(\.\d+|\d+|\D+)`, "g")
	reSlugInvalid   = jsregex.MustCompile(`[^\w\s-]`, "g")
)

// mustReplace applies an underscore.string literal pattern with a template
// replacement; with a string template the replacement cannot fail.
func mustReplace(re *jsregex.Regexp, s, template string) string {
	out, err := re.Replace(s, template)
	if err != nil {
		panic(err)
	}
	return out
}

func isBlank(s string) bool { return reBlank.Test(s) }

// capitalize upper-cases str.charAt(0) and, with lowercaseRest, lower-cases the
// rest. charAt works on code units, so an astral first character stays as it is.
func capitalize(v any, lowercaseRest bool) string {
	u := jsstr.ToUTF16(makeString(v))
	if len(u) == 0 {
		return ""
	}
	rest := u[1:]
	if lowercaseRest {
		rest = lowerUnits(rest)
	}
	return jsstr.FromUTF16(append(upperUnits(u[:1]), rest...))
}

func decapitalize(s string) string {
	u := jsstr.ToUTF16(s)
	if len(u) == 0 {
		return ""
	}
	return jsstr.FromUTF16(append(lowerUnits(u[:1]), u[1:]...))
}

// chop splits String(str) into pieces of ~~step characters with
// str.match(new RegExp('.{1,' + step + '}', 'g')): "." skips line terminators, and
// an empty string gives null.
func chop(a []any) (any, error) {
	v := js.Arg(a, 0)
	if js.IsNullish(v) {
		return js.NewArray(), nil
	}
	s := js.ToString(v)
	step := toInt32(js.Arg(a, 1))
	if step <= 0 {
		return js.NewArray(s), nil
	}
	re, err := jsregex.Compile(".{1,"+strconv.Itoa(step)+"}", "g")
	if err != nil {
		return nil, err
	}
	return matchArray(re.MatchAll(s)), nil
}

// matchArray turns a match list into an array, or null when it is nil.
func matchArray(items []string) any {
	if items == nil {
		return nil
	}
	arr := &js.Array{Items: make([]any, len(items))}
	for i, s := range items {
		arr.Items[i] = s
	}
	return arr
}

// trimChars is trim, ltrim and rtrim. Without characters (any falsy value) it is
// the native String.prototype.trim family; otherwise defaultToWhiteSpace turns the
// characters into a pattern (a RegExp's source, or a class of the escaped
// characters, so "a-z" is a range) and the matching ends are removed.
func trimChars(v, characters any, left, right bool) (string, error) {
	s := makeString(v)
	if !js.ToBoolean(characters) {
		return trimSpace(s, left, right), nil
	}
	chars := defaultToWhiteSpace(characters)
	var pattern, flags string
	switch {
	case left && right:
		pattern, flags = "^"+chars+"+|"+chars+"+$", "g"
	case left:
		pattern = "^" + chars + "+"
	default:
		pattern = chars + "+$"
	}
	re, err := jsregex.Compile(pattern, flags)
	if err != nil {
		return "", err
	}
	return re.Replace(s, "")
}

// defaultToWhiteSpace is underscore.string's helper: \s for null, the source of a
// value with a truthy .source (a RegExp, or an object with that key), otherwise a
// character class of the escaped characters.
func defaultToWhiteSpace(characters any) string {
	if js.IsNullish(characters) {
		return `\s`
	}
	switch c := characters.(type) {
	case *js.RegExp:
		return c.Source
	case *js.Object:
		if src := c.Get("source"); js.ToBoolean(src) {
			return js.ToString(src)
		}
	}
	return "[" + escapeRegExp(makeString(characters)) + "]"
}

// escapeRegExp backslash-escapes .*+?^=!:${}()|[]/\ (not "-").
func escapeRegExp(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`.*+?^=!:${}()|[]/\`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func clean(v any) (string, error) {
	s, err := trimChars(v, js.Undefined, true, true)
	if err != nil {
		return "", err
	}
	return reMultiSpace.Replace(s, " ")
}

// diacritics maps cleanDiacritics' from characters to their replacements: the
// lowercase table, its uppercase form and ß. The first occurrence wins, as with
// from.indexOf(c).
var diacritics = func() map[rune]string {
	from := "ąàáäâãåæăćčĉęèéëêĝĥìíïîĵłľńňòóöőôõðøśșşšŝťțţŭùúüűûñÿýçżźž"
	to := "aaaaaaaaaccceeeeeghiiiijllnnoooooooossssstttuuuuuunyyczzz"
	fromRunes := []rune(from + toUpperCase(from))
	toRunes := []rune(to + toUpperCase(to))
	m := map[rune]string{}
	for i, r := range fromRunes {
		if _, seen := m[r]; !seen {
			m[r] = string(toRunes[i])
		}
	}
	if _, seen := m['ß']; !seen {
		m['ß'] = "ss"
	}
	return m
}()

// cleanDiacritics replaces each character matched by /.{1}/g found in the table;
// line terminators are not matched by ".", and none is in the table anyway.
func cleanDiacritics(s string) string {
	var b strings.Builder
	for _, r := range s {
		if rep, ok := diacritics[r]; ok {
			b.WriteString(rep)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// count is str.split(substr).length - 1, with 0 for an empty string or substring.
func count(a []any) (any, error) {
	s, sub := makeString(js.Arg(a, 0)), makeString(js.Arg(a, 1))
	if s == "" || sub == "" {
		return 0.0, nil
	}
	return float64(len(jsstr.Split(s, sub, math.MaxUint32)) - 1), nil
}

// chars is str.split(”): one item per UTF-16 code unit.
func chars(s string) *js.Array {
	u := jsstr.ToUTF16(s)
	arr := &js.Array{Items: make([]any, len(u))}
	for i := range u {
		arr.Items[i] = jsstr.FromUTF16(u[i : i+1])
	}
	return arr
}

// swapCase replaces each non-space code unit c with c.toLowerCase() when c equals
// c.toUpperCase(), and with c.toUpperCase() otherwise.
func swapCase(s string) string {
	return jsstr.FromUTF16(replaceUnits(reNonSpace, jsstr.ToUTF16(s), func(c []uint16, _ []int) []uint16 {
		up := upperUnits(c)
		if slices.Equal(up, c) {
			return lowerUnits(c)
		}
		return up
	}))
}

// escapeChars are the characters escapeHTML replaces and their entity names.
var escapeChars = map[rune]string{
	'¢': "cent", '£': "pound", '¥': "yen", '€': "euro", '©': "copy", '®': "reg",
	'<': "lt", '>': "gt", '"': "quot", '&': "amp", '\'': "#39",
}

// usEscapeHTML is underscore.string's escapeHTML: the characters of escapeChars become
// their entities.
func usEscapeHTML(s string) string {
	var b strings.Builder
	for _, r := range s {
		if name, ok := escapeChars[r]; ok {
			b.WriteString("&" + name + ";")
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// htmlEntities are the entities unescapeHTML knows.
var htmlEntities = map[string]string{
	"nbsp": " ", "cent": "¢", "pound": "£", "yen": "¥", "euro": "€", "copy": "©",
	"reg": "®", "lt": "<", "gt": ">", "quot": "\"", "amp": "&", "apos": "'",
}

// usUnescapeHTML replaces &name; (up to ten code units between & and ;) with known
// entities and &#xHEX; / &#DEC; with String.fromCharCode of the code, which keeps
// only its low 16 bits.
func usUnescapeHTML(s string) string {
	return jsstr.FromUTF16(replaceUnits(reEntity, jsstr.ToUTF16(s), func(m []uint16, _ []int) []uint16 {
		code := jsstr.FromUTF16(m[1 : len(m)-1])
		if rep, ok := htmlEntities[code]; ok {
			return jsstr.ToUTF16(rep)
		}
		// `entityCode in htmlEntities` also finds what a plain object inherits;
		// the replacement is then String() of that value.
		if inherited, ok := jsrt.Inherited(js.NewObject(), code); ok {
			return jsstr.ToUTF16(js.ToString(inherited))
		}
		if reHexEntity.Test(code) {
			return []uint16{uint16(js.ToUint32(jsnum.ParseInt(code[2:], 16)))}
		}
		if reDecEntity.Test(code) {
			return []uint16{uint16(toInt32(code[1:]))}
		}
		return m
	}))
}

// splice is chars(str) spliced at ~~i, removing ~~howmany characters and inserting
// substr, then joined (null and undefined join as "").
func splice(str, i, howmany, substr any) string {
	u := jsstr.ToUTF16(makeString(str))
	n := len(u)
	start := toInt32(i)
	if start < 0 {
		start = max(n+start, 0)
	} else {
		start = min(start, n)
	}
	del := min(max(toInt32(howmany), 0), n-start)
	out := append([]uint16(nil), u[:start]...)
	out = append(out, jsstr.ToUTF16(makeString(substr))...)
	out = append(out, u[start+del:]...)
	return jsstr.FromUTF16(out)
}

// replaceAll is str.replace(new RegExp(find, ignorecase === true ? 'gi' : 'g'),
// replace): find is a pattern (a RegExp contributes its source; undefined is the
// empty pattern) and replace a template or a callback.
func replaceAll(a []any) (any, error) {
	flags := "g"
	if js.Arg(a, 3) == true {
		flags = "gi"
	}
	var source string
	switch find := js.Arg(a, 1).(type) {
	case *js.RegExp:
		source = find.Source
	case js.UndefinedType:
		source = ""
	default:
		source = js.ToString(find)
	}
	re, err := jsregex.Compile(source, flags)
	if err != nil {
		return nil, err
	}
	return re.Replace(makeString(js.Arg(a, 0)), js.Arg(a, 2))
}

// lines splits on \r\n, \r and \n; null and undefined give [].
func lines(a []any) (any, error) {
	v := js.Arg(a, 0)
	if js.IsNullish(v) {
		return js.NewArray(), nil
	}
	return &js.Array{Items: reLineBreak.Split(js.ToString(v), math.MaxUint32)}, nil
}

// dedent removes the common indentation. The indentation is measured with
// /^[\s\\t]*/gm, whose class also holds a backslash and the letter t, so those
// count as indentation too; it is then removed with /^[ \t]{n}/gm, or with
// new RegExp('^' + pattern, 'gm') when pattern is a string.
func dedent(a []any) (any, error) {
	s := makeString(js.Arg(a, 0))
	indent := -1
	for _, m := range reIndent.MatchAll(s) {
		if l := jsstr.Length(m); indent < 0 || l < indent {
			indent = l
		}
	}
	if indent == 0 {
		return s, nil
	}
	source := "^[ \\t]{" + strconv.Itoa(indent) + "}"
	if p, ok := js.Arg(a, 1).(string); ok {
		source = "^" + p
	}
	re, err := jsregex.Compile(source, "gm")
	if err != nil {
		return nil, err
	}
	return re.Replace(s, "")
}

// toPositive is underscore.string's helper: number < 0 ? 0 : (+number || 0).
func toPositive(v any) float64 {
	if lessThan(v, 0.0) {
		return 0
	}
	f := js.ToNumber(toPrimitiveNumber(v))
	if math.IsNaN(f) {
		return 0
	}
	return f
}

// startsWith is str.lastIndexOf(starts, position) === position with position
// clamped to 0..length; a fractional position never matches.
func startsWith(a []any) (any, error) {
	u := jsstr.ToUTF16(makeString(js.Arg(a, 0)))
	starts := jsstr.ToUTF16(js.ToString(js.Arg(a, 1)))
	position := 0.0
	if p := js.Arg(a, 2); !js.IsNullish(p) {
		position = math.Min(toPositive(p), float64(len(u)))
	}
	return float64(jsstr.LastIndex(u, starts, int(position))) == position, nil
}

// endsWith checks that ends occurs at position - ends.length, with position the
// string length when undefined and otherwise clamped to it.
func endsWith(a []any) (any, error) {
	u := jsstr.ToUTF16(makeString(js.Arg(a, 0)))
	ends := jsstr.ToUTF16(js.ToString(js.Arg(a, 1)))
	position := float64(len(u) - len(ends))
	if p := js.Arg(a, 2); p != js.Undefined {
		position = math.Min(toPositive(p), float64(len(u))) - float64(len(ends))
	}
	if !(position >= 0) {
		return false, nil
	}
	return float64(jsstr.Index(u, ends, int(position))) == position, nil
}

// adjacent is pred/succ: the last code unit moved by direction, wrapping as
// String.fromCharCode does.
func adjacent(s string, direction int) string {
	u := jsstr.ToUTF16(s)
	if len(u) == 0 {
		return ""
	}
	out := append([]uint16(nil), u...)
	out[len(out)-1] = uint16(int(out[len(out)-1]) + direction)
	return jsstr.FromUTF16(out)
}

// titleize lower-cases the string, then upper-cases each non-space character at
// the start or after white space or "-" (with the preceding character).
func titleize(s string) string {
	return jsstr.FromUTF16(replaceUnits(reTitleStart, lowerUnits(jsstr.ToUTF16(s)), func(m []uint16, _ []int) []uint16 {
		return upperUnits(m)
	}))
}

// camelize trims, then drops runs of - _ and white space, upper-casing the
// character after each run; decapitalize === true lowers the first character.
func camelize(v any, decap bool) (string, error) {
	s, err := trimChars(v, js.Undefined, true, true)
	if err != nil {
		return "", err
	}
	u := jsstr.ToUTF16(s)
	out := jsstr.FromUTF16(replaceUnits(reCamelBreak, u, func(_ []uint16, caps []int) []uint16 {
		if caps[2] < 0 {
			return nil
		}
		return upperUnits(u[caps[2]:caps[3]])
	}))
	if decap {
		return decapitalize(out), nil
	}
	return out, nil
}

func underscored(v any) (string, error) {
	s, err := trimChars(v, js.Undefined, true, true)
	if err != nil {
		return "", err
	}
	s = mustReplace(reLowerUpper, s, "$1_$2")
	return toLowerCase(mustReplace(reDashSpace, s, "_")), nil
}

func dasherize(v any) (string, error) {
	s, err := trimChars(v, js.Undefined, true, true)
	if err != nil {
		return "", err
	}
	s = mustReplace(reUpper, s, "-$1")
	return toLowerCase(mustReplace(reSeparators, s, "-")), nil
}

func classify(v any) (string, error) {
	s := mustReplace(reNonWord, makeString(v), " ")
	c, err := camelize(s, false)
	if err != nil {
		return "", err
	}
	return capitalize(mustReplace(reSpace, c, ""), false), nil
}

func humanize(v any) (string, error) {
	s, err := underscored(v)
	if err != nil {
		return "", err
	}
	s = mustReplace(reUnderscore, mustReplace(reIDSuffix, s, ""), " ")
	return capitalize(trimSpace(s, true, true), false), nil
}

// truncate cuts the string to ~~length code units (a negative length counts from
// the end, as slice does) and appends truncateStr || '...'.
func truncate(a []any) (any, error) {
	u := jsstr.ToUTF16(makeString(js.Arg(a, 0)))
	suffix := js.Arg(a, 2)
	if !js.ToBoolean(suffix) {
		suffix = "..."
	}
	length := toInt32(js.Arg(a, 1))
	if len(u) <= length {
		return jsstr.FromUTF16(u), nil
	}
	return jsstr.FromUTF16(u[:js.RelativeIndex(float64(length), len(u))]) + js.ToString(suffix), nil
}

// prune truncates without cutting a word: the original maps the first length+1
// characters to a template ("A" for cased characters, " " for others, but only
// where /.(?=\W*\w*$)/ holds), drops the last partial word, and keeps the original
// characters up to the template's length.
func prune(a []any) (any, error) {
	s := makeString(js.Arg(a, 0))
	u := jsstr.ToUTF16(s)
	length := toInt32(js.Arg(a, 1))
	pruneStr := "..."
	if p := js.Arg(a, 2); !js.IsNullish(p) {
		pruneStr = js.ToString(p)
	}
	if len(u) <= length {
		return s, nil
	}
	head := u[:js.RelativeIndex(float64(length)+1, len(u))]
	template := replaceUnits(rePruneTemplate, head, func(c []uint16, _ []int) []uint16 {
		if !slices.Equal(upperUnits(c), lowerUnits(c)) {
			return []uint16{'A'}
		}
		return []uint16{' '}
	})
	tail := jsstr.FromUTF16(template[js.RelativeIndex(float64(len(template)-2), len(template)):])
	if reTwoWord.Test(tail) {
		template = jsstr.ToUTF16(mustReplace(reLastWord, jsstr.FromUTF16(template), ""))
	} else {
		cut := template[:js.RelativeIndex(float64(len(template)-1), len(template))]
		template = jsstr.ToUTF16(trimSpace(jsstr.FromUTF16(cut), false, true))
	}
	if len(template)+jsstr.Length(pruneStr) > len(u) {
		return s, nil
	}
	return jsstr.FromUTF16(u[:len(template)]) + pruneStr, nil
}

// usWords is trim(str, delimiter).split(delimiter || /\s+/), or [] for a blank string.
func usWords(a []any) (any, error) {
	str, delimiter := js.Arg(a, 0), js.Arg(a, 1)
	if isBlank(makeString(str)) {
		return js.NewArray(), nil
	}
	s, err := trimChars(str, delimiter, true, true)
	if err != nil {
		return nil, err
	}
	switch d := delimiter.(type) {
	case *js.RegExp:
		re, err := regexpOf(d)
		if err != nil {
			return nil, err
		}
		return &js.Array{Items: re.Split(s, math.MaxUint32)}, nil
	}
	if !js.ToBoolean(delimiter) {
		return &js.Array{Items: reSpaces.Split(s, math.MaxUint32)}, nil
	}
	return stringArray(jsstr.Split(s, js.ToString(delimiter), math.MaxUint32)), nil
}

// pad pads str to ~~length with the first character of padStr (a space when padStr
// is falsy) on the left, right ("right") or both sides ("both", extra on the left).
func pad(str, length, padStr, typ any) (string, error) {
	s := makeString(str)
	n := toInt32(length) - jsstr.Length(s)
	switch {
	case !js.ToBoolean(padStr):
		padStr = " "
	default:
		l, err := propertyOf(padStr, "length")
		if err != nil {
			return "", err
		}
		if js.ToNumber(l) > 1 {
			p, ok := padStr.(string)
			if !ok {
				return "", notFunctionError("padStr.charAt")
			}
			padStr = jsstr.FromUTF16(jsstr.ToUTF16(p)[:1])
		}
	}
	switch typ {
	case "right":
		right, err := strRepeat(padStr, n)
		return s + right, err
	case "both":
		left, err := strRepeat(padStr, int(math.Ceil(float64(n)/2)))
		if err != nil {
			return "", err
		}
		right, err := strRepeat(padStr, int(math.Floor(float64(n)/2)))
		return left + s + right, err
	}
	left, err := strRepeat(padStr, n)
	return left + s, err
}

// strRepeat is underscore.string's strRepeat, which doubles its str argument with
// `str += str`: a number or boolean doubles numerically, so strRepeat(5, 3) is
// "5" + "10" = "510".
func strRepeat(str any, qty int) (string, error) {
	var b strings.Builder
	for qty > 0 {
		if qty&1 == 1 {
			piece := js.ToString(str)
			if b.Len()+len(piece) > js.MaxStringLength {
				return "", js.InvalidStringLength()
			}
			b.WriteString(piece)
		}
		qty >>= 1
		// `str += str` runs on every pass, the last one included.
		var err error
		if str, err = js.Add(str, str, toPrimitive); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

// toNumberPrecision is underscore.string's toNumber: num rounded with
// Math.round(num * 10^precision) / 10^precision (precision 0 unless finite).
func toNumberPrecision(num, precision any) float64 {
	if js.IsNullish(num) {
		return 0
	}
	p := js.ToNumber(toPrimitiveNumber(precision))
	if !isFinite(p) {
		p = 0
	}
	factor := jsnum.Pow10(p)
	return jsnum.Round(float64(js.ToNumber(toPrimitiveNumber(num))*factor)) / factor
}

// numberFormat formats with number.toFixed(~~dec), groups the integer digits with
// tsep (a string, default ","; it is part of a replacement template, so "$&" and
// the like expand) and uses dsep || "." as decimal point. NaN, null and undefined
// give ""; anything that is not a number primitive has no toFixed.
func numberFormat(a []any) (any, error) {
	number := js.Arg(a, 0)
	if math.IsNaN(js.ToNumber(toPrimitiveNumber(number))) || js.IsNullish(number) {
		return "", nil
	}
	x, ok := number.(float64)
	if !ok {
		return nil, notFunctionError("number.toFixed")
	}
	fixed, err := jsnum.ToFixed(x, float64(toInt32(js.Arg(a, 1))))
	if err != nil {
		return nil, err
	}
	tsep := ","
	if t, ok := js.Arg(a, 3).(string); ok {
		tsep = t
	}
	intPart, decPart, _ := strings.Cut(fixed, ".")
	decimals := ""
	if decPart != "" {
		dsep := js.Arg(a, 2)
		if !js.ToBoolean(dsep) {
			dsep = "."
		}
		decimals = js.ToString(dsep) + decPart
	}
	grouped, err := reThousands.Replace(intPart, "$1"+tsep)
	if err != nil {
		return nil, err
	}
	return grouped + decimals, nil
}

// strSide is strLeft, strLeftBack, strRight and strRightBack: the part before or
// after the first (or last) sep, or the whole string when sep is absent. An empty
// sep counts as absent, except for strLeftBack, which finds it at the end.
func strSide(a []any, left, back bool) string {
	u := jsstr.ToUTF16(makeString(js.Arg(a, 0)))
	sep := jsstr.ToUTF16(makeString(js.Arg(a, 1)))
	pos := -1
	switch {
	case back:
		if len(sep) > 0 || left {
			pos = jsstr.LastIndex(u, sep, len(u))
		}
	case len(sep) > 0:
		pos = jsstr.Index(u, sep, 0)
	}
	switch {
	case pos < 0:
		return jsstr.FromUTF16(u)
	case left:
		return jsstr.FromUTF16(u[:pos])
	}
	return jsstr.FromUTF16(u[pos+len(sep):])
}

// toSentence joins all but the last item with separator (default ", ") and adds
// lastSeparator (default " and ") and the last item; serial puts the trimmed
// separator before lastSeparator for three or more items. array must have slice
// and pop, i.e. be an array.
func toSentence(array, separator, lastSeparator, serial any) (any, error) {
	if !js.ToBoolean(separator) {
		separator = ", "
	}
	if !js.ToBoolean(lastSeparator) {
		lastSeparator = " and "
	}
	arr, ok := array.(*js.Array)
	switch {
	case js.IsNullish(array):
		return nil, readError(array, "slice")
	case !ok:
		if _, isString := array.(string); isString {
			// "abc".slice() is a string, which has no pop.
			return nil, notFunctionError("a.pop")
		}
		return nil, notFunctionError("array.slice")
	}
	if len(arr.Items) == 0 {
		return js.Undefined, nil
	}
	items := arr.Items[:len(arr.Items)-1]
	last := arr.Items[len(arr.Items)-1]
	if len(arr.Items) > 2 && js.ToBoolean(serial) {
		lastSeparator = trimSpace(makeString(separator), false, true) + js.ToString(lastSeparator)
	}
	if len(items) == 0 {
		return last, nil
	}
	return joinItems(items, js.ToString(separator)) + js.ToString(lastSeparator) + js.ToString(last), nil
}

// joinItems is Array.prototype.join: null and undefined items are empty.
func joinItems(items []any, sep string) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = makeString(it)
	}
	return strings.Join(parts, sep)
}

// surround is [wrapper, str, wrapper].join(”).
func surround(str, wrapper any) string {
	return makeString(wrapper) + makeString(str) + makeString(wrapper)
}

func quote(a []any) (any, error) {
	q := js.Arg(a, 1)
	if !js.ToBoolean(q) {
		q = `"`
	}
	return surround(js.Arg(a, 0), q), nil
}

// unquote strips quoteChar (default '"') when str[0] and str[str.length - 1] are
// both it; str is used as it is, so a non-string is returned unchanged unless it
// is an array with that shape, which is sliced.
func unquote(a []any) (any, error) {
	str, q := js.Arg(a, 0), js.Arg(a, 1)
	if !js.ToBoolean(q) {
		q = `"`
	}
	first, err := propertyOf(str, "0")
	if err != nil {
		return nil, err
	}
	if !js.StrictEquals(first, q) {
		return str, nil
	}
	l, err := propertyOf(str, "length")
	if err != nil {
		return nil, err
	}
	last, err := propertyOf(str, propertyKey(js.ToNumber(l)-1))
	if err != nil {
		return nil, err
	}
	if !js.StrictEquals(last, q) {
		return str, nil
	}
	switch x := str.(type) {
	case string:
		u := jsstr.ToUTF16(x)
		return jsstr.FromUTF16(u[1:max(len(u)-1, 1)]), nil
	case *js.Array:
		return &js.Array{Items: append([]any(nil), x.Items[1:max(len(x.Items)-1, 1)]...)}, nil
	}
	return nil, notFunctionError("str.slice")
}

// repeat repeats the string ~~qty times, joined by separator when one is given.
func repeat(a []any) (any, error) {
	s := makeString(js.Arg(a, 0))
	qty := toInt32(js.Arg(a, 1))
	sep := js.Arg(a, 2)
	if js.IsNullish(sep) {
		return strRepeat(s, qty)
	}
	if qty <= 0 {
		return "", nil
	}
	sepStr := js.ToString(sep)
	if jsstr.Length(s)*qty+jsstr.Length(sepStr)*(qty-1) > js.MaxStringLength {
		return nil, js.InvalidStringLength()
	}
	parts := make([]string, qty)
	for i := range parts {
		parts[i] = s
	}
	return strings.Join(parts, sepStr), nil
}

// naturalCmp compares with numbers in the strings ordered by value. It starts with
// loose equality (so 1 and "1" are equal) and ends with the < operator.
func naturalCmp(a, b any) float64 {
	if looseEquals(a, b) {
		return 0
	}
	if !js.ToBoolean(a) {
		return -1
	}
	if !js.ToBoolean(b) {
		return 1
	}
	t1 := reNaturalToken.MatchAll(js.ToString(a))
	t2 := reNaturalToken.MatchAll(js.ToString(b))
	for i := 0; i < len(t1) && i < len(t2); i++ {
		x, y := t1[i], t2[i]
		if x == y {
			continue
		}
		n1, n2 := js.StringToNumber(x), js.StringToNumber(y)
		if !math.IsNaN(n1) && !math.IsNaN(n2) {
			if n1 > n2 {
				return 1
			}
			return -1
		}
		if jsstr.Compare(x, y) < 0 {
			return -1
		}
		return 1
	}
	if len(t1) != len(t2) {
		return float64(len(t1) - len(t2))
	}
	if lessThan(a, b) {
		return -1
	}
	return 1
}

// levenshtein is the two-row edit distance over code units.
func levenshtein(s1, s2 string) float64 {
	if s1 == s2 {
		return 0
	}
	a, b := jsstr.ToUTF16(s1), jsstr.ToUTF16(s2)
	if len(a) == 0 || len(b) == 0 {
		return float64(max(len(a), len(b)))
	}
	prev := make([]int, len(b)+1)
	for i := range prev {
		prev[i] = i
	}
	next := 0
	for i := range a {
		next = i + 1
		for j := range b {
			cur := next
			next = prev[j]
			if a[i] != b[j] {
				next++
			}
			next = min(next, cur+1, prev[j+1]+1)
			prev[j] = cur
		}
		prev[len(b)] = next
	}
	return float64(next)
}

// toBoolean reads "true"/"1" and "false"/"0" (or the given matchers) from a
// trimmed string; numbers are read as strings, other non-strings give !!value, and
// an unmatched string gives undefined.
func toBoolean(a []any) (any, error) {
	v := js.Arg(a, 0)
	if f, ok := v.(float64); ok {
		v = js.NumberToString(f)
	}
	s, ok := v.(string)
	if !ok {
		return js.ToBoolean(v), nil
	}
	s = trimSpace(s, true, true)
	defaults := [2]any{js.NewArray("true", "1"), js.NewArray("false", "0")}
	for i, result := range [2]bool{true, false} {
		matchers := js.Arg(a, 1+i)
		if !js.ToBoolean(matchers) {
			matchers = defaults[i]
		}
		matched, err := boolMatch(s, matchers)
		if err != nil {
			return nil, err
		}
		if matched {
			return result, nil
		}
	}
	return js.Undefined, nil
}

// boolMatch tries each of [].concat(matchers): a RegExp by test, then (also for a
// RegExp that did not match) by matcher.toLowerCase() === s.toLowerCase(), which
// only strings support.
func boolMatch(s string, matchers any) (bool, error) {
	list := []any{matchers}
	if arr, ok := matchers.(*js.Array); ok {
		list = arr.Items
	}
	down := toLowerCase(s)
	for _, m := range list {
		if !js.ToBoolean(m) {
			continue
		}
		if r, ok := m.(*js.RegExp); ok {
			re, err := regexpOf(r)
			if err != nil {
				return false, err
			}
			if re.Test(s) {
				return true, nil
			}
		}
		str, ok := m.(string)
		if !ok {
			return false, notFunctionError("matcher.toLowerCase")
		}
		if toLowerCase(str) == down {
			return true, nil
		}
	}
	return false, nil
}

// slugify is trim(dasherize(cleanDiacritics(str).replace(/[^\w\s-]/g, '-')
// .toLowerCase()), '-').
func slugify(v any) (string, error) {
	s := toLowerCase(mustReplace(reSlugInvalid, cleanDiacritics(makeString(v)), "-"))
	d, err := dasherize(s)
	if err != nil {
		return "", err
	}
	return trimChars(d, "-", true, true)
}

// mapChars is str.replace(/./g, callback) when callback is a function, else str.
func mapChars(a []any) (any, error) {
	s := makeString(js.Arg(a, 0))
	f, ok := js.Arg(a, 1).(*js.Function)
	if s == "" || !ok {
		return s, nil
	}
	return reAnyChar.Replace(s, f)
}
