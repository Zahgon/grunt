//go:build !unix

package nodefs

import (
	"io/fs"
	"os"
)

// openDir opens a directory for reading its names.
func openDir(path string) (*os.File, error) {
	return os.Open(path)
}

// linkID reports no key: like realpathSync on Windows, every link is read.
func linkID(fs.FileInfo) (string, bool) {
	return "", false
}
