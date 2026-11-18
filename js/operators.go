package js

import (
	"math"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
)

// ToPrimitiveFunc is the ToPrimitive operation an operator applies to objects,
// with the hint it passes ("default" or "number"). The template runtime runs an
// object's own valueOf and toString; code outside it, where values carry no
// methods of their own, converts dates and other objects directly.
type ToPrimitiveFunc func(v any, hint string) (any, error)

// LooselyEqual is the == operator (IsLooselyEqual).
func LooselyEqual(a, b any, toPrimitive ToPrimitiveFunc) (bool, error) {
	for {
		if TypeOf(a) == TypeOf(b) && IsObject(a) == IsObject(b) {
			return StrictEquals(a, b), nil
		}
		if IsNullish(a) && IsNullish(b) {
			return true, nil
		}
		if IsNullish(a) || IsNullish(b) {
			return false, nil
		}
		switch x := a.(type) {
		case float64:
			if s, ok := b.(string); ok {
				return x == StringToNumber(s), nil
			}
		case string:
			if y, ok := b.(float64); ok {
				return StringToNumber(x) == y, nil
			}
		case bool:
			a = ToNumber(x)
			continue
		}
		if y, ok := b.(bool); ok {
			b = ToNumber(y)
			continue
		}
		if IsObject(a) && !IsObject(b) {
			p, err := toPrimitive(a, "default")
			if err != nil {
				return false, err
			}
			a = p
			continue
		}
		if IsObject(b) && !IsObject(a) {
			p, err := toPrimitive(b, "default")
			if err != nil {
				return false, err
			}
			b = p
			continue
		}
		return false, nil
	}
}

// LessThan is IsLessThan: whether a < b, with ok false when the result is
// undefined (NaN involved). leftFirst says which operand is converted first, as
// a < b converts a first and a > b, evaluated as b < a, converts b first.
func LessThan(a, b any, leftFirst bool, toPrimitive ToPrimitiveFunc) (lt bool, ok bool, err error) {
	var pa, pb any
	if leftFirst {
		if pa, err = toPrimitive(a, "number"); err != nil {
			return
		}
		if pb, err = toPrimitive(b, "number"); err != nil {
			return
		}
	} else {
		if pb, err = toPrimitive(b, "number"); err != nil {
			return
		}
		if pa, err = toPrimitive(a, "number"); err != nil {
			return
		}
	}
	sa, aStr := pa.(string)
	sb, bStr := pb.(string)
	if aStr && bStr {
		return jsstr.Compare(sa, sb) < 0, true, nil
	}
	x, y := ToNumber(pa), ToNumber(pb)
	if math.IsNaN(x) || math.IsNaN(y) {
		return false, false, nil
	}
	return x < y, true, nil
}

// Add is the + operator: string concatenation when either primitive is a string,
// numeric addition otherwise.
func Add(a, b any, toPrimitive ToPrimitiveFunc) (any, error) {
	if x, ok := a.(float64); ok {
		if y, ok := b.(float64); ok {
			return x + y, nil
		}
	}
	if x, ok := a.(string); ok {
		if y, ok := b.(string); ok {
			return Concat(x, y)
		}
	}
	pa, err := toPrimitive(a, "default")
	if err != nil {
		return nil, err
	}
	pb, err := toPrimitive(b, "default")
	if err != nil {
		return nil, err
	}
	_, as := pa.(string)
	_, bs := pb.(string)
	if as || bs {
		return Concat(ToString(pa), ToString(pb))
	}
	return ToNumber(pa) + ToNumber(pb), nil
}

// Concat joins two strings, failing as V8 does when the result would exceed
// MaxStringLength. A UTF-8 byte count is never below the UTF-16 length, so the
// exact length is only computed for very long strings.
func Concat(x, y string) (string, error) {
	if len(x)+len(y) > MaxStringLength && jsstr.Length(x)+jsstr.Length(y) > MaxStringLength {
		return "", InvalidStringLength()
	}
	return x + y, nil
}
