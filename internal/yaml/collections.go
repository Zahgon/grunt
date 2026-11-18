package yaml

import (
	"strconv"

	"github.com/gruntjs/grunt-go/js"
)

// maxTotalMergeKeys is js-yaml 3.15.2's default bound on merge work per stream.
const maxTotalMergeKeys = 10000

// chargeMergeWork counts one unit of merge work against maxTotalMergeKeys.
func (l *loader) chargeMergeWork() {
	l.totalMergeKeys++
	if l.totalMergeKeys > maxTotalMergeKeys {
		l.fail("merge keys exceeded maxTotalMergeKeys (" + strconv.Itoa(maxTotalMergeKeys) + ")")
	}
}

// mergeMappings copies source's own enumerable keys that dest lacks, marking them as
// overridable by later explicit keys. Like common.isObject, it accepts any object:
// an array or a Buffer merges its indexes, a Date or RegExp merges nothing.
func (l *loader) mergeMappings(dest *js.Object, source any, overridable map[string]bool) {
	keys, get, ok := ownKeys(source)
	if !ok {
		l.fail("cannot merge mappings; the provided source object is unacceptable")
	}
	// Count the source mapping itself to bound sequences of empty mappings.
	l.chargeMergeWork()
	for _, key := range keys {
		l.chargeMergeWork()
		if !dest.Has(key) {
			dest.Set(key, get(key))
			markOverridable(overridable, key)
		}
	}
}

// markOverridable is "overridableKeys[key] = true". overridableKeys is a plain
// object, so assigning "__proto__" changes its prototype instead of adding a key:
// a merged "__proto__" is never overridable and a later explicit one is a duplicate.
func markOverridable(overridable map[string]bool, key string) {
	if key != "__proto__" {
		overridable[key] = true
	}
}

// storeMappingPair adds key: value to result (creating it when nil), applying merge
// keys and rejecting duplicates. startLine and startPos, when non-zero, move the
// duplicate-key error to the start of the pair.
func (l *loader) storeMappingPair(result *js.Object, overridable map[string]bool, keyTag string, keyNode, valueNode any, startLine, startPos int) *js.Object {
	// The output is a plain object, so keys are strings. Array keys are flattened
	// one level; deeper nesting is refused because aliases can make it explode.
	if arr, ok := keyNode.(*js.Array); ok {
		items := append([]any(nil), arr.Items...)
		for i, item := range items {
			switch item.(type) {
			case *js.Array:
				l.fail("nested arrays are not supported inside keys")
			case *js.Object:
				items[i] = "[object Object]"
			}
		}
		keyNode = &js.Array{Items: items}
	}
	// Plain objects become "[object Object]" so that load() never runs a toString
	// from the document.
	if _, ok := keyNode.(*js.Object); ok {
		keyNode = "[object Object]"
	}
	key := js.ToString(keyNode)
	if result == nil {
		result = js.NewObject()
	}
	if keyTag == tagMerge {
		if arr, ok := valueNode.(*js.Array); ok {
			if len(arr.Items) > 100 {
				l.fail("abnormal merge sequence size")
			}
			for _, item := range arr.Items {
				l.mergeMappings(result, item, overridable)
			}
		} else {
			l.mergeMappings(result, valueNode, overridable)
		}
		return result
	}
	if !overridable[key] && result.Has(key) {
		// "state.line = startLine || state.line": zero means "not given".
		if startLine != 0 {
			l.line = startLine
		}
		if startPos != 0 {
			l.position = startPos
		}
		l.fail("duplicated mapping key")
	}
	result.Set(key, valueNode)
	delete(overridable, key)
	return result
}

// readFlowCollection reads a [...] sequence or {...} mapping. In a sequence, a
// "key: value" entry becomes a single-pair mapping.
func (l *loader) readFlowCollection(nodeIndent int) bool {
	var (
		terminator int
		isMapping  bool
		seq        *js.Array
		mapping    *js.Object
		collection any
	)
	switch l.at(l.position) {
	case '[':
		terminator, seq = ']', &js.Array{}
		collection = seq
	case '{':
		terminator, isMapping, mapping = '}', true, js.NewObject()
		collection = mapping
	default:
		return false
	}
	tag, hasTag, anchor := l.tag, l.hasTag, l.anchor
	overridable := map[string]bool{}
	readNext := true

	if l.anchor != "" {
		l.storeAnchor(l.anchor, collection)
	}

	l.position++
	for ch := l.at(l.position); ch != 0; {
		l.skipSeparationSpace(true)
		ch = l.at(l.position)
		if ch == terminator {
			l.position++
			l.tag, l.hasTag, l.anchor = tag, hasTag, anchor
			l.kind = kindSequence
			if isMapping {
				l.kind = kindMapping
			}
			l.result = collection
			return true
		} else if !readNext {
			l.fail("missed comma between flow collection entries")
		}

		var keyNode, valueNode any
		isPair, isExplicitPair := false, false
		if ch == '?' && isWSOrEOL(l.at(l.position+1)) {
			isPair, isExplicitPair = true, true
			l.position++
			l.skipSeparationSpace(true)
		}

		line := l.line
		l.composeNode(nodeIndent, contextFlowIn, false, true)
		keyTag := l.nodeTag()
		keyNode = l.result
		l.skipSeparationSpace(true)

		ch = l.at(l.position)
		if (isExplicitPair || l.line == line) && ch == ':' {
			isPair = true
			l.position++
			l.skipSeparationSpace(true)
			l.composeNode(nodeIndent, contextFlowIn, false, true)
			valueNode = l.result
		}

		switch {
		case isMapping:
			l.storeMappingPair(mapping, overridable, keyTag, keyNode, valueNode, 0, 0)
		case isPair:
			seq.Items = append(seq.Items, l.storeMappingPair(nil, overridable, keyTag, keyNode, valueNode, 0, 0))
		default:
			seq.Items = append(seq.Items, keyNode)
		}

		l.skipSeparationSpace(true)
		ch = l.at(l.position)
		if ch == ',' {
			readNext = true
			l.position++
			ch = l.at(l.position)
		} else {
			readNext = false
		}
	}
	l.fail("unexpected end of the stream within a flow collection")
	return false
}

// nodeTag returns state.tag for use as a key tag, where only the merge tag matters.
func (l *loader) nodeTag() string {
	if !l.hasTag {
		return ""
	}
	return l.tag
}

// readBlockSequence reads "- " entries at column nodeIndent.
func (l *loader) readBlockSequence(nodeIndent int) bool {
	tag, hasTag, anchor := l.tag, l.hasTag, l.anchor
	seq := &js.Array{}
	detected := false

	if l.anchor != "" {
		l.storeAnchor(l.anchor, seq)
	}

	for ch := l.at(l.position); ch != 0; {
		if ch != '-' || !isWSOrEOL(l.at(l.position+1)) {
			break
		}
		detected = true
		l.position++

		if l.skipSeparationSpace(true) > 0 && l.lineIndent <= nodeIndent {
			seq.Items = append(seq.Items, nil)
			ch = l.at(l.position)
			continue
		}

		line := l.line
		l.composeNode(nodeIndent, contextBlockIn, false, true)
		seq.Items = append(seq.Items, l.result)
		l.skipSeparationSpace(true)
		ch = l.at(l.position)

		if (l.line == line || l.lineIndent > nodeIndent) && ch != 0 {
			l.fail("bad indentation of a sequence entry")
		} else if l.lineIndent < nodeIndent {
			break
		}
	}

	if !detected {
		return false
	}
	l.tag, l.hasTag, l.anchor = tag, hasTag, anchor
	l.kind = kindSequence
	l.result = seq
	return true
}

// readBlockMapping reads implicit "key: value" and explicit "? key / : value"
// entries at column nodeIndent. When the first node turns out not to be a key, it
// reports true and leaves that node as the result, as js-yaml does.
func (l *loader) readBlockMapping(nodeIndent, flowIndent int) bool {
	tag, hasTag, anchor := l.tag, l.hasTag, l.anchor
	mapping := js.NewObject()
	overridable := map[string]bool{}
	var keyTag string
	var keyNode, valueNode any
	atExplicitKey, detected, allowCompact := false, false, false

	if l.anchor != "" {
		l.storeAnchor(l.anchor, mapping)
	}

	for ch := l.at(l.position); ch != 0; {
		following := l.at(l.position + 1)
		line := l.line
		pos := l.position

		if (ch == '?' || ch == ':') && isWSOrEOL(following) {
			// Explicit notation: "?" introduces the key and ":" the value.
			if ch == '?' {
				if atExplicitKey {
					l.storeMappingPair(mapping, overridable, keyTag, keyNode, nil, 0, 0)
					keyTag, keyNode, valueNode = "", nil, nil
				}
				detected = true
				atExplicitKey = true
				allowCompact = true
			} else if atExplicitKey {
				// The ":" after an explicit key.
				atExplicitKey = false
				allowCompact = true
			} else {
				l.fail("incomplete explicit mapping pair; a key node is missed; or followed by a non-tabulated empty line")
			}
			l.position++
			ch = following
		} else if l.composeNode(flowIndent, contextFlowOut, false, true) {
			// Implicit notation: a flow-style key, then ":" and the value.
			if l.line != line {
				if detected {
					l.fail("can not read a block mapping entry; a multiline key may not be an implicit key")
				}
				l.tag, l.hasTag, l.anchor = tag, hasTag, anchor
				return true // Keep the result of composeNode.
			}
			ch = l.at(l.position)
			for isWhiteSpace(ch) {
				l.position++
				ch = l.at(l.position)
			}
			if ch != ':' {
				if detected {
					l.fail("can not read an implicit mapping pair; a colon is missed")
				}
				l.tag, l.hasTag, l.anchor = tag, hasTag, anchor
				return true // Keep the result of composeNode.
			}
			l.position++
			ch = l.at(l.position)
			if !isWSOrEOL(ch) {
				l.fail("a whitespace character is expected after the key-value separator within a block mapping")
			}
			if atExplicitKey {
				l.storeMappingPair(mapping, overridable, keyTag, keyNode, nil, 0, 0)
				keyTag, keyNode, valueNode = "", nil, nil
			}
			detected = true
			atExplicitKey = false
			allowCompact = false
			keyTag = l.nodeTag()
			keyNode = l.result
		} else {
			break // Reading is done.
		}

		// Common reading code for both explicit and implicit notations.
		if l.line == line || l.lineIndent > nodeIndent {
			if l.composeNode(nodeIndent, contextBlockOut, true, allowCompact) {
				if atExplicitKey {
					keyNode = l.result
				} else {
					valueNode = l.result
				}
			}
			if !atExplicitKey {
				l.storeMappingPair(mapping, overridable, keyTag, keyNode, valueNode, line, pos)
				keyTag, keyNode, valueNode = "", nil, nil
			}
			l.skipSeparationSpace(true)
			ch = l.at(l.position)
		}

		if l.lineIndent > nodeIndent && ch != 0 {
			l.fail("bad indentation of a mapping entry")
		} else if l.lineIndent < nodeIndent {
			break
		}
	}

	// A last entry made of an explicit key alone.
	if atExplicitKey {
		l.storeMappingPair(mapping, overridable, keyTag, keyNode, nil, 0, 0)
	}
	if detected {
		l.tag, l.hasTag, l.anchor = tag, hasTag, anchor
		l.kind = kindMapping
		l.result = mapping
	}
	return detected
}
