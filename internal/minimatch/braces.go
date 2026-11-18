package minimatch

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// This file ports brace-expansion 1.1.22 (index.js) and the range function of
// balanced-match 1.0.2. Lengths and positions that the JavaScript code feeds into
// its limits are UTF-16 code-unit counts, so they are measured with utf16Len.

// Limits from brace-expansion's index.js. minimatch never passes options to
// expand(), so the defaults are the only values in use.
const (
	expansionMax         = 100000   // EXPANSION_MAX: results kept
	expansionMaxLength   = 4000000  // EXPANSION_MAX_LENGTH: characters held
	expansionMaxDepth    = 1000     // EXPANSION_MAX_DEPTH: brace nesting followed
	expansionMaxRewrites = 1000     // EXPANSION_MAX_REWRITES: {a},b} rescans
	expansionMaxSteps    = 20000000 // EXPANSION_MAX_STEPS: work per call
	callSteps            = 16       // CALL_STEPS: cost of one expand call
)

// The sentinels that hide escaped braces, commas, periods and backslashes while
// expanding. brace-expansion builds them as '\0NAME' + Math.random().toFixed(16)
// + '\0'; their length counts against the limits, so they are built the same way.
var (
	escSlash  = sentinel("SLASH")
	escOpen   = sentinel("OPEN")
	escClose  = sentinel("CLOSE")
	escComma  = sentinel("COMMA")
	escPeriod = sentinel("PERIOD")
)

func sentinel(name string) string {
	return "\x00" + name + fmt.Sprintf("0.%016d", rand.Int64N(1e16)) + "\x00"
}

var (
	braceEscaper = strings.NewReplacer(`\\`, escSlash, `\{`, escOpen, `\}`, escClose, `\,`, escComma, `\.`, escPeriod)
	// unescapeBraces turns escSlash back into a single backslash, so an escaped
	// backslash loses its escape once a pattern is expanded.
	braceUnescaper = strings.NewReplacer(escSlash, `\`, escOpen, "{", escClose, "}", escComma, ",", escPeriod, ".")
)

// escapeBraces applies the split/join chain of the original, which replaces
// left to right in a single pass per sentinel. The sentinels contain no
// backslash, so one strings.Replacer pass gives the same result.
func escapeBraces(s string) string { return braceEscaper.Replace(s) }

func unescapeBraces(s string) string { return braceUnescaper.Replace(s) }

// expandBraces is brace-expansion's expandTop with its default options.
func expandBraces(str string) []string {
	if str == "" {
		return []string{}
	}
	// Bash 4.3 keeps a leading {} literal, at the top level only.
	if strings.HasPrefix(str, "{}") {
		str = `\{\}` + str[2:]
	}
	e := braceExpander{remaining: expansionMaxSteps}
	out := e.expand(escapeBraces(str), 0, true)
	for i, s := range out {
		out[i] = unescapeBraces(s)
	}
	return out
}

// braceExpander carries the step budget shared by every level of one call
// (the `{ remaining }` object of the original).
type braceExpander struct {
	remaining int
}

// braceMatch is what balancedWithBudget returns.
type braceMatch struct {
	end       int
	pre       string
	body      string
	post      string
	unmatched bool
}

// balancedRange is balanced-match's range('{', '}', str): the outermost
// balanced pair, or the innermost pair after an unmatched open brace.
func balancedRange(str string) (int, int, bool) {
	ai := strings.IndexByte(str, '{')
	bi := indexByteFrom(str, '}', ai+1)
	i := ai
	if ai < 0 || bi <= 0 {
		return 0, 0, false
	}
	var begs []int
	left, right := len(str), 0
	found := false
	var r0, r1 int
	for i >= 0 && !found {
		switch {
		case i == ai:
			begs = append(begs, i)
			ai = indexByteFrom(str, '{', i+1)
		case len(begs) == 1:
			r0, r1 = begs[0], bi
			begs = begs[:0]
			found = true
		default:
			beg := begs[len(begs)-1]
			begs = begs[:len(begs)-1]
			if beg < left {
				left, right = beg, bi
			}
			bi = indexByteFrom(str, '}', i+1)
		}
		if ai < bi && ai >= 0 {
			i = ai
		} else {
			i = bi
		}
	}
	if len(begs) > 0 {
		return left, right, true
	}
	return r0, r1, found
}

// indexByteFrom is String.prototype.indexOf with a start position.
func indexByteFrom(s string, c byte, from int) int {
	if from >= len(s) {
		return -1
	}
	if from < 0 {
		from = 0
	}
	i := strings.IndexByte(s[from:], c)
	if i < 0 {
		return -1
	}
	return from + i
}

// balancedWithBudget runs balanced on masked, slices str, and charges the budget
// for the characters walked. Once the budget is overdrawn it returns nil.
func (e *braceExpander) balancedWithBudget(str, masked string) *braceMatch {
	if e.remaining < 0 {
		return nil
	}
	r0, r1, ok := balancedRange(masked)
	unmatched := ok && strings.IndexByte(masked, '{') < r0
	if ok && !unmatched {
		e.remaining -= jsstr.Length(masked[:r1+1])
	} else {
		e.remaining -= jsstr.Length(str)
	}
	if !ok || e.remaining < 0 {
		return nil
	}
	return &braceMatch{end: r1, pre: str[:r0], body: str[r0+1 : r1], post: str[r1+1:], unmatched: unmatched}
}

// maskUnmatchedOpens replaces every { that has no matching } with NUL, keeping
// positions aligned with the string it was made from.
func (e *braceExpander) maskUnmatchedOpens(masked string) string {
	e.remaining -= jsstr.Length(masked)
	var open []int
	for i := 0; i < len(masked); i++ {
		switch masked[i] {
		case '{':
			open = append(open, i)
		case '}':
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
		}
	}
	if len(open) == 0 {
		return masked
	}
	b := []byte(masked)
	for _, i := range open {
		b[i] = 0
	}
	return string(b)
}

// commaSetFollows answers /,(?!,).*\}/.test(str) in one pass from the end.
func (e *braceExpander) commaSetFollows(str string) bool {
	e.remaining -= jsstr.Length(str)
	if e.remaining < 0 {
		return false
	}
	closeSeen := false
	next := rune(-1)
	for i := len(str); i > 0; {
		r, size := utf8.DecodeLastRuneInString(str[:i])
		i -= size
		switch {
		case r == '}':
			closeSeen = true
		case jsstr.IsLineTerminator(r):
			closeSeen = false
		case r == ',' && closeSeen && next != ',':
			return true
		}
		next = r
	}
	return false
}

// parseCommaParts splits a set body on top-level commas. It returns false when
// the step budget runs out first.
func (e *braceExpander) parseCommaParts(str string) ([]string, bool) {
	var parts []string
	carry := ""
	for {
		m := e.balancedWithBudget(str, str)
		if e.remaining < 0 {
			return nil, false
		}
		if m == nil {
			tail := strings.Split(str, ",")
			tail[0] = carry + tail[0]
			return append(parts, tail...), true
		}
		p := strings.Split(m.pre, ",")
		p[0] = carry + p[0]
		p[len(p)-1] += "{" + m.body + "}"
		if m.post == "" {
			return append(parts, p...), true
		}
		carry = p[len(p)-1]
		parts = append(parts, p[:len(p)-1]...)
		str = m.post
	}
}

// combine builds acc[a] + pre + values[v] for every combination, bounded by the
// result and length limits. With dropEmpties, results that add nothing past
// base[a] are skipped. The baselines of kept results are appended to outBase.
func (e *braceExpander) combine(acc []string, base []int, pre string, values []string, dropEmpties bool, outBase *[]int) []string {
	out := []string{}
	length := 0
	var nonEmpty []string
	nonEmptyDone := false
outer:
	for a := range acc {
		prefix := acc[a] + pre
		vs := values
		if dropEmpties && jsstr.Length(prefix) == base[a] {
			if !nonEmptyDone {
				for _, v := range values {
					if v != "" {
						nonEmpty = append(nonEmpty, v)
					}
				}
				nonEmptyDone = true
			}
			vs = nonEmpty
		}
		for _, v := range vs {
			if len(out) >= expansionMax {
				break outer
			}
			expansion := prefix + v
			n := jsstr.Length(expansion)
			if length+n > expansionMaxLength {
				break outer
			}
			out = append(out, expansion)
			if outBase != nil {
				*outBase = append(*outBase, base[a])
			}
			length += n
		}
	}
	e.remaining -= len(out)
	return out
}

// numeric is the original's numeric(): the integer value when str is one,
// otherwise the code of its first character.
func numeric(str string) float64 {
	if v, ok := parseIntEqual(str); ok {
		return v
	}
	return float64(str[0])
}

// parseIntEqual reports parseInt(str, 10) == str for the digit strings a
// sequence body can hold (an optional minus and decimal digits).
func parseIntEqual(str string) (float64, bool) {
	digits := strings.TrimPrefix(str, "-")
	if digits == "" {
		return 0, false
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0, false
		}
	}
	v, err := strconv.ParseFloat(digits, 64)
	if err != nil {
		// Only a range error is possible here; ParseFloat still returns ±Inf,
		// which is also what parseInt yields for such input.
		v = math.Inf(1)
	}
	if str[0] == '-' {
		v = -v
	}
	return v, true
}

// isPadded is /^-?0\d/.
func isPadded(el string) bool {
	el = strings.TrimPrefix(el, "-")
	return len(el) >= 2 && el[0] == '0' && el[1] >= '0' && el[1] <= '9'
}

// expandSequence yields the values of a numeric (1..5) or alphabetic (a..e..2)
// sequence body. It returns false when the step budget runs out.
func (e *braceExpander) expandSequence(body string, isAlpha bool) ([]string, bool) {
	n := strings.Split(body, "..")
	x := numeric(n[0])
	y := numeric(n[1])
	width := max(len(n[0]), len(n[1]))
	incr := 1.0
	if len(n) == 3 {
		incr = math.Max(math.Abs(numeric(n[2])), 1)
	}
	reverse := y < x
	if reverse {
		incr = -incr
	}
	test := func(i float64) bool {
		if reverse {
			return i >= y
		}
		return i <= y
	}
	pad := false
	for _, part := range n {
		if isPadded(part) {
			pad = true
		}
	}
	cost := 1
	if pad {
		cost = width
	}
	out := []string{}
	length := 0
	for i := x; test(i) && len(out) < expansionMax; i += incr {
		e.remaining -= cost
		if e.remaining < 0 {
			return nil, false
		}
		var c string
		if isAlpha {
			c = string(rune(uint16(i)))
			if c == `\` {
				c = ""
			}
		} else {
			c = js.NumberToString(i)
			if pad {
				if need := width - len(c); need > 0 {
					z := strings.Repeat("0", need)
					if i < 0 {
						c = "-" + z + c[1:]
					} else {
						c = z + c
					}
				}
			}
		}
		if length+len(c) > expansionMaxLength {
			break
		}
		out = append(out, c)
		length += len(c)
	}
	return out, true
}

// isNumericSequence is /^-?\d+\.\.-?\d+(?:\.\.-?\d+)?$/.
func isNumericSequence(body string) bool {
	parts := strings.Split(body, "..")
	if len(parts) != 2 && len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if _, ok := parseIntEqual(p); !ok {
			return false
		}
	}
	return true
}

// isAlphaSequence is /^[a-zA-Z]\.\.[a-zA-Z](?:\.\.-?\d+)?$/.
func isAlphaSequence(body string) bool {
	parts := strings.Split(body, "..")
	if len(parts) != 2 && len(parts) != 3 {
		return false
	}
	for _, p := range parts[:2] {
		if len(p) != 1 || !isASCIILetter(p[0]) {
			return false
		}
	}
	if len(parts) == 3 {
		if _, ok := parseIntEqual(parts[2]); !ok {
			return false
		}
	}
	return true
}

func isASCIILetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

func embrace(s string) string { return "{" + s + "}" }

// expand consumes the top-level brace groups of str left to right, threading
// the combined prefixes through acc as the original does.
func (e *braceExpander) expand(str string, depth int, isTop bool) []string {
	e.remaining -= callSteps
	if depth > expansionMaxDepth {
		return []string{str}
	}
	acc := []string{""}
	accBase := []int{0}
	rewrites := 0
	dropEmpties := false
	firstGroup := true
	masked := str
	for {
		m := e.balancedWithBudget(str, masked)
		if m == nil {
			return e.combine(acc, accBase, str, []string{""}, dropEmpties, nil)
		}
		if m.unmatched {
			masked = e.maskUnmatchedOpens(masked)
		}
		pre := m.pre
		// The set and the rest of the string, kept literal.
		literal := func() []string {
			return e.combine(acc, accBase, pre+"{"+m.body+"}"+m.post, []string{""}, dropEmpties, nil)
		}
		// `${` suppresses expansion of the whole remainder on the 1.x line.
		if strings.HasSuffix(pre, "$") {
			return e.combine(acc, accBase, str, []string{""}, dropEmpties, nil)
		}
		numericSeq := isNumericSequence(m.body)
		alphaSeq := isAlphaSequence(m.body)
		isSequence := numericSeq || alphaSeq
		isOptions := strings.IndexByte(m.body, ',') >= 0
		if !isSequence && !isOptions {
			// {a},b}
			if rewrites < expansionMaxRewrites && e.commaSetFollows(m.post) {
				rewrites++
				str = m.pre + "{" + m.body + escClose + m.post
				masked = masked[:m.end] + escClose + masked[m.end+1:]
				isTop = true
				firstGroup = true
				dropEmpties = false
				accBase = make([]int, len(acc))
				for b, a := range acc {
					accBase[b] = jsstr.Length(a)
				}
				continue
			}
			return literal()
		}
		if firstGroup {
			dropEmpties = isTop && !isSequence
			firstGroup = false
		}
		var values []string
		if isSequence {
			var ok bool
			values, ok = e.expandSequence(m.body, alphaSeq)
			if !ok {
				return literal()
			}
		} else {
			n, ok := e.parseCommaParts(m.body)
			if !ok {
				return literal()
			}
			if len(n) == 1 {
				// x{{a,b}}y ==> x{a}y x{b}y
				inner := e.expand(n[0], depth+1, false)
				n = make([]string, len(inner))
				for i, s := range inner {
					n[i] = embrace(s)
				}
				if e.remaining < 0 {
					return literal()
				}
				if len(n) == 1 {
					var nextBase []int
					single := e.combine(acc, accBase, pre+n[0], []string{""}, dropEmpties && m.post == "", &nextBase)
					if e.remaining < 0 {
						return literal()
					}
					acc, accBase = single, nextBase
					if m.post == "" {
						break
					}
					str = m.post
					masked = masked[m.end+1:]
					continue
				}
			}
			// Values combine would drop as empty must not count against the limits.
			dropsEmpties := dropEmpties && m.post == "" && pre == ""
			for d := 0; dropsEmpties && d < len(acc); d++ {
				if jsstr.Length(acc[d]) != accBase[d] {
					dropsEmpties = false
				}
			}
			values = []string{}
			valuesLength := 0
		members:
			for _, part := range n {
				expanded := e.expand(part, depth+1, false)
				if e.remaining < 0 {
					return literal()
				}
				for _, v := range expanded {
					if dropsEmpties && v == "" {
						continue
					}
					vl := jsstr.Length(v)
					if len(values) >= expansionMax || valuesLength+vl > expansionMaxLength {
						break members
					}
					values = append(values, v)
					valuesLength += vl
				}
			}
		}
		var nextBase []int
		next := e.combine(acc, accBase, pre, values, dropEmpties && m.post == "", &nextBase)
		if e.remaining < 0 {
			return literal()
		}
		acc, accBase = next, nextBase
		if m.post == "" {
			break
		}
		str = m.post
		masked = masked[m.end+1:]
	}
	return acc
}
