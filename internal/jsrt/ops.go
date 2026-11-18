package jsrt

import (
	"math"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsnum"
	"github.com/gruntjs/grunt-go/js"
)

// toPrimitive is the ToPrimitive abstract operation. hint is "string", "number" or
// "default"; Date objects treat "default" as "string". Objects use their (possibly
// user-defined) valueOf and toString methods.
func (r *Realm) toPrimitive(v any, hint string) (any, error) {
	if !js.IsObject(v) {
		return v, nil
	}
	if hint == "default" {
		hint = "number"
		if _, isDate := v.(js.Date); isDate {
			hint = "string"
		}
	}
	order := [2]string{"valueOf", "toString"}
	if hint == "string" {
		order = [2]string{"toString", "valueOf"}
	}
	for _, name := range order {
		m, _ := r.lookup(v, name)
		if f, ok := m.(*js.Function); ok {
			res, err := r.call(f, v, nil)
			if err != nil {
				return nil, err
			}
			if !js.IsObject(res) {
				return res, nil
			}
		}
	}
	return nil, js.NewTypeError("Cannot convert object to primitive value")
}

// toString is the ToString abstract operation. Primitives convert with
// js.ToString; objects go through ToPrimitive with hint string.
func (r *Realm) toString(v any) (string, error) {
	if js.IsObject(v) {
		p, err := r.toPrimitive(v, "string")
		if err != nil {
			return "", err
		}
		v = p
	}
	return js.ToString(v), nil
}

// toNumber is the ToNumber abstract operation.
func (r *Realm) toNumber(v any) (float64, error) {
	if js.IsObject(v) {
		p, err := r.toPrimitive(v, "number")
		if err != nil {
			return 0, err
		}
		v = p
	}
	return js.ToNumber(v), nil
}

// toPropertyKey converts a computed member key to the string the property is stored
// under.
func (r *Realm) toPropertyKey(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case float64:
		return js.NumberToString(x), nil
	}
	p, err := r.toPrimitive(v, "string")
	if err != nil {
		return "", err
	}
	return js.ToString(p), nil
}

// looseEquals is the == operator.
func (r *Realm) looseEquals(a, b any) (bool, error) { return js.LooselyEqual(a, b, r.toPrimitive) }

// lessThan is IsLessThan (see js.LessThan) with this realm's ToPrimitive.
func (r *Realm) lessThan(a, b any, leftFirst bool) (lt bool, ok bool, err error) {
	return js.LessThan(a, b, leftFirst, r.toPrimitive)
}

// relational evaluates < > <= >=.
func (r *Realm) relational(op string, a, b any) (bool, error) {
	switch op {
	case "<":
		lt, ok, err := r.lessThan(a, b, true)
		return ok && lt, err
	case ">":
		lt, ok, err := r.lessThan(b, a, false)
		return ok && lt, err
	case "<=":
		lt, ok, err := r.lessThan(b, a, false)
		return ok && !lt, err
	default: // ">="
		lt, ok, err := r.lessThan(a, b, true)
		return ok && !lt, err
	}
}

// add is the + operator.
func (r *Realm) add(a, b any) (any, error) { return js.Add(a, b, r.toPrimitive) }

// arith evaluates the numeric binary operators other than +.
func (r *Realm) arith(op string, a, b any) (any, error) {
	x, err := r.toNumber(a)
	if err != nil {
		return nil, err
	}
	y, err := r.toNumber(b)
	if err != nil {
		return nil, err
	}
	switch op {
	case "-":
		return x - y, nil
	case "*":
		return x * y, nil
	case "/":
		return x / y, nil
	case "%":
		return jsMod(x, y), nil
	case "**":
		return jsnum.Pow(x, y), nil
	case "|":
		return float64(js.ToInt32(x) | js.ToInt32(y)), nil
	case "&":
		return float64(js.ToInt32(x) & js.ToInt32(y)), nil
	case "^":
		return float64(js.ToInt32(x) ^ js.ToInt32(y)), nil
	case "<<":
		return float64(js.ToInt32(x) << (js.ToUint32(y) & 31)), nil
	case ">>":
		return float64(js.ToInt32(x) >> (js.ToUint32(y) & 31)), nil
	case ">>>":
		return float64(js.ToUint32(x) >> (js.ToUint32(y) & 31)), nil
	}
	return nil, js.NewSyntaxError("Unexpected token '" + op + "'")
}

// jsMod is Number::remainder: the sign follows the dividend.
func jsMod(x, y float64) float64 {
	if math.IsInf(y, 0) && !math.IsInf(x, 0) && !math.IsNaN(x) {
		return x
	}
	return math.Mod(x, y)
}

// instanceOf is the instanceof operator for function right-hand sides.
func (r *Realm) instanceOf(v any, f *js.Function) bool {
	if bound, ok := r.boundTargets[f]; ok {
		f = bound
	}
	proto, _ := r.lookup(f, "prototype")
	if !js.IsObject(proto) || !js.IsObject(v) {
		return false
	}
	for p := r.protoOf(v); p != nil; p = r.protoOf(p) {
		if p == proto {
			return true
		}
	}
	return false
}
