package jsdate

import (
	"math"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
)

// Parse is Date.parse (and new Date(string)) as V8 implements it. It first tries the
// ECMAScript date-time string format ([+-YY]YYYY[-MM[-DD]][THH:mm[:ss[.sss]][Z|+-HH:mm]];
// a date-only form is UTC and a date-time without offset is local time) and falls
// back to V8's legacy parser for the rest, which also reads a date without a zone as
// local time, so "Jan 2 2024", "2024/01/02 10:00",
// "Tue, 02 Jan 2024 10:00:00 GMT" and "1/2/2024" parse as in Node. Anything else
// gives NaN.
//
// This is a port of V8's DateParser (src/date/dateparser-inl.h): it scans UTF-16
// code units, reads at most nine significant digits of a number, keys words on
// their first three letters and composes day, time and zone from the tokens.
func Parse(s string) float64 {
	var out [outputSize]float64
	if !parseDate(jsstr.ToUTF16(s), &out) {
		return math.NaN()
	}
	date := MakeDate(MakeDay(out[outYear], out[outMonth], out[outDay]),
		MakeTime(out[outHour], out[outMinute], out[outSecond], out[outMillisecond]))
	if math.IsNaN(out[outUTCOffset]) {
		// No offset: a local time.
		return FromLocal(date)
	}
	return TimeClip(date - out[outUTCOffset]*1000)
}

const (
	outYear = iota
	outMonth
	outDay
	outHour
	outMinute
	outSecond
	outMillisecond
	outUTCOffset
	outputSize
)

// none marks a component that was not given (V8's kNone).
const none = math.MaxInt32

// maxSignificantDigits is how many leading digits of a numeral V8 keeps.
const maxSignificantDigits = 9

func between(x, lo, hi int) bool { return lo <= x && x <= hi }

func isMonth(x int) bool       { return between(x, 1, 12) }
func isDay(x int) bool         { return between(x, 1, 31) }
func isHour(x int) bool        { return between(x, 0, 23) }
func isMinute(x int) bool      { return between(x, 0, 59) }
func isSecond(x int) bool      { return between(x, 0, 59) }
func isHour12(x int) bool      { return between(x, 0, 12) }
func isMillisecond(x int) bool { return between(x, 0, 999) }

// keywordType classifies a word token.
type keywordType int

const (
	kwInvalid keywordType = iota
	kwMonthName
	kwTimeZoneName
	kwTimeSeparator
	kwAMPM
)

// keywords is V8's KeywordTable: a word matches an entry when its first three
// lowercased characters equal the entry's (padded with zeros); only month names may
// be longer than three characters.
var keywords = [...]struct {
	prefix [3]uint16
	typ    keywordType
	value  int
}{
	{[3]uint16{'j', 'a', 'n'}, kwMonthName, 1},
	{[3]uint16{'f', 'e', 'b'}, kwMonthName, 2},
	{[3]uint16{'m', 'a', 'r'}, kwMonthName, 3},
	{[3]uint16{'a', 'p', 'r'}, kwMonthName, 4},
	{[3]uint16{'m', 'a', 'y'}, kwMonthName, 5},
	{[3]uint16{'j', 'u', 'n'}, kwMonthName, 6},
	{[3]uint16{'j', 'u', 'l'}, kwMonthName, 7},
	{[3]uint16{'a', 'u', 'g'}, kwMonthName, 8},
	{[3]uint16{'s', 'e', 'p'}, kwMonthName, 9},
	{[3]uint16{'o', 'c', 't'}, kwMonthName, 10},
	{[3]uint16{'n', 'o', 'v'}, kwMonthName, 11},
	{[3]uint16{'d', 'e', 'c'}, kwMonthName, 12},
	{[3]uint16{'a', 'm', 0}, kwAMPM, 0},
	{[3]uint16{'p', 'm', 0}, kwAMPM, 12},
	{[3]uint16{'u', 't', 0}, kwTimeZoneName, 0},
	{[3]uint16{'u', 't', 'c'}, kwTimeZoneName, 0},
	{[3]uint16{'z', 0, 0}, kwTimeZoneName, 0},
	{[3]uint16{'g', 'm', 't'}, kwTimeZoneName, 0},
	{[3]uint16{'c', 'd', 't'}, kwTimeZoneName, -5},
	{[3]uint16{'c', 's', 't'}, kwTimeZoneName, -6},
	{[3]uint16{'e', 'd', 't'}, kwTimeZoneName, -4},
	{[3]uint16{'e', 's', 't'}, kwTimeZoneName, -5},
	{[3]uint16{'m', 'd', 't'}, kwTimeZoneName, -6},
	{[3]uint16{'m', 's', 't'}, kwTimeZoneName, -7},
	{[3]uint16{'p', 'd', 't'}, kwTimeZoneName, -7},
	{[3]uint16{'p', 's', 't'}, kwTimeZoneName, -8},
	{[3]uint16{'t', 0, 0}, kwTimeSeparator, 0},
}

func lookupKeyword(prefix [3]uint16, length int) (keywordType, int) {
	for _, k := range keywords {
		if k.prefix == prefix && (length <= 3 || k.typ == kwMonthName) {
			return k.typ, k.value
		}
	}
	return kwInvalid, 0
}

// tokenTag is the kind of a DateToken.
type tokenTag int

const (
	tagInvalid tokenTag = iota
	tagUnknown
	tagNumber
	tagSymbol
	tagWhiteSpace
	tagKeyword
	tagEndOfInput
)

type token struct {
	tag    tokenTag
	length int
	value  int // number value, symbol character or keyword value
	kwType keywordType
}

func (t token) isNumber() bool                 { return t.tag == tagNumber }
func (t token) isSymbol(c uint16) bool         { return t.tag == tagSymbol && t.value == int(c) }
func (t token) isFixedLengthNumber(n int) bool { return t.tag == tagNumber && t.length == n }
func (t token) isAsciiSign() bool {
	return t.tag == tagSymbol && (t.value == '+' || t.value == '-')
}

// asciiSign is 1 for '+' and -1 for '-'.
func (t token) asciiSign() int { return 44 - t.value }
func (t token) isKeywordType(k keywordType) bool {
	return t.tag == tagKeyword && t.kwType == k
}
func (t token) isKeywordZ() bool {
	return t.tag == tagKeyword && t.kwType == kwTimeZoneName && t.length == 1 && t.value == 0
}

// isWhiteSpace is V8's IsWhiteSpace: ECMAScript WhiteSpace without line terminators.
func isWhiteSpace(c uint16) bool { return jsstr.IsWhiteSpace(rune(c)) }

func isWhiteSpaceOrLineTerminator(c uint16) bool {
	return jsstr.IsWhiteSpace(rune(c)) || jsstr.IsLineTerminator(rune(c))
}

// tokenizer is V8's DateStringTokenizer over an InputReader: it keeps one token of
// lookahead. As in V8, the current character is 0 past the end and a NUL character
// therefore ends the input too.
type tokenizer struct {
	s    []uint16
	pos  int
	next token
}

func newTokenizer(s []uint16) *tokenizer {
	t := &tokenizer{s: s}
	t.next = t.scan()
	return t
}

func (t *tokenizer) ch() uint16 {
	if t.pos < len(t.s) {
		return t.s[t.pos]
	}
	return 0
}

func (t *tokenizer) atEnd() bool { return t.ch() == 0 }

func (t *tokenizer) isDigit() bool { c := t.ch(); return '0' <= c && c <= '9' }

func (t *tokenizer) scan() token {
	start := t.pos
	if t.atEnd() {
		return token{tag: tagEndOfInput}
	}
	if t.isDigit() {
		n := t.readUnsignedNumeral()
		return token{tag: tagNumber, value: n, length: t.pos - start}
	}
	switch c := t.ch(); c {
	case ':', '-', '+', '.', ')':
		t.pos++
		return token{tag: tagSymbol, value: int(c), length: 1}
	}
	if c := t.ch(); c >= 'A' && !isWhiteSpace(c) {
		var prefix [3]uint16
		length := 0
		for ; !t.atEnd() && t.ch() >= 'A' && !isWhiteSpace(t.ch()); t.pos++ {
			if length < len(prefix) {
				// V8's AsciiAlphaToLower is c | 0x20, applied to every character.
				prefix[length] = t.ch() | 0x20
			}
			length++
		}
		typ, value := lookupKeyword(prefix, length)
		return token{tag: tagKeyword, kwType: typ, value: value, length: length}
	}
	if isWhiteSpaceOrLineTerminator(t.ch()) {
		t.pos++
		return token{tag: tagWhiteSpace, length: 1}
	}
	if t.ch() == '(' {
		// Skip a parenthesised comment, nesting included, up to its close or the end.
		balance := 0
		for {
			switch t.ch() {
			case ')':
				balance--
			case '(':
				balance++
			}
			t.pos++
			if balance <= 0 || t.ch() == 0 {
				break
			}
		}
		return token{tag: tagUnknown}
	}
	t.pos++
	return token{tag: tagUnknown}
}

// readUnsignedNumeral reads a run of digits, skipping leading zeros and keeping only
// the first nine significant digits.
func (t *tokenizer) readUnsignedNumeral() int {
	for t.ch() == '0' {
		t.pos++
	}
	n, i := 0, 0
	for ; t.isDigit(); t.pos++ {
		if i < maxSignificantDigits {
			n = n*10 + int(t.ch()-'0')
		}
		i++
	}
	return n
}

func (t *tokenizer) Next() token {
	tok := t.next
	t.next = t.scan()
	return tok
}

func (t *tokenizer) Peek() token { return t.next }

func (t *tokenizer) skipSymbol(c uint16) bool {
	if t.next.isSymbol(c) {
		t.Next()
		return true
	}
	return false
}

// readMilliseconds turns a fraction-of-second numeral into milliseconds from its
// first three significant digits, inferring leading zeros from the token length.
func readMilliseconds(tok token) int {
	n, length := tok.value, tok.length
	switch {
	case length == 1:
		n *= 100
	case length == 2:
		n *= 10
	case length > 3:
		length = min(length, maxSignificantDigits)
		factor := 1
		for ; length > 3; length-- {
			factor *= 10
		}
		n /= factor
	}
	return n
}

// dayComposer collects year, month and day numbers (V8's DayComposer).
type dayComposer struct {
	comp       [3]int
	index      int
	namedMonth int
	isISODate  bool
}

func (d *dayComposer) add(n int) bool {
	if d.index == len(d.comp) {
		return false
	}
	d.comp[d.index] = n
	d.index++
	return true
}

func (d *dayComposer) write(out *[outputSize]float64) bool {
	if d.index < 1 {
		return false
	}
	// Day and month default to 1. The padding also makes index 3 below, so V8's
	// "one number with a named month" case never applies: "Jan 5" reads the padded
	// 1 as the year, 2001.
	for d.index < len(d.comp) {
		d.comp[d.index] = 1
		d.index++
	}
	year, month, day := 0, none, none
	if d.namedMonth == none {
		if d.isISODate || (d.index == 3 && !isDay(d.comp[0])) {
			year, month, day = d.comp[0], d.comp[1], d.comp[2]
		} else {
			month, day = d.comp[0], d.comp[1]
			if d.index == 3 {
				year = d.comp[2]
			}
		}
	} else {
		month = d.namedMonth
		switch {
		case d.index == 1:
			day = d.comp[0]
		case !isDay(d.comp[0]):
			year, day = d.comp[0], d.comp[1]
		default:
			day, year = d.comp[0], d.comp[1]
		}
	}
	if !d.isISODate {
		switch {
		case between(year, 0, 49):
			year += 2000
		case between(year, 50, 99):
			year += 1900
		}
	}
	if !isMonth(month) || !isDay(day) {
		return false
	}
	out[outYear] = float64(year)
	out[outMonth] = float64(month - 1)
	out[outDay] = float64(day)
	return true
}

// timeComposer collects hour, minute, second and millisecond (V8's TimeComposer).
type timeComposer struct {
	comp       [4]int
	index      int
	hourOffset int
}

func (t *timeComposer) isEmpty() bool { return t.index == 0 }

func (t *timeComposer) isExpecting(n int) bool {
	return (t.index == 1 && isMinute(n)) || (t.index == 2 && isSecond(n)) || (t.index == 3 && isMillisecond(n))
}

func (t *timeComposer) add(n int) bool {
	if t.index == len(t.comp) {
		return false
	}
	t.comp[t.index] = n
	t.index++
	return true
}

func (t *timeComposer) addFinal(n int) bool {
	if !t.add(n) {
		return false
	}
	for t.index < len(t.comp) {
		t.comp[t.index] = 0
		t.index++
	}
	return true
}

func (t *timeComposer) write(out *[outputSize]float64) bool {
	for t.index < len(t.comp) {
		t.comp[t.index] = 0
		t.index++
	}
	hour, minute, second, ms := t.comp[0], t.comp[1], t.comp[2], t.comp[3]
	if t.hourOffset != none {
		if !isHour12(hour) {
			return false
		}
		hour = hour%12 + t.hourOffset
	}
	if !isHour(hour) || !isMinute(minute) || !isSecond(second) || !isMillisecond(ms) {
		// Hour 24 is allowed when everything after it is zero.
		if hour != 24 || minute != 0 || second != 0 || ms != 0 {
			return false
		}
	}
	out[outHour] = float64(hour)
	out[outMinute] = float64(minute)
	out[outSecond] = float64(second)
	out[outMillisecond] = float64(ms)
	return true
}

// zoneComposer collects a UTC offset (V8's TimeZoneComposer).
type zoneComposer struct {
	sign, hour, minute int
}

func (z *zoneComposer) set(offsetHours int) {
	z.sign = 1
	if offsetHours < 0 {
		z.sign = -1
	}
	z.hour = offsetHours * z.sign
	z.minute = 0
}

func (z *zoneComposer) setSign(sign int) {
	z.sign = 1
	if sign < 0 {
		z.sign = -1
	}
}

func (z *zoneComposer) isExpecting(n int) bool {
	return z.hour != none && z.minute == none && isMinute(n)
}

func (z *zoneComposer) isUTC() bool   { return z.hour == 0 && z.minute == 0 }
func (z *zoneComposer) isEmpty() bool { return z.hour == none }

func (z *zoneComposer) write(out *[outputSize]float64) bool {
	if z.sign == none {
		out[outUTCOffset] = math.NaN() // local time
		return true
	}
	if z.hour == none {
		z.hour = 0
	}
	if z.minute == none {
		z.minute = 0
	}
	// V8 computes the total in unsigned 32-bit arithmetic and rejects anything above
	// the largest Smi, 2^31-1 in Node's 64-bit builds.
	total := uint32(z.hour)*3600 + uint32(z.minute)*60
	if total > math.MaxInt32 {
		return false
	}
	seconds := int(total)
	if z.sign < 0 {
		seconds = -seconds
	}
	out[outUTCOffset] = float64(seconds)
	return true
}

// parseDate is V8's DateParser::Parse: the ES5 format first, then the legacy rules
// for whatever token the ES5 pass stopped at.
func parseDate(s []uint16, out *[outputSize]float64) bool {
	scanner := newTokenizer(s)
	day := dayComposer{namedMonth: none}
	tm := timeComposer{hourOffset: none}
	tz := zoneComposer{sign: none, hour: none, minute: none}

	next := parseES5DateTime(scanner, &day, &tm, &tz)
	if next.tag == tagInvalid {
		return false
	}
	hasReadNumber := day.index != 0
	for tok := next; tok.tag != tagEndOfInput; tok = scanner.Next() {
		switch {
		case tok.isNumber():
			hasReadNumber = true
			n := tok.value
			switch {
			case scanner.skipSymbol(':'):
				if scanner.skipSymbol(':') {
					// n followed by "::"
					if !tm.isEmpty() {
						return false
					}
					tm.add(n)
					tm.add(0)
				} else {
					if !tm.add(n) {
						return false
					}
					if scanner.Peek().isSymbol('.') {
						scanner.Next()
					}
				}
			// V8 consumes the '.' even when the time is not expecting n.
			case scanner.skipSymbol('.') && tm.isExpecting(n):
				tm.add(n)
				if !scanner.Peek().isNumber() {
					return false
				}
				ms := readMilliseconds(scanner.Next())
				tm.addFinal(ms)
			case tz.isExpecting(n):
				tz.minute = n
			case tm.isExpecting(n):
				tm.addFinal(n)
				// The end of the time must be followed by the end, white space, Z or a sign.
				p := scanner.Peek()
				if p.tag != tagEndOfInput && p.tag != tagWhiteSpace && !p.isKeywordZ() && !p.isAsciiSign() {
					return false
				}
			default:
				if !day.add(n) {
					return false
				}
				scanner.skipSymbol('-')
			}
		case tok.tag == tagKeyword:
			switch {
			case tok.kwType == kwAMPM && !tm.isEmpty():
				tm.hourOffset = tok.value
			case tok.kwType == kwMonthName:
				day.namedMonth = tok.value
				scanner.skipSymbol('-')
			case tok.kwType == kwTimeZoneName && hasReadNumber:
				tz.set(tok.value)
			default:
				// Unknown words are only allowed before the first number, and must be
				// separated from it.
				if hasReadNumber || scanner.Peek().isNumber() {
					return false
				}
			}
		case tok.isAsciiSign() && (tz.isUTC() || !tm.isEmpty()):
			// A UTC offset, only after a zone name or a time.
			tz.setSign(tok.asciiSign())
			n, length := 0, 0
			if scanner.Peek().isNumber() {
				t := scanner.Next()
				n, length = t.value, t.length
			}
			hasReadNumber = true
			switch {
			case scanner.Peek().isSymbol(':'):
				tz.hour, tz.minute = n, none
			case length == 1 || length == 2:
				tz.hour, tz.minute = n, 0
			case length == 3 || length == 4:
				tz.hour, tz.minute = n/100, n%100
			default:
				return false
			}
		case (tok.isAsciiSign() || tok.isSymbol(')')) && hasReadNumber:
			return false
		}
	}
	return day.write(out) && tm.write(out) && tz.write(out)
}

// parseES5DateTime reads the ECMAScript date-time string format. It returns the end
// of input when the whole string matched, an invalid token when the string started
// like that format but broke its rules, and otherwise the first token it could not
// use, from which the legacy parser continues.
func parseES5DateTime(scanner *tokenizer, day *dayComposer, tm *timeComposer, tz *zoneComposer) token {
	invalid := token{tag: tagInvalid}
	if scanner.Peek().isAsciiSign() {
		// Keep the sign token so the legacy parser sees it.
		sign := scanner.Next()
		if !scanner.Peek().isFixedLengthNumber(6) {
			return sign
		}
		year := scanner.Next().value
		if sign.asciiSign() < 0 && year == 0 {
			return sign
		}
		day.add(sign.asciiSign() * year)
	} else if scanner.Peek().isFixedLengthNumber(4) {
		day.add(scanner.Next().value)
	} else {
		return scanner.Next()
	}
	if scanner.skipSymbol('-') {
		if !scanner.Peek().isFixedLengthNumber(2) || !isMonth(scanner.Peek().value) {
			return scanner.Next()
		}
		day.add(scanner.Next().value)
		if scanner.skipSymbol('-') {
			if !scanner.Peek().isFixedLengthNumber(2) || !isDay(scanner.Peek().value) {
				return scanner.Next()
			}
			day.add(scanner.Next().value)
		}
	}
	if !scanner.Peek().isKeywordType(kwTimeSeparator) {
		if scanner.Peek().tag != tagEndOfInput {
			return scanner.Next()
		}
	} else {
		scanner.Next()
		if !scanner.Peek().isFixedLengthNumber(2) || !between(scanner.Peek().value, 0, 24) {
			return invalid
		}
		// 24:00[:00[.000]] is allowed, no other time starting with 24.
		hourIs24 := scanner.Peek().value == 24
		tm.add(scanner.Next().value)
		if !scanner.skipSymbol(':') {
			return invalid
		}
		if !scanner.Peek().isFixedLengthNumber(2) || !isMinute(scanner.Peek().value) || (hourIs24 && scanner.Peek().value > 0) {
			return invalid
		}
		tm.add(scanner.Next().value)
		if scanner.skipSymbol(':') {
			if !scanner.Peek().isFixedLengthNumber(2) || !isSecond(scanner.Peek().value) || (hourIs24 && scanner.Peek().value > 0) {
				return invalid
			}
			tm.add(scanner.Next().value)
			if scanner.skipSymbol('.') {
				if !scanner.Peek().isNumber() || (hourIs24 && scanner.Peek().value > 0) {
					return invalid
				}
				// More or fewer than three digits are allowed.
				tm.add(readMilliseconds(scanner.Next()))
			}
		}
		switch {
		case scanner.Peek().isKeywordZ():
			scanner.Next()
			tz.set(0)
		case scanner.Peek().isSymbol('+') || scanner.Peek().isSymbol('-'):
			sign := 1
			if scanner.Next().value == '-' {
				sign = -1
			}
			tz.setSign(sign)
			if scanner.Peek().isFixedLengthNumber(4) {
				// The hhmm extension.
				hourMin := scanner.Next().value
				hour, minute := hourMin/100, hourMin%100
				if !isHour(hour) || !isMinute(minute) {
					return invalid
				}
				tz.hour, tz.minute = hour, minute
			} else {
				if !scanner.Peek().isFixedLengthNumber(2) || !isHour(scanner.Peek().value) {
					return invalid
				}
				tz.hour = scanner.Next().value
				if !scanner.skipSymbol(':') {
					return invalid
				}
				if !scanner.Peek().isFixedLengthNumber(2) || !isMinute(scanner.Peek().value) {
					return invalid
				}
				tz.minute = scanner.Next().value
			}
		}
		if scanner.Peek().tag != tagEndOfInput {
			return invalid
		}
	}
	// A date-only form without an offset is UTC; a date-time form is local time.
	if tz.isEmpty() && tm.isEmpty() {
		tz.set(0)
	}
	day.isISODate = true
	return token{tag: tagEndOfInput}
}
