// Package nodefs reproduces how Node's synchronous fs functions report the file
// system: the errors they throw (code, errno, syscall, path and the message built
// from libuv's descriptions), fs.readdirSync's view of a directory and the
// JavaScript fs.realpathSync. glob and grunt's file API share it.
package nodefs

import (
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"
	"syscall"

	"github.com/gruntjs/grunt-go/internal/nodepath"
	"github.com/gruntjs/grunt-go/js"
)

// Error is a file-system error as Node reports it: Error() is the message of
// the Error object fs.readdirSync, fs.lstatSync and friends throw, and the
// fields are its errno, code, syscall and path properties.
type Error struct {
	Errno   int    // negative errno, as libuv reports it
	Code    string // ENOENT, EACCES, ...
	Syscall string // scandir, lstat, stat or readlink
	Path    string
	// Dest is the second path of a two-path syscall (symlink, rename, chdir).
	Dest string
	// Message, when set, is the whole message: fs.rmSync words its errors its own
	// way (see RemoveAll).
	Message string
	Err     error // the underlying Go error
}

// Error is the message of the Error object Node throws.
func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	// lib/internal/errors.js uvException adds the paths only when they are set.
	msg := e.Code + ": " + uvDescriptions[e.Code] + ", " + e.Syscall
	if e.Path != "" {
		msg += " '" + e.Path + "'"
	}
	if e.Dest != "" {
		msg += " -> '" + e.Dest + "'"
	}
	return msg
}

// Unwrap returns the underlying Go error.
func (e *Error) Unwrap() error { return e.Err }

// JSError is the Error object Node throws for e: its message, then the errno, code,
// syscall, path and dest properties in the order Node's UVException sets them.
func (e *Error) JSError() *js.Error {
	props := js.ObjectOf("errno", float64(e.Errno), "code", e.Code, "syscall", e.Syscall)
	if e.Path != "" {
		props.Set("path", e.Path)
	}
	if e.Dest != "" {
		props.Set("dest", e.Dest)
	}
	return &js.Error{Name: "Error", Message: e.Error(), Orig: e, Props: props}
}

// errnoCodes names the errno values libuv reports; anything else is UNKNOWN.
var errnoCodes = map[syscall.Errno]string{
	syscall.EACCES:       "EACCES",
	syscall.EBADF:        "EBADF",
	syscall.EBUSY:        "EBUSY",
	syscall.EEXIST:       "EEXIST",
	syscall.EINVAL:       "EINVAL",
	syscall.EIO:          "EIO",
	syscall.EISDIR:       "EISDIR",
	syscall.ELOOP:        "ELOOP",
	syscall.EMFILE:       "EMFILE",
	syscall.ENAMETOOLONG: "ENAMETOOLONG",
	syscall.ENFILE:       "ENFILE",
	syscall.ENODEV:       "ENODEV",
	syscall.ENOENT:       "ENOENT",
	syscall.ENOMEM:       "ENOMEM",
	syscall.ENOSPC:       "ENOSPC",
	syscall.ENOTDIR:      "ENOTDIR",
	syscall.ENOTEMPTY:    "ENOTEMPTY",
	syscall.ENOTSUP:      "ENOTSUP",
	syscall.ENXIO:        "ENXIO",
	syscall.EPERM:        "EPERM",
}

// uvDescriptions are libuv's uv_strerror texts.
var uvDescriptions = map[string]string{
	"EACCES":       "permission denied",
	"EBADF":        "bad file descriptor",
	"EBUSY":        "resource busy or locked",
	"EEXIST":       "file already exists",
	"EINVAL":       "invalid argument",
	"EIO":          "i/o error",
	"EISDIR":       "illegal operation on a directory",
	"ELOOP":        "too many symbolic links encountered",
	"EMFILE":       "too many open files",
	"ENAMETOOLONG": "name too long",
	"ENFILE":       "file table overflow",
	"ENODEV":       "no such device",
	"ENOENT":       "no such file or directory",
	"ENOMEM":       "not enough memory",
	"ENOSPC":       "no space left on device",
	"ENOTDIR":      "not a directory",
	"ENOTEMPTY":    "directory not empty",
	"ENOTSUP":      "operation not supported on socket",
	"ENXIO":        "no such device or address",
	"EPERM":        "operation not permitted",
	"UNKNOWN":      "unknown error",
}

// NewError wraps err, returned by the named syscall on path.
func NewError(err error, syscallName, path string) *Error {
	e := &Error{Code: "UNKNOWN", Syscall: syscallName, Path: path, Err: err}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		e.Errno = -int(errno)
		if code, ok := errnoCodes[errno]; ok {
			e.Code = code
		}
	}
	return e
}

// Lstat is fs.lstatSync.
func Lstat(path string) (fs.FileInfo, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, NewError(err, "lstat", path)
	}
	return fi, nil
}

// Stat is fs.statSync.
func Stat(path string) (fs.FileInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, NewError(err, "stat", path)
	}
	return fi, nil
}

// Readdir is fs.readdirSync: the names without . and .., sorted bytewise as
// libuv's scandir sorts them (with strcmp).
func Readdir(path string) ([]string, error) {
	f, err := openDir(path)
	if err != nil {
		return nil, NewError(err, "scandir", path)
	}
	names, err := f.Readdirnames(-1)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, NewError(err, "scandir", path)
	}
	sort.Strings(names)
	return names, nil
}

// MkdirAll is fs.mkdirSync(path, {recursive: true, mode}), following Node's
// MKDirpSync: missing parents are created first, an existing directory is fine,
// and an existing non-directory fails with EEXIST, or with ENOTDIR when it is one
// of the parents.
func MkdirAll(path string, mode fs.FileMode) error {
	stack := []string{path}
	for len(stack) > 0 {
		next := stack[len(stack)-1]
		err := os.Mkdir(next, mode)
		var errno syscall.Errno
		if err != nil && !errors.As(err, &errno) {
			return NewError(err, "mkdir", path)
		}
		switch {
		case err == nil:
			stack = stack[:len(stack)-1]
		case errno == syscall.EACCES || errno == syscall.ENOSPC || errno == syscall.ENOTDIR || errno == syscall.EPERM:
			return NewError(err, "mkdir", path)
		case errno == syscall.ENOENT:
			parent := nodepath.Dirname(next)
			if parent == next {
				return NewError(err, "mkdir", path)
			}
			stack = append(stack, parent)
		default:
			stack = stack[:len(stack)-1]
			fi, serr := os.Stat(next)
			if serr != nil {
				return NewError(serr, "mkdir", path)
			}
			if !fi.IsDir() {
				if errno == syscall.EEXIST && len(stack) > 0 {
					return NewError(syscall.ENOTDIR, "mkdir", path)
				}
				return NewError(syscall.EEXIST, "mkdir", path)
			}
		}
	}
	return nil
}

// Realpath is Node's fs.realpathSync (the JavaScript implementation in
// lib/fs.js): walk the path one component at a time, lstat each, and restart
// from the target of every symbolic link. A link whose target can't be
// stat-ed fails with syscall "stat", which glob treats specially.
func Realpath(cwd, p string) (string, error) {
	p = nodepath.Resolve(cwd, p)
	seenLinks := map[string]string{}
	knownHard := map[string]bool{}
	var lastMode fs.FileMode // the mode of the most recent (l)stat, as statValues holds it
	splitRoot := func(s string) string {
		return s[:len(s)-len(strings.TrimLeft(s, "/"))]
	}
	current := splitRoot(p)
	pos := len(current)
	for pos < len(p) {
		previous := current
		var base string
		if next := strings.IndexByte(p[pos:], '/'); next == -1 {
			current += p[pos:]
			base = previous + p[pos:]
			pos = len(p)
		} else {
			current += p[pos : pos+next+1]
			base = previous + p[pos:pos+next]
			pos += next + 1
		}
		// Continue if not a symlink, break if a pipe or socket.
		if knownHard[base] {
			if lastMode.Type() == fs.ModeNamedPipe || lastMode.Type() == fs.ModeSocket {
				break
			}
			continue
		}
		fi, err := Lstat(base)
		if err != nil {
			return "", err
		}
		lastMode = fi.Mode()
		if fi.Mode().Type() != fs.ModeSymlink {
			knownHard[base] = true
			continue
		}
		id, hasID := linkID(fi)
		linkTarget, seen := seenLinks[id]
		if !hasID || !seen {
			target, err := Stat(base)
			if err != nil {
				return "", err
			}
			lastMode = target.Mode()
			if linkTarget, err = os.Readlink(base); err != nil {
				return "", NewError(err, "readlink", base)
			}
		}
		resolvedLink := nodepath.Resolve(cwd, previous, linkTarget)
		if hasID {
			seenLinks[id] = linkTarget
		}
		p = nodepath.Resolve(cwd, resolvedLink, p[pos:])
		current = splitRoot(p)
		pos = len(current)
	}
	return p, nil
}

// RemoveAll is fs.rmSync(path, {recursive: true, force: true}) as Node 24 implements
// it in C++ with std::filesystem::remove_all: a missing path is fine; otherwise the
// tree is removed depth first and the first failure stops it, so removing "."
// empties the directory before rmdir(".") fails with EINVAL. The error's message
// is the one RmSync builds: "<code>, <description> '<path>'", where an unexpected
// error has no code and the description "Unknown error: <strerror>".
func RemoveAll(path string) error {
	err := removeAll(path)
	var errno syscall.Errno
	if err == nil || errors.As(err, &errno) && errno == syscall.ENOENT {
		return nil
	}
	code, desc := "", "Unknown error: "+strerror(err)
	switch errno {
	case syscall.EPERM:
		code, desc = "EPERM", "Operation not permitted: "+path
	case syscall.ENOTEMPTY:
		code, desc = "EACCES", "Directory not empty: "+path
	case syscall.ENOTDIR:
		code, desc = "ENOTDIR", "Not a directory: "+path
	case syscall.EACCES:
		code, desc = "EACCES", "Permission denied: "+path
	}
	e := NewError(err, "rm", path)
	e.Message = code + ", " + desc + " '" + path + "'"
	return e
}

// removeAll is std::filesystem::remove_all without following symbolic links.
func removeAll(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return unwrapErrno(err)
	}
	if !fi.IsDir() {
		return unwrapErrno(syscall.Unlink(path))
	}
	f, err := openDir(path)
	if err != nil {
		return unwrapErrno(err)
	}
	names, err := f.Readdirnames(-1)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return unwrapErrno(err)
	}
	for _, name := range names {
		if err := removeAll(path + "/" + name); err != nil {
			return err
		}
	}
	return unwrapErrno(syscall.Rmdir(path))
}

func unwrapErrno(err error) error {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno
	}
	return err
}

// strerror is the C library's description of err ("Invalid argument"), which is
// Go's errno text with a capital first letter.
func strerror(err error) string {
	s := err.Error()
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
