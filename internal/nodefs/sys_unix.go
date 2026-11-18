//go:build unix

package nodefs

import (
	"io/fs"
	"os"
	"strconv"
	"syscall"
)

// openDir opens a directory the way libuv's scandir does, with O_DIRECTORY:
// a file fails with ENOTDIR at once, and a FIFO is not opened for reading
// (which would block).
func openDir(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

// linkID is the dev:ino key realpathSync remembers link targets by.
func linkID(fi fs.FileInfo) (string, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false
	}
	return strconv.FormatUint(uint64(st.Dev), 32) + ":" + strconv.FormatUint(uint64(st.Ino), 32), true
}
