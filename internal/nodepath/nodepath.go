// Package nodepath ports the functions of Node's path.posix module that glob and
// grunt use. Go's path.Clean differs from them (it drops trailing slashes, and
// resolve and relative have no Go counterpart), and the result strings grunt prints
// depend on the details.
package nodepath

import "strings"

// normalizeString is Node's normalizeString for "/": it resolves . and ..
// segments and collapses slashes, without leading or trailing slashes.
func normalizeString(path string, allowAboveRoot bool) string {
	res := ""
	lastSegmentLength := 0
	lastSlash := -1
	dots := 0
	var code byte
	for i := 0; i <= len(path); i++ {
		if i < len(path) {
			code = path[i]
		} else if code == '/' {
			break
		} else {
			code = '/'
		}
		switch {
		case code == '/':
			switch {
			case lastSlash == i-1 || dots == 1:
				// NOOP
			case dots == 2:
				if len(res) < 2 || lastSegmentLength != 2 || !strings.HasSuffix(res, "..") {
					if len(res) > 2 {
						if idx := strings.LastIndexByte(res, '/'); idx == -1 {
							res = ""
							lastSegmentLength = 0
						} else {
							res = res[:idx]
							lastSegmentLength = len(res) - 1 - strings.LastIndexByte(res, '/')
						}
						lastSlash = i
						dots = 0
						continue
					} else if len(res) != 0 {
						res = ""
						lastSegmentLength = 0
						lastSlash = i
						dots = 0
						continue
					}
				}
				if allowAboveRoot {
					if len(res) > 0 {
						res += "/.."
					} else {
						res = ".."
					}
					lastSegmentLength = 2
				}
			default:
				if len(res) > 0 {
					res += "/" + path[lastSlash+1:i]
				} else {
					res = path[lastSlash+1 : i]
				}
				lastSegmentLength = i - lastSlash - 1
			}
			lastSlash = i
			dots = 0
		case code == '.' && dots != -1:
			dots++
		default:
			dots = -1
		}
	}
	return res
}

// Normalize is path.posix.normalize.
func Normalize(path string) string {
	if path == "" {
		return "."
	}
	isAbsolute := path[0] == '/'
	trailingSeparator := path[len(path)-1] == '/'
	path = normalizeString(path, !isAbsolute)
	if path == "" {
		if isAbsolute {
			return "/"
		}
		if trailingSeparator {
			return "./"
		}
		return "."
	}
	if trailingSeparator {
		path += "/"
	}
	if isAbsolute {
		return "/" + path
	}
	return path
}

// Join is path.posix.join.
func Join(parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	if len(nonEmpty) == 0 {
		return "."
	}
	return Normalize(strings.Join(nonEmpty, "/"))
}

// Resolve is path.posix.resolve, with cwd standing for process.cwd().
func Resolve(cwd string, parts ...string) string {
	resolved := ""
	absolute := false
	for i := len(parts) - 1; i >= -1 && !absolute; i-- {
		p := cwd
		if i >= 0 {
			p = parts[i]
		}
		if p == "" {
			continue
		}
		resolved = p + "/" + resolved
		absolute = p[0] == '/'
	}
	resolved = normalizeString(resolved, !absolute)
	if absolute {
		return "/" + resolved
	}
	if resolved == "" {
		return "."
	}
	return resolved
}

// IsAbsolute is path.posix.isAbsolute.
func IsAbsolute(path string) bool {
	return strings.HasPrefix(path, "/")
}

// Relative is path.posix.relative, with cwd standing for process.cwd().
func Relative(cwd, from, to string) string {
	if from == to {
		return ""
	}
	from = Resolve(cwd, from)
	to = Resolve(cwd, to)
	if from == to {
		return ""
	}
	// Both are absolute; compare them without the leading slash.
	fromStart, toStart := 1, 1
	fromLen, toLen := len(from)-fromStart, len(to)-toStart
	length := min(fromLen, toLen)
	lastCommonSep := -1
	i := 0
	for ; i < length; i++ {
		c := from[fromStart+i]
		if c != to[toStart+i] {
			break
		}
		if c == '/' {
			lastCommonSep = i
		}
	}
	if i == length {
		switch {
		case toLen > length:
			if to[toStart+i] == '/' {
				// from is the exact base path of to ("/foo/bar" and "/foo/bar/baz").
				return to[toStart+i+1:]
			}
			if i == 0 {
				// from is the root.
				return to[toStart+i:]
			}
		case fromLen > length:
			if from[fromStart+i] == '/' {
				// to is the exact base path of from.
				lastCommonSep = i
			} else if i == 0 {
				// to is the root.
				lastCommonSep = 0
			}
		}
	}
	out := ""
	// One ".." for every segment of from after the common part.
	for i = fromStart + lastCommonSep + 1; i <= len(from); i++ {
		if i == len(from) || from[i] == '/' {
			if out == "" {
				out = ".."
			} else {
				out += "/.."
			}
		}
	}
	return out + to[toStart+lastCommonSep:]
}

// Dirname is path.posix.dirname.
func Dirname(path string) string {
	if path == "" {
		return "."
	}
	hasRoot := path[0] == '/'
	end := -1
	matchedSlash := true
	for i := len(path) - 1; i >= 1; i-- {
		if path[i] == '/' {
			if !matchedSlash {
				end = i
				break
			}
		} else {
			matchedSlash = false
		}
	}
	if end == -1 {
		if hasRoot {
			return "/"
		}
		return "."
	}
	if hasRoot && end == 1 {
		return "//"
	}
	return path[:end]
}

// Basename is path.posix.basename without the suffix argument.
func Basename(path string) string {
	start, end := 0, -1
	matchedSlash := true
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			if !matchedSlash {
				start = i + 1
				break
			}
		} else if end == -1 {
			matchedSlash = false
			end = i + 1
		}
	}
	if end == -1 {
		return ""
	}
	return path[start:end]
}
