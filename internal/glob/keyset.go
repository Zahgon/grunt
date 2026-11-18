package glob

import (
	"cmp"
	"slices"

	"github.com/gruntjs/grunt-go/js"
)

// keySet is a set of strings kept the way glob keeps its matches, as the keys
// of a JavaScript object: Object.keys lists array-index keys ("0", "17") first
// in ascending numeric order, then the other keys in insertion order. The
// unsorted (nosort) output and the order of collation ties depend on this.
type keySet struct {
	has   map[string]bool
	order []string
}

func newKeySet() *keySet {
	return &keySet{has: map[string]bool{}}
}

func (s *keySet) add(k string) {
	if !s.has[k] {
		s.has[k] = true
		s.order = append(s.order, k)
	}
}

func (s *keySet) contains(k string) bool { return s.has[k] }

func (s *keySet) len() int { return len(s.order) }

// keys is Object.keys.
func (s *keySet) keys() []string {
	out := slices.Clone(s.order)
	slices.SortStableFunc(out, func(a, b string) int {
		ia, aIndex := js.ArrayIndex(a)
		ib, bIndex := js.ArrayIndex(b)
		switch {
		case aIndex && bIndex:
			return cmp.Compare(ia, ib)
		case aIndex:
			return -1
		case bIndex:
			return 1
		}
		return 0
	})
	return out
}
