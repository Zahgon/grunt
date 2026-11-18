package jsregexp

import (
	"slices"
	"unicode/utf16"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/jsrt/unicase"
)

// The parser follows V8's RegExpParserImpl (src/regexp/regexp-parser.cc) step by
// step, so that the first error it meets, and therefore the message, is V8's.

// node is a parsed pattern element.
type node interface{}

type (
	// altNode is a disjunction.
	altNode struct{ alts []node }
	// seqNode is an alternative: terms matched one after another.
	seqNode struct{ items []node }
	// charNode matches one character; with icase, c is already canonical.
	charNode struct {
		c     rune
		icase bool
	}
	// setNode matches one character of set (or, when invert, one not in it), or
	// with the v flag one of strs, tried longest first before the characters.
	setNode struct {
		set    charSet
		strs   [][]rune
		invert bool
		icase  bool
	}
	// anyNode is "." : any character but a line terminator, unless dotAll.
	anyNode struct{ dotAll bool }
	// assertNode is ^, $, \b or \B.
	assertNode struct {
		kind      assertKind
		multiline bool
		word      charSet
	}
	// groupNode is a capturing group.
	groupNode struct {
		index int
		body  node
	}
	// lookNode is a lookahead or lookbehind.
	lookNode struct {
		behind, negate bool
		body           node
	}
	// repeatNode is a quantified atom; captures capLo..capHi (inclusive, capLo >
	// capHi when none) are reset before every iteration.
	repeatNode struct {
		body         node
		min, max     int
		greedy       bool
		capLo, capHi int
	}
	// backrefNode matches the text of the first participating group in groups.
	backrefNode struct {
		groups []int
		name   string
		icase  bool
	}
	// emptyNode matches the empty string.
	emptyNode struct{}
)

type assertKind uint8

const (
	assertStart assertKind = iota
	assertEnd
	assertWordBoundary
	assertNotWordBoundary
)

// infinity is V8's RegExpTree::kInfinity, the bound of an open quantifier and the
// value a quantifier number saturates to.
const infinity = 1<<31 - 1

// maxCaptures is V8's limit on capture groups and backreference numbers.
const maxCaptures = 1 << 16

// endMarker is the character current() returns past the end of the pattern. As
// in V8 it is larger than every character, which decides range-order checks
// against a class cut short by the end of the pattern.
const endMarker rune = 1 << 21

// syntaxError is panicked by fail and recovered in parse.
type syntaxError string

// disjState is the part of V8's RegExpParserState that decides whether a
// duplicate group name is allowed: the capture count when the disjunction began
// and, once a '|' was seen, the count at the latest '|'. Captures numbered in
// [captureIndex, lastBar] are taken to lie in an earlier alternative. For a
// non-capturing group captureIndex is the number of the last capture before it,
// so V8 (and this parser) also accepts "(?<a>x)(?:y|(?<a>z))".
type disjState struct {
	captureIndex int
	lastBar      int
	hasBar       bool
}

// openGroup is a capture group whose closing parenthesis is not yet parsed.
type openGroup struct {
	index int
	name  string
}

// classOperandType is V8's ClassSetOperandType for the v flag's class syntax.
type classOperandType uint8

const (
	operandCharacter classOperandType = iota
	operandStrings
	operandEscape
	operandNested
	operandRange
)

type parser struct {
	src           []uint16
	pos           int
	unicode       bool // u or v: code points and strict escapes
	vmode         bool
	icase         bool
	multiline     bool
	dotAll        bool
	ncap          int
	names         []string
	states        []*disjState
	open          []openGroup // the capture groups being parsed
	totalCaptures int
	hasNamed      bool
	scanned       bool
	refs          []*backrefNode
}

func newParser(src []uint16, f flagSet) *parser {
	return &parser{
		src:       src,
		unicode:   f.unicode || f.unicodeSets,
		vmode:     f.unicodeSets,
		icase:     f.ignoreCase,
		multiline: f.multiline,
		dotAll:    f.dotAll,
		names:     []string{""},
	}
}

func (p *parser) fail(msg string) {
	panic(syntaxError(msg))
}

// parse parses the whole pattern, returning the tree or V8's error message.
func (p *parser) parse() (tree node, msg string) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(syntaxError)
			if !ok {
				panic(r)
			}
			tree, msg = nil, string(e)
		}
	}()
	tree = p.disjunction(false)
	p.states = p.states[:0]
	p.resolveNamedReferences()
	return tree, ""
}

// charAt decodes the character at unit index i: a code point (pairing
// surrogates) in unicode mode, a code unit otherwise.
func (p *parser) charAt(i int, pairs bool) (rune, int) {
	if i >= len(p.src) {
		return endMarker, 0
	}
	if pairs {
		return jsstr.CodePointAt(p.src, i)
	}
	return rune(p.src[i]), 1
}

func (p *parser) current() rune {
	c, _ := p.charAt(p.pos, p.unicode)
	return c
}

func (p *parser) next() rune {
	_, n := p.charAt(p.pos, p.unicode)
	if n == 0 {
		return endMarker
	}
	c, _ := p.charAt(p.pos+n, p.unicode)
	return c
}

func (p *parser) advance() {
	_, n := p.charAt(p.pos, p.unicode)
	if n == 0 {
		p.pos = len(p.src) + 1
		return
	}
	p.pos += n
}

func (p *parser) advanceN(n int) {
	for range n {
		p.advance()
	}
}

func (p *parser) hasMore() bool { return p.pos < len(p.src) }

// disjunction parses alternatives up to the closing parenthesis of a group (sub)
// or the end of the pattern. It pushes the disjunction's state, which the caller
// pops once it has registered the group's name.
func (p *parser) disjunction(sub bool) node {
	state := &disjState{captureIndex: p.ncap}
	p.states = append(p.states, state)
	var alts []node
	for {
		alts = append(alts, p.alternative())
		switch p.current() {
		case '|':
			p.advance()
			state.hasBar, state.lastBar = true, p.ncap
			continue
		case endMarker:
			if sub {
				p.fail("Unterminated group")
			}
		case ')':
			if !sub {
				p.fail("Unmatched ')'")
			}
		}
		break
	}
	if len(alts) == 1 {
		return alts[0]
	}
	return &altNode{alts: alts}
}

// alternative parses terms up to '|', ')' or the end.
func (p *parser) alternative() node {
	var items []node
	for {
		var atom node
		quantifiable := true
		capsBefore := p.ncap
		switch c := p.current(); c {
		case endMarker, '|', ')':
			return &seqNode{items: items}
		case '*', '+', '?':
			p.fail("Nothing to repeat")
		case '^':
			p.advance()
			items = append(items, &assertNode{kind: assertStart, multiline: p.multiline})
			continue
		case '$':
			p.advance()
			items = append(items, &assertNode{kind: assertEnd, multiline: p.multiline})
			continue
		case '.':
			p.advance()
			atom = &anyNode{dotAll: p.dotAll}
		case '(':
			atom, quantifiable = p.group()
		case '[':
			atom = p.characterClass()
		case '\\':
			var assertion bool
			atom, assertion = p.atomEscape()
			if assertion {
				items = append(items, atom)
				continue
			}
		case '{':
			if _, _, ok := p.intervalQuantifier(); ok {
				p.fail("Nothing to repeat")
			}
			if p.unicode {
				p.fail("Lone quantifier brackets")
			}
			p.advance()
			atom = p.literal(c)
		case '}', ']':
			if p.unicode {
				p.fail("Lone quantifier brackets")
			}
			p.advance()
			atom = p.literal(c)
		default:
			p.advance()
			atom = p.literal(c)
		}
		lo, hi, isQuantifier := 0, 0, true
		switch p.current() {
		case '*':
			lo, hi = 0, infinity
			p.advance()
		case '+':
			lo, hi = 1, infinity
			p.advance()
		case '?':
			lo, hi = 0, 1
			p.advance()
		case '{':
			var ok bool
			if lo, hi, ok = p.intervalQuantifier(); ok {
				if hi < lo {
					p.fail("numbers out of order in {} quantifier")
				}
			} else if p.unicode {
				p.fail("Incomplete quantifier")
			} else {
				isQuantifier = false
			}
		default:
			isQuantifier = false
		}
		if !isQuantifier {
			items = append(items, atom)
			continue
		}
		greedy := true
		if p.current() == '?' {
			greedy = false
			p.advance()
		}
		if !quantifiable {
			p.fail("Invalid quantifier")
		}
		if _, empty := atom.(*emptyNode); empty {
			// V8 drops a quantifier on an empty atom (a backreference inside its
			// own group).
			items = append(items, atom)
			continue
		}
		items = append(items, &repeatNode{body: atom, min: lo, max: hi, greedy: greedy, capLo: capsBefore + 1, capHi: p.ncap})
	}
}

// literal returns the node matching the pattern character c.
func (p *parser) literal(c rune) node {
	if p.icase {
		return &charNode{c: unicase.Canonicalize(c, p.unicode), icase: true}
	}
	return &charNode{c: c}
}

// intervalQuantifier parses {n}, {n,} or {n,m} at the current '{', leaving the
// position unchanged when the text is not a complete quantifier.
func (p *parser) intervalQuantifier() (lo, hi int, ok bool) {
	start := p.pos
	p.advance()
	if !isDecimalDigit(p.current()) {
		p.pos = start
		return 0, 0, false
	}
	lo = p.decimal()
	switch p.current() {
	case '}':
		hi = lo
		p.advance()
	case ',':
		p.advance()
		if p.current() == '}' {
			hi = infinity
			p.advance()
			break
		}
		hi = p.decimal()
		if p.current() != '}' {
			p.pos = start
			return 0, 0, false
		}
		p.advance()
	default:
		p.pos = start
		return 0, 0, false
	}
	return lo, hi, true
}

// decimal reads decimal digits, saturating at infinity as V8 does.
func (p *parser) decimal() int {
	v := 0
	for isDecimalDigit(p.current()) {
		d := int(p.current() - '0')
		if v > (infinity-d)/10 {
			for isDecimalDigit(p.current()) {
				p.advance()
			}
			return infinity
		}
		v = v*10 + d
		p.advance()
	}
	return v
}

func isDecimalDigit(c rune) bool { return c >= '0' && c <= '9' }

// group parses a parenthesized group at '(' and reports whether a quantifier may
// follow it.
func (p *parser) group() (node, bool) {
	type kind uint8
	const (
		capture kind = iota
		grouping
		lookahead
		lookbehind
	)
	k := capture
	negate := false
	named := false
	icase, multiline, dotAll := p.icase, p.multiline, p.dotAll
	parsingModifiers := false
	polarity := true
	var seen [3]bool // i, m, s already named in this group
	p.advance()
	if p.current() == '?' {
		for {
			switch p.next() {
			case '-':
				p.advance()
				parsingModifiers = true
				if !polarity {
					p.fail("Multiple dashes in flag group")
				}
				polarity = false
			case 'i', 'm', 's':
				p.advance()
				parsingModifiers = true
				var flag *bool
				idx := 0
				switch p.current() {
				case 'i':
					flag, idx = &icase, 0
				case 'm':
					flag, idx = &multiline, 1
				default:
					flag, idx = &dotAll, 2
				}
				if seen[idx] {
					p.fail("Repeated flag in flag group")
				}
				seen[idx] = true
				*flag = polarity
			case ':':
				p.advanceN(2)
				parsingModifiers = false
				k = grouping
			case '=', '!':
				negate = p.next() == '!'
				p.advanceN(2)
				k = lookahead
			case '<':
				p.advance()
				if parsingModifiers {
					p.fail("Invalid group")
				}
				if p.next() == '=' || p.next() == '!' {
					negate = p.next() == '!'
					p.advanceN(2)
					k = lookbehind
					break
				}
				named = true
				p.advance()
			default:
				p.fail("Invalid group")
			}
			if !parsingModifiers {
				break
			}
		}
	}
	if !polarity && !seen[0] && !seen[1] && !seen[2] {
		p.fail("Invalid flag group")
	}
	index, name := 0, ""
	if k == capture {
		if p.ncap >= maxCaptures {
			p.fail("Too many captures")
		}
		p.ncap++
		index = p.ncap
		p.names = append(p.names, "")
		if named {
			p.hasNamed = true
			name = p.captureGroupName()
		}
		p.open = append(p.open, openGroup{index: index, name: name})
	}
	saved := [3]bool{p.icase, p.multiline, p.dotAll}
	p.icase, p.multiline, p.dotAll = icase, multiline, dotAll
	body := p.disjunction(true)
	p.icase, p.multiline, p.dotAll = saved[0], saved[1], saved[2]
	p.advance() // ')'
	if named {
		// V8 registers a name when its group closes.
		p.defineName(index, name)
	}
	p.states = p.states[:len(p.states)-1]
	switch k {
	case capture:
		p.open = p.open[:len(p.open)-1]
		return &groupNode{index: index, body: body}, true
	case lookahead:
		return &lookNode{negate: negate, body: body}, !p.unicode
	case lookbehind:
		return &lookNode{behind: true, negate: negate, body: body}, false
	}
	return body, true
}

// defineName records the name of capture index. A name already used is a
// duplicate unless the earlier group lies in an earlier alternative of a
// disjunction enclosing the new group (see disjState).
func (p *parser) defineName(index int, name string) {
	for i, other := range p.names {
		if other != name || i == index {
			continue
		}
		if !slices.ContainsFunc(p.states, func(s *disjState) bool {
			return s.hasBar && s.captureIndex <= i && i <= s.lastBar
		}) {
			p.fail("Duplicate capture group name")
		}
	}
	p.names[index] = name
}

// captureGroupName parses a group name after "(?<" or "\k<" up to and including
// '>'. Surrogate pairs and \u escapes are read as in unicode mode whatever the
// flags, as V8 does.
func (p *parser) captureGroupName() string {
	var name []uint16
	for atStart := true; ; atStart = false {
		c, n := p.charAt(p.pos, true)
		p.pos += max(n, 1)
		if c == '\\' && p.pos < len(p.src) && p.src[p.pos] == 'u' {
			p.pos++
			v, ok := p.unicodeEscape(true)
			if !ok {
				p.fail("Invalid Unicode escape")
			}
			c = v
		}
		if c == '\\' {
			p.fail("Invalid capture group name")
		}
		switch {
		case atStart:
			if !isIdentifierStart(c) {
				p.fail("Invalid capture group name")
			}
		case c == '>':
			return jsstr.FromUTF16WellFormed(name)
		case !isIdentifierPart(c):
			p.fail("Invalid capture group name")
		}
		name = utf16.AppendRune(name, c)
	}
}

func isIdentifierStart(c rune) bool {
	if c == '$' || c == '_' || (c|0x20) >= 'a' && (c|0x20) <= 'z' {
		return true
	}
	return c >= 0x80 && propertySet("bin=ID_Start").has(c)
}

func isIdentifierPart(c rune) bool {
	if c == '$' || c == '_' || c == 0x200C || c == 0x200D || isDecimalDigit(c) || (c|0x20) >= 'a' && (c|0x20) <= 'z' {
		return true
	}
	return c >= 0x80 && propertySet("bin=ID_Continue").has(c)
}

// unicodeEscape parses the rest of \u (after the 'u'): \uXXXX, and with
// unicode also \u{X...} and an escaped surrogate pair \uXXXX\uXXXX.
func (p *parser) unicodeEscape(unicodeMode bool) (rune, bool) {
	if unicodeMode && p.pos < len(p.src) && p.src[p.pos] == '{' {
		start := p.pos
		p.pos++
		v, digits := rune(0), 0
		for p.pos < len(p.src) && jsstr.HexDigitValue(rune(p.src[p.pos])) >= 0 {
			v = v*16 + rune(jsstr.HexDigitValue(rune(p.src[p.pos])))
			digits++
			p.pos++
			if v > 0x10FFFF {
				p.pos = start
				return 0, false
			}
		}
		if digits > 0 && p.pos < len(p.src) && p.src[p.pos] == '}' {
			p.pos++
			return v, true
		}
		p.pos = start
		return 0, false
	}
	v, ok := p.hexEscape(4)
	if ok && unicodeMode && v >= 0xD800 && v < 0xDC00 && p.pos+1 < len(p.src) && p.src[p.pos] == '\\' && p.src[p.pos+1] == 'u' {
		start := p.pos
		p.pos += 2
		if trail, ok := p.hexEscape(4); ok && trail >= 0xDC00 && trail < 0xE000 {
			return utf16.DecodeRune(v, trail), true
		}
		p.pos = start
	}
	return v, ok
}

// hexEscape reads exactly n hex digits, leaving the position unchanged when
// there are fewer.
func (p *parser) hexEscape(n int) (rune, bool) {
	if p.pos+n > len(p.src) {
		return 0, false
	}
	v := rune(0)
	for i := range n {
		c := rune(p.src[p.pos+i])
		d := jsstr.HexDigitValue(c)
		if d < 0 {
			return 0, false
		}
		v = v*16 + rune(d)
	}
	p.pos += n
	return v, true
}

// atomEscape parses an escape at '\' outside a class. assertion reports \b and \B.
func (p *parser) atomEscape() (n node, assertion bool) {
	switch next := p.next(); next {
	case endMarker:
		p.fail("\\ at end of pattern")
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
		if index, ok := p.backReferenceIndex(); ok {
			if slices.ContainsFunc(p.open, func(g openGroup) bool { return g.index == index }) {
				return &emptyNode{}, false
			}
			return &backrefNode{groups: []int{index}, icase: p.icase}, false
		}
		if p.unicode {
			p.fail("Invalid escape")
		}
		if next == '8' || next == '9' {
			p.advanceN(2)
			return p.literal(next), false
		}
		p.advance()
		return p.literal(p.octal()), false
	case '0':
		p.advance()
		if p.unicode && isDecimalDigit(p.next()) {
			p.fail("Invalid decimal escape")
		}
		return p.literal(p.octal()), false
	case 'b', 'B':
		p.advanceN(2)
		kind := assertWordBoundary
		if next == 'B' {
			kind = assertNotWordBoundary
		}
		return &assertNode{kind: kind, word: wordCharacters(p.unicode, p.icase)}, true
	case 'd', 'D', 's', 'S', 'w', 'W', 'p', 'P':
		if set, strs, ok := p.classEscape(next, false); ok {
			return p.setAtom(set, strs, false), false
		}
	case 'k':
		if p.unicode || p.namedCaptures(false) {
			p.advanceN(2)
			return p.namedBackReference(), false
		}
	}
	p.advance()
	c := p.characterEscape(false)
	return p.literal(c), false
}

// setAtom wraps a resolved class in a node, applying the case rules of the mode.
func (p *parser) setAtom(set charSet, strs [][]rune, invert bool) node {
	return &setNode{set: set, strs: strs, invert: invert, icase: p.icase}
}

// backReferenceIndex parses \N at '\' and reports whether N names an existing
// group; otherwise the position is left unchanged.
func (p *parser) backReferenceIndex() (int, bool) {
	start := p.pos
	p.advance()
	v := 0
	for isDecimalDigit(p.current()) {
		v = v*10 + int(p.current()-'0')
		if v > maxCaptures {
			p.pos = start
			return 0, false
		}
		p.advance()
	}
	if v > p.ncap && v > p.captureCount() {
		p.pos = start
		return 0, false
	}
	return v, true
}

// octal parses a legacy octal escape at its first digit.
func (p *parser) octal() rune {
	v := p.current() - '0'
	p.advance()
	if c := p.current(); c >= '0' && c <= '7' {
		v = v*8 + c - '0'
		p.advance()
		if c := p.current(); v < 32 && c >= '0' && c <= '7' {
			v = v*8 + c - '0'
			p.advance()
		}
	}
	return v
}

// captureCount returns the number of capturing groups in the whole pattern.
func (p *parser) captureCount() int {
	p.scanForCaptures(false)
	return p.totalCaptures
}

// namedCaptures reports whether the pattern has any named group; inClass tells
// whether the position is inside a character class.
func (p *parser) namedCaptures(inClass bool) bool {
	if p.hasNamed || p.scanned {
		return p.hasNamed
	}
	p.scanForCaptures(inClass)
	return p.hasNamed
}

// scanForCaptures is V8's ScanForCaptures: a lexical pass over the rest of the
// pattern counting capturing groups and noting named ones. inClass skips the
// rest of the class the position is in.
func (p *parser) scanForCaptures(inClass bool) {
	if p.scanned {
		return
	}
	p.scanned = true
	count := p.ncap
	i := p.pos
	skipClass := func() {
		for i < len(p.src) {
			c := p.src[i]
			i++
			if c == '\\' {
				i++
			} else if c == ']' {
				return
			}
		}
	}
	if inClass {
		skipClass()
	}
	for i < len(p.src) {
		c := p.src[i]
		i++
		switch c {
		case '\\':
			i++
		case '[':
			skipClass()
		case '(':
			if i < len(p.src) && p.src[i] == '?' {
				i++
				if i >= len(p.src) || p.src[i] != '<' {
					continue
				}
				i++
				if i < len(p.src) && (p.src[i] == '=' || p.src[i] == '!') {
					continue
				}
				p.hasNamed = true
			}
			count++
		}
	}
	p.totalCaptures = count
}

// namedBackReference parses \k<name> after "\k".
func (p *parser) namedBackReference() node {
	if p.current() != '<' {
		p.fail("Invalid named reference")
	}
	p.advance()
	name := p.captureGroupName()
	for _, g := range p.open {
		if g.name == name {
			return &emptyNode{}
		}
	}
	ref := &backrefNode{name: name, icase: p.icase}
	p.refs = append(p.refs, ref)
	return ref
}

// resolveNamedReferences binds every \k<name> once all groups are known.
func (p *parser) resolveNamedReferences() {
	if len(p.refs) == 0 {
		return
	}
	if !slices.ContainsFunc(p.names, func(n string) bool { return n != "" }) {
		p.fail("Invalid named capture referenced")
	}
	for _, ref := range p.refs {
		for i, n := range p.names {
			if n == ref.name {
				ref.groups = append(ref.groups, i)
			}
		}
		if len(ref.groups) == 0 {
			p.fail("Invalid named capture referenced")
		}
	}
}

// characterEscape is V8's ParseCharacterEscape with the position on the
// character after '\'.
func (p *parser) characterEscape(inClass bool) rune {
	c := p.current()
	switch c {
	case 'f':
		p.advance()
		return '\f'
	case 'n':
		p.advance()
		return '\n'
	case 'r':
		p.advance()
		return '\r'
	case 't':
		p.advance()
		return '\t'
	case 'v':
		p.advance()
		return '\v'
	case 'c':
		control := p.next()
		if letter := control &^ 0x20; letter >= 'A' && letter <= 'Z' {
			p.advanceN(2)
			return control & 0x1F
		}
		if p.unicode {
			p.fail("Invalid Unicode escape")
		}
		if inClass && (isDecimalDigit(control) || control == '_') {
			p.advanceN(2)
			return control & 0x1F
		}
		// \c is a literal backslash; the 'c' is read again as a character.
		return '\\'
	case '0':
		if p.unicode && !isDecimalDigit(p.next()) {
			p.advance()
			return 0
		}
		fallthrough
	case '1', '2', '3', '4', '5', '6', '7':
		if p.unicode {
			p.fail("Invalid decimal escape")
		}
		return p.octal()
	case 'x':
		p.advance()
		if v, ok := p.hexEscape(2); ok {
			return v
		}
		if p.unicode {
			p.fail("Invalid escape")
		}
		return 'x'
	case 'u':
		p.advance()
		if v, ok := p.unicodeEscape(p.unicode); ok {
			return v
		}
		if p.unicode {
			p.fail("Invalid Unicode escape")
		}
		return 'u'
	case 'k':
		if !p.unicode && inClass && p.namedCaptures(true) {
			p.fail("Invalid escape")
		}
	}
	if p.unicode && !isSyntaxCharacter(c) && c != '/' && !(inClass && (c == '-' || p.vmode && isClassSetReservedPunctuator(c))) {
		p.fail("Invalid escape")
	}
	p.advance()
	return c
}

func isSyntaxCharacter(c rune) bool {
	switch c {
	case '^', '$', '\\', '.', '*', '+', '?', '(', ')', '[', ']', '{', '}', '|':
		return true
	}
	return false
}

func isClassSetReservedPunctuator(c rune) bool {
	switch c {
	case '&', '-', '!', '#', '%', ',', ':', ';', '<', '=', '>', '@', '`', '~':
		return true
	}
	return false
}

// isClassSetReservedDoublePunctuator reports whether c doubled ("&&", "!!", ...)
// is reserved inside a v-mode class.
func isClassSetReservedDoublePunctuator(c rune) bool {
	switch c {
	case '&', '!', '#', '$', '%', '*', '+', ',', '.', ':', ';', '<', '=', '>', '?', '@', '^', '`', '~':
		return true
	}
	return false
}

func isClassSetSyntaxCharacter(c rune) bool {
	switch c {
	case '(', ')', '[', ']', '{', '}', '/', '-', '\\', '|':
		return true
	}
	return false
}

// classEscape parses \d \D \s \S \w \W \p{..} \P{..} at '\' (kind is the letter),
// returning the set (and, for v-mode string properties, strings). It reports
// false, leaving the position unchanged, for \p and \P without u or v.
func (p *parser) classEscape(kind rune, inClass bool) (charSet, [][]rune, bool) {
	var set charSet
	switch kind {
	case 'd', 'D':
		set = digitSet
	case 's', 'S':
		set = spaceSet
	case 'w', 'W':
		set = wordCharacters(p.unicode, p.icase)
	case 'p', 'P':
		if !p.unicode {
			return nil, nil, false
		}
		p.advanceN(2)
		name, value, ok := p.propertyName()
		msg := "Invalid property name"
		if inClass {
			msg = "Invalid property name in character class"
		}
		if !ok {
			p.fail(msg)
		}
		key, ok := propertyKey(name, value)
		if !ok {
			if p.vmode && value == "" && stringProperties[name] {
				p.fail("unsupported: the property of strings \\p{" + name + "} is not supported")
			}
			p.fail(msg)
		}
		set = propertySet(key)
		if p.vmode && p.icase {
			set = set.fold()
		}
		if kind == 'P' {
			set = p.complement(set)
		}
		return set, nil, true
	default:
		return nil, nil, false
	}
	p.advanceN(2)
	if p.vmode && p.icase {
		set = set.fold()
	}
	if kind == 'D' || kind == 'S' || kind == 'W' {
		set = p.complement(set)
	}
	return set, nil, true
}

// complement is CharacterComplement: with v and i the universe is the canonical
// characters only.
func (p *parser) complement(set charSet) charSet {
	if p.vmode && p.icase {
		return canonicalUniverse().minus(set, 0x10FFFF)
	}
	return set.complement(maxChar(p.unicode))
}

// propertyName parses {name} or {name=value} after \p.
func (p *parser) propertyName() (name, value string, ok bool) {
	if p.current() != '{' {
		return "", "", false
	}
	var b []byte
	p.advance()
	for c := p.current(); c != '}' && c != '='; c = p.current() {
		if !isPropertyCharacter(c) || p.next() == endMarker {
			return "", "", false
		}
		b = append(b, byte(c))
		p.advance()
	}
	name = string(b)
	if p.current() == '=' {
		b = b[:0]
		p.advance()
		for c := p.current(); c != '}'; c = p.current() {
			if !isPropertyCharacter(c) || p.next() == endMarker {
				return "", "", false
			}
			b = append(b, byte(c))
			p.advance()
		}
		value = string(b)
		if value == "" {
			return "", "", false
		}
	}
	p.advance()
	if name == "" {
		return "", "", false
	}
	return name, value, true
}

func isPropertyCharacter(c rune) bool {
	return isDecimalDigit(c) || c == '_' || (c|0x20) >= 'a' && (c|0x20) <= 'z'
}

// characterClass parses a class at '['.
func (p *parser) characterClass() node {
	p.advance()
	negated := false
	if p.current() == '^' {
		negated = true
		p.advance()
	}
	if p.current() == ']' {
		p.advance()
		if negated {
			return p.setAtom(newCharSet(0, maxChar(p.unicode)), nil, false)
		}
		return p.setAtom(nil, nil, false)
	}
	if p.vmode {
		set, strs := p.classSetExpression(negated)
		return p.setAtom(set, strs, false)
	}
	var pairs []rune
	for p.hasMore() && p.current() != ']' {
		c1, set1, isClass1 := p.classAtom()
		if isClass1 {
			pairs = append(pairs, set1...)
		}
		if p.current() == '-' {
			p.advance()
			if p.current() == endMarker {
				break
			}
			if p.current() == ']' {
				if !isClass1 {
					pairs = append(pairs, c1, c1)
				}
				pairs = append(pairs, '-', '-')
				break
			}
			c2, set2, isClass2 := p.classAtom()
			if isClass1 || isClass2 {
				if p.unicode {
					p.fail("Invalid character class")
				}
				if !isClass1 {
					pairs = append(pairs, c1, c1)
				}
				pairs = append(pairs, '-', '-')
				if isClass2 {
					pairs = append(pairs, set2...)
				} else {
					pairs = append(pairs, c2, c2)
				}
				continue
			}
			if c1 > c2 {
				p.fail("Range out of order in character class")
			}
			pairs = append(pairs, c1, c2)
			continue
		}
		if !isClass1 {
			pairs = append(pairs, c1, c1)
		}
	}
	if !p.hasMore() {
		p.fail("Unterminated character class")
	}
	p.advance()
	return p.setAtom(newCharSet(pairs...), nil, negated)
}

// classAtom is V8's ParseClassEscape: one class character or class escape.
func (p *parser) classAtom() (c rune, set charSet, isClass bool) {
	if p.current() != '\\' {
		c = p.current()
		p.advance()
		return c, nil, false
	}
	switch next := p.next(); next {
	case 'b':
		p.advanceN(2)
		return '\b', nil, false
	case '-':
		if p.unicode {
			p.advanceN(2)
			return '-', nil, false
		}
	case endMarker:
		p.fail("\\ at end of pattern")
	default:
		if set, _, ok := p.classEscape(next, true); ok {
			return 0, set, true
		}
	}
	p.advance()
	return p.characterEscape(true), nil, false
}

// classSetExpression parses the body of a v-mode class after '[' or '[^',
// through its ']'.
func (p *parser) classSetExpression(negated bool) (charSet, [][]rune) {
	set, strs, typ, c := p.classSetOperand()
	switch p.current() {
	case '-':
		if p.next() == '-' {
			return p.classSubtraction(negated, set, strs)
		}
	case '&':
		if p.next() == '&' {
			return p.classIntersection(negated, set, strs)
		}
	}
	return p.classUnion(negated, set, strs, typ, c)
}

// classSetOperand parses one operand: a nested class, a class escape, \q{...}
// or a single character (returned in c for ranges).
func (p *parser) classSetOperand() (set charSet, strs [][]rune, typ classOperandType, c rune) {
	if p.current() == '\\' {
		next := p.next()
		if next == 'q' {
			set, strs = p.classStringDisjunction()
			return set, strs, operandStrings, 0
		}
		if set, strs, ok := p.classEscape(next, true); ok {
			return set, strs, operandEscape, 0
		}
	}
	if p.current() == '[' {
		p.advance()
		negated := false
		if p.current() == '^' {
			negated = true
			p.advance()
		}
		if p.current() == ']' {
			p.advance()
			if negated {
				return p.complement(nil), nil, operandNested, 0
			}
			return nil, nil, operandNested, 0
		}
		set, strs = p.classSetExpression(negated)
		return set, strs, operandNested, 0
	}
	c = p.classSetCharacter()
	k := p.foldChar(c)
	return newCharSet(k, k), nil, operandCharacter, c
}

// foldChar is MaybeSimpleCaseFolding of one character.
func (p *parser) foldChar(c rune) rune {
	if p.icase {
		return unicase.Canonicalize(c, true)
	}
	return c
}

// classSetCharacter parses one ClassSetCharacter.
func (p *parser) classSetCharacter() rune {
	c := p.current()
	if c == '\\' {
		switch p.next() {
		case 'b':
			p.advanceN(2)
			return '\b'
		case endMarker:
			p.fail("\\ at end of pattern")
		}
		p.advance()
		return p.characterEscape(true)
	}
	if isClassSetSyntaxCharacter(c) {
		p.fail("Invalid character in character class")
	}
	if isClassSetReservedDoublePunctuator(c) && p.next() == c {
		p.fail("Invalid set operation in character class")
	}
	p.advance()
	return c
}

// classStringDisjunction parses \q{a|bc|...}.
func (p *parser) classStringDisjunction() (charSet, [][]rune) {
	p.advanceN(2)
	if p.current() != '{' {
		p.fail("Invalid escape")
	}
	p.advance()
	var pairs []rune
	var strs [][]rune
	var cur []rune
	add := func() {
		if len(cur) == 1 {
			pairs = append(pairs, cur[0], cur[0])
		} else {
			strs = append(strs, cur)
		}
		cur = nil
	}
	for p.hasMore() && p.current() != '}' {
		if p.current() == '|' {
			add()
			p.advance()
			continue
		}
		cur = append(cur, p.foldChar(p.classSetCharacter()))
	}
	add()
	p.advance()
	return newCharSet(pairs...), strs
}

// classUnion parses the rest of a union class (ranges and juxtaposed operands).
func (p *parser) classUnion(negated bool, set charSet, strs [][]rune, last classOperandType, lastChar rune) (charSet, [][]rune) {
	acc := set
	for p.hasMore() && p.current() != ']' {
		if (p.current() == '-' || p.current() == '&') && p.next() == p.current() {
			// Operators other than union are not allowed in a union.
			p.fail("Invalid set operation in character class")
		}
		if p.current() == '-' {
			if p.next() == endMarker {
				// V8 reports the missing ']' before an invalid range.
				p.fail("Unterminated character class")
			}
			if last != operandCharacter {
				p.fail("Invalid character class")
			}
			p.advance()
			_, _, typ, to := p.classSetOperand()
			if typ != operandCharacter {
				p.fail("Invalid character class")
			}
			if lastChar > to {
				p.fail("Range out of order in character class")
			}
			r := newCharSet(lastChar, to)
			if p.icase {
				r = r.fold()
			}
			acc = acc.union(r)
			last = operandRange
			continue
		}
		s, ss, typ, c := p.classSetOperand()
		acc = acc.union(s)
		strs = append(strs, ss...)
		last, lastChar = typ, c
	}
	if !p.hasMore() {
		p.fail("Unterminated character class")
	}
	p.advance()
	return p.finishClass(negated, acc, strs)
}

// classSubtraction parses A--B--C... after the first operand.
func (p *parser) classSubtraction(negated bool, set charSet, strs [][]rune) (charSet, [][]rune) {
	for p.current() == '-' && p.next() == '-' {
		p.advanceN(2)
		s, ss, _, _ := p.classSetOperand()
		set = set.minus(s, 0x10FFFF)
		strs = slices.DeleteFunc(strs, func(x []rune) bool { return containsString(ss, x) || len(x) == 1 && s.has(x[0]) })
	}
	if !p.hasMore() {
		p.fail("Unterminated character class")
	}
	if p.current() != ']' {
		p.fail("Invalid set operation in character class")
	}
	p.advance()
	return p.finishClass(negated, set, strs)
}

// classIntersection parses A&&B&&C... after the first operand.
func (p *parser) classIntersection(negated bool, set charSet, strs [][]rune) (charSet, [][]rune) {
	for p.current() == '&' && p.next() == '&' {
		p.advanceN(2)
		if p.current() == '&' {
			p.fail("Invalid character in character class")
		}
		s, ss, _, _ := p.classSetOperand()
		set = set.intersect(s)
		strs = slices.DeleteFunc(strs, func(x []rune) bool { return !containsString(ss, x) })
	}
	if !p.hasMore() {
		p.fail("Unterminated character class")
	}
	if p.current() != ']' {
		p.fail("Invalid set operation in character class")
	}
	p.advance()
	return p.finishClass(negated, set, strs)
}

func containsString(list [][]rune, s []rune) bool {
	return slices.ContainsFunc(list, func(x []rune) bool { return slices.Equal(x, s) })
}

// finishClass applies a v-mode class's negation.
func (p *parser) finishClass(negated bool, set charSet, strs [][]rune) (charSet, [][]rune) {
	if !negated {
		return set, strs
	}
	if len(strs) > 0 {
		p.fail("Negated character class may contain strings")
	}
	return p.complement(set), nil
}
