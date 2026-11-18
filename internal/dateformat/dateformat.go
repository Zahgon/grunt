// Package dateformat is dateformat 4.6.3, the formatter behind grunt.template.date
// and grunt.template.today. It formats with the masks and i18n names that package
// ships; grunt never changes dateFormat.masks or dateFormat.i18n, so they are fixed
// here. Without utc the fields are those of the local time zone (package jsdate), as
// the original reads them with the local Date getters.
package dateformat

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsdate"
	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/js"
)

// Masks are dateFormat.masks, the named formats a mask argument may refer to.
var Masks = map[string]string{
	"default":             "ddd mmm dd yyyy HH:MM:ss",
	"shortDate":           "m/d/yy",
	"paddedShortDate":     "mm/dd/yyyy",
	"mediumDate":          "mmm d, yyyy",
	"longDate":            "mmmm d, yyyy",
	"fullDate":            "dddd, mmmm d, yyyy",
	"shortTime":           "h:MM TT",
	"mediumTime":          "h:MM:ss TT",
	"longTime":            "h:MM:ss TT Z",
	"isoDate":             "yyyy-mm-dd",
	"isoTime":             "HH:MM:ss",
	"isoDateTime":         "yyyy-mm-dd'T'HH:MM:sso",
	"isoUtcDateTime":      "UTC:yyyy-mm-dd'T'HH:MM:ss'Z'",
	"expiresHeaderFormat": "ddd, dd mmm yyyy HH:MM:ss Z",
}

// dayNames, monthNames and timeNames are dateFormat.i18n.
var (
	dayNames = [...]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat",
		"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	monthNames = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
		"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	timeNames = [...]string{"a", "p", "am", "pm", "A", "P", "AM", "PM"}
)

const msPerDay = 86400000.0

// Format is dateFormat(date, mask, utc, gmt). now is the current time in
// milliseconds, used when date is missing and for the Today/Yesterday/Tomorrow names
// of DDD and DDDD.
//
// date may be a js.Date, a number of milliseconds, a string for new Date(string),
// or any other value new Date(value) accepts; a falsy date other than 0 means now.
// As in the original, a date that is a string without ASCII digits is taken as the
// mask instead (the published build tests the arguments object of its outer wrapper,
// whose length is always 1, so this happens even when a mask is also given). mask is
// a mask name from Masks, a mask string, or a falsy value for "default"; a mask that
// starts with "UTC:" or "GMT:" sets utc (and gmt). An invalid date is a TypeError
// "Invalid date".
func Format(date, mask any, utc, gmt bool, now float64) (string, error) {
	if s, ok := date.(string); ok && !strings.ContainsAny(s, "0123456789") {
		mask, date = s, js.Undefined
	}
	if !js.ToBoolean(date) && date != 0.0 {
		date = js.Date{Ms: now}
	}
	ms := toTimeValue(date)
	if math.IsNaN(ms) {
		return "", js.NewTypeError("Invalid date")
	}
	m := resolveMask(mask)
	if strings.HasPrefix(m, "UTC:") || strings.HasPrefix(m, "GMT:") {
		utc = true
		gmt = gmt || m[0] == 'G'
		m = m[4:]
	}
	f := formatter{ms: ms, utc: utc, gmt: gmt, now: now, fields: jsdate.LocalFields}
	if utc {
		f.fields = jsdate.Fields
	}
	f.year, f.month, f.day, f.hours, f.minutes, f.seconds, f.millis, f.weekday = f.fields(ms)
	return f.replaceTokens(m), nil
}

// toTimeValue is the time value of new Date(date).
func toTimeValue(date any) float64 {
	switch d := date.(type) {
	case js.Date:
		return d.Ms
	case float64:
		return jsdate.TimeClip(d)
	case bool:
		return jsdate.TimeClip(js.ToNumber(d))
	case string:
		return jsdate.Parse(d)
	}
	// Objects go through ToPrimitive, which for the value model is their string form.
	return jsdate.Parse(js.ToString(date))
}

// resolveMask is String(dateFormat.masks[mask] || mask || dateFormat.masks["default"]).
func resolveMask(mask any) string {
	key := js.ToString(mask)
	if named, ok := Masks[key]; ok {
		return named
	}
	// masks is a plain object, so masks[mask] also finds what it inherits from
	// Object.prototype; String() of that value becomes the mask.
	if inherited, ok := jsrt.Inherited(js.NewObject(), key); ok {
		return js.ToString(inherited)
	}
	if js.ToBoolean(mask) {
		return key
	}
	return Masks["default"]
}

type formatter struct {
	ms       float64
	utc, gmt bool
	now      float64
	// fields are the getters the original calls through date[_ + "Date"]() and its
	// siblings: the UTC ones with utc, the local ones otherwise.
	fields func(float64) (year, month, day, hours, minutes, seconds, millis, weekday int)

	year, month, day, hours, minutes, seconds, millis, weekday int
}

// replaceTokens is mask.replace(token, ...) with dateFormat's token pattern
// /d{1,4}|D{3,4}|m{1,4}|yy(?:yy)?|([HhMsTt])\1?|W{1,2}|[LlopSZN]|"[^"]*"|'[^']*'/g.
// Quoted text loses its quotes; everything else is copied.
func (f *formatter) replaceTokens(mask string) string {
	var b strings.Builder
	for i := 0; i < len(mask); {
		c := mask[i]
		n := run(mask, i, c)
		switch {
		case c == 'd' || c == 'm':
			n = min(n, 4)
		case c == 'D' && n >= 3:
			n = min(n, 4)
		case c == 'y' && n >= 2:
			n = 2
			if run(mask, i, 'y') >= 4 {
				n = 4
			}
		case strings.IndexByte("HhMsTtW", c) >= 0:
			n = min(n, 2)
		case strings.IndexByte("LlopSZN", c) >= 0:
			n = 1
		case c == '"' || c == '\'':
			end := strings.IndexByte(mask[i+1:], c)
			if end < 0 {
				b.WriteByte(c)
				i++
				continue
			}
			b.WriteString(mask[i+1 : i+1+end])
			i += end + 2
			continue
		default:
			b.WriteByte(c)
			i++
			continue
		}
		b.WriteString(f.flag(mask[i : i+n]))
		i += n
	}
	return b.String()
}

// run counts the repetitions of c starting at mask[i].
func run(mask string, i int, c byte) int {
	n := 0
	for i+n < len(mask) && mask[i+n] == c {
		n++
	}
	return n
}

// flag is the value of one entry of dateFormat's flags table.
func (f *formatter) flag(token string) string {
	switch token {
	case "d":
		return strconv.Itoa(f.day)
	case "dd":
		return pad(strconv.Itoa(f.day), 2)
	case "ddd":
		return dayNames[f.weekday]
	case "DDD":
		return f.relativeDayName(dayNames[f.weekday], true)
	case "dddd":
		return dayNames[f.weekday+7]
	case "DDDD":
		return f.relativeDayName(dayNames[f.weekday+7], false)
	case "m":
		return strconv.Itoa(f.month + 1)
	case "mm":
		return pad(strconv.Itoa(f.month+1), 2)
	case "mmm":
		return monthNames[f.month]
	case "mmmm":
		return monthNames[f.month+12]
	case "yy":
		// String(year).slice(2): the year's characters after the first two, which
		// for a negative year drops the sign and first digit.
		return sliceFrom(strconv.Itoa(f.year), 2)
	case "yyyy":
		// pad pads String(year), so year -5 is "00-5".
		return pad(strconv.Itoa(f.year), 4)
	case "h":
		return strconv.Itoa(hour12(f.hours))
	case "hh":
		return pad(strconv.Itoa(hour12(f.hours)), 2)
	case "H":
		return strconv.Itoa(f.hours)
	case "HH":
		return pad(strconv.Itoa(f.hours), 2)
	case "M":
		return strconv.Itoa(f.minutes)
	case "MM":
		return pad(strconv.Itoa(f.minutes), 2)
	case "s":
		return strconv.Itoa(f.seconds)
	case "ss":
		return pad(strconv.Itoa(f.seconds), 2)
	case "l":
		return pad(strconv.Itoa(f.millis), 3)
	case "L":
		return pad(strconv.Itoa(f.millis/10), 2)
	case "t", "tt", "T", "TT":
		i := map[string]int{"t": 0, "tt": 2, "T": 4, "TT": 6}[token]
		if f.hours >= 12 {
			i++
		}
		return timeNames[i]
	case "Z":
		switch {
		case f.gmt:
			return "GMT"
		case f.utc:
			return "UTC"
		}
		return formatTimezone(f.ms)
	case "o":
		o := f.offset()
		return offsetSign(o) + pad(strconv.Itoa(abs(o)/60*100+abs(o)%60), 4)
	case "p":
		o := f.offset()
		return offsetSign(o) + pad(strconv.Itoa(abs(o)/60), 2) + ":" + pad(strconv.Itoa(abs(o)%60), 2)
	case "S":
		return ordinalSuffix(f.day)
	case "W":
		return js.NumberToString(f.week())
	case "WW":
		return pad(js.NumberToString(f.week()), 2)
	case "N":
		// getDayOfWeek reads date.getDay(), local even with utc.
		_, _, _, _, _, _, _, weekday := jsdate.LocalFields(f.ms)
		if weekday == 0 {
			return "7"
		}
		return strconv.Itoa(weekday)
	}
	panic("dateformat: unknown token " + token)
}

// offset is the original's o(): 0 with utc, otherwise date.getTimezoneOffset().
func (f *formatter) offset() int {
	if f.utc {
		return 0
	}
	return int(jsdate.TimezoneOffset(f.ms))
}

// offsetSign is the sign the o and p flags print: "-" for a zone behind UTC, whose
// getTimezoneOffset is positive.
func offsetSign(o int) string {
	if o > 0 {
		return "-"
	}
	return "+"
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// timezonePattern and timezoneClip are the original's timezone and timezoneClip
// patterns. JavaScript's \b and \d without the u flag are ASCII-only, as in Go.
var (
	timezonePattern = regexp.MustCompile(`\b(?:[PMCEA][SDP]T|(?:Pacific|Mountain|Central|Eastern|Atlantic) (?:Standard|Daylight|Prevailing) Time|(?:GMT|UTC)(?:[-+]\d{4})?)\b`)
	timezoneClip    = regexp.MustCompile(`[^-+\dA-Z]`)
)

// formatTimezone is the Z flag without utc or gmt: the last zone-like word of
// String(date), such as "GMT-0500" or "Eastern Standard Time", with everything but
// capitals, digits and signs removed ("EST"), and "GMT+0000" written "UTC".
func formatTimezone(ms float64) string {
	matches := timezonePattern.FindAllString(jsdate.ToString(ms), -1)
	last := ""
	if len(matches) > 0 {
		last = matches[len(matches)-1]
	}
	return strings.ReplaceAll(timezoneClip.ReplaceAllString(last, ""), "GMT+0000", "UTC")
}

func hour12(h int) int {
	if h%12 == 0 {
		return 12
	}
	return h % 12
}

// ordinalSuffix is ["th","st","nd","rd"][d%10>3 ? 0 : (d%100-d%10 != 10) * d%10].
func ordinalSuffix(d int) string {
	suffixes := [...]string{"th", "st", "nd", "rd"}
	if d%10 > 3 || d%100-d%10 == 10 {
		return suffixes[0]
	}
	return suffixes[d%10]
}

// pad is dateFormat's pad: zeros in front of the string until it has width characters.
func pad(s string, width int) string {
	if len(s) < width {
		return strings.Repeat("0", width-len(s)) + s
	}
	return s
}

func sliceFrom(s string, n int) string {
	if len(s) <= n {
		return ""
	}
	return s[n:]
}

// relativeDayName is getDayName: "Today", "Yesterday" or "Tomorrow" (or "Tdy",
// "Ysd", "Tmw" for the short form) when the date falls on that calendar day of now,
// and dayName otherwise. Yesterday and tomorrow are now with
// setDate(date[_ + "Date"]() ∓ 1): the local setter given the day the utc-or-local
// getter reads.
func (f *formatter) relativeDayName(dayName string, short bool) string {
	names := [3][2]string{{"Today", "Tdy"}, {"Yesterday", "Ysd"}, {"Tomorrow", "Tmw"}}
	pick := 0
	if short {
		pick = 1
	}
	if math.IsNaN(f.now) {
		return dayName
	}
	for i, delta := range [...]float64{0, -1, 1} {
		t := f.now
		if delta != 0 {
			_, _, d, _, _, _, _, _ := f.fields(t)
			t = setDate(t, float64(d)+delta)
		}
		if math.IsNaN(t) {
			continue
		}
		y, m, d, _, _, _, _, _ := f.fields(t)
		if y == f.year && m == f.month && d == f.day {
			return names[i][pick]
		}
	}
	return dayName
}

// week is getWeek, the ISO 8601 week number, which reads the local fields even with
// utc. It builds dates with the local Date constructor, so years 0 to 99 are read as
// 1900 to 1999 as in the original, and a date near the end of the time range whose
// Thursday is out of range gives NaN.
func (f *formatter) week() float64 {
	year, month, day, _, _, _, _, _ := jsdate.LocalFields(f.ms)
	target := jsdate.Local(float64(year), float64(month), float64(day), 0, 0, 0, 0)
	if math.IsNaN(target) {
		return math.NaN()
	}
	_, _, d, _, _, _, _, wd := jsdate.LocalFields(target)
	target = setDate(target, float64(d-(wd+6)%7+3))
	if math.IsNaN(target) {
		return math.NaN()
	}
	ty, _, _, _, _, _, _, _ := jsdate.LocalFields(target)
	first := jsdate.Local(float64(ty), 0, 4, 0, 0, 0, 0)
	if math.IsNaN(first) {
		return math.NaN()
	}
	_, _, d, _, _, _, _, wd = jsdate.LocalFields(first)
	first = setDate(first, float64(d-(wd+6)%7+3))
	if math.IsNaN(first) {
		return math.NaN()
	}
	// The original's daylight-saving correction subtracts the difference of the two
	// offsets, in minutes, from the hours.
	ds := jsdate.TimezoneOffset(target) - jsdate.TimezoneOffset(first)
	_, _, _, h, _, _, _, _ := jsdate.LocalFields(target)
	target = setHours(target, float64(h)-ds)
	return 1 + math.Floor((target-first)/(msPerDay*7))
}

// setDate is Date.prototype.setDate on time value t: the local day of the month
// replaced by date.
func setDate(t, date float64) float64 {
	local := jsdate.LocalTime(t)
	y, m, _, _, _, _, _, _ := jsdate.Fields(local)
	return jsdate.FromLocal(jsdate.MakeDate(jsdate.MakeDay(float64(y), float64(m), date), timeWithinDay(local)))
}

// setHours is Date.prototype.setHours(hours) on time value t: the local hour
// replaced, keeping the minutes, seconds and milliseconds.
func setHours(t, hours float64) float64 {
	local := jsdate.LocalTime(t)
	_, _, _, _, mi, s, ms, _ := jsdate.Fields(local)
	day := math.Floor(local / msPerDay)
	return jsdate.FromLocal(jsdate.MakeDate(day, jsdate.MakeTime(hours, float64(mi), float64(s), float64(ms))))
}

// timeWithinDay is ECMA-262 TimeWithinDay.
func timeWithinDay(t float64) float64 {
	return t - math.Floor(t/msPerDay)*msPerDay
}
