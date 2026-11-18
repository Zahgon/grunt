package jsdate

import (
	"math"
	"strconv"
)

// The string forms of a Date. Each gives "Invalid Date" for NaN, as the
// Date.prototype methods do.

const invalidDate = "Invalid Date"

var (
	dayNames   = [...]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	monthNames = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
)

// yearString formats a year as V8's date strings do: four digits at least, and a
// negative year's magnitude padded to four digits after the minus sign ("-0001").
func yearString(y int) string {
	if y < 0 {
		return "-" + pad(-y, 4)
	}
	return pad(y, 4)
}

// localeYear is the en-US calendar year ICU prints: years before 1 count back from
// 1 BC without an era (0 is 1, -1 is 2).
func localeYear(y int) int {
	if y <= 0 {
		return 1 - y
	}
	return y
}

// ToString is Date.prototype.toString: the local date and time, the offset and the
// zone name, as in "Tue Jan 02 2024 10:00:00 GMT+0000 (Coordinated Universal Time)".
func ToString(t float64) string {
	if math.IsNaN(t) {
		return invalidDate
	}
	return dateString(t) + " " + timeString(t)
}

// ToDateString is Date.prototype.toDateString: "Tue Jan 02 2024" in local time.
func ToDateString(t float64) string {
	if math.IsNaN(t) {
		return invalidDate
	}
	return dateString(t)
}

// ToTimeString is Date.prototype.toTimeString: "10:00:00 GMT+0000 (Coordinated
// Universal Time)" in local time.
func ToTimeString(t float64) string {
	if math.IsNaN(t) {
		return invalidDate
	}
	return timeString(t)
}

func dateString(t float64) string {
	y, mo, d, _, _, _, _, wd := LocalFields(t)
	return dayNames[wd] + " " + monthNames[mo] + " " + pad(d, 2) + " " + yearString(y)
}

func timeString(t float64) string {
	_, _, _, h, mi, s, _, _ := LocalFields(t)
	return pad(h, 2) + ":" + pad(mi, 2) + ":" + pad(s, 2) + " " + gmtOffset(t) + " (" + zoneName(t) + ")"
}

// ToUTCString is Date.prototype.toUTCString (and toGMTString):
// "Tue, 02 Jan 2024 10:00:00 GMT".
func ToUTCString(t float64) string {
	if math.IsNaN(t) {
		return invalidDate
	}
	y, mo, d, h, mi, s, _, wd := Fields(t)
	return dayNames[wd] + ", " + pad(d, 2) + " " + monthNames[mo] + " " + yearString(y) + " " + pad(h, 2) + ":" + pad(mi, 2) + ":" + pad(s, 2) + " GMT"
}

// ToLocaleDateString is Date.prototype.toLocaleDateString without arguments in the
// en-US locale: "1/2/2024" in local time.
func ToLocaleDateString(t float64) string {
	if math.IsNaN(t) {
		return invalidDate
	}
	return localeDate(t)
}

// ToLocaleTimeString is Date.prototype.toLocaleTimeString without arguments in the
// en-US locale: "10:00:00 AM" in local time.
func ToLocaleTimeString(t float64) string {
	if math.IsNaN(t) {
		return invalidDate
	}
	return localeTime(t)
}

// ToLocaleString is Date.prototype.toLocaleString without arguments in the en-US
// locale: "1/2/2024, 10:00:00 AM" in local time.
func ToLocaleString(t float64) string {
	if math.IsNaN(t) {
		return invalidDate
	}
	return localeDate(t) + ", " + localeTime(t)
}

func localeDate(t float64) string {
	y, mo, d, _, _, _, _, _ := LocalFields(t)
	return strconv.Itoa(mo+1) + "/" + strconv.Itoa(d) + "/" + strconv.Itoa(localeYear(y))
}

func localeTime(t float64) string {
	_, _, _, h, mi, s, _, _ := LocalFields(t)
	ampm := "AM"
	if h >= 12 {
		ampm = "PM"
	}
	h12 := h % 12
	if h12 == 0 {
		h12 = 12
	}
	return strconv.Itoa(h12) + ":" + pad(mi, 2) + ":" + pad(s, 2) + " " + ampm
}
