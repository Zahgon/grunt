package glob

import (
	_ "embed"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// glob 7 sorts its results with a.localeCompare(b, 'en'), which on Node 24 is
// ICU's root collation at tertiary strength with normalization on: bring both
// strings to NFD, map each character to its collation elements, and compare
// the primary weights (base letters), then the secondary weights (accents),
// then the tertiary weights (case and variants). The elements, canonical
// decompositions and combining classes are read from collation.txt; see
// gen/collation.py for how it was extracted from ICU 78 and for its format.
//
// The table covers Latin, Greek, Cyrillic, Armenian, Hebrew, Arabic, Hangul,
// kana, punctuation, symbols, fullwidth forms and common emoji. Other
// characters get an implicit weight from their code point: Han ideographs
// after every covered script (ICU orders them by radical and stroke count
// instead, so two ideographs may compare differently), and everything else
// after Han.

//go:embed collation.txt
var collationData string

// collationElement is one ICU collation element. A zero weight is ignorable
// at its level.
type collationElement struct {
	primary   uint32
	secondary uint16
	tertiary  uint16
}

// contraction is a sequence collated as a unit, keyed by its first rune.
type contraction struct {
	rest     []rune
	elements []collationElement
}

type collationTable struct {
	elements       map[rune][]collationElement
	contractions   map[rune][]contraction
	decompositions map[rune][]rune // full canonical decompositions
	combiningClass map[rune]int    // non-zero canonical combining classes
}

const commonWeight = 0x0500

var (
	collationOnce sync.Once
	collation     *collationTable
)

func loadCollation() *collationTable {
	collationOnce.Do(func() {
		t, err := parseCollation(collationData)
		if err != nil {
			panic("glob: corrupt collation.txt: " + err.Error())
		}
		collation = t
	})
	return collation
}

func parseCollation(data string) (*collationTable, error) {
	t := &collationTable{
		elements:       map[rune][]collationElement{},
		contractions:   map[rune][]contraction{},
		decompositions: map[rune][]rune{},
		combiningClass: map[rune]int{},
	}
	for line := range strings.Lines(data) {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		runes, err := parseRunes(fields[0])
		if err != nil {
			return nil, err
		}
		if len(fields) == 2 && strings.HasPrefix(fields[1], "=") {
			if t.decompositions[runes[0]], err = parseRunes(fields[1][1:]); err != nil {
				return nil, err
			}
			continue
		}
		elements := make([]collationElement, 0, len(fields)-1)
		for _, f := range fields[1:] {
			if ccc, ok := strings.CutPrefix(f, "ccc"); ok {
				if t.combiningClass[runes[0]], err = strconv.Atoi(ccc); err != nil {
					return nil, err
				}
				continue
			}
			e, err := parseElement(f)
			if err != nil {
				return nil, err
			}
			elements = append(elements, e)
		}
		if len(runes) == 1 {
			t.elements[runes[0]] = elements
		} else {
			t.contractions[runes[0]] = append(t.contractions[runes[0]], contraction{rest: runes[1:], elements: elements})
		}
	}
	// Try the longest contraction first.
	for _, cs := range t.contractions {
		slices.SortStableFunc(cs, func(a, b contraction) int { return len(b.rest) - len(a.rest) })
	}
	return t, nil
}

// parseRunes reads comma-separated hex code points.
func parseRunes(s string) ([]rune, error) {
	var runes []rune
	for _, h := range strings.Split(s, ",") {
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return nil, err
		}
		runes = append(runes, rune(v))
	}
	return runes, nil
}

// parseElement reads primary[,secondary[,tertiary]], hex with trailing zero
// bytes dropped and the common weight as the default.
func parseElement(f string) (collationElement, error) {
	parts := strings.Split(f, ",")
	weight := func(s string, width int) (uint64, error) {
		if s == "" {
			return 0, nil
		}
		v, err := strconv.ParseUint(s, 16, 64)
		return v << (4 * (width - len(s))), err
	}
	e := collationElement{secondary: commonWeight, tertiary: commonWeight}
	p, err := weight(parts[0], 8)
	if err != nil {
		return e, err
	}
	e.primary = uint32(p)
	if len(parts) > 1 {
		s, err := weight(parts[1], 4)
		if err != nil {
			return e, err
		}
		e.secondary = uint16(s)
	}
	if len(parts) > 2 {
		t, err := weight(parts[2], 4)
		if err != nil {
			return e, err
		}
		e.tertiary = uint16(t)
	}
	return e, nil
}

// Implicit primaries for code points outside the table.
const (
	hanImplicitBase   = 0xFA000000
	otherImplicitBase = 0xFE000000
)

func isHan(r rune) bool {
	return 0x3400 <= r && r <= 0x4DBF || 0x4E00 <= r && r <= 0x9FFF || 0xF900 <= r && r <= 0xFAFF ||
		0x20000 <= r && r <= 0x3FFFF
}

// appendElements appends the collation elements of r, a character without a
// canonical decomposition.
func (t *collationTable) appendElements(out []collationElement, r rune) []collationElement {
	if e, ok := t.elements[r]; ok {
		return append(out, e...)
	}
	implicit := func(primary uint32) []collationElement {
		return append(out, collationElement{primary: primary, secondary: commonWeight, tertiary: commonWeight})
	}
	switch {
	case r == utf8.RuneError:
		// U+FFFD, which file names that are not UTF-8 decode to, has the
		// highest primary weight short of U+FFFF.
		return implicit(0xFFFD0000)
	case isHan(r):
		return implicit(hanImplicitBase | uint32(r))
	}
	return implicit(otherImplicitBase | uint32(r))
}

// nfd is the canonical decomposition of s with its combining marks in
// canonical order, as far as the table knows them.
func (t *collationTable) nfd(s string) []rune {
	var out []rune
	for _, r := range s {
		switch {
		case 0xAC00 <= r && r <= 0xD7A3:
			// A Hangul syllable decomposes into leading, vowel and trailing jamo.
			v := r - 0xAC00
			out = append(out, 0x1100+v/588, 0x1161+v%588/28)
			if v%28 != 0 {
				out = append(out, 0x11A7+v%28)
			}
		case t.decompositions[r] != nil:
			out = append(out, t.decompositions[r]...)
		default:
			out = append(out, r)
		}
	}
	// Canonical ordering: sort each run of combining marks by class, stably.
	for i := 0; i < len(out); {
		if t.combiningClass[out[i]] == 0 {
			i++
			continue
		}
		j := i
		for j < len(out) && t.combiningClass[out[j]] != 0 {
			j++
		}
		slices.SortStableFunc(out[i:j], func(a, b rune) int { return t.combiningClass[a] - t.combiningClass[b] })
		i = j
	}
	return out
}

// collationKey lists the collation elements of s. At each position it takes
// the longest contraction; a two-character contraction may also skip
// combining marks that do not block its mark (UCA S2.1, discontiguous
// matching), as ICU does for и with a breve after another accent.
func (t *collationTable) collationKey(s string) []collationElement {
	runes := t.nfd(s)
	var out []collationElement
	for i := 0; i < len(runes); {
		if c, n := t.contiguousContraction(runes[i:]); c != nil {
			out = append(out, c.elements...)
			i += n
			continue
		}
		if c, j := t.discontiguousContraction(runes, i); c != nil {
			out = append(out, c.elements...)
			runes = slices.Delete(runes, j, j+1)
			i++
			continue
		}
		out = t.appendElements(out, runes[i])
		i++
	}
	return out
}

func (t *collationTable) contiguousContraction(runes []rune) (*contraction, int) {
	for k, c := range t.contractions[runes[0]] {
		if len(runes)-1 >= len(c.rest) && slices.Equal(runes[1:1+len(c.rest)], c.rest) {
			return &t.contractions[runes[0]][k], 1 + len(c.rest)
		}
	}
	return nil, 0
}

// discontiguousContraction finds a mark after runes[i] that completes a
// two-character contraction with it and is not blocked by an earlier mark of
// the same or a higher class. It returns the contraction and the mark's index.
func (t *collationTable) discontiguousContraction(runes []rune, i int) (*contraction, int) {
	cs := t.contractions[runes[i]]
	if len(cs) == 0 {
		return nil, 0
	}
	highest := 0
	for j := i + 1; j < len(runes); j++ {
		ccc := t.combiningClass[runes[j]]
		if ccc == 0 {
			break
		}
		if ccc > highest {
			for k, c := range cs {
				if len(c.rest) == 1 && c.rest[0] == runes[j] {
					return &cs[k], j
				}
			}
		}
		highest = max(highest, ccc)
	}
	return nil, 0
}

// compareKeys compares two collation keys level by level, skipping weights
// that are ignorable at each level; a key that runs out first is smaller.
func compareKeys(a, b []collationElement) int {
	levels := []func(collationElement) uint32{
		func(e collationElement) uint32 { return e.primary },
		func(e collationElement) uint32 { return uint32(e.secondary) },
		func(e collationElement) uint32 { return uint32(e.tertiary) },
	}
	for _, weight := range levels {
		i, j := 0, 0
		for {
			for i < len(a) && weight(a[i]) == 0 {
				i++
			}
			for j < len(b) && weight(b[j]) == 0 {
				j++
			}
			switch {
			case i == len(a) && j == len(b):
			case i == len(a):
				return -1
			case j == len(b):
				return 1
			default:
				if wa, wb := weight(a[i]), weight(b[j]); wa != wb {
					if wa < wb {
						return -1
					}
					return 1
				}
				i++
				j++
				continue
			}
			break
		}
	}
	return 0
}

// sortLocale sorts list in place as list.sort((a, b) => a.localeCompare(b,
// 'en')) does: stably, so strings that collate equal keep their order.
func sortLocale(list []string) {
	t := loadCollation()
	keys := make(map[string][]collationElement, len(list))
	for _, s := range list {
		if _, ok := keys[s]; !ok {
			keys[s] = t.collationKey(s)
		}
	}
	slices.SortStableFunc(list, func(a, b string) int { return compareKeys(keys[a], keys[b]) })
}
