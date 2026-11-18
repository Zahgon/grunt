package nopt

import (
	"slices"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
)

// abbrev is abbrev 1.1.1: it maps every unambiguous prefix of each word to the word
// (and each word to itself). Words are compared and cut in UTF-16 code units, as the
// original's sort and charAt do.
func abbrev(list []string) map[string]string {
	words := make([][]uint16, len(list))
	for i, s := range list {
		words[i] = jsstr.ToUTF16(s)
	}
	slices.SortFunc(words, slices.Compare[[]uint16])

	abbrevs := map[string]string{}
	var prev []uint16
	for i, current := range words {
		var next []uint16
		if i+1 < len(words) {
			next = words[i+1]
		}
		if slices.Equal(current, next) {
			continue
		}
		nextMatches, prevMatches := true, true
		j, cl := 0, len(current)
		for ; j < cl; j++ {
			c := current[j]
			nextMatches = nextMatches && j < len(next) && c == next[j]
			prevMatches = prevMatches && j < len(prev) && c == prev[j]
			if !nextMatches && !prevMatches {
				j++
				break
			}
		}
		prev = current
		word := jsstr.FromUTF16WellFormed(current)
		if j == cl {
			abbrevs[word] = word
			continue
		}
		for ; j <= cl; j++ {
			abbrevs[jsstr.FromUTF16WellFormed(current[:j])] = word
		}
	}
	return abbrevs
}

// length16 is String.prototype.length.
func length16(s string) int {
	return jsstr.Length(s)
}
