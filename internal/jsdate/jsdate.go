// Package jsdate implements the parts of ECMAScript's Date that grunt and its
// templates reach: Date.parse with V8's parser, Date.UTC, the conversions between
// UTC and local time, the field getters and the string forms of a date. Time values
// are float64 milliseconds since the epoch, NaN for an invalid date, exactly as in
// JavaScript. Local time is the host's time zone (see local.go).
package jsdate

import (
	"math"
	"strconv"
	"strings"
)

const (
	msPerSecond = 1000.0
	msPerMinute = 60000.0
	msPerHour   = 3600000.0
	msPerDay    = 86400000.0

	// maxTime is the largest magnitude of a valid time value (ECMA-262 21.4.1.1).
	maxTime = 8.64e15

	// minYear and maxYear bound the years V8's MakeDay accepts before giving NaN.
	minYear = -1000000
	maxYear = 1000000
	// minMonth and maxMonth bound the months V8's MakeDay accepts.
	minMonth = -10000000
	maxMonth = 10000000
)

// toInteger is ToIntegerOrInfinity for a finite or infinite number (NaN gives 0).
// Package js, which has the exported js.ToIntegerOrInfinity, imports this package
// for its date strings, so this one cannot use it.
func toInteger(x float64) float64 {
	if math.IsNaN(x) {
		return 0
	}
	return math.Trunc(x) + 0 // + 0 turns -0 into +0
}

// cumulative day counts before each month, for common and leap years.
var daysBeforeMonth = [2][12]int{
	{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334},
	{0, 31, 60, 91, 121, 152, 182, 213, 244, 274, 305, 335},
}

// MakeDay is ECMA-262 MakeDay as V8 computes it: the day number of date within
// month of year, with month overflowing into the year. Years outside ±1000000 or
// months outside ±10000000 give NaN. The arithmetic follows V8's MakeDay literally,
// including its fixed year offset and truncating integer division, so far-out years
// that a large date brings back into range agree with V8 too.
func MakeDay(year, month, date float64) float64 {
	if !(minYear <= year && year <= maxYear && minMonth <= month && month <= maxMonth) || math.IsInf(date, 0) || math.IsNaN(date) {
		return math.NaN()
	}
	y, m := int(year), int(month)
	y += m / 12
	m %= 12
	if m < 0 {
		m += 12
		y--
	}
	// V8's kYearDelta is -1 modulo 400 and keeps y + yearDelta positive for every
	// year a valid time value can have.
	const yearDelta = 399999
	const baseDay = 365*(1970+yearDelta) + (1970+yearDelta)/4 - (1970+yearDelta)/100 + (1970+yearDelta)/400
	dayFromYear := 365*(y+yearDelta) + (y+yearDelta)/4 - (y+yearDelta)/100 + (y+yearDelta)/400 - baseDay
	leap := 0
	if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
		leap = 1
	}
	dayFromYear += daysBeforeMonth[leap][m]
	return float64(dayFromYear-1) + toInteger(date)
}

// MakeTime is ECMA-262 MakeTime. The products are converted explicitly so the
// compiler cannot fuse them into multiply-adds, which would round differently from
// V8's separate multiplications and additions.
func MakeTime(hour, min, sec, ms float64) float64 {
	for _, v := range [...]float64{hour, min, sec, ms} {
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return math.NaN()
		}
	}
	h := float64(toInteger(hour) * msPerHour)
	m := float64(toInteger(min) * msPerMinute)
	s := float64(toInteger(sec) * msPerSecond)
	return h + m + s + toInteger(ms)
}

// MakeDate is ECMA-262 MakeDate.
func MakeDate(day, time float64) float64 {
	if math.IsInf(day, 0) || math.IsNaN(day) || math.IsInf(time, 0) || math.IsNaN(time) {
		return math.NaN()
	}
	d := float64(day * msPerDay)
	if time == 0 {
		return d
	}
	return d + time
}

// TimeClip is ECMA-262 TimeClip: NaN outside ±8.64e15, otherwise the integer part.
func TimeClip(t float64) float64 {
	if math.IsNaN(t) || math.Abs(t) > maxTime {
		return math.NaN()
	}
	return toInteger(t)
}

// UTC is Date.UTC(year, month, day, hours, minutes, seconds, ms) with every argument
// already converted by ToNumber. Callers pass JavaScript's defaults for omitted
// arguments: month 0, day 1 and 0 for the time fields (a missing year is NaN). As in
// the specification, an integral year from 0 to 99 means 1900 plus that year.
func UTC(year, month, day, hours, minutes, seconds, ms float64) float64 {
	return TimeClip(MakeDate(MakeDay(fullYear(year), month, day), MakeTime(hours, minutes, seconds, ms)))
}

// Local is new Date(year, month, day, hours, minutes, seconds, ms): the arguments,
// converted and defaulted as for UTC, read as a local date and time.
func Local(year, month, day, hours, minutes, seconds, ms float64) float64 {
	return FromLocal(MakeDate(MakeDay(fullYear(year), month, day), MakeTime(hours, minutes, seconds, ms)))
}

// fullYear maps a year argument of Date.UTC or the Date constructor whose integral
// part is 0 to 99 to 1900 plus that part.
func fullYear(year float64) float64 {
	if !math.IsNaN(year) {
		if y := toInteger(year); 0 <= y && y <= 99 {
			return 1900 + y
		}
	}
	return year
}

// Fields splits a valid time value into its UTC calendar fields: the full year, the
// month from 0, the day of the month from 1, hours, minutes, seconds, milliseconds and
// the weekday from 0 (Sunday). It is getUTCFullYear and its siblings (LocalFields
// gives the local ones); ms must not be NaN.
func Fields(ms float64) (year, month, day, hours, minutes, seconds, millis, weekday int) {
	t := int64(ms)
	days := floorDiv(t, msPerDay)
	within := int(t - days*msPerDay)
	hours = within / int(msPerHour)
	minutes = within / int(msPerMinute) % 60
	seconds = within / int(msPerSecond) % 60
	millis = within % int(msPerSecond)
	weekday = int(((days+4)%7 + 7) % 7)
	year, month, day = civilFromDays(days)
	return
}

func floorDiv(a int64, b float64) int64 {
	d := int64(b)
	q := a / d
	if a%d != 0 && a < 0 {
		q--
	}
	return q
}

// civilFromDays converts a day number to a proleptic Gregorian date (month from 0).
func civilFromDays(days int64) (year, month, day int) {
	// Howard Hinnant's days_from_civil inverse, with 400-year eras.
	z := days + 719468
	era := z / 146097
	if z < 0 && z%146097 != 0 {
		era--
	}
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y := yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d := doy - (153*mp+2)/5 + 1
	m := mp + 2
	if mp >= 10 {
		m = mp - 10
	}
	if m <= 1 {
		y++
	}
	return int(y), int(m), int(d)
}

// ISOString is Date.prototype.toISOString: YYYY-MM-DDTHH:mm:ss.sssZ, with a signed
// six-digit year outside 0 to 9999. It reports false for an invalid date, for which
// toISOString throws a RangeError "Invalid time value".
func ISOString(ms float64) (string, bool) {
	if math.IsNaN(ms) {
		return "", false
	}
	year, month, day, h, m, s, milli, _ := Fields(ms)
	var b strings.Builder
	switch {
	case year < 0:
		b.WriteString("-" + pad(-year, 6))
	case year > 9999:
		b.WriteString("+" + pad(year, 6))
	default:
		b.WriteString(pad(year, 4))
	}
	b.WriteString("-" + pad(month+1, 2) + "-" + pad(day, 2) + "T" + pad(h, 2) + ":" + pad(m, 2) + ":" + pad(s, 2) + "." + pad(milli, 3) + "Z")
	return b.String(), true
}

func pad(n, width int) string {
	s := strconv.Itoa(n)
	if len(s) < width {
		s = strings.Repeat("0", width-len(s)) + s
	}
	return s
}
