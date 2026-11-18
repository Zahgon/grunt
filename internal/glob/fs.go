package glob

import (
	"github.com/gruntjs/grunt-go/internal/nodefs"
)

// FSError is a file-system error as Node reports it (see nodefs.Error).
type FSError = nodefs.Error
