// Package jsnum holds the number conversions of ECMAScript that need more than
// js.NumberToString: Number.prototype.toFixed, toExponential, toPrecision and
// toString(radix), the global parseInt and parseFloat, and Math.pow(10, p),
// computed exactly as V8 does.
package jsnum

import (
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/js"
)

// Digits come from the exact decimal expansion of the double and ties round up
// (away from zero), as ECMA-262 asks ("if there are two such n, pick the larger
// n"), not to even.

// exactDigits returns the significant decimal digits of x (finite, non-zero) with
// no trailing zeros and the decimal exponent of the first digit, so that
// |x| = d.ddd × 10^exp.
func exactDigits(x float64) (string, int) {
	// A double has at most 767 significant decimal digits, so 800 digits after
	// the point are exact.
	s := new(big.Float).SetFloat64(math.Abs(x)).Text('e', 800)
	mant, expPart, _ := strings.Cut(s, "e")
	exp, err := strconv.Atoi(expPart)
	if err != nil {
		panic("jsnum: malformed big.Float exponent " + expPart)
	}
	digits := strings.TrimRight(strings.Replace(mant, ".", "", 1), "0")
	return digits, exp
}

// roundDigits keeps the first n of digits (n may exceed len(digits), which pads
// with zeros), rounding half up on the dropped part. It returns the n digits and
// whether rounding carried into a new leading digit (then the digits are "1" and
// zeros, and the exponent must grow by one).
func roundDigits(digits string, n int) (string, bool) {
	if n <= len(digits) {
		kept := []byte(digits[:n])
		if n < len(digits) && digits[n] >= '5' {
			i := n - 1
			for ; i >= 0 && kept[i] == '9'; i-- {
				kept[i] = '0'
			}
			if i < 0 {
				return "1" + strings.Repeat("0", max(n-1, 0)), true
			}
			kept[i]++
		}
		return string(kept), false
	}
	return digits + strings.Repeat("0", n-len(digits)), false
}

// ToFixed is Number.prototype.toFixed(digits) for a digits argument already
// converted with ToIntegerOrInfinity.
func ToFixed(x, digits float64) (string, error) {
	if digits < 0 || digits > 100 {
		return "", js.NewRangeError("toFixed() digits argument must be between 0 and 100")
	}
	if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) >= 1e21 {
		return js.NumberToString(x), nil
	}
	f := int(digits)
	sign := ""
	if x < 0 {
		sign, x = "-", -x
	}
	// n is the integer nearest to x × 10^f, as a digit string.
	n := "0"
	if x != 0 {
		d, e := exactDigits(x)
		k := e + 1 + f // digits of n: those before the rounding position
		switch {
		case k > 0:
			r, carry := roundDigits(d, k)
			if carry {
				r = "1" + strings.Repeat("0", k)
			}
			n = r
		case k == 0 && d[0] >= '5':
			n = "1"
		}
	}
	if f == 0 {
		return sign + n, nil
	}
	if len(n) <= f {
		n = strings.Repeat("0", f+1-len(n)) + n
	}
	return sign + n[:len(n)-f] + "." + n[len(n)-f:], nil
}

// ToExponential is Number.prototype.toExponential. digits is the converted
// fractionDigits argument; undefined (shortest form) is passed as hasDigits false.
func ToExponential(x, digits float64, hasDigits bool) (string, error) {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return js.NumberToString(x), nil
	}
	if digits < 0 || digits > 100 {
		return "", js.NewRangeError("toExponential() argument must be between 0 and 100")
	}
	f := int(digits)
	sign := ""
	if x < 0 {
		sign, x = "-", -x
	}
	var m string
	e := 0
	switch {
	case x == 0:
		m = strings.Repeat("0", f+1)
	case !hasDigits:
		mant, expPart, _ := strings.Cut(strconv.FormatFloat(x, 'e', -1, 64), "e")
		m = strings.Replace(mant, ".", "", 1)
		e, _ = strconv.Atoi(expPart)
		f = len(m) - 1
	default:
		d, exp := exactDigits(x)
		var carry bool
		m, carry = roundDigits(d, f+1)
		e = exp
		if carry {
			m = "1" + strings.Repeat("0", f)
			e++
		}
	}
	if f != 0 {
		m = m[:1] + "." + m[1:]
	}
	return sign + m + "e" + exponentSuffix(e), nil
}

func exponentSuffix(e int) string {
	if e < 0 {
		return "-" + strconv.Itoa(-e)
	}
	return "+" + strconv.Itoa(e)
}

// ToPrecision is Number.prototype.toPrecision for a precision argument already
// converted with ToIntegerOrInfinity.
func ToPrecision(x, precision float64) (string, error) {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return js.NumberToString(x), nil
	}
	if precision < 1 || precision > 100 {
		return "", js.NewRangeError("toPrecision() argument must be between 1 and 100")
	}
	p := int(precision)
	sign := ""
	if x < 0 {
		sign, x = "-", -x
	}
	var m string
	e := 0
	if x == 0 {
		m = strings.Repeat("0", p)
	} else {
		d, exp := exactDigits(x)
		var carry bool
		m, carry = roundDigits(d, p)
		e = exp
		if carry {
			m = "1" + strings.Repeat("0", p-1)
			e++
		}
	}
	switch {
	case e < -6 || e >= p:
		if p != 1 {
			m = m[:1] + "." + m[1:]
		}
		return sign + m + "e" + exponentSuffix(e), nil
	case e == p-1:
		return sign + m, nil
	case e >= 0:
		return sign + m[:e+1] + "." + m[e+1:], nil
	}
	return sign + "0." + strings.Repeat("0", -(e+1)) + m, nil
}

const radixDigits = "0123456789abcdefghijklmnopqrstuvwxyz"

// ToStringRadix is Number.prototype.toString(radix) for radix 2 to 36, following
// V8's DoubleToRadixCString: fraction digits only up to the double's precision and
// zeros for integer digits below its precision.
func ToStringRadix(x float64, radix int) string {
	if radix == 10 || math.IsNaN(x) || math.IsInf(x, 0) {
		return js.NumberToString(x)
	}
	if x == 0 {
		return "0"
	}
	negative := x < 0
	if negative {
		x = -x
	}
	integer := math.Floor(x)
	fraction := x - integer
	delta := math.Max(0.5*(math.Nextafter(x, math.Inf(1))-x), math.Nextafter(0, 1))
	var frac []byte
	if fraction >= delta {
		for {
			// The explicit conversions keep the compiler from fusing the product
			// with the following subtraction, which V8 does not do.
			fraction = float64(fraction * float64(radix))
			delta = float64(delta * float64(radix))
			digit := int(fraction)
			frac = append(frac, radixDigits[digit])
			fraction -= float64(digit)
			if fraction > 0.5 || (fraction == 0.5 && digit&1 == 1) {
				if fraction+delta > 1 {
					// Round up, propagating the carry through the written digits.
					for {
						if len(frac) == 0 {
							integer++
							break
						}
						last := strings.IndexByte(radixDigits, frac[len(frac)-1])
						frac = frac[:len(frac)-1]
						if last+1 < radix {
							frac = append(frac, radixDigits[last+1])
							break
						}
					}
					break
				}
			}
			if fraction < delta {
				break
			}
		}
	}
	var intDigits []byte
	// Integer digits below the double's precision are written as zeros.
	for exponentOf(integer/float64(radix)) > 0 {
		integer /= float64(radix)
		intDigits = append(intDigits, '0')
	}
	for {
		remainder := math.Mod(integer, float64(radix))
		intDigits = append(intDigits, radixDigits[int(remainder)])
		integer = (integer - remainder) / float64(radix)
		if integer <= 0 {
			break
		}
	}
	var b strings.Builder
	if negative {
		b.WriteByte('-')
	}
	for i := len(intDigits) - 1; i >= 0; i-- {
		b.WriteByte(intDigits[i])
	}
	if len(frac) > 0 {
		b.WriteByte('.')
		b.Write(frac)
	}
	return b.String()
}

// exponentOf is V8's Double::Exponent: the power of two of the lowest bit of the
// 53-bit significand.
func exponentOf(x float64) int {
	bits := math.Float64bits(x)
	biased := int(bits >> 52 & 0x7ff)
	if biased == 0 {
		return -1074
	}
	return biased - 1075
}

// ParseFloat is the global parseFloat applied to a string.
func ParseFloat(s string) float64 {
	s = strings.TrimLeftFunc(s, js.IsSpace)
	body := s
	if body != "" && (body[0] == '+' || body[0] == '-') {
		body = body[1:]
	}
	if strings.HasPrefix(body, "Infinity") {
		if s[0] == '-' {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}
	i, digits := 0, 0
	for i < len(body) && body[i] >= '0' && body[i] <= '9' {
		i, digits = i+1, digits+1
	}
	if i < len(body) && body[i] == '.' {
		i++
		for i < len(body) && body[i] >= '0' && body[i] <= '9' {
			i, digits = i+1, digits+1
		}
	}
	if digits == 0 {
		return math.NaN()
	}
	if i < len(body) && (body[i] == 'e' || body[i] == 'E') {
		j := i + 1
		if j < len(body) && (body[j] == '+' || body[j] == '-') {
			j++
		}
		start := j
		for j < len(body) && body[j] >= '0' && body[j] <= '9' {
			j++
		}
		if j > start {
			i = j
		}
	}
	text := s[:len(s)-len(body)+i]
	// The text is a valid decimal literal, so only ErrRange is possible, and then
	// the result is the correctly rounded ±Inf or ±0.
	f, _ := strconv.ParseFloat(strings.TrimSuffix(text, "."), 64)
	if f == 0 && s[0] == '-' {
		return math.Copysign(0, -1)
	}
	return f
}

// ParseInt is the global parseInt(s, radix) with the radix already converted by
// ToInt32. Radix 10 and the powers of two round correctly; other radixes
// accumulate in 32-bit chunks as V8 does.
func ParseInt(s string, radix int) float64 {
	s = strings.TrimLeftFunc(s, js.IsSpace)
	negative := false
	if s != "" && (s[0] == '+' || s[0] == '-') {
		negative = s[0] == '-'
		s = s[1:]
	}
	stripPrefix := true
	if radix != 0 {
		if radix < 2 || radix > 36 {
			return math.NaN()
		}
		stripPrefix = radix == 16
	} else {
		radix = 10
	}
	if stripPrefix && len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		s, radix = s[2:], 16
	}
	end := 0
	for end < len(s) && digitIn(s[end], radix) {
		end++
	}
	if end == 0 {
		return math.NaN()
	}
	digits := s[:end]
	var v float64
	switch {
	case radix == 10:
		// Digits only, so the parse cannot fail; overflow gives the rounded +Inf.
		v, _ = strconv.ParseFloat(digits, 64)
	case radix&(radix-1) == 0:
		n, ok := new(big.Int).SetString(digits, radix)
		if !ok {
			panic("jsnum: validated digits did not parse")
		}
		v, _ = new(big.Float).SetInt(n).Float64()
	default:
		v = parseIntChunked(digits, radix)
	}
	if negative {
		return -v
	}
	return v
}

func digitIn(c byte, radix int) bool {
	d := jsstr.DigitValue(rune(c))
	return d >= 0 && d < radix
}

// parseIntChunked is V8's HandleGenericCase for radixes that are neither 10 nor a
// power of two: digits are gathered into parts whose multiplier stays below
// 2^32/36, and each part is folded in as result*multiplier + part in double
// precision. The reference Node 24 build (linux/arm64) compiles that expression to
// a fused multiply-add, so it is fused here too; an x86-64 build rounds the product
// separately, which can change the last bits of values above 2^53.
func parseIntChunked(digits string, radix int) float64 {
	const maxMultiplier = 0xFFFFFFFF / 36
	result := 0.0
	i := 0
	for i < len(digits) {
		part, multiplier := uint32(0), uint32(1)
		for i < len(digits) {
			m := multiplier * uint32(radix)
			if m > maxMultiplier {
				break
			}
			part = part*uint32(radix) + uint32(jsstr.DigitValue(rune(digits[i])))
			multiplier = m
			i++
		}
		result = math.FMA(result, float64(multiplier), float64(part))
	}
	return result
}

// Pow10 is Math.pow(10, p). V8 calls the C library's pow, whose glibc version is
// correctly rounded except within a tiny distance of a rounding tie, so the power
// is computed here with 128-bit precision and rounded once. 10^23, the only exact
// tie among the integral powers, rounds up as glibc's pow does
// (1.0000000000000001e+23).
func Pow10(p float64) float64 {
	switch {
	case math.IsNaN(p):
		return math.NaN()
	case p > 400:
		return math.Inf(1)
	case p < -400:
		return 0
	}
	const prec = 128
	whole := math.Floor(p)
	scale := new(big.Float).SetPrec(prec).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(math.Abs(whole))), nil))
	power := new(big.Float).SetPrec(prec).SetInt64(1)
	// The fraction is taken in big arithmetic, where p - whole is exact.
	frac := new(big.Float).SetPrec(prec).Sub(new(big.Float).SetPrec(prec).SetFloat64(p), big.NewFloat(whole))
	if frac.Sign() != 0 {
		power = expBig(frac.Mul(frac, ln10()))
	}
	if whole >= 0 {
		power.Mul(power, scale)
	} else {
		power.Quo(power, scale)
	}
	result := new(big.Float).SetPrec(53)
	if p == whole && p >= 0 {
		result.SetMode(big.ToNearestAway)
	}
	f, _ := result.Set(power).Float64()
	return f
}

// ln10 is ln(10) to more than 128 bits.
func ln10() *big.Float {
	v, _, err := big.ParseFloat("2.30258509299404568401799145468436420760110148862877297603332790096757", 10, 160, big.ToNearestEven)
	if err != nil {
		panic(err)
	}
	return v
}

// expBig is e^z for |z| < 3 by its Taylor series, at z's precision.
func expBig(z *big.Float) *big.Float {
	prec := z.Prec()
	sum := new(big.Float).SetPrec(prec).SetInt64(1)
	term := new(big.Float).SetPrec(prec).SetInt64(1)
	for k := int64(1); k < 200; k++ {
		term.Mul(term, z)
		term.Quo(term, new(big.Float).SetPrec(prec).SetInt64(k))
		if term.Sign() == 0 || term.MantExp(nil)-sum.MantExp(nil) < -int(prec)-8 {
			break
		}
		sum.Add(sum, term)
	}
	return sum
}
