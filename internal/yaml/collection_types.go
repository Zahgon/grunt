package yaml

import (
	"github.com/gruntjs/grunt-go/js"
)

// The collection types of lib/js-yaml/type. Their data is the *js.Array or
// *js.Object read for the node, or nil for an empty node.

// seqType is lib/js-yaml/type/seq.js.
var seqType = &yamlType{
	tag:     "tag:yaml.org,2002:seq",
	kind:    kindSequence,
	resolve: func(any) bool { return true },
	construct: func(data any) (any, error) {
		if data == nil {
			return &js.Array{}, nil
		}
		return data, nil
	},
}

// mapType is lib/js-yaml/type/map.js.
var mapType = &yamlType{
	tag:     "tag:yaml.org,2002:map",
	kind:    kindMapping,
	resolve: func(any) bool { return true },
	construct: func(data any) (any, error) {
		if data == nil {
			return js.NewObject(), nil
		}
		return data, nil
	},
}

// singleKeyPairs returns the items of a sequence whose entries are all plain objects
// with exactly one key, the shape !!omap and !!pairs require.
func singleKeyPairs(data any) ([]*js.Object, bool) {
	arr, ok := data.(*js.Array)
	if !ok {
		return nil, false
	}
	pairs := make([]*js.Object, len(arr.Items))
	for i, item := range arr.Items {
		pair, ok := item.(*js.Object)
		if !ok || pair.Len() != 1 {
			return nil, false
		}
		pairs[i] = pair
	}
	return pairs, true
}

// omapType is lib/js-yaml/type/omap.js: a sequence of single-key mappings with
// distinct keys, kept as the sequence itself.
var omapType = &yamlType{
	tag:  "tag:yaml.org,2002:omap",
	kind: kindSequence,
	resolve: func(data any) bool {
		if data == nil {
			return true
		}
		pairs, ok := singleKeyPairs(data)
		if !ok {
			return false
		}
		seen := map[string]bool{}
		for _, pair := range pairs {
			key := pair.Keys()[0]
			if seen[key] {
				return false
			}
			seen[key] = true
		}
		return true
	},
	construct: func(data any) (any, error) {
		if data == nil {
			return &js.Array{}, nil
		}
		return data, nil
	},
}

// pairsType is lib/js-yaml/type/pairs.js: a sequence of single-key mappings, built
// into a new array of [key, value] arrays.
var pairsType = &yamlType{
	tag:  "tag:yaml.org,2002:pairs",
	kind: kindSequence,
	resolve: func(data any) bool {
		if data == nil {
			return true
		}
		_, ok := singleKeyPairs(data)
		return ok
	},
	construct: func(data any) (any, error) {
		result := &js.Array{Items: []any{}}
		if data == nil {
			return result, nil
		}
		pairs, _ := singleKeyPairs(data) // resolve has accepted data
		for _, pair := range pairs {
			key := pair.Keys()[0]
			result.Items = append(result.Items, js.NewArray(key, pair.Get(key)))
		}
		return result, nil
	},
}

// setType is lib/js-yaml/type/set.js: a mapping whose values are all null.
var setType = &yamlType{
	tag:  "tag:yaml.org,2002:set",
	kind: kindMapping,
	resolve: func(data any) bool {
		if data == nil {
			return true
		}
		obj, ok := data.(*js.Object)
		if !ok {
			return false
		}
		for _, key := range obj.Keys() {
			if obj.Get(key) != nil {
				return false
			}
		}
		return true
	},
	construct: func(data any) (any, error) {
		if data == nil {
			return js.NewObject(), nil
		}
		return data, nil
	},
}
