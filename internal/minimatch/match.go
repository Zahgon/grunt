package minimatch

// This file ports Minimatch.prototype.match, matchOne and the globstar helpers
// of minimatch 3.1.5.

// Match tests a path against the compiled pattern, with partial matching as
// set by Options.Partial.
func (m *Minimatch) Match(f string) (bool, error) {
	return m.MatchPartial(f, m.Options.Partial)
}

// MatchPartial tests a path against the compiled pattern. With partial, a path
// that runs out before the pattern does still matches if every segment so far
// did, which is how a file-system walker prunes directories.
func (m *Minimatch) MatchPartial(f string, partial bool) (bool, error) {
	if m.Comment {
		return false, nil
	}
	if m.Empty {
		return f == "", nil
	}
	if f == "/" && partial {
		return true, nil
	}
	file := splitSlashes(f)
	// The basename is the last non-empty segment.
	var filename string
	for i := len(file) - 1; i >= 0; i-- {
		filename = file[i]
		if filename != "" {
			break
		}
	}
	for _, pattern := range m.Set {
		subject := file
		if m.Options.MatchBase && len(pattern) == 1 {
			subject = []string{filename}
		}
		hit, err := m.matchOne(subject, pattern, partial)
		if err != nil {
			return false, err
		}
		if hit {
			if m.Options.FlipNegate {
				return true, nil
			}
			return !m.Negate, nil
		}
	}
	// No hits: success for a negated pattern, failure otherwise.
	if m.Options.FlipNegate {
		return false, nil
	}
	return m.Negate, nil
}

func (m *Minimatch) matchOne(file []string, pattern []Part, partial bool) (bool, error) {
	for _, p := range pattern {
		if p.Globstar {
			return m.matchGlobstar(file, pattern, partial, 0, 0)
		}
	}
	return m.matchSegments(file, pattern, partial, 0, 0), nil
}

// sliceParts is Array.prototype.slice (and String.prototype.slice on code units)
// for non-negative offsets, which are clamped to the length.
func sliceParts[T any](s []T, start, end int) []T {
	start = min(start, len(s))
	end = min(end, len(s))
	if end < start {
		return nil
	}
	return s[start:end]
}

// isBadDot reports a segment ** may not consume: . and .., and dotfiles
// unless Dot is set.
func (m *Minimatch) isBadDot(f string) bool {
	return f == "." || f == ".." || !m.Options.Dot && len(f) > 0 && f[0] == '.'
}

// matchGlobstar is _matchGlobstar: match the fixed head and tail around the
// first and last **, then the segments between them.
func (m *Minimatch) matchGlobstar(file []string, pattern []Part, partial bool, fileIndex, patternIndex int) (bool, error) {
	firstgs := -1
	for i := patternIndex; i < len(pattern); i++ {
		if pattern[i].Globstar {
			firstgs = i
			break
		}
	}
	lastgs := -1
	for i := len(pattern) - 1; i >= 0; i-- {
		if pattern[i].Globstar {
			lastgs = i
			break
		}
	}
	head := sliceParts(pattern, patternIndex, firstgs)
	var body, tail []Part
	if partial {
		body = sliceParts(pattern, firstgs+1, len(pattern))
	} else {
		body = sliceParts(pattern, firstgs+1, lastgs)
		tail = sliceParts(pattern, lastgs+1, len(pattern))
	}

	if len(head) > 0 {
		fileHead := sliceParts(file, fileIndex, fileIndex+len(head))
		if !m.matchSegments(fileHead, head, partial, 0, 0) {
			return false, nil
		}
		fileIndex += len(head)
	}

	fileTailMatch := 0
	if len(tail) > 0 {
		if len(tail)+fileIndex > len(file) {
			return false, nil
		}
		tailStart := len(file) - len(tail)
		if m.matchSegments(file, tail, partial, tailStart, 0) {
			fileTailMatch = len(tail)
		} else {
			// Affordance for stuff like a/**/* matching a/b/.
			if file[len(file)-1] != "" || fileIndex+len(tail) == len(file) {
				return false, nil
			}
			tailStart--
			if !m.matchSegments(file, tail, partial, tailStart, 0) {
				return false, nil
			}
			fileTailMatch = len(tail) + 1
		}
	}

	// A single ** between head and tail.
	if len(body) == 0 {
		sawSome := fileTailMatch > 0
		for i := fileIndex; i < len(file)-fileTailMatch; i++ {
			sawSome = true
			if m.isBadDot(file[i]) {
				return false, nil
			}
		}
		return partial || sawSome, nil
	}

	// Split the body into segments at each **. Each records the last file
	// index it may start at, computed exactly as the original does.
	type bodySegment struct {
		parts []Part
		after int
	}
	segments := []*bodySegment{{}}
	current := segments[0]
	nonGsParts := 0
	nonGsPartsSums := []int{0}
	for _, b := range body {
		if b.Globstar {
			nonGsPartsSums = append(nonGsPartsSums, nonGsParts)
			current = &bodySegment{}
			segments = append(segments, current)
		} else {
			current.parts = append(current.parts, b)
			nonGsParts++
		}
	}
	idx := len(segments) - 1
	fileLength := len(file) - fileTailMatch
	for _, s := range segments {
		s.after = fileLength - (nonGsPartsSums[idx] + len(s.parts))
		idx--
	}

	var matchSections func(fileIndex, bodyIndex, depth int, sawTail bool) (matchResult, error)
	// _matchGlobStarBodySections: false means "no match here", null means
	// "no match, stop trying".
	matchSections = func(fileIndex, bodyIndex, depth int, sawTail bool) (matchResult, error) {
		if bodyIndex >= len(segments) {
			// Just make sure there are no bad dots.
			for i := fileIndex; i < len(file); i++ {
				sawTail = true
				if m.isBadDot(file[i]) {
					return resultFalse, nil
				}
			}
			return boolResult(sawTail), nil
		}
		bs := segments[bodyIndex]
		for ; fileIndex <= bs.after; fileIndex++ {
			hit := m.matchSegments(sliceParts(file, 0, fileIndex+len(bs.parts)), bs.parts, partial, fileIndex, 0)
			// Past the recursion limit there is no match: an intentional
			// false negative.
			if hit && depth < m.maxGlobstarRecursion {
				sub, err := matchSections(fileIndex+len(bs.parts), bodyIndex+1, depth+1, sawTail)
				if err != nil {
					return resultFalse, err
				}
				if sub != resultFalse {
					return sub, nil
				}
			}
			if fileIndex >= len(file) {
				// file[fileIndex] is undefined: comparing it with "." and ".."
				// is false, and without dot the original then calls
				// undefined.charAt(0).
				if !m.Options.Dot {
					return resultFalse, errUndefinedCharAt
				}
				continue
			}
			if m.isBadDot(file[fileIndex]) {
				return resultFalse, nil
			}
		}
		if partial {
			return resultTrue, nil
		}
		return resultNull, nil
	}
	res, err := matchSections(fileIndex, 0, 0, fileTailMatch > 0)
	return res == resultTrue, err
}

type matchResult int

const (
	resultFalse matchResult = iota
	resultNull
	resultTrue
)

func boolResult(b bool) matchResult {
	if b {
		return resultTrue
	}
	return resultFalse
}

// matchSegments is _matchOne: match file[fileIndex:] against
// pattern[patternIndex:], segment by segment, without globstars.
func (m *Minimatch) matchSegments(file []string, pattern []Part, partial bool, fileIndex, patternIndex int) bool {
	fi, pi := fileIndex, patternIndex
	fl, pl := len(file), len(pattern)
	for ; fi < fl && pi < pl; fi, pi = fi+1, pi+1 {
		p := pattern[pi]
		f := file[fi]
		var hit bool
		switch {
		case p.Globstar:
			return false
		case p.Regexp != nil:
			hit = p.Regexp.MatchString(f)
		default:
			hit = f == p.Literal
		}
		if !hit {
			return false
		}
	}
	switch {
	case fi == fl && pi == pl:
		// Ran out of pattern and filename at the same time: an exact hit.
		return true
	case fi == fl:
		// Ran out of file with pattern left: fine for a partial match.
		return partial
	default:
		// Ran out of pattern with file left: only a trailing empty segment
		// (a/* matching a/b/) is acceptable.
		return fi == fl-1 && file[fi] == ""
	}
}
