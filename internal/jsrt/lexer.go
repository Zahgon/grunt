package jsrt

import (
	"math/big"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
)

// tokKind classifies a token the way V8's scanner does for its error messages.
type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tKeyword
	tPunct
	tNumber
	tString
	tTemplate
	tRegExp
	tIllegal
	// tPrivate is a private name such as #x, which is only valid in classes.
	tPrivate
)

// token is one lexical token. Positions are byte offsets into the source.
type token struct {
	kind tokKind
	// text is the punctuator, the keyword or the identifier name (escapes decoded).
	text string
	num  float64
	// str is a string literal's value or a template chunk's cooked value.
	str string
	// raw is a template chunk's raw text.
	raw string
	// cookedOK is false for a template chunk with an invalid escape.
	cookedOK bool
	// tail marks the template chunk that ends with a backtick.
	tail bool
	// flags is a regular expression literal's flags.
	flags    string
	pos, end int
	// nlBefore reports a line terminator between the previous token and this one.
	nlBefore bool
	// escaped reports an identifier written with \u escapes.
	escaped bool
	// msg is the SyntaxError message of an illegal token ("" for the generic one).
	msg string
	// templErr is the message for a template chunk whose escape is invalid.
	templErr string
	// bigint marks a BigInt literal such as 1n.
	bigint bool
	// unterminated marks a template that runs to the end of the source.
	unterminated bool
}

// keywords are the reserved words of sloppy-mode scripts. null, true and false are
// literals but are reported as tokens like keywords.
var keywords = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true, "continue": true,
	"debugger": true, "default": true, "delete": true, "do": true, "else": true, "export": true,
	"extends": true, "finally": true, "for": true, "function": true, "if": true, "import": true,
	"in": true, "instanceof": true, "new": true, "return": true, "super": true, "switch": true,
	"this": true, "throw": true, "try": true, "typeof": true, "var": true, "void": true,
	"while": true, "with": true, "null": true, "true": true, "false": true, "enum": true,
}

// punctuators lists every punctuator, longest first within a shared prefix.
var punctuators = []string{
	">>>=", "...", "===", "!==", "**=", "<<=", ">>=", ">>>", "&&=", "||=", "??=",
	"=>", "==", "!=", "<=", ">=", "&&", "||", "??", "?.", "++", "--", "+=", "-=", "*=", "/=",
	"%=", "&=", "|=", "^=", "<<", ">>", "**",
	"{", "}", "(", ")", "[", "]", ";", ",", "<", ">", "+", "-", "*", "/", "%", "&", "|",
	"^", "!", "~", "?", ":", "=", ".", "@", "#",
}

// lexer scans JavaScript source text on demand; the parser decides whether a slash
// starts a regular expression and when a template continues.
type lexer struct {
	src string
	pos int
}

func isIDStart(r rune) bool {
	if r < 0x80 {
		return r == '$' || r == '_' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
	}
	return unicode.IsLetter(r) || unicode.Is(unicode.Nl, r) || unicode.Is(unicode.Other_ID_Start, r)
}

func isIDPart(r rune) bool {
	if r < 0x80 {
		return isIDStart(r) || ('0' <= r && r <= '9')
	}
	return isIDStart(r) || unicode.In(r, unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc, unicode.Other_ID_Continue) || r == 0x200C || r == 0x200D
}

// peekRune decodes the character at at, reading jsstr's WTF-8 encoding of an
// unpaired surrogate as that surrogate.
func (l *lexer) peekRune(at int) (rune, int) {
	if at >= len(l.src) {
		return -1, 0
	}
	c := l.src[at]
	if c < utf8.RuneSelf {
		return rune(c), 1
	}
	if c == 0xED && at+2 < len(l.src) && l.src[at+1] >= 0xA0 && l.src[at+1] <= 0xBF {
		return rune(jsstr.ToUTF16(l.src[at : at+3])[0]), 3
	}
	return utf8.DecodeRuneInString(l.src[at:])
}

// skipSpace skips white space and comments and reports whether a line terminator
// was crossed. An unterminated block comment yields an illegal-token message.
func (l *lexer) skipSpace() (nl bool, illegal bool) {
	for l.pos < len(l.src) {
		r, n := l.peekRune(l.pos)
		switch {
		case jsstr.IsLineTerminator(r):
			nl = true
			l.pos += n
		case jsstr.IsWhiteSpace(r):
			l.pos += n
		case r == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '/':
			l.pos += 2
			for l.pos < len(l.src) {
				r, n := l.peekRune(l.pos)
				if jsstr.IsLineTerminator(r) {
					break
				}
				l.pos += n
			}
		case r == '/' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*':
			end := strings.Index(l.src[l.pos+2:], "*/")
			if end < 0 {
				l.pos = len(l.src)
				return nl, true
			}
			body := l.src[l.pos+2 : l.pos+2+end]
			if strings.ContainsAny(body, "\n\r  ") {
				nl = true
			}
			l.pos += end + 4
		default:
			return nl, false
		}
	}
	return nl, false
}

// next scans the next token, treating '/' as division.
func (l *lexer) next() token {
	nl, badComment := l.skipSpace()
	start := l.pos
	if badComment {
		return token{kind: tIllegal, pos: start, end: start, nlBefore: nl}
	}
	if l.pos >= len(l.src) {
		return token{kind: tEOF, pos: start, end: start, nlBefore: nl}
	}
	t := l.scan()
	t.pos = start
	t.end = l.pos
	t.nlBefore = nl
	return t
}

func (l *lexer) scan() token {
	r, n := l.peekRune(l.pos)
	switch {
	case isIDStart(r) || r == '\\':
		return l.scanIdent()
	case r >= '0' && r <= '9':
		return l.scanNumber()
	case r == '.' && l.pos+1 < len(l.src) && l.src[l.pos+1] >= '0' && l.src[l.pos+1] <= '9':
		return l.scanNumber()
	case r == '"' || r == '\'':
		return l.scanString(byte(r))
	case r == '`':
		l.pos++
		return l.scanTemplate()
	case r == '#':
		if next, _ := l.peekRune(l.pos + 1); isIDStart(next) || next == '\\' {
			l.pos++
			name, _, ok := l.scanIdentName()
			if !ok {
				return token{kind: tIllegal}
			}
			return token{kind: tPrivate, text: "#" + name}
		}
	}
	for _, p := range punctuators {
		if strings.HasPrefix(l.src[l.pos:], p) {
			// "?." followed by a digit is the conditional operator and a number.
			if p == "?." && l.pos+2 < len(l.src) && l.src[l.pos+2] >= '0' && l.src[l.pos+2] <= '9' {
				continue
			}
			if p == "@" || p == "#" {
				break
			}
			l.pos += len(p)
			return token{kind: tPunct, text: p}
		}
	}
	l.pos += n
	return token{kind: tIllegal}
}

// scanIdentName reads an IdentifierName, decoding \u escapes. ok is false when an
// escape is malformed or decodes to a character not allowed there.
func (l *lexer) scanIdentName() (name string, escaped, ok bool) {
	var b strings.Builder
	first := true
	for l.pos < len(l.src) {
		r, n := l.peekRune(l.pos)
		if r == '\\' {
			if l.pos+1 >= len(l.src) || l.src[l.pos+1] != 'u' {
				return "", false, false
			}
			l.pos += 2
			cp, msg := l.scanUnicodeEscapeBody()
			if msg != "" || (first && !isIDStart(cp)) || (!first && !isIDPart(cp)) {
				return "", true, false
			}
			escaped = true
			b.WriteRune(cp)
			first = false
			continue
		}
		if (first && !isIDStart(r)) || (!first && !isIDPart(r)) {
			break
		}
		b.WriteRune(r)
		l.pos += n
		first = false
	}
	return b.String(), escaped, true
}

func (l *lexer) scanIdent() token {
	name, escaped, ok := l.scanIdentName()
	if !ok {
		// A backslash not starting \u is a plain illegal token.
		if escaped {
			return token{kind: tIllegal, msg: "Invalid Unicode escape sequence"}
		}
		return token{kind: tIllegal}
	}
	if keywords[name] {
		return token{kind: tKeyword, text: name, escaped: escaped}
	}
	return token{kind: tIdent, text: name, escaped: escaped}
}

// scanUnicodeEscapeBody reads the part of \uXXXX or \u{X...} after "\u". It
// returns V8's message for a malformed escape.
func (l *lexer) scanUnicodeEscapeBody() (rune, string) {
	const invalid = "Invalid Unicode escape sequence"
	if l.pos < len(l.src) && l.src[l.pos] == '{' {
		l.pos++
		var v rune
		digits := 0
		for l.pos < len(l.src) && l.src[l.pos] != '}' {
			d := jsstr.HexDigitValue(rune(l.src[l.pos]))
			if d < 0 {
				return 0, invalid
			}
			v = v*16 + rune(d)
			if v > 0x10FFFF {
				return 0, "Undefined Unicode code-point"
			}
			digits++
			l.pos++
		}
		if l.pos >= len(l.src) || digits == 0 {
			return 0, invalid
		}
		l.pos++
		return v, ""
	}
	if l.pos+4 > len(l.src) {
		return 0, invalid
	}
	var v rune
	for i := 0; i < 4; i++ {
		d := jsstr.HexDigitValue(rune(l.src[l.pos+i]))
		if d < 0 {
			return 0, invalid
		}
		v = v*16 + rune(d)
	}
	l.pos += 4
	return v, ""
}

// scanDigits reads digits of the given base with numeric separators, returning
// V8's message for a misplaced separator.
func (l *lexer) scanDigits(base int, allowSep bool) (string, string) {
	var b strings.Builder
	lastSep := false
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '_' && allowSep {
			if lastSep {
				return "", "Only one underscore is allowed as numeric separator"
			}
			if b.Len() == 0 {
				return "", "Invalid or unexpected token"
			}
			lastSep = true
			l.pos++
			continue
		}
		d := jsstr.HexDigitValue(rune(c))
		if d < 0 || d >= base {
			break
		}
		lastSep = false
		b.WriteByte(c)
		l.pos++
	}
	if lastSep {
		return "", "Numeric separators are not allowed at the end of numeric literals"
	}
	return b.String(), ""
}

func (l *lexer) scanNumber() token {
	start := l.pos
	illegal := func(msg string) token {
		if msg == "Invalid or unexpected token" {
			msg = ""
		}
		return token{kind: tIllegal, msg: msg}
	}
	if l.src[l.pos] == '0' && l.pos+1 < len(l.src) {
		base := 0
		switch l.src[l.pos+1] | 0x20 {
		case 'x':
			base = 16
		case 'o':
			base = 8
		case 'b':
			base = 2
		}
		if base != 0 {
			l.pos += 2
			digits, msg := l.scanDigits(base, true)
			if msg != "" || digits == "" {
				return illegal(msg)
			}
			bigint := l.eatBigIntSuffix()
			if !l.numberEndOK() {
				return illegal("")
			}
			return token{kind: tNumber, num: digitsValue(digits, base), bigint: bigint}
		}
		if c := l.src[l.pos+1]; c >= '0' && c <= '9' || c == '_' {
			// A legacy octal literal (sloppy mode); with an 8 or 9 it is decimal.
			l.pos++
			if c == '_' {
				return illegal("Numeric separator can not be used after leading 0.")
			}
			digits, _ := l.scanDigits(10, false)
			if !strings.ContainsAny(digits, "89") {
				if !l.numberEndOK() {
					return illegal("")
				}
				return token{kind: tNumber, num: digitsValue(digits, 8)}
			}
			l.pos = start
		}
	}
	legacyDecimal := l.src[l.pos] == '0' && l.pos+1 < len(l.src) && l.src[l.pos+1] >= '0' && l.src[l.pos+1] <= '9'
	var text strings.Builder
	intPart, msg := l.scanDigits(10, !legacyDecimal)
	if msg != "" {
		return illegal(msg)
	}
	text.WriteString(intPart)
	simpleInt := true
	if l.pos < len(l.src) && l.src[l.pos] == '.' {
		simpleInt = false
		l.pos++
		if l.pos < len(l.src) && l.src[l.pos] == '_' {
			return illegal("")
		}
		frac, msg := l.scanDigits(10, true)
		if msg != "" {
			return illegal(msg)
		}
		text.WriteString(".")
		text.WriteString(frac)
	}
	if l.pos < len(l.src) && l.src[l.pos]|0x20 == 'e' {
		simpleInt = false
		l.pos++
		text.WriteByte('e')
		if l.pos < len(l.src) && (l.src[l.pos] == '+' || l.src[l.pos] == '-') {
			text.WriteByte(l.src[l.pos])
			l.pos++
		}
		exp, msg := l.scanDigits(10, true)
		if msg != "" || exp == "" {
			return illegal(msg)
		}
		text.WriteString(exp)
	}
	bigint := false
	if simpleInt && intPart != "" && !legacyDecimal {
		bigint = l.eatBigIntSuffix()
	}
	if !l.numberEndOK() {
		return illegal("")
	}
	s := text.String()
	if strings.HasPrefix(s, ".") {
		s = "0" + s
	}
	// The literal was validated above; ParseFloat only reports ErrRange, with the
	// correctly rounded ±Inf or 0 that the literal denotes.
	v, _ := strconv.ParseFloat(s, 64)
	return token{kind: tNumber, num: v, bigint: bigint}
}

// eatBigIntSuffix consumes the n of a BigInt literal.
func (l *lexer) eatBigIntSuffix() bool {
	if l.pos < len(l.src) && l.src[l.pos] == 'n' {
		l.pos++
		return true
	}
	return false
}

// numberEndOK applies the rule that a numeric literal may not be followed directly
// by an identifier start or a digit.
func (l *lexer) numberEndOK() bool {
	r, _ := l.peekRune(l.pos)
	return r < 0 || !(isIDStart(r) || r == '\\' || (r >= '0' && r <= '9'))
}

// digitsValue converts validated digits in the given base, rounding the exact
// value once to the nearest double (ties to even) as a numeric literal does.
func digitsValue(digits string, base int) float64 {
	if base == 10 {
		v, _ := strconv.ParseFloat(digits, 64)
		return v
	}
	n, _ := new(big.Int).SetString(digits, base)
	v, _ := new(big.Float).SetInt(n).Float64()
	return v
}

// scanEscape decodes the escape sequence after a backslash in a string or template.
// It returns the code units, or a message for an invalid escape. legacyOctal is set
// for \1-\7 and \0 followed by a digit, and for \8 \9.
func (l *lexer) scanEscape(out []uint16) ([]uint16, string, bool) {
	r, n := l.peekRune(l.pos)
	l.pos += n
	switch r {
	case 'n':
		return append(out, '\n'), "", false
	case 't':
		return append(out, '\t'), "", false
	case 'r':
		return append(out, '\r'), "", false
	case 'b':
		return append(out, '\b'), "", false
	case 'f':
		return append(out, '\f'), "", false
	case 'v':
		return append(out, '\v'), "", false
	case '\r':
		if l.pos < len(l.src) && l.src[l.pos] == '\n' {
			l.pos++
		}
		return out, "", false
	case '\n', 0x2028, 0x2029:
		return out, "", false
	case 'x':
		if l.pos+2 <= len(l.src) && jsstr.HexDigitValue(rune(l.src[l.pos])) >= 0 && jsstr.HexDigitValue(rune(l.src[l.pos+1])) >= 0 {
			v := jsstr.HexDigitValue(rune(l.src[l.pos]))*16 + jsstr.HexDigitValue(rune(l.src[l.pos+1]))
			l.pos += 2
			return append(out, uint16(v)), "", false
		}
		return out, "Invalid hexadecimal escape sequence", false
	case 'u':
		cp, msg := l.scanUnicodeEscapeBody()
		if msg != "" {
			return out, msg, false
		}
		return jsstr.AppendCodePoint(out, cp), "", false
	case '0', '1', '2', '3', '4', '5', '6', '7':
		v := int(r - '0')
		next := func() (int, bool) {
			if l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '7' {
				return int(l.src[l.pos] - '0'), true
			}
			return 0, false
		}
		if r == '0' {
			if l.pos >= len(l.src) || l.src[l.pos] < '0' || l.src[l.pos] > '9' {
				return append(out, 0), "", false
			}
		}
		if d, ok := next(); ok {
			v = v*8 + d
			l.pos++
			if r <= '3' {
				if d, ok := next(); ok {
					v = v*8 + d
					l.pos++
				}
			}
		}
		return append(out, uint16(v)), "", true
	case '8', '9':
		return append(out, uint16(r)), "", true
	case -1:
		return out, "", false
	}
	return jsstr.AppendCodePoint(out, r), "", false
}

func (l *lexer) scanString(quote byte) token {
	l.pos++
	var out []uint16
	for {
		if l.pos >= len(l.src) {
			return token{kind: tIllegal}
		}
		r, n := l.peekRune(l.pos)
		switch {
		case r == rune(quote):
			l.pos++
			return token{kind: tString, str: jsstr.FromUTF16(out)}
		case r == '\\':
			l.pos++
			var msg string
			out, msg, _ = l.scanEscape(out)
			if msg != "" {
				return token{kind: tIllegal, msg: msg}
			}
		case r == '\n' || r == '\r':
			return token{kind: tIllegal}
		default:
			out = jsstr.AppendCodePoint(out, r)
			l.pos += n
		}
	}
}

// scanTemplate reads a template chunk after "`" or after the "}" that closes a
// substitution, up to "${" or the closing backtick.
func (l *lexer) scanTemplate() token {
	start := l.pos
	var out []uint16
	t := token{kind: tTemplate, cookedOK: true}
	for {
		if l.pos >= len(l.src) {
			// V8 reports a template running to the end as the end of input.
			return token{kind: tTemplate, unterminated: true}
		}
		r, n := l.peekRune(l.pos)
		switch {
		case r == '`':
			t.raw = normalizeRawCR(l.src[start:l.pos])
			l.pos++
			t.tail = true
			t.str = jsstr.FromUTF16(out)
			return t
		case r == '$' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '{':
			t.raw = normalizeRawCR(l.src[start:l.pos])
			l.pos += 2
			t.str = jsstr.FromUTF16(out)
			return t
		case r == '\\':
			l.pos++
			escStart := l.pos
			var msg string
			var legacy bool
			out, msg, legacy = l.scanEscape(out)
			if legacy && t.cookedOK {
				if l.src[escStart] == '8' || l.src[escStart] == '9' {
					msg = "\\8 and \\9 are not allowed in template strings."
				} else if !(l.src[escStart] == '0' && l.pos == escStart+1) {
					msg = "Octal escape sequences are not allowed in template strings."
				}
			}
			if msg != "" && t.cookedOK {
				t.cookedOK = false
				t.templErr = msg
			}
		case r == '\r':
			// CR and CRLF are normalized to LF in both cooked and raw values.
			l.pos++
			if l.pos < len(l.src) && l.src[l.pos] == '\n' {
				l.pos++
			}
			out = append(out, '\n')
		default:
			out = jsstr.AppendCodePoint(out, r)
			l.pos += n
		}
	}
}

func normalizeRawCR(s string) string {
	if !strings.Contains(s, "\r") {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// scanRegExp rescans from pos (a '/') as a regular expression literal.
func (l *lexer) scanRegExp(pos int) token {
	l.pos = pos + 1
	inClass := false
	missing := token{kind: tIllegal, msg: "Invalid regular expression: missing /"}
	for {
		if l.pos >= len(l.src) {
			return missing
		}
		r, n := l.peekRune(l.pos)
		if jsstr.IsLineTerminator(r) {
			return missing
		}
		l.pos += n
		switch r {
		case '\\':
			if l.pos >= len(l.src) {
				return missing
			}
			r2, n2 := l.peekRune(l.pos)
			if jsstr.IsLineTerminator(r2) {
				return missing
			}
			l.pos += n2
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if inClass {
				continue
			}
			body := l.src[pos+1 : l.pos-1]
			flagStart := l.pos
			for l.pos < len(l.src) {
				r, n := l.peekRune(l.pos)
				if !isIDPart(r) && r != '\\' {
					break
				}
				l.pos += n
			}
			return token{kind: tRegExp, text: body, flags: l.src[flagStart:l.pos], pos: pos, end: l.pos}
		}
	}
}
