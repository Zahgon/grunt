package yaml

import (
	"time"
)

func init() {
	// The Node fixtures were recorded with TZ=UTC; local time must match.
	time.Local = time.UTC
}
