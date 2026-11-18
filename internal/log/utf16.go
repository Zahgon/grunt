package log

import (
	"strings"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
)

// The original measures and slices strings in UTF-16 code units (String.length,
// regular expressions without the u flag). These helpers give Go strings the same view.

// padStart and padEnd are String.prototype.padStart/padEnd with a space filler and a
// target length in code units.
func padStart(s string, target int) string {
	if n := jsstr.Length(s); n < target {
		return strings.Repeat(" ", target-n) + s
	}
	return s
}

func padEnd(s string, target int) string {
	if n := jsstr.Length(s); n < target {
		return s + strings.Repeat(" ", target-n)
	}
	return s
}
