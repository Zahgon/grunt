package jsdate

import (
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

// Local time follows the host's time zone, as in Node: Go's time.Local reads the TZ
// environment variable (or /etc/localtime) the way Node's ICU does, and the offsets
// below are those V8 asks ICU for.

const (
	// maxTimeBeforeUTC is V8's kMaxTimeBeforeUTCInMs: a local time value further from
	// the epoch than this is NaN before it is converted to UTC.
	maxTimeBeforeUTC = maxTime + 864000000
	// maxEpochTime is V8's kMaxEpochTimeInMs, the last instant (in 2038) whose
	// time-zone name V8 looks up directly; others use an equivalent year.
	maxEpochTime = math.MaxInt32 * msPerSecond
)

// offsetAt is the offset of local time from UTC, in milliseconds, at the instant t.
func offsetAt(t int64) int64 {
	_, sec := time.UnixMilli(t).In(time.Local).Zone()
	return int64(sec) * 1000
}

// LocalOffset is V8's LocalOffsetInMs(t, is_utc = true): the offset of local time
// from UTC, in milliseconds, at the valid time value t.
func LocalOffset(t float64) float64 { return float64(offsetAt(int64(t))) }

// LocalTime is ECMA-262 LocalTime(t): the valid time value t shifted into local
// time. Its calendar fields are what the local getters (getHours, getDate, ...)
// return.
func LocalTime(t float64) float64 { return t + LocalOffset(t) }

// LocalFields is Fields of LocalTime(t): the local calendar fields of a valid time
// value.
func LocalFields(t float64) (year, month, day, hours, minutes, seconds, millis, weekday int) {
	return Fields(LocalTime(t))
}

// FromLocal converts a local time value, such as MakeDate of the arguments of
// new Date(year, month, ...), to a time value: NaN beyond V8's range for local
// times, otherwise V8's DateCache::ToUTC followed by TimeClip.
//
// ToUTC asks ICU for the offset with UCAL_TZ_LOCAL_FORMER for both skipped and
// repeated wall-clock times, so a time in a spring-forward gap or a fall-back overlap
// is read with the offset in force before the transition, as ECMA-262 requires.
func FromLocal(local float64) float64 {
	if math.IsNaN(local) || math.Abs(local) > maxTimeBeforeUTC {
		return math.NaN()
	}
	l := int64(local)
	return TimeClip(float64(l - offsetFromLocal(l)))
}

// offsetFromLocal is the offset that turns local time l into UTC. Offsets stay well
// within a day of UTC, so the offsets a day before and after l bracket any
// transition near it: the earlier one applies unless it does not reproduce l while
// the later one does.
func offsetFromLocal(l int64) int64 {
	const day = int64(msPerDay)
	before, after := offsetAt(l-day), offsetAt(l+day)
	if before == after || offsetAt(l-before) == before || offsetAt(l-after) != after {
		return before
	}
	return after
}

// TimezoneOffset is Date.prototype.getTimezoneOffset for the valid time value t:
// UTC minus local time in whole minutes, truncated toward zero as V8's integer
// division does (a local mean time offset of -4:56:02 gives 296).
func TimezoneOffset(t float64) float64 {
	return float64(-offsetAt(int64(t)) / int64(msPerMinute))
}

// gmtOffset formats the time-zone part of Date.prototype.toString: "GMT" followed by
// the sign and the hours and minutes of -TimezoneOffset(t), such as "GMT-0500".
func gmtOffset(t float64) string {
	offset := -int(TimezoneOffset(t))
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	return "GMT" + sign + pad(offset/60, 2) + pad(offset%60, 2)
}

// zoneName is the name V8 prints in parentheses after the offset in
// Date.prototype.toString: ICU's long name of the local zone, for standard or
// daylight-saving time. As in V8's DateCache::LocalTimezone, an instant before 1970
// or after 2038 is first moved to the equivalent year between 2008 and 2037, so the
// choice between the two names follows that year's rules.
//
// The names come from zoneNames, recorded from Node's ICU. A zone missing from it
// is named with ICU's fallback, the localized GMT format of its offset ("GMT+05:30");
// that is also what Node prints when TZ is set but empty.
func zoneName(t float64) string {
	if t < 0 || t > maxEpochTime {
		t = equivalentTime(t)
	}
	local := time.UnixMilli(int64(t)).In(time.Local)
	if names, ok := zoneNames[localZoneID()]; ok {
		if daylightSaving(local) {
			return zoneNameList[names.dst]
		}
		return zoneNameList[names.std]
	}
	_, offset := local.Zone()
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	name := "GMT" + sign + pad(offset/3600, 2) + ":" + pad(offset/60%60, 2)
	if offset%60 != 0 {
		name += ":" + pad(offset%60, 2)
	}
	return name
}

// daylightSaving reports whether local is in daylight-saving time as ICU sees it.
// ICU's zone data has only positive daylight saving, while the system's tzdata may
// mark Europe/Dublin's winter time as a negative daylight saving: a change of the
// flag that goes against the change of offset from the previous period is read the
// other way round.
func daylightSaving(local time.Time) bool {
	dst := local.IsDST()
	start, _ := local.ZoneBounds()
	if start.IsZero() {
		return dst
	}
	prev := start.Add(-time.Millisecond)
	_, offset := local.Zone()
	_, prevOffset := prev.Zone()
	if prev.IsDST() != dst && offset != prevOffset && (offset > prevOffset) != dst {
		return !dst
	}
	return dst
}

// zoneNamePair is an entry of zoneNames: the indexes in zoneNameList of a zone's
// standard and daylight-saving names. A zone without daylight-saving time has the
// same name twice.
type zoneNamePair struct {
	std, dst int
}

// localZoneID is the IANA name of the local time zone: the name TZ gave Go's
// time.Local, or the zone /etc/localtime links to. A TZ that is set but empty, which
// ICU treats as an unknown zone, gives "".
func localZoneID() string {
	id := time.Local.String()
	switch {
	case id == "Local":
		id = systemZoneID()
	case id == "UTC" && time.Local != time.UTC:
		// Go also calls the zone UTC when TZ is set but empty.
		if tz, ok := os.LookupEnv("TZ"); ok && strings.TrimPrefix(tz, ":") == "" {
			return ""
		}
	}
	if i := strings.LastIndex(id, "zoneinfo/"); i >= 0 {
		id = id[i+len("zoneinfo/"):]
	}
	return id
}

// systemZoneID is the target of the /etc/localtime link, read once.
var systemZoneID = sync.OnceValue(func() string {
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		// Not a link (or absent): ICU cannot name the zone either and falls back to
		// the GMT format.
		return ""
	}
	return target
})

// equivalentTime is V8's DateCache::EquivalentTime: t moved to the same month, day
// and time of day in a year between 2008 and 2037 that is a leap year when t's year
// is and starts on the same weekday.
func equivalentTime(t float64) float64 {
	days := floorDiv(int64(t), msPerDay)
	within := int64(t) - days*int64(msPerDay)
	year, month, day, _, _, _, _, _ := Fields(float64(days) * msPerDay)
	newDays := MakeDay(float64(equivalentYear(year)), float64(month), float64(day))
	return newDays*msPerDay + float64(within)
}

// equivalentYear is V8's DateCache::EquivalentYear.
func equivalentYear(year int) int {
	weekday := int(((int64(MakeDay(float64(year), 0, 1))+4)%7 + 7) % 7)
	recent := 1967
	if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
		recent = 1956
	}
	recent += weekday * 12 % 28
	return 2008 + (recent+3*28-2008)%28
}
