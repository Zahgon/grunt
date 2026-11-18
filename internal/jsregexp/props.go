package jsregexp

import (
	"slices"
	"strings"
	"sync"
)

// propFix lists the ranges, as lo, hi pairs, that Node 24 adds to or removes from
// a property set derived in derive.go.
type propFix struct {
	add, del []rune
}

// stringProperties are the properties of strings that only the v flag accepts.
var stringProperties = map[string]bool{
	"Basic_Emoji": true, "Emoji_Keycap_Sequence": true, "RGI_Emoji": true, "RGI_Emoji_Flag_Sequence": true,
	"RGI_Emoji_Modifier_Sequence": true, "RGI_Emoji_Tag_Sequence": true, "RGI_Emoji_ZWJ_Sequence": true,
}

var (
	propMu    sync.Mutex
	propCache = map[string]charSet{}
)

// propertyKey resolves the body of \p{name} or \p{name=value} (value is "" for
// the first form) to its key ("gc=Lu", "bin=Alphabetic", "sc=Latin",
// "scx=Latin"), reporting false when V8 rejects the name.
func propertyKey(name, value string) (string, bool) {
	if value == "" {
		if gc, ok := gcAliases[name]; ok {
			return "gc=" + gc, true
		}
		if b, ok := binaryAliases[name]; ok {
			return "bin=" + b, true
		}
		return "", false
	}
	switch name {
	case "General_Category", "gc":
		if gc, ok := gcAliases[value]; ok {
			return "gc=" + gc, true
		}
	case "Script", "sc":
		if s, ok := scriptAliases[value]; ok {
			return "sc=" + s, true
		}
	case "Script_Extensions", "scx":
		if s, ok := scriptAliases[value]; ok {
			return "scx=" + s, true
		}
	}
	return "", false
}

// propertySet returns the code points of a property key, building and caching
// the set on first use.
func propertySet(key string) charSet {
	propMu.Lock()
	defer propMu.Unlock()
	if s, ok := propCache[key]; ok {
		return s
	}
	var pred func(rune) bool
	switch kind, name, _ := strings.Cut(key, "="); kind {
	case "gc":
		pred = gcPredicate(name)
	case "bin":
		pred = binaryPredicate(name)
	default: // "sc" and "scx"
		pred = scriptPredicate(name)
	}
	var pairs []rune
	start := rune(-1)
	for r := rune(0); r <= 0x110000; r++ {
		in := r <= 0x10FFFF && pred(r)
		switch {
		case in && start < 0:
			start = r
		case !in && start >= 0:
			pairs = append(pairs, start, r-1)
			start = -1
		}
	}
	s := charSet(pairs)
	if fix, ok := propFixes[key]; ok {
		s = s.union(newCharSet(slices.Clone(fix.add)...)).minus(newCharSet(slices.Clone(fix.del)...), 0x10FFFF)
	}
	propCache[key] = s
	return s
}
