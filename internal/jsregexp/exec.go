package jsregexp

import (
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/jsrt/unicase"
)

// frameKind tells what a backtracking frame records.
type frameKind uint8

const (
	frameChoice     frameKind = iota // resume at pc, pos
	frameCapture                     // undo: caps[a] = b
	frameRegister                    // undo: regs[a] = b
	frameLook                        // a lookaround in progress; a = 1 when negative
	frameStarGreedy                  // a greedy opStar that can give back characters down to position a
	frameStarLazy                    // a lazy opStar (instruction b) that has taken a characters
)

// frame is an entry of the backtracking stack. Choice points and undo records
// share the stack, so failing past a choice point restores every capture and
// register written after it.
type frame struct {
	kind    frameKind
	pc, pos int32
	a, b    int32
}

// machine is the state of one match attempt.
type machine struct {
	re    *Regexp
	input []uint16
	caps  []int
	regs  []int
	stack []frame
}

// Exec runs the matcher of RegExpBuiltinExec on input from code-unit index
// start: with the sticky flag it tries only start, otherwise every index from
// start to the end. It returns 2*(NumCaptures()+1) code-unit offsets, -1 for
// groups that did not participate, or nil when there is no match.
//
// With u or v this follows V8 rather than the letter of the specification:
// matching scans every code-unit index, no character can be read from the middle
// of a surrogate pair (so only zero-width matches start there), and a global or
// sticky pattern tried at an index inside a pair first tries the index of the
// pair's lead surrogate. For example /\B/u matches "_😀 " at index 2, and /(?:)/gu
// with lastIndex 1 matches "😀" at index 0.
func (re *Regexp) Exec(input []uint16, start int) []int {
	if start < 0 || start > len(input) {
		return nil
	}
	unicodeMode := re.flags.unicode || re.flags.unicodeSets
	stepBack := unicodeMode && (re.flags.global || re.flags.sticky)
	m := &machine{re: re, input: input, caps: make([]int, 2*(re.ncap+1)), regs: make([]int, re.nregs)}
	first := rune(-1)
	if in := re.prog[1]; in.op == opChar && in.c < 0xD800 {
		first = in.c
	}
	for s := start; s <= len(input); s++ {
		if stepBack && m.midPair(s) {
			if caps := m.run(s - 1); caps != nil {
				return caps
			}
		}
		if first < 0 || s < len(input) && rune(input[s]) == first {
			if caps := m.run(s); caps != nil {
				return caps
			}
		}
		if re.flags.sticky {
			break
		}
	}
	return nil
}

// midPair reports whether index i lies between the halves of a surrogate pair.
func (m *machine) midPair(i int) bool {
	return i > 0 && i < len(m.input) && isLead(m.input[i-1]) && isTrail(m.input[i])
}

func isLead(c uint16) bool  { return c >= 0xD800 && c < 0xDC00 }
func isTrail(c uint16) bool { return c >= 0xDC00 && c < 0xE000 }

// run attempts a match starting exactly at s.
func (m *machine) run(s int) []int {
	for i := range m.caps {
		m.caps[i] = -1
	}
	m.stack = m.stack[:0]
	prog := m.re.prog
	pc, pos := 0, s
	for {
		in := &prog[pc]
		ok := true
		switch in.op {
		case opChar, opCharFold, opSet, opSetFold, opAny, opAll:
			pos, ok = m.step(in, pos)
			pc++
		case opString:
			pos, ok = m.matchString(in, pos)
			pc++
		case opAssert:
			ok = m.assert(in, pos)
			pc++
		case opSave:
			m.setCapture(in.x, pos)
			pc++
		case opReset:
			for i := in.x; i <= in.y; i++ {
				m.setCapture(i, -1)
			}
			pc++
		case opFork:
			m.push(frame{kind: frameChoice, pc: int32(in.x), pos: int32(pos)})
			pc++
		case opJmp:
			pc = in.x
		case opBackref:
			pos, ok = m.backref(in, pos)
			pc++
		case opLook:
			negate := int32(0)
			if in.negate {
				negate = 1
			}
			m.push(frame{kind: frameLook, pc: int32(in.x), pos: int32(pos), a: negate})
			pc++
		case opLookEnd:
			pc, pos, ok = m.lookEnd()
		case opRepInit:
			m.setRegister(in.x, 0)
			pc++
		case opRepTest:
			count := m.regs[in.x]
			switch {
			case count >= in.max:
				pc = in.y
			case count < in.min:
				pc++
			case in.greedy:
				m.push(frame{kind: frameChoice, pc: int32(in.y), pos: int32(pos)})
				pc++
			default:
				m.push(frame{kind: frameChoice, pc: int32(pc + 1), pos: int32(pos)})
				pc = in.y
			}
		case opRepMark:
			m.setRegister(in.x, pos)
			pc++
		case opRepNext:
			// RepeatMatcher's empty check: an optional iteration that consumed
			// nothing fails.
			count := m.regs[in.x]
			if count >= in.min && pos == m.regs[in.mark] {
				ok = false
				break
			}
			m.setRegister(in.x, count+1)
			pc = in.y
		case opStar:
			pos, ok = m.star(in, pc, pos)
			pc++
		case opMatch:
			out := make([]int, len(m.caps))
			for i := 0; i < len(out); i += 2 {
				if m.caps[i] < 0 || m.caps[i+1] < 0 {
					out[i], out[i+1] = -1, -1
				} else {
					out[i], out[i+1] = m.caps[i], m.caps[i+1]
				}
			}
			return out
		}
		if !ok {
			if pc, pos, ok = m.backtrack(); !ok {
				return nil
			}
		}
	}
}

func (m *machine) push(f frame) { m.stack = append(m.stack, f) }

func (m *machine) setCapture(i, v int) {
	if m.caps[i] != v {
		m.push(frame{kind: frameCapture, a: int32(i), b: int32(m.caps[i])})
		m.caps[i] = v
	}
}

func (m *machine) setRegister(i, v int) {
	if m.regs[i] != v {
		m.push(frame{kind: frameRegister, a: int32(i), b: int32(m.regs[i])})
		m.regs[i] = v
	}
}

// backtrack unwinds to the most recent choice point, undoing captures and
// registers on the way, and reports where to resume.
func (m *machine) backtrack() (pc, pos int, ok bool) {
	for len(m.stack) > 0 {
		top := len(m.stack) - 1
		f := &m.stack[top]
		switch f.kind {
		case frameCapture:
			m.caps[f.a] = int(f.b)
		case frameRegister:
			m.regs[f.a] = int(f.b)
		case frameChoice:
			m.stack = m.stack[:top]
			return int(f.pc), int(f.pos), true
		case frameLook:
			// The body failed: a negative lookaround succeeds.
			m.stack = m.stack[:top]
			if f.a == 1 {
				return int(f.pc), int(f.pos), true
			}
			continue
		case frameStarGreedy:
			if int(f.pos) != int(f.a) {
				in := &m.re.prog[f.b]
				f.pos = int32(m.giveBack(in, int(f.pos)))
				return int(f.b) + 1, int(f.pos), true
			}
		case frameStarLazy:
			in := &m.re.prog[f.b]
			if int(f.a) < in.max {
				if np, ok := m.step(in.sub, int(f.pos)); ok {
					f.pos = int32(np)
					f.a++
					return int(f.b) + 1, np, true
				}
			}
		}
		m.stack = m.stack[:top]
	}
	return 0, 0, false
}

// lookEnd finishes the innermost lookaround whose body just matched.
func (m *machine) lookEnd() (pc, pos int, ok bool) {
	i := len(m.stack) - 1
	for m.stack[i].kind != frameLook {
		i--
	}
	look := m.stack[i]
	if look.a == 0 {
		// A positive lookaround is atomic: its choice points go, the undo
		// records of the captures it set stay.
		j := i
		for _, f := range m.stack[i+1:] {
			if f.kind == frameCapture || f.kind == frameRegister {
				m.stack[j] = f
				j++
			}
		}
		m.stack = m.stack[:j]
		return int(look.pc), int(look.pos), true
	}
	// A negative lookaround whose body matched fails, leaving no trace.
	for len(m.stack) > i {
		top := len(m.stack) - 1
		switch f := m.stack[top]; f.kind {
		case frameCapture:
			m.caps[f.a] = int(f.b)
		case frameRegister:
			m.regs[f.a] = int(f.b)
		}
		m.stack = m.stack[:top]
	}
	return 0, 0, false
}

// star runs opStar at pc: it takes as many characters as allowed (greedy) or the
// minimum (lazy), and leaves a frame that gives back or takes more on failure.
func (m *machine) star(in *inst, pc, pos int) (int, bool) {
	if in.greedy {
		p, n := pos, 0
		minPos := -1
		if in.min == 0 {
			minPos = pos
		}
		for n < in.max {
			np, ok := m.step(in.sub, p)
			if !ok {
				break
			}
			p = np
			n++
			if n == in.min {
				minPos = p
			}
		}
		if n < in.min {
			return pos, false
		}
		if p != minPos {
			m.push(frame{kind: frameStarGreedy, pos: int32(p), a: int32(minPos), b: int32(pc)})
		}
		return p, true
	}
	p := pos
	for range in.min {
		np, ok := m.step(in.sub, p)
		if !ok {
			return pos, false
		}
		p = np
	}
	if in.max > in.min {
		m.push(frame{kind: frameStarLazy, pos: int32(p), a: int32(in.min), b: int32(pc)})
	}
	return p, true
}

// giveBack moves a greedy opStar's end back by one character.
func (m *machine) giveBack(in *inst, pos int) int {
	unicodeMode := in.sub.unicode
	if !in.back {
		if unicodeMode && pos >= 2 && isTrail(m.input[pos-1]) && isLead(m.input[pos-2]) {
			return pos - 2
		}
		return pos - 1
	}
	if unicodeMode && pos+1 < len(m.input) && isLead(m.input[pos]) && isTrail(m.input[pos+1]) {
		return pos + 2
	}
	return pos + 1
}

// read returns the character next to pos in the instruction's direction and the
// position past it.
func (m *machine) read(in *inst, pos int) (rune, int, bool) {
	if in.unicode && m.midPair(pos) {
		// V8 reads no character, in either direction, from inside a pair.
		return 0, pos, false
	}
	if !in.back {
		if pos >= len(m.input) {
			return 0, pos, false
		}
		if in.unicode {
			c, n := jsstr.CodePointAt(m.input, pos)
			return c, pos + n, true
		}
		return rune(m.input[pos]), pos + 1, true
	}
	if pos <= 0 {
		return 0, pos, false
	}
	if in.unicode {
		c, n := jsstr.CodePointBefore(m.input, pos)
		return c, pos - n, true
	}
	return rune(m.input[pos-1]), pos - 1, true
}

// step matches one character with a single-character instruction.
func (m *machine) step(in *inst, pos int) (int, bool) {
	c, np, ok := m.read(in, pos)
	if !ok {
		return pos, false
	}
	switch in.op {
	case opChar:
		ok = c == in.c
	case opCharFold:
		ok = unicase.Canonicalize(c, in.unicode) == in.c
	case opSet:
		ok = in.set.has(c) != in.invert
	case opSetFold:
		ok = in.set.has(unicase.Canonicalize(c, in.unicode)) != in.invert
	case opAny:
		ok = !jsstr.IsLineTerminator(c)
	case opAll:
		ok = true
	}
	return np, ok
}

// matchString matches the code points of a v-mode class string.
func (m *machine) matchString(in *inst, pos int) (int, bool) {
	n := len(in.str)
	for i := range n {
		want := in.str[i]
		if in.back {
			want = in.str[n-1-i]
		}
		c, np, ok := m.read(in, pos)
		if !ok {
			return pos, false
		}
		if in.fold {
			c = unicase.Canonicalize(c, true)
		}
		if c != want {
			return pos, false
		}
		pos = np
	}
	return pos, true
}

// assert evaluates ^, $, \b or \B at pos.
func (m *machine) assert(in *inst, pos int) bool {
	switch in.kind {
	case assertStart:
		return pos == 0 || in.multiline && jsstr.IsLineTerminator(rune(m.input[pos-1]))
	case assertEnd:
		return pos == len(m.input) || in.multiline && jsstr.IsLineTerminator(rune(m.input[pos]))
	}
	before := pos > 0 && in.set.has(rune(m.input[pos-1]))
	after := pos < len(m.input) && in.set.has(rune(m.input[pos]))
	return (before != after) == (in.kind == assertWordBoundary)
}

// backref matches the text captured by the first participating group of the
// instruction; a reference to a group that did not participate matches empty.
func (m *machine) backref(in *inst, pos int) (int, bool) {
	s, e := -1, -1
	for _, g := range in.groups {
		if m.caps[2*g] >= 0 && m.caps[2*g+1] >= 0 {
			s, e = m.caps[2*g], m.caps[2*g+1]
			break
		}
	}
	if s < 0 {
		return pos, true
	}
	n := e - s
	start := pos
	if in.back {
		start = pos - n
	}
	if start < 0 || start+n > len(m.input) {
		return pos, false
	}
	a, b := m.input[s:e], m.input[start:start+n]
	if !in.fold {
		for i := range a {
			if a[i] != b[i] {
				return pos, false
			}
		}
	} else if !in.unicode {
		for i := range a {
			if unicase.Canonicalize(rune(a[i]), false) != unicase.Canonicalize(rune(b[i]), false) {
				return pos, false
			}
		}
	} else {
		for i := 0; i < n; {
			ca, na := jsstr.CodePointAt(a, i)
			cb, nb := jsstr.CodePointAt(b, i)
			if na != nb || unicase.Canonicalize(ca, true) != unicase.Canonicalize(cb, true) {
				return pos, false
			}
			i += na
		}
	}
	if in.back {
		return start, true
	}
	return start + n, true
}

// Test reports whether the expression matches anywhere in s, as RegExp.prototype.test
// does for a pattern without the g or y flag.
func (re *Regexp) Test(s string) bool {
	return re.Exec(jsstr.ToUTF16(s), 0) != nil
}
