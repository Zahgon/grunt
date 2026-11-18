package jsrt

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsnum"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// Global functions, Object, Function, Boolean and the Error constructors.

func (r *Realm) setupGlobalFunctions() {
	g := r.global
	r.method(g, "parseInt", 2, func(_ any, args []any) (any, error) {
		s, err := r.toString(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		radix, err := r.toNumber(js.Arg(args, 1))
		if err != nil {
			return nil, err
		}
		return jsnum.ParseInt(s, int(js.ToInt32(radix))), nil
	})
	r.method(g, "parseFloat", 1, func(_ any, args []any) (any, error) {
		s, err := r.toString(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return jsnum.ParseFloat(s), nil
	})
	r.method(g, "isNaN", 1, func(_ any, args []any) (any, error) {
		n, err := r.toNumber(js.Arg(args, 0))
		return math.IsNaN(n), err
	})
	r.method(g, "isFinite", 1, func(_ any, args []any) (any, error) {
		n, err := r.toNumber(js.Arg(args, 0))
		return !math.IsNaN(n) && !math.IsInf(n, 0), err
	})
	uri := func(name string, fn func(string) (string, error)) {
		r.method(g, name, 1, func(_ any, args []any) (any, error) {
			s, err := r.toString(js.Arg(args, 0))
			if err != nil {
				return nil, err
			}
			return fn(s)
		})
	}
	const unreserved = "-_.!~*'()"
	const reserved = ";/?:@&=+$,#"
	uri("encodeURIComponent", func(s string) (string, error) { return uriEncode(s, unreserved) })
	uri("encodeURI", func(s string) (string, error) { return uriEncode(s, unreserved+reserved) })
	uri("decodeURIComponent", func(s string) (string, error) { return uriDecode(s, "") })
	uri("decodeURI", func(s string) (string, error) { return uriDecode(s, reserved) })
}

func uriError() error { return &js.Error{Name: "URIError", Message: "URI malformed"} }

// uriEncode is the Encode operation: characters outside the alphanumerics and keep
// are percent-encoded as UTF-8; an unpaired surrogate is an error.
func uriEncode(s, keep string) (string, error) {
	u := jsstr.ToUTF16(s)
	var b strings.Builder
	const hex = "0123456789ABCDEF"
	for i := 0; i < len(u); i++ {
		c := u[i]
		if c < 0x80 && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte(keep, byte(c)) >= 0) {
			b.WriteByte(byte(c))
			continue
		}
		cp, n := jsstr.CodePointAt(u, i)
		if cp >= 0xD800 && cp <= 0xDFFF {
			return "", uriError()
		}
		i += n - 1
		var buf [4]byte
		for _, octet := range buf[:utf8.EncodeRune(buf[:], cp)] {
			b.WriteByte('%')
			b.WriteByte(hex[octet>>4])
			b.WriteByte(hex[octet&15])
		}
	}
	return b.String(), nil
}

// uriDecode is the Decode operation; escapes of characters in keep stay encoded.
func uriDecode(s, keep string) (string, error) {
	u := jsstr.ToUTF16(s)
	var out []uint16
	for i := 0; i < len(u); i++ {
		if u[i] != '%' {
			out = append(out, u[i])
			continue
		}
		start := i
		octet, ok := hexOctet(u, i)
		if !ok {
			return "", uriError()
		}
		i += 2
		if octet < 0x80 {
			if strings.IndexByte(keep, octet) >= 0 {
				out = append(out, u[start:i+1]...)
			} else {
				out = append(out, uint16(octet))
			}
			continue
		}
		n := 0
		switch {
		case octet&0xE0 == 0xC0:
			n = 2
		case octet&0xF0 == 0xE0:
			n = 3
		case octet&0xF8 == 0xF0:
			n = 4
		default:
			return "", uriError()
		}
		buf := []byte{octet}
		for k := 1; k < n; k++ {
			i++
			if i >= len(u) || u[i] != '%' {
				return "", uriError()
			}
			o, ok := hexOctet(u, i)
			if !ok || o&0xC0 != 0x80 {
				return "", uriError()
			}
			buf = append(buf, o)
			i += 2
		}
		cp, size := utf8.DecodeRune(buf)
		if cp == utf8.RuneError || size != n {
			return "", uriError()
		}
		out = jsstr.AppendCodePoint(out, cp)
	}
	return jsstr.FromUTF16(out), nil
}

func hexOctet(u []uint16, i int) (byte, bool) {
	if i+2 >= len(u) || u[i+1] > 0x7f || u[i+2] > 0x7f {
		return 0, false
	}
	hi, lo := jsstr.HexDigitValue(rune(u[i+1])), jsstr.HexDigitValue(rune(u[i+2]))
	if hi < 0 || lo < 0 {
		return 0, false
	}
	return byte(hi<<4 | lo), true
}

// classOf is the tag Object.prototype.toString reports.
func (r *Realm) classOf(v any) string {
	switch x := v.(type) {
	case *arguments:
		return "Arguments"
	case *iterator:
		return x.tag
	case *js.Object:
		switch x {
		case r.global:
			return "global"
		case r.global.Get("Math"):
			return "Math"
		case r.global.Get("JSON"):
			return "JSON"
		}
	}
	return js.ClassOf(v)
}

// toObject is ToObject for the builtins that accept any non-nullish value.
func toObject(v any) (any, error) {
	if js.IsNullish(v) {
		return nil, js.NewTypeError("Cannot convert undefined or null to object")
	}
	return v, nil
}

func (r *Realm) setupObject() {
	op := r.objectProto
	obj := r.constructor("Object", 1, op, func(_ any, args []any) (any, error) {
		return r.objectFromValue(js.Arg(args, 0)), nil
	}, func(args []any) (any, error) {
		return r.objectFromValue(js.Arg(args, 0)), nil
	})
	r.method(op, "hasOwnProperty", 1, func(this any, args []any) (any, error) {
		key, err := r.toPropertyKey(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		if _, err := toObject(this); err != nil {
			return nil, err
		}
		return r.hasOwn(this, key), nil
	})
	r.method(op, "isPrototypeOf", 1, func(this any, args []any) (any, error) {
		v := js.Arg(args, 0)
		if !js.IsObject(v) {
			return false, nil
		}
		for p := r.protoOf(v); p != nil; p = r.protoOf(p) {
			if p == this {
				return true, nil
			}
		}
		return false, nil
	})
	r.method(op, "propertyIsEnumerable", 1, func(this any, args []any) (any, error) {
		key, err := r.toPropertyKey(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return r.hasOwn(this, key) && !r.isOwnNonEnumerable(this, key), nil
	})
	r.method(op, "toString", 0, func(this any, _ []any) (any, error) {
		return "[object " + r.classOf(this) + "]", nil
	})
	r.method(op, "toLocaleString", 0, func(this any, _ []any) (any, error) {
		m, err := r.getV(this, "toString")
		if err != nil {
			return nil, err
		}
		return r.callValue(m, this, nil)
	})
	r.method(op, "valueOf", 0, func(this any, _ []any) (any, error) {
		return toObject(this)
	})

	keysOf := func(args []any) (any, []string, error) {
		v, err := toObject(js.Arg(args, 0))
		if err != nil {
			return nil, nil, err
		}
		return v, r.ownKeys(v), nil
	}
	r.method(obj, "keys", 1, func(_ any, args []any) (any, error) {
		_, keys, err := keysOf(args)
		if err != nil {
			return nil, err
		}
		out := make([]any, len(keys))
		for i, k := range keys {
			out[i] = k
		}
		return js.NewArray(out...), nil
	})
	r.method(obj, "values", 1, func(_ any, args []any) (any, error) {
		v, keys, err := keysOf(args)
		if err != nil {
			return nil, err
		}
		out := make([]any, len(keys))
		for i, k := range keys {
			out[i], _ = r.lookup(v, k)
		}
		return js.NewArray(out...), nil
	})
	r.method(obj, "entries", 1, func(_ any, args []any) (any, error) {
		v, keys, err := keysOf(args)
		if err != nil {
			return nil, err
		}
		out := make([]any, len(keys))
		for i, k := range keys {
			val, _ := r.lookup(v, k)
			out[i] = js.NewArray(k, val)
		}
		return js.NewArray(out...), nil
	})
	r.method(obj, "getOwnPropertyNames", 1, func(_ any, args []any) (any, error) {
		v, keys, err := keysOf(args)
		if err != nil {
			return nil, err
		}
		switch x := v.(type) {
		case *js.Object:
			keys = x.Keys()
		case *js.Array, string:
			keys = append(keys, "length")
		}
		out := make([]any, len(keys))
		for i, k := range keys {
			out[i] = k
		}
		return js.NewArray(out...), nil
	})
	r.method(obj, "assign", 2, func(_ any, args []any) (any, error) {
		target, err := toObject(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		for _, src := range args[1:] {
			if js.IsNullish(src) {
				continue
			}
			for _, k := range r.ownKeys(src) {
				v, _ := r.getOwn(src, k)
				if err := r.put(target, k, v); err != nil {
					return nil, err
				}
			}
		}
		return target, nil
	})
	r.method(obj, "fromEntries", 1, func(_ any, args []any) (any, error) {
		src := js.Arg(args, 0)
		entries, err := r.iterate(src, func() error {
			return js.NewTypeError(describeValue(src) + " is not iterable")
		})
		if err != nil {
			return nil, err
		}
		o := js.NewObject()
		for _, en := range entries {
			if !js.IsObject(en) {
				return nil, js.NewTypeError("Iterator value " + js.ToString(en) + " is not an entry object")
			}
			k, err := r.getV(en, "0")
			if err != nil {
				return nil, err
			}
			v, err := r.getV(en, "1")
			if err != nil {
				return nil, err
			}
			key, err := r.toPropertyKey(k)
			if err != nil {
				return nil, err
			}
			o.Set(key, v)
		}
		return o, nil
	})
	r.method(obj, "create", 2, func(_ any, args []any) (any, error) {
		proto := js.Arg(args, 0)
		if !js.IsObject(proto) && proto != nil {
			return nil, js.NewTypeError("Object prototype may only be an Object or null: " + js.ToString(proto))
		}
		if !isUndefined(js.Arg(args, 1)) {
			return nil, js.NewTypeError("Object.create with property descriptors is not supported")
		}
		return &Derived{Own: js.NewObject(), Proto: proto}, nil
	})
	r.method(obj, "getPrototypeOf", 1, func(_ any, args []any) (any, error) {
		v, err := toObject(js.Arg(args, 0))
		if err != nil {
			return nil, err
		}
		return r.protoOf(v), nil
	})
	r.method(obj, "freeze", 1, func(_ any, args []any) (any, error) {
		v := js.Arg(args, 0)
		if o, ok := v.(*js.Object); ok {
			r.frozen[o] = true
		}
		return v, nil
	})
	r.method(obj, "isFrozen", 1, func(_ any, args []any) (any, error) {
		switch x := js.Arg(args, 0).(type) {
		case *js.Object:
			return r.frozen[x], nil
		case string, float64, bool, nil, js.UndefinedType:
			return true, nil
		}
		return false, nil
	})
	r.method(obj, "is", 2, func(_ any, args []any) (any, error) {
		return sameValue(js.Arg(args, 0), js.Arg(args, 1)), nil
	})
}

// sameValue is Object.is: like === but NaN equals NaN and +0 differs from -0.
func sameValue(a, b any) bool {
	x, ok1 := a.(float64)
	y, ok2 := b.(float64)
	if ok1 && ok2 {
		if math.IsNaN(x) && math.IsNaN(y) {
			return true
		}
		return x == y && math.Signbit(x) == math.Signbit(y)
	}
	return js.StrictEquals(a, b)
}

// objectFromValue is Object(v): a new object for null or undefined, v itself
// otherwise. Primitive wrapper objects are not modelled, so primitives are
// returned unchanged.
func (r *Realm) objectFromValue(v any) any {
	if js.IsNullish(v) {
		return js.NewObject()
	}
	return v
}

// funcSource is Function.prototype.toString.
func (r *Realm) funcSource(f *js.Function) string {
	if c, ok := r.closures[f]; ok {
		return c.fi.source
	}
	if _, bound := r.boundTargets[f]; bound {
		return "function () { [native code] }"
	}
	return "function " + f.Name + "() { [native code] }"
}

func (r *Realm) setupFunction() {
	fp := r.functionProto
	r.constructor("Function", 1, fp, func(_ any, args []any) (any, error) {
		return r.functionConstructor(args)
	}, r.functionConstructor)
	thisFunc := func(this any, method string) (*js.Function, error) {
		f, ok := this.(*js.Function)
		if !ok {
			return nil, js.NewTypeError("Function.prototype." + method + " called on non-function")
		}
		return f, nil
	}
	r.method(fp, "call", 1, func(this any, args []any) (any, error) {
		f, ok := this.(*js.Function)
		if !ok {
			return nil, js.NewTypeError("Function.prototype.call is not a function")
		}
		var rest []any
		if len(args) > 1 {
			rest = args[1:]
		}
		return r.call(f, js.Arg(args, 0), rest)
	})
	r.method(fp, "apply", 2, func(this any, args []any) (any, error) {
		f, ok := this.(*js.Function)
		if !ok {
			return nil, js.NewTypeError("Function.prototype.apply was called on " + describeValue(this) + ", which is " + js.TypeOf(this) + " and not a function")
		}
		list := js.Arg(args, 1)
		var callArgs []any
		if !js.IsNullish(list) {
			if !js.IsObject(list) {
				return nil, js.NewTypeError("CreateListFromArrayLike called on non-object")
			}
			items, err := r.arrayLike(list)
			if err != nil {
				return nil, err
			}
			callArgs = items
		}
		return r.call(f, js.Arg(args, 0), callArgs)
	})
	r.method(fp, "bind", 1, func(this any, args []any) (any, error) {
		target, err := thisFunc(this, "bind")
		if err != nil {
			return nil, js.NewTypeError("Bind must be called on a function")
		}
		boundThis := js.Arg(args, 0)
		var bound []any
		if len(args) > 1 {
			bound = append(bound, args[1:]...)
		}
		f := &js.Function{Name: "bound " + target.Name, Length: max(target.Length-len(bound), 0)}
		f.Call = func(_ any, callArgs []any) (any, error) {
			return r.call(target, boundThis, append(append([]any(nil), bound...), callArgs...))
		}
		r.boundTargets[f] = target
		r.boundArgs[f] = bound
		return f, nil
	})
	r.method(fp, "toString", 0, func(this any, _ []any) (any, error) {
		f, err := thisFunc(this, "toString")
		if err != nil {
			return nil, js.NewTypeError("Function.prototype.toString requires that 'this' be a Function")
		}
		return r.funcSource(f), nil
	})
}

// functionConstructor is new Function(p1, ..., body): the source is parsed as V8
// parses it and the function closes over the global scope.
func (r *Realm) functionConstructor(args []any) (any, error) {
	var params []string
	body := ""
	for i, a := range args {
		s, err := r.toString(a)
		if err != nil {
			return nil, err
		}
		if i == len(args)-1 {
			body = s
		} else {
			params = append(params, s)
		}
	}
	fi, err := parseFunctionConstructor(params, body)
	if err != nil {
		return nil, err
	}
	return r.makeFunction(fi, r.globalEnv(), "anonymous"), nil
}

// globalEnv is the outermost environment, backed by the global object.
func (r *Realm) globalEnv() *env {
	return &env{obj: r.global}
}

// arrayLike reads length and indexes of an array-like object.
func (r *Realm) arrayLike(v any) ([]any, error) {
	switch x := v.(type) {
	case *js.Array:
		return append([]any(nil), x.Items...), nil
	case *arguments:
		return append([]any(nil), x.items...), nil
	}
	lv, err := r.getV(v, "length")
	if err != nil {
		return nil, err
	}
	n, err := r.toNumber(lv)
	if err != nil {
		return nil, err
	}
	l := toLength(n)
	if l > 1<<24 {
		return nil, js.InvalidArrayLength()
	}
	out := make([]any, int(l))
	for i := range out {
		out[i], _ = r.lookup(v, js.NumberToString(float64(i)))
	}
	return out, nil
}

// toLength is ToLength on a number.
func toLength(n float64) float64 {
	n = js.ToIntegerOrInfinity(n)
	if n <= 0 {
		return 0
	}
	return math.Min(n, 1<<53-1)
}

func (r *Realm) setupBoolean() {
	bp := r.booleanProto
	r.constructor("Boolean", 1, bp, func(_ any, args []any) (any, error) {
		return js.ToBoolean(js.Arg(args, 0)), nil
	}, nil)
	thisBool := func(this any, method string) (bool, error) {
		b, ok := this.(bool)
		if !ok {
			return false, js.NewTypeError("Boolean.prototype." + method + " requires that 'this' be a Boolean")
		}
		return b, nil
	}
	r.method(bp, "toString", 0, func(this any, _ []any) (any, error) {
		b, err := thisBool(this, "toString")
		if err != nil {
			return nil, err
		}
		return js.ToString(b), nil
	})
	r.method(bp, "valueOf", 0, func(this any, _ []any) (any, error) {
		return thisBool(this, "valueOf")
	})
}

func (r *Realm) setupErrors() {
	for _, name := range js.NativeErrorNames {
		proto := js.NewObject()
		r.errorProtos[name] = proto
		if name != "Error" {
			r.protoLinks[proto] = r.errorProtos["Error"]
		}
		r.define(proto, "name", name)
		r.define(proto, "message", "")
		build := func(args []any) (any, error) {
			e := &js.Error{Name: name}
			if msg := js.Arg(args, 0); !isUndefined(msg) {
				s, err := r.toString(msg)
				if err != nil {
					return nil, err
				}
				e.Message = s
			}
			return e, nil
		}
		r.constructor(name, 1, proto, func(_ any, args []any) (any, error) { return build(args) }, build)
		if name == "Error" {
			r.method(proto, "toString", 0, func(this any, _ []any) (any, error) {
				if !js.IsObject(this) {
					return nil, js.NewTypeError("Method Error.prototype.toString called on incompatible receiver " + js.ToString(this))
				}
				nv, _ := r.lookup(this, "name")
				mv, _ := r.lookup(this, "message")
				name, msg := "Error", ""
				var err error
				if !isUndefined(nv) {
					if name, err = r.toString(nv); err != nil {
						return nil, err
					}
				}
				if !isUndefined(mv) {
					if msg, err = r.toString(mv); err != nil {
						return nil, err
					}
				}
				switch {
				case name == "":
					return msg, nil
				case msg == "":
					return name, nil
				}
				return name + ": " + msg, nil
			})
		}
	}
}

// setupIterator installs %IteratorPrototype%.next for the built-in iterators.
func (r *Realm) setupIterator() {
	r.method(r.iteratorProto, "next", 0, func(this any, _ []any) (any, error) {
		it, ok := this.(*iterator)
		if !ok {
			return nil, js.NewTypeError("next method called on incompatible receiver " + r.safeString(this))
		}
		if it.pos >= len(it.items) {
			return js.ObjectOf("value", js.Undefined, "done", true), nil
		}
		v := it.items[it.pos]
		it.pos++
		return js.ObjectOf("value", v, "done", false), nil
	})
}
