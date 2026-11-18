// Package glob is a port of glob.sync from the glob 7.1.7 npm package
// (sync.js and common.js), on POSIX paths, with its fs.realpath dependency.
//
// Results are the strings glob 7 returns, in its order: deduplicated through
// a JavaScript object (so array-index-like names come first when unsorted) and
// sorted with String#localeCompare in the "en" locale.
package glob

import (
	"errors"
	"io/fs"
	"strings"
	"syscall"

	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/minimatch"
	"github.com/gruntjs/grunt-go/internal/nodefs"
	"github.com/gruntjs/grunt-go/internal/nodepath"
)

// Options are glob 7's options. A boolean field is set when the corresponding
// JavaScript option is truthy. glob 7 has no maxDepth option; grunt's is
// ignored, as glob ignores it.
type Options struct {
	// Cwd is the directory relative patterns start from (cwd). Empty means
	// the process's working directory.
	Cwd string
	// Root is the directory a pattern starting with "/" is mounted on (root).
	// Empty means "/".
	Root     string
	Dot      bool // dot: match dotfiles
	NoMount  bool // nomount: do not mount "/" patterns on Root
	Mark     bool // mark: add "/" to directories
	NoSort   bool // nosort
	Stat     bool // stat: lstat every match
	Silent   bool // silent: do not report unusual read errors
	NoStrict bool // strict: false (strict, the default, throws them)
	NoUnique bool // nounique
	NoNull   bool // nonull: return the pattern when nothing matches
	NoDir    bool // nodir: no directories
	// Ignore patterns (ignore) are matched with dot set; a pattern ending in
	// "/**" also ignores the directory itself.
	Ignore     []string
	Follow     bool // follow: follow symlinked directories under **
	Realpath   bool // realpath: resolve matches with realpath
	Absolute   bool // absolute: return absolute paths
	MatchBase  bool // matchBase: match a slashless pattern in any directory
	NoCase     bool // nocase (on the pattern's magic segments)
	NoExt      bool // noext
	NoBrace    bool // nobrace
	NoGlobstar bool // noglobstar
	// MaxLength is maxLength: a path longer than this (in UTF-16 units) is
	// treated as missing. Zero means no limit (maxLength || Infinity).
	MaxLength int
	// OnError receives the errors glob prints with console.error('glob
	// error', er) when NoStrict is set and Silent is not. Nil drops them.
	OnError func(error)
}

// cacheKind is a value of glob's cache object: true, false, 'FILE', 'DIR' or
// an array of directory entries.
type cacheKind int

const (
	cacheMissing cacheKind = iota // false
	cacheExists                   // true: exists, type unknown
	cacheFile                     // 'FILE'
	cacheDir                      // 'DIR'
	cacheEntries                  // the entries array
)

type cacheEntry struct {
	kind    cacheKind
	entries []string
}

func (c cacheEntry) isDir() bool { return c.kind == cacheDir || c.kind == cacheEntries }

type ignoreMatcher struct {
	matcher  *minimatch.Minimatch
	gmatcher *minimatch.Minimatch // the pattern without its trailing /**, or nil
}

// globber is a GlobSync object.
type globber struct {
	opts       Options
	processCwd string
	strict     bool
	mark       bool
	maxLength  int
	cwd        string
	cwdAbs     string
	root       string
	changedCwd bool
	ignore     []ignoreMatcher
	mm         *minimatch.Minimatch

	cache     map[string]cacheEntry
	statCache map[string]fs.FileInfo // a nil value is a falsy entry
	symlinks  map[string]bool
	matches   []*keySet
}

var errNoPattern = errors.New("must provide pattern")

// Sync returns the paths matching pattern (glob.sync(pattern, options)).
//
// Errors are those glob.sync throws: "must provide pattern", "base matching
// requires globstar", a *minimatch.TypeError for a pattern minimatch rejects,
// a *CwdError when cwd is not a directory, an *FSError from realpath, and,
// with strict on (the default), the *FSError of an unexpected read failure.
//
// glob.sync sets nonegate and nocomment on the options object it is given;
// callers that reuse a JavaScript options object must do the same.
func Sync(pattern string, opts Options) ([]string, error) {
	if pattern == "" {
		return nil, errNoPattern
	}
	processCwd, err := syscall.Getwd()
	if err != nil {
		return nil, err
	}
	g, err := newGlobber(pattern, opts, processCwd)
	if err != nil {
		return nil, err
	}
	g.matches = make([]*keySet, len(g.mm.Set))
	for i, parts := range g.mm.Set {
		if err := g.process(parts, i, false); err != nil {
			return nil, err
		}
	}
	return g.finish()
}

// newGlobber is common.js setopts.
func newGlobber(pattern string, opts Options, processCwd string) (*globber, error) {
	if opts.MatchBase && !strings.Contains(pattern, "/") {
		if opts.NoGlobstar {
			return nil, errors.New("base matching requires globstar")
		}
		pattern = "**/" + pattern
	}
	g := &globber{
		opts:       opts,
		processCwd: processCwd,
		strict:     !opts.NoStrict,
		mark:       opts.Mark || opts.NoDir,
		maxLength:  opts.MaxLength,
		cache:      map[string]cacheEntry{},
		statCache:  map[string]fs.FileInfo{},
		symlinks:   map[string]bool{},
	}
	for _, p := range opts.Ignore {
		im, err := newIgnoreMatcher(p)
		if err != nil {
			return nil, err
		}
		g.ignore = append(g.ignore, im)
	}
	g.cwd = processCwd
	if opts.Cwd != "" {
		g.cwd = nodepath.Resolve(processCwd, opts.Cwd)
		g.changedCwd = g.cwd != processCwd
	}
	g.root = opts.Root
	if g.root == "" {
		g.root = nodepath.Resolve(processCwd, g.cwd, "/")
	}
	g.root = nodepath.Resolve(processCwd, g.root)
	// cwd is resolved, so it is always absolute.
	g.cwdAbs = g.cwd
	mm, err := minimatch.New(pattern, minimatch.Options{
		Dot:        opts.Dot,
		NoCase:     opts.NoCase,
		NoExt:      opts.NoExt,
		NoBrace:    opts.NoBrace,
		NoGlobstar: opts.NoGlobstar,
		MatchBase:  opts.MatchBase,
		NoNull:     opts.NoNull,
		// Comments and negation are not supported in glob itself.
		NoNegate:  true,
		NoComment: true,
	})
	if err != nil {
		return nil, err
	}
	g.mm = mm
	return g, nil
}

// newIgnoreMatcher is common.js ignoreMap: ignore patterns are always in dot
// mode, and "a/**" also ignores "a".
func newIgnoreMatcher(pattern string) (ignoreMatcher, error) {
	var im ignoreMatcher
	if strings.HasSuffix(pattern, "/**") {
		gpattern := pattern
		for strings.HasSuffix(gpattern, "/**") {
			gpattern = strings.TrimSuffix(gpattern, "/**")
		}
		gm, err := minimatch.New(gpattern, minimatch.Options{Dot: true})
		if err != nil {
			return im, err
		}
		im.gmatcher = gm
	}
	m, err := minimatch.New(pattern, minimatch.Options{Dot: true})
	if err != nil {
		return im, err
	}
	im.matcher = m
	return im, nil
}

// CwdError is the error glob throws when its cwd is not a directory.
type CwdError struct {
	Code string // ENOTDIR or ENOTSUP
	Path string // the cwd
	Err  error
}

// Error is the message glob throws: "ENOTDIR invalid cwd <path>".
func (e *CwdError) Error() string { return e.Code + " invalid cwd " + e.Path }

// Unwrap returns the file-system error behind e.
func (e *CwdError) Unwrap() error { return e.Err }

// isIgnored is common.js isIgnored.
func (g *globber) isIgnored(path string) (bool, error) {
	for _, item := range g.ignore {
		hit, err := item.matcher.Match(path)
		if err != nil || hit {
			return hit, err
		}
		if item.gmatcher != nil {
			if hit, err = item.gmatcher.Match(path); err != nil || hit {
				return hit, err
			}
		}
	}
	return false, nil
}

// childrenIgnored is common.js childrenIgnored.
func (g *globber) childrenIgnored(path string) (bool, error) {
	for _, item := range g.ignore {
		if item.gmatcher == nil {
			continue
		}
		if hit, err := item.gmatcher.Match(path); err != nil || hit {
			return hit, err
		}
	}
	return false, nil
}

// makeAbs is common.js makeAbs.
func (g *globber) makeAbs(f string) string {
	switch {
	case strings.HasPrefix(f, "/"):
		return nodepath.Join(g.root, f)
	case f == "":
		return f
	case g.changedCwd:
		return nodepath.Resolve(g.processCwd, g.cwd, f)
	}
	return nodepath.Resolve(g.processCwd, f)
}

// joinParts joins pattern segments that are all literals.
func joinParts(parts []minimatch.Part) string {
	s := make([]string, len(parts))
	for i, p := range parts {
		s[i] = p.Literal
	}
	return strings.Join(s, "/")
}

func isLiteral(p minimatch.Part) bool { return !p.Globstar && p.Regexp == nil }

// process is GlobSync.prototype._process.
func (g *globber) process(pattern []minimatch.Part, index int, inGlobStar bool) error {
	// The leading segments that are all literals.
	n := 0
	for n < len(pattern) && isLiteral(pattern[n]) {
		n++
	}
	if n == len(pattern) {
		return g.processSimple(joinParts(pattern), index)
	}
	// prefix is null (hasPrefix false) when the pattern starts with magic.
	var prefix, read string
	hasPrefix := n > 0
	if hasPrefix {
		prefix = joinParts(pattern[:n])
		// isAbsolute(prefix) || isAbsolute(pattern.join('/')) holds when
		// prefix starts with "/" or is empty (a pattern starting with "/");
		// an empty prefix then becomes "/". Either way prefix is not empty
		// from here on.
		if prefix == "" {
			prefix = "/"
		}
		read = prefix
	} else {
		read = "."
	}
	remain := pattern[n:]
	abs := g.makeAbs(read)
	// If ignored, skip processing.
	if ignored, err := g.childrenIgnored(read); err != nil || ignored {
		return err
	}
	if remain[0].Globstar {
		return g.processGlobStar(prefix, hasPrefix, read, abs, remain, index, inGlobStar)
	}
	return g.processReaddir(prefix, hasPrefix, abs, remain, index, inGlobStar)
}

// processReaddir is GlobSync.prototype._processReaddir.
func (g *globber) processReaddir(prefix string, hasPrefix bool, abs string, remain []minimatch.Part, index int, inGlobStar bool) error {
	entries, ok, err := g.readdir(abs, inGlobStar)
	if err != nil || !ok {
		return err
	}
	// It will only match dot entries if it starts with a dot, or if dot is set.
	pn := remain[0].Regexp
	rawGlob, hasGlob := pn.Glob()
	if !hasGlob {
		// The never-matching /$./ has no _glob; glob reads its charAt.
		return &minimatch.TypeError{Message: "Cannot read properties of undefined (reading 'charAt')"}
	}
	dotOk := g.opts.Dot || strings.HasPrefix(rawGlob, ".")
	// minimatch.negate is always false here: setopts turns on nonegate.
	var matched []string
	for _, e := range entries {
		if (!strings.HasPrefix(e, ".") || dotOk) && pn.MatchString(e) {
			matched = append(matched, e)
		}
	}
	if len(matched) == 0 {
		return nil
	}
	// The last segment needs no further stat, unless mark or stat asks for it.
	if len(remain) == 1 && !g.mark && !g.opts.Stat {
		if g.matches[index] == nil {
			g.matches[index] = newKeySet()
		}
		for _, e := range matched {
			if hasPrefix {
				if !strings.HasSuffix(prefix, "/") {
					e = prefix + "/" + e
				} else {
					e = prefix + e
				}
			}
			if strings.HasPrefix(e, "/") && !g.opts.NoMount {
				e = nodepath.Join(g.root, e)
			}
			if err := g.emitMatch(index, e); err != nil {
				return err
			}
		}
		return nil
	}
	// Each matched entry takes the place of that part of the pattern.
	rest := remain[1:]
	for _, e := range matched {
		var newPattern []minimatch.Part
		if hasPrefix {
			newPattern = append(newPattern, minimatch.Part{Literal: prefix})
		}
		newPattern = append(newPattern, minimatch.Part{Literal: e})
		newPattern = append(newPattern, rest...)
		if err := g.process(newPattern, index, inGlobStar); err != nil {
			return err
		}
	}
	return nil
}

// emitMatch is GlobSync.prototype._emitMatch.
func (g *globber) emitMatch(index int, e string) error {
	if ignored, err := g.isIgnored(e); err != nil || ignored {
		return err
	}
	abs := g.makeAbs(e)
	if g.mark {
		e = g.markPath(e)
	}
	if g.opts.Absolute {
		e = abs
	}
	if g.matches[index].contains(e) {
		return nil
	}
	if g.opts.NoDir {
		if c, ok := g.cache[abs]; ok && c.isDir() {
			return nil
		}
	}
	g.matches[index].add(e)
	if g.opts.Stat {
		g.stat(e)
	}
	return nil
}

// readdirInGlobStar is GlobSync.prototype._readdirInGlobStar: symlinked
// directories under ** are listed but not descended into, unless follow.
func (g *globber) readdirInGlobStar(abs string) ([]string, bool, error) {
	if g.opts.Follow {
		return g.readdir(abs, false)
	}
	fi, err := nodefs.Lstat(abs)
	if err != nil && fsCode(err) == "ENOENT" {
		return nil, false, nil
	}
	isSym := fi != nil && fi.Mode().Type() == fs.ModeSymlink
	g.symlinks[abs] = isSym
	// Neither a symlink nor a directory: a regular file, no need to readdir.
	if !isSym && fi != nil && !fi.IsDir() {
		g.cache[abs] = cacheEntry{kind: cacheFile}
		return nil, false, nil
	}
	return g.readdir(abs, false)
}

// readdir is GlobSync.prototype._readdir. It reports false where the original
// returns null.
func (g *globber) readdir(abs string, inGlobStar bool) ([]string, bool, error) {
	if _, known := g.symlinks[abs]; inGlobStar && !known {
		return g.readdirInGlobStar(abs)
	}
	if c, ok := g.cache[abs]; ok {
		switch c.kind {
		case cacheMissing, cacheFile:
			return nil, false, nil
		case cacheEntries:
			return c.entries, true, nil
		}
	}
	names, err := nodefs.Readdir(abs)
	if err != nil {
		return nil, false, g.readdirError(abs, err)
	}
	return g.readdirEntries(abs, names), true, nil
}

// readdirEntries is GlobSync.prototype._readdirEntries.
func (g *globber) readdirEntries(abs string, entries []string) []string {
	// Without mark or stat, assume everything listed exists, to avoid
	// statting it again.
	if !g.mark && !g.opts.Stat {
		for _, e := range entries {
			if abs == "/" {
				e = abs + e
			} else {
				e = abs + "/" + e
			}
			g.cache[e] = cacheEntry{kind: cacheExists}
		}
	}
	g.cache[abs] = cacheEntry{kind: cacheEntries, entries: entries}
	return entries
}

func fsCode(err error) string {
	var fe *FSError
	if errors.As(err, &fe) {
		return fe.Code
	}
	return ""
}

// readdirError is GlobSync.prototype._readdirError: record what a failed
// readdir says about f, and decide whether to throw.
func (g *globber) readdirError(f string, err error) error {
	switch code := fsCode(err); code {
	case "ENOTSUP", "ENOTDIR":
		// Totally normal: it exists but is not a directory.
		abs := g.makeAbs(f)
		g.cache[abs] = cacheEntry{kind: cacheFile}
		if abs == g.cwdAbs {
			return &CwdError{Code: code, Path: g.cwd, Err: err}
		}
	case "ENOENT", "ELOOP", "ENAMETOOLONG", "UNKNOWN":
		g.cache[g.makeAbs(f)] = cacheEntry{kind: cacheMissing}
	default:
		// Some unusual error: a failure unless strict is off.
		g.cache[g.makeAbs(f)] = cacheEntry{kind: cacheMissing}
		if g.strict {
			return err
		}
		if !g.opts.Silent && g.opts.OnError != nil {
			g.opts.OnError(err)
		}
	}
	return nil
}

// processGlobStar is GlobSync.prototype._processGlobStar.
func (g *globber) processGlobStar(prefix string, hasPrefix bool, read, abs string, remain []minimatch.Part, index int, inGlobStar bool) error {
	entries, ok, err := g.readdir(abs, inGlobStar)
	// No entries means not a dir, so it can never have matches: foo.txt/**
	// doesn't match foo.txt.
	if err != nil || !ok {
		return err
	}
	// Test without the globstar, and with every child both below and
	// replacing the globstar.
	remainWithoutGlobStar := remain[1:]
	var gspref []minimatch.Part
	if hasPrefix {
		gspref = []minimatch.Part{{Literal: prefix}}
	}
	// The noGlobStar pattern exits the inGlobStar state.
	noGlobStar := concatParts(gspref, remainWithoutGlobStar)
	if err := g.process(noGlobStar, index, false); err != nil {
		return err
	}
	// If it's a symlink, and we're in a globstar, then stop.
	if g.symlinks[abs] && inGlobStar {
		return nil
	}
	for _, e := range entries {
		if strings.HasPrefix(e, ".") && !g.opts.Dot {
			continue
		}
		// These two cases enter the inGlobStar state.
		entry := []minimatch.Part{{Literal: e}}
		if err := g.process(concatParts(gspref, entry, remainWithoutGlobStar), index, true); err != nil {
			return err
		}
		if err := g.process(concatParts(gspref, entry, remain), index, true); err != nil {
			return err
		}
	}
	return nil
}

func concatParts(parts ...[]minimatch.Part) []minimatch.Part {
	var out []minimatch.Part
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// processSimple is GlobSync.prototype._processSimple: a pattern without magic
// matches if the path exists.
func (g *globber) processSimple(prefix string, index int) error {
	exists := g.stat(prefix)
	if g.matches[index] == nil {
		g.matches[index] = newKeySet()
	}
	if exists == cacheMissing {
		return nil
	}
	if prefix != "" && prefix[0] == '/' && !g.opts.NoMount {
		prefix = nodepath.Join(g.root, prefix)
	}
	return g.emitMatch(index, prefix)
}

// stat is GlobSync.prototype._stat. It returns cacheMissing for false and
// otherwise cacheExists (true), cacheFile or cacheDir.
func (g *globber) stat(f string) cacheKind {
	abs := g.makeAbs(f)
	needDir := strings.HasSuffix(f, "/")
	if g.maxLength != 0 && jsstr.Length(f) > g.maxLength {
		return cacheMissing
	}
	if c, ok := g.cache[abs]; !g.opts.Stat && ok {
		kind := c.kind
		if kind == cacheEntries {
			kind = cacheDir
		}
		// It exists, but maybe not how we need it.
		if !needDir || kind == cacheDir {
			return kind
		}
		if kind == cacheFile {
			return cacheMissing
		}
		// Otherwise stat, because maybe it is only known to exist.
	}
	st := g.statCache[abs]
	if st == nil {
		fi, err := nodefs.Lstat(abs)
		if err != nil {
			if code := fsCode(err); code == "ENOENT" || code == "ENOTDIR" {
				g.statCache[abs] = nil
				return cacheMissing
			}
		}
		// Any other lstat error leaves st unset, which counts as existing.
		st = fi
		if fi != nil && fi.Mode().Type() == fs.ModeSymlink {
			if target, err := nodefs.Stat(abs); err == nil {
				st = target
			}
		}
	}
	g.statCache[abs] = st
	c := cacheExists
	if st != nil {
		if st.IsDir() {
			c = cacheDir
		} else {
			c = cacheFile
		}
	}
	// this.cache[abs] = this.cache[abs] || c
	if old, ok := g.cache[abs]; !ok || old.kind == cacheMissing {
		g.cache[abs] = cacheEntry{kind: c}
	}
	if needDir && c == cacheFile {
		return cacheMissing
	}
	return c
}

// markPath is common.js mark: add a trailing slash to a directory and remove
// one from anything else, as far as the cache knows.
func (g *globber) markPath(p string) string {
	abs := g.makeAbs(p)
	c, ok := g.cache[abs]
	m := p
	if ok && c.kind != cacheMissing {
		isDir := c.isDir()
		slash := strings.HasSuffix(p, "/")
		if isDir && !slash {
			m += "/"
		} else if !isDir && slash {
			m = m[:len(m)-1]
		}
		if m != p {
			mabs := g.makeAbs(m)
			g.statCache[mabs] = g.statCache[abs]
			g.cache[mabs] = c
		}
	}
	return m
}

// finish is GlobSync.prototype._finish and common.js finish.
func (g *globber) finish() ([]string, error) {
	if g.opts.Realpath {
		if err := g.resolveRealpaths(); err != nil {
			return nil, err
		}
	}
	var all []string
	unique := newKeySet()
	for i, matches := range g.matches {
		if matches == nil || matches.len() == 0 {
			if g.opts.NoNull {
				// Do like the shell, and spit out the literal glob.
				literal := g.mm.GlobSet[i]
				if g.opts.NoUnique {
					all = append(all, literal)
				} else {
					unique.add(literal)
				}
			}
			continue
		}
		if g.opts.NoUnique {
			all = append(all, matches.keys()...)
		} else {
			for _, m := range matches.keys() {
				unique.add(m)
			}
		}
	}
	if !g.opts.NoUnique {
		all = unique.keys()
	}
	if !g.opts.NoSort {
		sortLocale(all)
	}
	if g.mark {
		for i := range all {
			all[i] = g.markPath(all[i])
		}
		if g.opts.NoDir {
			kept := all[:0]
			for _, e := range all {
				notDir := !strings.HasSuffix(e, "/")
				c, ok := g.cache[e]
				if !ok || c.kind == cacheMissing {
					c, ok = g.cache[g.makeAbs(e)]
				}
				if notDir && ok && c.kind != cacheMissing {
					notDir = !c.isDir()
				}
				if notDir {
					kept = append(kept, e)
				}
			}
			all = kept
		}
	}
	if len(g.ignore) > 0 {
		kept := all[:0]
		for _, m := range all {
			ignored, err := g.isIgnored(m)
			if err != nil {
				return nil, err
			}
			if !ignored {
				kept = append(kept, m)
			}
		}
		all = kept
	}
	if all == nil {
		all = []string{}
	}
	return all, nil
}

// resolveRealpaths replaces every match with its realpath. A match whose
// realpath fails in stat (a dangling or looping link) is kept as an absolute
// path; any other failure is thrown.
func (g *globber) resolveRealpaths() error {
	for index, matchset := range g.matches {
		set := newKeySet()
		g.matches[index] = set
		if matchset == nil {
			continue
		}
		for _, p := range matchset.keys() {
			p = g.makeAbs(p)
			real, err := nodefs.Realpath(g.processCwd, p)
			if err != nil {
				var fe *FSError
				if errors.As(err, &fe) && fe.Syscall == "stat" {
					set.add(g.makeAbs(p))
					continue
				}
				return err
			}
			set.add(real)
		}
	}
	return nil
}
