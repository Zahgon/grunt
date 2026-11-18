// Package nopt ports nopt 5.0.0 and abbrev 1.1.1, the option parser grunt and
// grunt-cli run over process.argv with grunt-known-options' type table.
//
// The supported types are the ones grunt uses (Boolean, String, Number, path, Array),
// explicit allowed values (Literal) and lists of alternatives (OneOf). nopt's url,
// Date and Stream types are not ported.
//
// Divergences from the original, all for inputs grunt never produces: option and
// shorthand names are looked up in Go maps, so names such as "constructor" or
// "__proto__" do not hit Object.prototype the way they do in nopt; and the "argv" key
// is never part of the parsed options (nopt overwrites it with the argv record, and
// grunt deletes that).
package nopt

import (
	"os"
	"os/user"
	"slices"
	"strings"

	"github.com/gruntjs/grunt-go/internal/nodepath"
	"github.com/gruntjs/grunt-go/js"
)

type kind int

const (
	kindLiteral kind = iota + 1
	kindBoolean
	kindString
	kindNumber
	kindPath
	kindArray
	kindList
)

// Type is a nopt type: a type constructor, an explicitly allowed value, or a list of
// alternatives.
type Type struct {
	kind  kind
	value any    // the allowed value, for kindLiteral
	list  []Type // the alternatives, for kindList
}

// The type constructors nopt knows, as grunt-known-options uses them.
var (
	// Boolean accepts flags; "--x true/false" and "--x=1" style values convert.
	Boolean = Type{kind: kindBoolean}
	// String takes the next argument (or "" when it looks like an option).
	String = Type{kind: kindString}
	// Number takes the next argument and requires it to convert to a number.
	Number = Type{kind: kindNumber}
	// Path is require('path'): the value is resolved against the working directory,
	// with a leading "~/" resolved against the home directory.
	Path = Type{kind: kindPath}
	// Array allows the option to repeat, collecting its values in an array.
	Array = Type{kind: kindArray}
)

// Literal is an explicitly allowed value: a string, a float64, a bool or nil (null).
// A NaN literal is nopt's poison value, which rejects every value.
func Literal(v any) Type {
	return Type{kind: kindLiteral, value: js.Normalize(v)}
}

// OneOf is a list of alternatives, such as grunt's [Number, Boolean] for --debug or
// [String, Array] for a repeatable string option.
func OneOf(types ...Type) Type {
	return Type{kind: kindList, list: append([]Type(nil), types...)}
}

// same is JavaScript === between two type entries; lists are never equal.
func (t Type) same(u Type) bool {
	if t.kind != u.kind {
		return false
	}
	switch t.kind {
	case kindLiteral:
		return js.StrictEquals(t.value, u.value)
	case kindList:
		return false
	}
	return true
}

// falsy reports whether the type itself is a falsy JavaScript value (a null, false,
// 0, "" or NaN literal), which clean replaces with its default type list.
func (t Type) falsy() bool {
	return t.kind == kindLiteral && !js.ToBoolean(t.value)
}

// has is list.indexOf(t) !== -1.
func has(list []Type, t Type) bool {
	for _, x := range list {
		if x.same(t) {
			return true
		}
	}
	return false
}

// hasLiteral is list.indexOf(v) !== -1 for a value.
func hasLiteral(list []Type, v any) bool {
	return has(list, Type{kind: kindLiteral, value: v})
}

// Result is what nopt returns: the parsed options in JavaScript key order and the
// argv record.
type Result struct {
	// Options maps option names to bool, string, float64, nil (null), js.Undefined
	// or *js.Array values.
	Options *js.Object
	// Remain lists the arguments that are not options.
	Remain []string
	// Cooked is the argument list after "=" splitting and shorthand expansion.
	Cooked []string
	// Original is the argument list as given (after slicing).
	Original []string
}

// Parse is nopt(types, shorthands, args, slice). Shorthand values are the
// whitespace-split expansions nopt makes of string shorthands (grunt's aliases are
// {"v": "--verbose"}, so []string{"--verbose"} here). nopt splits a string shorthand
// only when it is looked up by its own name, not when it is reached through a run of
// single-character shorthands ("-ab"); string shorthands containing whitespace are
// therefore not reproduced on that path (grunt has none). Path values resolve against the
// process's working directory and home directory; the only errors are failures to
// determine those, which Node reports by throwing.
func Parse(types map[string]Type, shorthands map[string][]string, args []string, slice int) (*Result, error) {
	p := &parser{
		cwd:  os.Getwd,
		home: homeDir,
	}
	return p.run(types, shorthands, args, slice)
}

// ParseIn is Parse with a fixed working directory and home directory ("" meaning no
// home directory, so "~/" is not expanded).
func ParseIn(cwd, home string, types map[string]Type, shorthands map[string][]string, args []string, slice int) *Result {
	p := &parser{
		cwd:  func() (string, error) { return cwd, nil },
		home: func() (string, error) { return home, nil },
	}
	// The fixed lookups cannot fail.
	r, _ := p.run(types, shorthands, args, slice)
	return r
}

// homeDir is os.homedir(): $HOME when set and non-empty, else the user database.
func homeDir() (string, error) {
	if h := os.Getenv("HOME"); h != "" {
		return h, nil
	}
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

type parser struct {
	cwd  func() (string, error)
	home func() (string, error)
}

func (p *parser) run(types map[string]Type, shorthands map[string][]string, args []string, slice int) (*Result, error) {
	if types == nil {
		types = map[string]Type{}
	}
	if slice > len(args) {
		slice = len(args)
	}
	if slice < 0 {
		slice = 0
	}
	cooked := append([]string{}, args[slice:]...)
	r := &Result{
		Options:  js.NewObject(),
		Remain:   []string{},
		Original: append([]string{}, cooked...),
	}
	r.Cooked = parse(cooked, r.Options, &r.Remain, types, shorthands)
	if err := p.clean(r.Options, types); err != nil {
		return nil, err
	}
	r.Options.Delete("argv")
	return r, nil
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// splice is Array.prototype.splice(i, 1, ...items).
func splice(args []string, i int, items []string) []string {
	out := make([]string, 0, len(args)-1+len(items))
	out = append(out, args[:i]...)
	out = append(out, items...)
	return append(out, args[i+1:]...)
}

// parse is nopt's parse; it returns the cooked argument list it rewrote in place.
func parse(args []string, data *js.Object, remain *[]string, types map[string]Type, shorthands map[string][]string) []string {
	abbrevs := abbrev(keys(types))
	shortAbbr := abbrev(keys(shorthands))
	singles := map[string]bool{}
	for k := range shorthands {
		if length16(k) == 1 {
			singles[k] = true
		}
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if allDashes(arg, 2) {
			// Done with options; the rest are arguments.
			*remain = append(*remain, args[i+1:]...)
			args[i] = "--"
			break
		}
		hadEq := false
		if !strings.HasPrefix(arg, "-") || len(arg) == 1 {
			*remain = append(*remain, arg)
			continue
		}
		if at := strings.IndexByte(arg, '='); at > -1 {
			hadEq = true
			v := arg[at+1:]
			arg = arg[:at]
			args = splice(args, i, []string{arg, v})
		}

		// A shorthand is spliced in, and parsed again unless it expands to itself.
		if shRes, ok := resolveShort(arg, shorthands, shortAbbr, abbrevs, singles); ok {
			args = splice(args, i, shRes)
			if len(shRes) == 0 || arg != shRes[0] {
				i--
				continue
			}
		}
		arg = strings.TrimLeft(arg, "-")
		var no *bool
		for len(arg) >= 3 && strings.EqualFold(arg[:3], "no-") {
			v := no == nil || !*no
			no = &v
			arg = arg[3:]
		}
		if full, ok := abbrevs[arg]; ok {
			arg = full
		}

		argType, known := types[arg]
		isTypeArray := known && argType.kind == kindList
		if isTypeArray && len(argType.list) == 1 {
			isTypeArray = false
			argType = argType.list[0]
		}
		isArray := (known && argType.kind == kindArray) || (isTypeArray && has(argType.list, Array))

		// Unknown options become arrays when given more than once.
		if !known && data.Has(arg) {
			if _, ok := data.Get(arg).(*js.Array); !ok {
				data.Set(arg, js.NewArray(data.Get(arg)))
			}
			isArray = true
		}

		la, laDefined := "", i+1 < len(args)
		if laDefined {
			la = args[i+1]
		}

		isBool := no != nil ||
			(known && argType.kind == kindBoolean) ||
			(isTypeArray && has(argType.list, Boolean)) ||
			(!known && !hadEq) ||
			(laDefined && la == "false" &&
				((known && argType.kind == kindLiteral && argType.value == nil) || (isTypeArray && hasLiteral(argType.list, nil))))

		if isBool {
			val := any(no == nil || !*no)
			// Also support "--bool true" and "--bool false".
			if laDefined && (la == "true" || la == "false") {
				b := la == "true"
				if no != nil && *no {
					b = !b
				}
				val = b
				laDefined = false
				i++
			}
			// Also support "foo": [Boolean, "bar"] with "--foo bar".
			if isTypeArray && laDefined && la != "" {
				switch {
				case hasLiteral(argType.list, la):
					val = la
					i++
				case la == "null" && hasLiteral(argType.list, nil):
					val = nil
					i++
				case !dashesThenNonDash(la, 2) && !isNaN(la) && has(argType.list, Number):
					val = js.StringToNumber(la)
					i++
				case !dashesThenNonDash(la, 1) && has(argType.list, String):
					val = la
					i++
				}
			}
			store(data, arg, val, isArray)
			continue
		}

		var val any
		if known && argType.kind == kindString {
			if !laDefined {
				la, laDefined = "", true
			} else if looksLikeOption(la) {
				la = ""
				i--
			}
		}
		if laDefined && allDashes(la, 2) {
			laDefined = false
			i--
		}
		if laDefined {
			val = la
		} else {
			val = true
		}
		store(data, arg, val, isArray)
		i++
	}
	return args
}

// store sets data[arg], or appends to it when the option collects an array.
func store(data *js.Object, arg string, val any, isArray bool) {
	if !isArray {
		data.Set(arg, val)
		return
	}
	a, ok := data.Get(arg).(*js.Array)
	if !ok {
		a = js.NewArray()
		data.Set(arg, a)
	}
	a.Items = append(a.Items, val)
}

// allDashes is /^-{n,}$/.
func allDashes(s string, n int) bool {
	return len(s) >= n && strings.Trim(s, "-") == ""
}

// dashesThenNonDash is /^-{n,}[^-]/ for n = 2, and /^-[^-]/ for n = 1.
func dashesThenNonDash(s string, n int) bool {
	if n == 1 {
		return len(s) >= 2 && s[0] == '-' && s[1] != '-'
	}
	d := len(s) - len(strings.TrimLeft(s, "-"))
	return d >= n && d < len(s)
}

// looksLikeOption is /^-{1,2}[^-]+/: one or two dashes, then a non-dash.
func looksLikeOption(s string) bool {
	switch {
	case len(s) >= 2 && s[0] == '-' && s[1] != '-':
		return true
	case len(s) >= 3 && s[0] == '-' && s[1] == '-' && s[2] != '-':
		return true
	}
	return false
}

// isNaN is the global isNaN applied to a string.
func isNaN(s string) bool {
	f := js.StringToNumber(s)
	return f != f
}

// resolveShort handles single-character shorthands run together ("-glp"), exact
// shorthands and abbreviations of shorthands; ok is false when arg is no shorthand.
func resolveShort(arg string, shorthands map[string][]string, shortAbbr, abbrevs map[string]string, singles map[string]bool) ([]string, bool) {
	arg = strings.TrimLeft(arg, "-")
	// An exact known option goes no further.
	if full, ok := abbrevs[arg]; ok && full == arg {
		return nil, false
	}
	if sh, ok := shorthands[arg]; ok {
		return sh, true
	}
	// A run of single-character shorthands. The empty string (from "-=x") counts as
	// an empty run, as in the original, and expands to nothing.
	all := true
	var out []string
	for _, c := range arg {
		if !singles[string(c)] {
			all = false
			break
		}
		out = append(out, shorthands[string(c)]...)
	}
	if all {
		if out == nil {
			out = []string{}
		}
		return out, true
	}
	// An option abbreviation that is not a literal shorthand prefers the option.
	if _, ok := abbrevs[arg]; ok {
		return nil, false
	}
	if full, ok := shortAbbr[arg]; ok {
		arg = full
	}
	sh, ok := shorthands[arg]
	return sh, ok
}

// typeDefault is the type list clean uses for unknown options and falsy types.
var typeDefault = []Type{Literal(false), Literal(true), Literal(nil), String, Array}

// clean converts and validates the parsed values against their types, as nopt's
// clean does, deleting options left without a valid value.
func (p *parser) clean(data *js.Object, types map[string]Type) error {
	for _, k := range data.Keys() {
		if k == "argv" {
			continue
		}
		val := data.Get(k)
		arr, isArray := val.(*js.Array)
		vals := []any{val}
		if isArray {
			vals = arr.Items
		}
		typ, known := types[k]
		var list []Type
		switch {
		case !known || typ.falsy():
			list = typeDefault
		case typ.kind == kindArray:
			list = append(append([]Type(nil), typeDefault...), Array)
		case typ.kind == kindList:
			list = typ.list
		default:
			list = []Type{typ}
		}

		var kept []any
		for _, v := range vals {
			if s, ok := v.(string); ok {
				s = strings.TrimFunc(s, js.IsSpace)
				v = s
				switch {
				case s == "null" && hasLiteral(list, nil),
					s == "true" && (hasLiteral(list, true) || has(list, Boolean)),
					s == "false" && (hasLiteral(list, false) || has(list, Boolean)):
					v = map[string]any{"null": nil, "true": true, "false": false}[s]
				case has(list, Number) && !isNaN(s):
					v = js.StringToNumber(s)
				}
			}
			if !known {
				kept = append(kept, v)
				continue
			}
			// Allow --no-blah to set blah to null if null is allowed.
			if v == false && hasLiteral(list, nil) && !(hasLiteral(list, false) || has(list, Boolean)) {
				v = nil
			}
			got, ok, err := p.validate(v, typ)
			if err != nil {
				return err
			}
			if ok {
				kept = append(kept, got)
			}
		}

		switch {
		case len(kept) == 0 && !has(list, Array):
			data.Delete(k)
		case isArray:
			data.Set(k, js.NewArray(kept...))
		case len(kept) == 0:
			data.Set(k, js.Undefined)
		default:
			data.Set(k, kept[0])
		}
	}
	return nil
}

// validate is nopt's validate for one value: it returns the converted value and
// whether the type accepts it.
func (p *parser) validate(val any, t Type) (any, bool, error) {
	switch t.kind {
	case kindList:
		for _, sub := range t.list {
			if sub.kind == kindArray {
				continue
			}
			got, ok, err := p.validate(val, sub)
			if err != nil || ok {
				return got, ok, err
			}
		}
		return js.Undefined, false, nil
	case kindArray:
		return val, true, nil
	case kindLiteral:
		// NaN is poisonous: it means nothing is allowed.
		if f, ok := t.value.(float64); ok && f != f {
			return js.Undefined, false, nil
		}
		return val, js.StrictEquals(val, t.value), nil
	case kindString:
		return js.ToString(val), true, nil
	case kindBoolean:
		if s, ok := val.(string); ok {
			switch {
			case !isNaN(s):
				return js.ToBoolean(js.StringToNumber(s)), true, nil
			case s == "null" || s == "false":
				return false, true, nil
			}
			return true, true, nil
		}
		return js.ToBoolean(val), true, nil
	case kindNumber:
		f := js.ToNumber(val)
		if f != f {
			return js.Undefined, false, nil
		}
		return f, true, nil
	case kindPath:
		return p.validatePath(val)
	}
	return js.Undefined, false, nil
}

// validatePath resolves a path value; true is rejected and null kept as undefined
// (nopt's validatePath accepts null without storing a value).
func (p *parser) validatePath(val any) (any, bool, error) {
	if val == true {
		return js.Undefined, false, nil
	}
	if val == nil {
		return js.Undefined, true, nil
	}
	s := js.ToString(val)
	if rest, ok := strings.CutPrefix(s, "~/"); ok {
		home, err := p.home()
		if err != nil {
			return nil, false, err
		}
		if home != "" {
			r, err := p.resolve(home, rest)
			return r, err == nil, err
		}
	}
	r, err := p.resolve(s)
	return r, err == nil, err
}

// resolve is path.resolve: the working directory is read only when no segment is
// absolute, as path.resolve calls process.cwd() only then.
func (p *parser) resolve(segments ...string) (string, error) {
	if slices.ContainsFunc(segments, nodepath.IsAbsolute) {
		return nodepath.Resolve("", segments...), nil
	}
	cwd, err := p.cwd()
	if err != nil {
		return "", err
	}
	return nodepath.Resolve(cwd, segments...), nil
}
