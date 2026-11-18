package jsnum

import (
	"math"
	"math/big"
)

// Math functions whose Go versions do not round like V8's. Explicit float64
// conversions around products keep the Go compiler from fusing them into
// multiply-adds, which V8's code does not use.

// Round is Math.round: halves round towards +Infinity, and -0.5 <= x < 0 gives
// -0.
func Round(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) || x == math.Trunc(x) {
		return x
	}
	if x < 0 && x >= -0.5 {
		return math.Copysign(0, -1)
	}
	f := math.Floor(x)
	if x-f >= 0.5 {
		f++
	}
	return f
}

// Pow is Math.pow and the ** operator. V8 calls the C library's pow, which glibc
// rounds correctly; this computes x^y with 192-bit precision and rounds once.
// Integral exponents are computed exactly, so exact results and exact rounding
// ties come out as glibc gives them.
func Pow(x, y float64) float64 {
	switch {
	case math.IsNaN(y):
		return math.NaN()
	case y == 0:
		return 1
	case math.IsNaN(x):
		return math.NaN()
	case math.IsInf(y, 0):
		ax := math.Abs(x)
		switch {
		case ax == 1:
			// ECMAScript's Number::exponentiate, unlike C, gives NaN here.
			return math.NaN()
		case (ax > 1) == (y > 0):
			return math.Inf(1)
		}
		return 0
	case x == 0 || math.IsInf(x, 0):
		// The C rules for zero and infinite bases: math.Pow follows them.
		return math.Pow(x, y)
	}
	integral := y == math.Trunc(y)
	if x < 0 && !integral {
		return math.NaN()
	}
	negate := x < 0 && integral && math.Mod(y, 2) != 0
	ax := math.Abs(x)
	var r float64
	if integral && math.Abs(y) <= 1100 {
		r = powIntegral(ax, int(y))
	} else {
		r = powReal(ax, y)
	}
	if negate {
		return -r
	}
	return r
}

const powPrec = 192

// powIntegral computes ax^n exactly (or, for n < 0, its reciprocal to 192 bits)
// and rounds to the nearest double, ties to even.
func powIntegral(ax float64, n int) float64 {
	base := new(big.Float).SetFloat64(ax)
	k := n
	if k < 0 {
		k = -k
	}
	// An exact product needs at most 53 bits per factor.
	prec := uint(53*k + 64)
	acc := new(big.Float).SetPrec(prec).SetInt64(1)
	sq := new(big.Float).SetPrec(prec).Set(base)
	for k > 0 {
		if k&1 == 1 {
			acc.Mul(acc, sq)
		}
		k >>= 1
		if k > 0 {
			sq.Mul(sq, sq)
		}
	}
	if n < 0 {
		acc = new(big.Float).SetPrec(powPrec).Quo(new(big.Float).SetPrec(powPrec).SetInt64(1), acc)
	}
	f, _ := acc.Float64()
	return f
}

// powReal computes exp(y·ln ax) for ax > 0.
func powReal(ax, y float64) float64 {
	l := lnBig(new(big.Float).SetPrec(powPrec).SetFloat64(ax))
	l.Mul(l, new(big.Float).SetPrec(powPrec).SetFloat64(y))
	// Results beyond the double range overflow or underflow before rounding.
	if lf, _ := l.Float64(); lf > 710 {
		return math.Inf(1)
	} else if lf < -746 {
		return 0
	}
	f, _ := expBigReduced(l).Float64()
	return f
}

// ln2 is ln 2 to powPrec bits, 2·atanh(1/3).
var ln2 = func() *big.Float {
	third := new(big.Float).SetPrec(powPrec).Quo(big.NewFloat(1).SetPrec(powPrec), big.NewFloat(3).SetPrec(powPrec))
	a := atanhSeries(third)
	return a.Mul(a, big.NewFloat(2))
}()

// atanhSeries sums z + z^3/3 + z^5/5 + ... for |z| <= 1/3.
func atanhSeries(z *big.Float) *big.Float {
	sum := new(big.Float).SetPrec(powPrec).Set(z)
	z2 := new(big.Float).SetPrec(powPrec).Mul(z, z)
	term := new(big.Float).SetPrec(powPrec).Set(z)
	for k := int64(3); k < 1000; k += 2 {
		term.Mul(term, z2)
		t := new(big.Float).SetPrec(powPrec).Quo(term, new(big.Float).SetPrec(powPrec).SetInt64(k))
		if t.Sign() == 0 || t.MantExp(nil)-sum.MantExp(nil) < -powPrec-8 {
			break
		}
		sum.Add(sum, t)
	}
	return sum
}

// lnBig is ln x for x > 0: x = m·2^e with m in [0.5, 1), ln m = 2·atanh((m-1)/(m+1)).
func lnBig(x *big.Float) *big.Float {
	m := new(big.Float).SetPrec(powPrec)
	e := x.MantExp(m)
	one := new(big.Float).SetPrec(powPrec).SetInt64(1)
	num := new(big.Float).SetPrec(powPrec).Sub(m, one)
	den := new(big.Float).SetPrec(powPrec).Add(m, one)
	z := num.Quo(num, den)
	lm := atanhSeries(z)
	lm.Mul(lm, big.NewFloat(2))
	le := new(big.Float).SetPrec(powPrec).Mul(ln2, new(big.Float).SetPrec(powPrec).SetInt64(int64(e)))
	return lm.Add(lm, le)
}

// expBigReduced is e^z for |z| < 750: z = k·ln2 + r with |r| <= ln2/2.
func expBigReduced(z *big.Float) *big.Float {
	q := new(big.Float).SetPrec(powPrec).Quo(z, ln2)
	qf, _ := q.Float64()
	k := math.Round(qf)
	r := new(big.Float).SetPrec(powPrec).Mul(ln2, new(big.Float).SetPrec(powPrec).SetFloat64(k))
	r.Sub(z, r)
	sum := new(big.Float).SetPrec(powPrec).SetInt64(1)
	term := new(big.Float).SetPrec(powPrec).SetInt64(1)
	for n := int64(1); n < 200; n++ {
		term.Mul(term, r)
		term.Quo(term, new(big.Float).SetPrec(powPrec).SetInt64(n))
		if term.Sign() == 0 || term.MantExp(nil)-sum.MantExp(nil) < -powPrec-8 {
			break
		}
		sum.Add(sum, term)
	}
	return sum.SetMantExp(sum, int(k))
}

// Cbrt is Math.cbrt: V8's ieee754::cbrt, FreeBSD's s_cbrt.c.
func Cbrt(x float64) float64 {
	const (
		b1 = 715094163 // (1023-1023/3-0.03306235651)*2**20
		b2 = 696219795 // (1023-1023/3-54/3-0.03306235651)*2**20
		p0 = 1.87595182427177009643
		p1 = -1.88497979543377169875
		p2 = 1.621429720105354466140
		p3 = -0.758397934778766047437
		p4 = 0.145996192886612446982
	)
	bits := math.Float64bits(x)
	hx := uint32(bits >> 32)
	low := uint32(bits)
	sign := hx & 0x80000000
	hx ^= sign
	if hx >= 0x7ff00000 {
		return x + x
	}
	var t float64
	if hx < 0x00100000 {
		if hx|low == 0 {
			return x
		}
		t = float64(math.Float64frombits(0x43500000<<32) * x)
		high := uint32(math.Float64bits(t) >> 32)
		t = math.Float64frombits(uint64(sign|((high&0x7fffffff)/3+b2)) << 32)
	} else {
		t = math.Float64frombits(uint64(sign|(hx/3+b1)) << 32)
	}
	r := float64(float64(t*t) * float64(t/x))
	t = float64(t * float64(float64(p0+float64(r*float64(p1+float64(r*p2))))+float64(float64(float64(r*r)*r)*float64(p3+float64(r*p4)))))
	t = math.Float64frombits((math.Float64bits(t) + 0x80000000) & 0xffffffffc0000000)
	s := float64(t * t)
	r = x / s
	w := t + t
	r = (r - t) / (w + r)
	return t + float64(t*r)
}

// Hypot is Math.hypot as V8's builtin computes it: the arguments are scaled by
// the largest magnitude and summed with Kahan compensation. An infinite
// argument wins over NaN.
func Hypot(values []float64) float64 {
	maxAbs := 0.0
	nan := false
	for _, v := range values {
		a := math.Abs(v)
		switch {
		case math.IsInf(a, 1):
			return math.Inf(1)
		case math.IsNaN(a):
			nan = true
		case a > maxAbs:
			maxAbs = a
		}
	}
	if nan {
		return math.NaN()
	}
	if maxAbs == 0 {
		return 0
	}
	sum, compensation := 0.0, 0.0
	for _, v := range values {
		n := math.Abs(v) / maxAbs
		summand := float64(n*n) - compensation
		preliminary := sum + summand
		compensation = (preliminary - sum) - summand
		sum = preliminary
	}
	return float64(math.Sqrt(sum) * maxAbs)
}
