package lodash

import (
	"github.com/gruntjs/grunt-go/js"
)

// Underscore returns a new `_` object as templates and grunt.util._ see it:
// grunt-legacy-util's lodash 4.18.1 with underscore.string 3.3.6 mixed in, so the
// underscore.string functions replace lodash's functions of the same name (trim,
// pad, repeat, words, startsWith, endsWith, escapeRegExp, toNumber, truncate,
// capitalize). Each value is a *js.Function; callbacks it receives must be
// *js.Function values too. The keys come in lodash's order followed by the
// underscore.string additions, as _.mixin appends them. VERSION is lodash's.
func Underscore() *js.Object {
	o := js.NewObject()
	o.Set("VERSION", "4.18.1")
	for _, group := range [][]entry{collectionEntries(), lodashStringEntries(), ustringEntries()} {
		for _, e := range group {
			o.Set(e.name, &js.Function{Name: e.name, Length: e.length, Call: func(_ any, args []any) (any, error) {
				return e.call(args)
			}})
		}
	}
	return o
}
