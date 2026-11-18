package jsregexp

import (
	"slices"
)

// opcode is an instruction of the backtracking machine.
type opcode uint8

const (
	opChar     opcode = iota // match c exactly
	opCharFold               // match a character whose canonical form is c
	opSet                    // match a character in set (xor invert)
	opSetFold                // match a character whose canonical form is in set (xor invert)
	opAny                    // match any character but a line terminator
	opAll                    // match any character
	opAssert                 // zero-width ^ $ \b \B
	opSave                   // caps[x] = position
	opReset                  // caps[x..y] = -1
	opFork                   // try the next instruction, then x
	opJmp                    // continue at x
	opBackref                // match the text of the first participating group in groups
	opLook                   // start a lookaround whose body ends at the opLookEnd before x
	opLookEnd                // the lookaround body matched
	opRepInit                // regs[x] = 0
	opRepTest                // decide whether to run another iteration; exit at y
	opRepMark                // regs[x] = position
	opRepNext                // empty check, count the iteration, loop to y
	opStar                   // a quantified single-character matcher (sub), no captures
	opString                 // match the code points str
	opMatch                  // success
)

// inst is one instruction. Fields not used by an opcode are zero.
type inst struct {
	op        opcode
	back      bool // consume input right to left (inside a lookbehind)
	unicode   bool // read code points rather than code units
	c         rune
	set       charSet
	invert    bool
	fold      bool
	kind      assertKind
	multiline bool
	x, y      int
	mark      int // the position register of opRepNext
	min, max  int
	greedy    bool
	negate    bool
	groups    []int
	str       []rune
	sub       *inst
}

// compiler turns a parsed tree into instructions.
type compiler struct {
	unicode bool
	prog    []inst
	nregs   int
}

// compileProgram compiles the whole pattern: save the match start, the body,
// save the end, succeed.
func (c *compiler) compileProgram(tree node) ([]inst, int) {
	c.emit(inst{op: opSave, x: 0})
	c.compile(tree, false)
	c.emit(inst{op: opSave, x: 1})
	c.emit(inst{op: opMatch})
	return c.prog, c.nregs
}

func (c *compiler) emit(in inst) int {
	in.unicode = c.unicode
	c.prog = append(c.prog, in)
	return len(c.prog) - 1
}

// single returns the one-character matcher for n, or nil when n is not one.
func (c *compiler) single(n node, back bool) *inst {
	var in inst
	switch n := n.(type) {
	case *charNode:
		in = inst{op: opChar, c: n.c}
		if n.icase {
			in.op = opCharFold
		}
	case *setNode:
		if len(n.strs) > 0 {
			return nil
		}
		in = inst{op: opSet, set: n.set, invert: n.invert}
		if n.icase {
			in.op = opSetFold
			in.set = n.set.caseClosure(c.unicode)
		}
	case *anyNode:
		in = inst{op: opAny}
		if n.dotAll {
			in.op = opAll
		}
	default:
		return nil
	}
	in.back = back
	in.unicode = c.unicode
	return &in
}

// compile emits the instructions for n; back compiles for right-to-left
// matching inside a lookbehind.
func (c *compiler) compile(n node, back bool) {
	if in := c.single(n, back); in != nil {
		c.emit(*in)
		return
	}
	switch n := n.(type) {
	case *seqNode:
		items := n.items
		if back {
			items = slices.Clone(items)
			slices.Reverse(items)
		}
		for _, it := range items {
			c.compile(it, back)
		}
	case *altNode:
		var jumps []int
		for i, alt := range n.alts {
			if i < len(n.alts)-1 {
				fork := c.emit(inst{op: opFork})
				c.compile(alt, back)
				jumps = append(jumps, c.emit(inst{op: opJmp}))
				c.prog[fork].x = len(c.prog)
			} else {
				c.compile(alt, back)
			}
		}
		for _, j := range jumps {
			c.prog[j].x = len(c.prog)
		}
	case *setNode:
		c.compileStrings(n, back)
	case *assertNode:
		c.emit(inst{op: opAssert, kind: n.kind, multiline: n.multiline, set: n.word})
	case *groupNode:
		// Matching right to left meets the end of the group first.
		first, last := 2*n.index, 2*n.index+1
		if back {
			first, last = last, first
		}
		c.emit(inst{op: opSave, x: first})
		c.compile(n.body, back)
		c.emit(inst{op: opSave, x: last})
	case *lookNode:
		look := c.emit(inst{op: opLook, negate: n.negate})
		c.compile(n.body, n.behind)
		c.emit(inst{op: opLookEnd})
		c.prog[look].x = len(c.prog)
	case *backrefNode:
		c.emit(inst{op: opBackref, groups: n.groups, fold: n.icase, back: back})
	case *repeatNode:
		c.compileRepeat(n, back)
	case *stringNode:
		c.emit(inst{op: opString, str: n.str, fold: n.icase, back: back})
	case *emptyNode:
	}
}

// compileStrings emits a v-mode class holding strings: the strings longest
// first, then the single characters.
func (c *compiler) compileStrings(n *setNode, back bool) {
	strs := slices.Clone(n.strs)
	slices.SortStableFunc(strs, func(a, b []rune) int { return len(b) - len(a) })
	var alts []node
	for _, s := range strs {
		if len(s) == 0 {
			continue
		}
		alts = append(alts, &stringNode{str: s, icase: n.icase})
	}
	alts = append(alts, &setNode{set: n.set, icase: n.icase})
	for _, s := range strs {
		if len(s) == 0 {
			alts = append(alts, &emptyNode{})
			break
		}
	}
	if len(alts) == 1 {
		c.compile(alts[0], back)
		return
	}
	c.compile(&altNode{alts: alts}, back)
}

// stringNode matches a sequence of code points, from a v-mode \q{...}.
type stringNode struct {
	str   []rune
	icase bool
}

// compileRepeat emits a quantifier: a counted loop with RepeatMatcher's
// semantics, or a single opStar for a one-character atom.
func (c *compiler) compileRepeat(n *repeatNode, back bool) {
	if n.max == 0 {
		return
	}
	if sub := c.single(n.body, back); sub != nil {
		c.emit(inst{op: opStar, sub: sub, min: n.min, max: n.max, greedy: n.greedy, back: back})
		return
	}
	count, mark := c.nregs, c.nregs+1
	c.nregs += 2
	c.emit(inst{op: opRepInit, x: count})
	test := c.emit(inst{op: opRepTest, x: count, min: n.min, max: n.max, greedy: n.greedy})
	c.emit(inst{op: opRepMark, x: mark})
	if n.capLo <= n.capHi {
		c.emit(inst{op: opReset, x: 2 * n.capLo, y: 2*n.capHi + 1})
	}
	c.compile(n.body, back)
	c.emit(inst{op: opRepNext, x: count, y: test, min: n.min, mark: mark})
	c.prog[test].y = len(c.prog)
}
