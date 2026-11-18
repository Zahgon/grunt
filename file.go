package grunt

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"syscall"

	"github.com/gruntjs/grunt-go/internal/glob"
	"github.com/gruntjs/grunt-go/internal/iconv"
	"github.com/gruntjs/grunt-go/internal/jsregexp"
	"github.com/gruntjs/grunt-go/internal/jsrt"
	"github.com/gruntjs/grunt-go/internal/jsrt/jsstr"
	"github.com/gruntjs/grunt-go/internal/lodash"
	"github.com/gruntjs/grunt-go/internal/log"
	"github.com/gruntjs/grunt-go/internal/minimatch"
	"github.com/gruntjs/grunt-go/internal/nodefs"
	"github.com/gruntjs/grunt-go/internal/nodepath"
	"github.com/gruntjs/grunt-go/internal/yaml"
	"github.com/gruntjs/grunt-go/js"
)

// File is grunt.file: globbing, reading, writing and path helpers. Paths are
// relative to the working directory, which grunt sets to the Gruntfile's directory
// (or --base).
type File struct {
	// DefaultEncoding is grunt.file.defaultEncoding, used by Read and Write when no
	// encoding option is given ("utf8").
	DefaultEncoding string
	// PreserveBOM is grunt.file.preserveBOM: Read keeps a leading byte order mark.
	PreserveBOM bool

	g *Grunt
}

// cwd is process.cwd(), which throws when the working directory is gone.
func cwd() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(nodefs.NewError(err, "uv_cwd", ""))
	}
	return dir
}

// errorCode is e.code of a Node fs error, "undefined" for other errors.
func errorCode(err error) string {
	var fe *nodefs.Error
	if errors.As(err, &fe) {
		return fe.Code
	}
	return "undefined"
}

// optionsObject is the options argument at index i, or an empty object.
func optionsObject(options []any, i int) *js.Object {
	if i < len(options) {
		if o, ok := js.Normalize(options[i]).(*js.Object); ok {
			return o
		}
	}
	return js.NewObject()
}

// SetBase is grunt.file.setBase: it changes the working directory to the joined
// path.
func (f *File) SetBase(parts ...string) error {
	dir := nodepath.Join(parts...)
	if err := os.Chdir(dir); err != nil {
		e := nodefs.NewError(err, "chdir", cwd())
		e.Dest = dir
		return e
	}
	return nil
}

// processPatterns applies fn to each pattern of the flattened list in turn, adding
// its matches to the result, or removing them for a pattern that starts with "!".
func processPatterns(patterns any, fn func(pattern string) []string) []string {
	result := js.NewArray()
	for _, p := range lodash.FlattenDeep(patterns).Items {
		pattern, ok := p.(string)
		if !ok {
			if js.IsNullish(p) {
				panic(js.NewTypeError("Cannot read properties of " + js.ToString(p) + " (reading 'indexOf')"))
			}
			panic(js.NewTypeError("pattern.indexOf is not a function"))
		}
		exclusion := strings.HasPrefix(pattern, "!")
		if exclusion {
			pattern = pattern[1:]
		}
		matches := stringsArray(fn(pattern))
		if exclusion {
			result = lodash.Difference(result, matches)
		} else {
			result = lodash.Union(result, matches)
		}
	}
	out := make([]string, len(result.Items))
	for i, s := range result.Items {
		out[i] = s.(string)
	}
	return out
}

// Match is grunt.file.match([options,] patterns, filepaths): the file paths that
// match any of the minimatch patterns (patterns starting with "!" exclude). A
// pattern minimatch rejects panics with its TypeError.
func (f *File) Match(args ...any) []string {
	var options, patterns, filepaths any = js.Undefined, js.Undefined, js.Undefined
	for i, a := range args {
		switch i {
		case 0:
			options = js.Normalize(a)
		case 1:
			patterns = js.Normalize(a)
		case 2:
			filepaths = js.Normalize(a)
		}
	}
	if kindOf(options) != "object" {
		options, patterns, filepaths = js.NewObject(), options, patterns
	}
	if js.IsNullish(patterns) || js.IsNullish(filepaths) {
		return []string{}
	}
	pats, ok := patterns.(*js.Array)
	if !ok {
		pats = js.NewArray(patterns)
	}
	paths, ok := filepaths.(*js.Array)
	if !ok {
		paths = js.NewArray(filepaths)
	}
	if len(pats.Items) == 0 || len(paths.Items) == 0 {
		return []string{}
	}
	list := make([]string, len(paths.Items))
	for i, p := range paths.Items {
		list[i] = js.ToString(p)
	}
	opts := minimatchOptions(options)
	return processPatterns(pats, func(pattern string) []string {
		m, err := minimatch.MatchList(list, pattern, opts)
		if err != nil {
			panic(err)
		}
		return m
	})
}

// IsMatch is grunt.file.isMatch: whether Match finds anything.
func (f *File) IsMatch(args ...any) bool { return len(f.Match(args...)) > 0 }

func minimatchOptions(options any) minimatch.Options {
	o, _ := options.(*js.Object)
	flag := func(name string) bool { return o != nil && js.ToBoolean(o.Get(name)) }
	return minimatch.Options{
		Dot: flag("dot"), NoCase: flag("nocase"), NoComment: flag("nocomment"), NoNegate: flag("nonegate"),
		FlipNegate: flag("flipNegate"), NoExt: flag("noext"), NoBrace: flag("nobrace"),
		NoGlobstar: flag("noglobstar"), MatchBase: flag("matchBase"), NoNull: flag("nonull"),
	}
}

// printGlobError is how glob reports an unusual read error when strict is off and
// silent is not set: console.error('glob error', er).
func (f *File) printGlobError(err error) {
	var value any = &js.Error{Name: "Error", Message: err.Error(), Orig: err}
	var fe *nodefs.Error
	if errors.As(err, &fe) {
		value = fe.JSError()
	}
	consoleWrite(f.g.proc.stderr, log.Format("glob error", value)+"\n")
}

func globOptions(o *js.Object) glob.Options {
	flag := func(name string) bool { return js.ToBoolean(o.Get(name)) }
	str := func(name string) string {
		if v := o.Get(name); js.ToBoolean(v) {
			return js.ToString(v)
		}
		return ""
	}
	opts := glob.Options{
		Cwd: str("cwd"), Root: str("root"), Dot: flag("dot"), NoMount: flag("nomount"), Mark: flag("mark"),
		NoSort: flag("nosort"), Stat: flag("stat"), Silent: flag("silent"), NoStrict: o.Get("strict") == false,
		NoUnique: flag("nounique"), NoNull: flag("nonull"), NoDir: flag("nodir"), Follow: flag("follow"),
		Realpath: flag("realpath"), Absolute: flag("absolute"), MatchBase: flag("matchBase"),
		NoCase: flag("nocase"), NoExt: flag("noext"), NoBrace: flag("nobrace"), NoGlobstar: flag("noglobstar"),
	}
	switch ig := o.Get("ignore").(type) {
	case string:
		opts.Ignore = []string{ig}
	case *js.Array:
		for _, p := range ig.Items {
			opts.Ignore = append(opts.Ignore, js.ToString(p))
		}
	}
	if n := js.ToNumber(o.Get("maxLength")); n > 0 {
		opts.MaxLength = int(min(n, 1<<53))
	}
	return opts
}

// Expand is grunt.file.expand([options,] patterns...): the paths matching the glob
// patterns, in order, without duplicates; patterns starting with "!" remove
// matches. options are glob's (cwd, dot, nonull, ...) plus filter: the name of an
// fs.Stats method ("isFile", "isDirectory") or a function of the path. A glob
// error panics with the error glob.sync throws.
func (f *File) Expand(args ...any) []string {
	for i, a := range args {
		args[i] = js.Normalize(a)
	}
	options := js.NewObject()
	if len(args) > 0 {
		if o, ok := args[0].(*js.Object); ok {
			options, args = o, args[1:]
		}
	}
	var patterns any = js.NewArray(args...)
	if len(args) > 0 {
		if a, ok := args[0].(*js.Array); ok {
			patterns = a
		}
	}
	if len(patterns.(*js.Array).Items) == 0 {
		return []string{}
	}
	matches := processPatterns(patterns, func(pattern string) []string {
		opts := globOptions(options)
		opts.OnError = f.printGlobError
		m, err := glob.Sync(pattern, opts)
		// glob.sync disables minimatch's comments and negation on the options
		// object it is given, which is the caller's here.
		options.Set("nonegate", true)
		options.Set("nocomment", true)
		if err != nil {
			panic(err)
		}
		return m
	})
	filter := options.Get("filter")
	if !js.ToBoolean(filter) {
		return matches
	}
	base := ""
	if c := options.Get("cwd"); js.ToBoolean(c) {
		base = js.ToString(c)
	}
	kept := []string{}
	for _, m := range matches {
		if statFilter(filter, nodepath.Join(base, m)) {
			kept = append(kept, m)
		}
	}
	return kept
}

// statFilter is expand's filter: a function of the path, or the name of the
// fs.Stats method to call. Any error means false, as the original's
// try { ... } catch (e) { return false; } around the call.
func statFilter(filter any, path string) bool {
	if fn, ok := filter.(*js.Function); ok {
		v, err := fn.Call(js.Undefined, []any{path})
		return err == nil && js.ToBoolean(v)
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	mode := fi.Mode()
	switch js.ToString(filter) {
	case "isFile":
		return mode.IsRegular()
	case "isDirectory":
		return mode.IsDir()
	case "isBlockDevice":
		return mode&fs.ModeDevice != 0 && mode&fs.ModeCharDevice == 0
	case "isCharacterDevice":
		return mode&fs.ModeCharDevice != 0
	case "isFIFO":
		return mode&fs.ModeNamedPipe != 0
	case "isSocket":
		return mode&fs.ModeSocket != 0
	}
	// isSymbolicLink is false after stat; anything else is not a method.
	return false
}

// ExpandMapping is grunt.file.expandMapping(patterns, destBase, options): one
// {src, dest} object per destination, where each matched path is mapped to
// destBase joined with the path (its basename with flatten, its extension replaced
// with ext, renamed with the rename function), and src paths get cwd prepended.
func (f *File) ExpandMapping(patterns any, destBase any, options ...any) []*js.Object {
	var userOptions any = js.Undefined
	if len(options) > 0 {
		userOptions = js.Normalize(options[0])
	}
	opts := lodash.Defaults(js.NewObject(), userOptions, js.ObjectOf(
		"extDot", "first",
		"rename", &js.Function{Name: "rename", Length: 2, Call: defaultRename},
	))
	destBase = js.Normalize(destBase)
	files := []*js.Object{}
	byDest := map[string]*js.Object{}
	for _, src := range f.Expand(opts, patterns) {
		destPath := src
		if js.ToBoolean(opts.Get("flatten")) {
			destPath = nodepath.Basename(destPath)
		}
		if opts.Has("ext") {
			destPath = replaceExt(destPath, js.ToString(opts.Get("extDot")), js.ToString(opts.Get("ext")))
		}
		rename, ok := opts.Get("rename").(*js.Function)
		if !ok {
			panic(js.NewTypeError("options.rename is not a function"))
		}
		d, err := rename.Call(js.Undefined, []any{destBase, destPath, opts})
		if err != nil {
			panic(err)
		}
		dest := js.ToString(d)
		if c := opts.Get("cwd"); js.ToBoolean(c) {
			src = nodepath.Join(js.ToString(c), src)
		}
		dest = strings.ReplaceAll(dest, `\`, "/")
		src = strings.ReplaceAll(src, `\`, "/")
		if m, ok := byDest[dest]; ok {
			a := m.Get("src").(*js.Array)
			a.Items = append(a.Items, src)
		} else {
			m := js.ObjectOf("src", js.NewArray(src), "dest", dest)
			files = append(files, m)
			byDest[dest] = m
		}
	}
	return files
}

// defaultRename is expandMapping's default rename, path.join(destBase || "", destPath).
func defaultRename(_ any, args []any) (any, error) {
	base, destPath := "", ""
	if len(args) > 0 && js.ToBoolean(args[0]) {
		base = js.ToString(args[0])
	}
	if len(args) > 1 {
		destPath = js.ToString(args[1])
	}
	return nodepath.Join(base, destPath), nil
}

// replaceExt is destPath.replace(extDotRe[extDot], ext): with extDot "first" the
// extension starts at the first dot of the file name, with "last" at the last
// dot; a name without one gets ext appended. Any other extDot has no regular
// expression, so the replace looks for the text "undefined".
func replaceExt(destPath, extDot, ext string) string {
	nameStart := strings.LastIndexByte(destPath, '/') + 1
	var at int
	switch extDot {
	case "first":
		at = strings.IndexByte(destPath[nameStart:], '.')
	case "last":
		at = strings.LastIndexByte(destPath[nameStart:], '.')
	default:
		i := strings.Index(destPath, "undefined")
		if i < 0 {
			return destPath
		}
		return destPath[:i] + substitute(ext, destPath, i, i+len("undefined"), false) + destPath[i+len("undefined"):]
	}
	if at < 0 {
		at = len(destPath)
	} else {
		at += nameStart
	}
	return destPath[:at] + substitute(ext, destPath, at, len(destPath), true)
}

// substitute is GetSubstitution for the replacement repl of the match s[start:end]
// (byte offsets). With group, the match comes from extDotRe, whose one capture
// group is the whole extension and does not participate in an empty match; without
// it, the pattern was a string and has no capture groups.
func substitute(repl, s string, start, end int, group bool) string {
	if !strings.Contains(repl, "$") {
		return repl
	}
	us := jsstr.Length(s[:start])
	ue := us + jsstr.Length(s[start:end])
	caps := []int{us, ue}
	if group {
		if ue > us {
			caps = append(caps, us, ue)
		} else {
			caps = append(caps, -1, -1)
		}
	}
	return jsstr.FromUTF16(jsregexp.Substitute(jsstr.ToUTF16(repl), jsstr.ToUTF16(s), caps, nil))
}

// Mkdir is grunt.file.mkdir: like mkdir -p, with an optional mode (0777 &^ umask by
// default). Nothing happens with --no-write.
func (f *File) Mkdir(dir string, mode ...any) error {
	if f.g.option.truthy("no-write") {
		return nil
	}
	perm := fs.FileMode(0o777)
	if len(mode) > 0 && !js.IsNullish(mode[0]) {
		perm = fileMode(js.Normalize(mode[0]))
	}
	if err := nodefs.MkdirAll(dir, perm); err != nil {
		return utilError(`Unable to create directory "`+dir+`" (Error code: `+errorCode(err)+`).`, err)
	}
	return nil
}

// fileMode is the permission bits of a numeric mode option.
func fileMode(v any) fs.FileMode {
	n := js.ToNumber(v)
	if n != n || n < 0 || n > 0o7777 {
		return 0o666
	}
	return fs.FileMode(uint32(n)) & fs.ModePerm
}

// Recurse is grunt.file.recurse: it calls cb for every file below root, in
// directory order, with the file's path, root, its directory relative to root
// (undefined at the top level) and its name.
func (f *File) Recurse(root string, cb func(abspath, rootdir string, subdir any, filename string)) error {
	return recurseDir(root, cb, js.Undefined)
}

func recurseDir(root string, cb func(abspath, rootdir string, subdir any, filename string), subdir any) error {
	abspath := root
	if js.ToBoolean(subdir) {
		abspath = nodepath.Join(root, subdir.(string))
	}
	names, err := nodefs.Readdir(abspath)
	if err != nil {
		return err
	}
	for _, name := range names {
		path := nodepath.Join(abspath, name)
		fi, err := nodefs.Stat(path)
		if err != nil {
			return err
		}
		if fi.IsDir() {
			sub := ""
			if js.ToBoolean(subdir) {
				sub = subdir.(string)
			}
			if err := recurseDir(root, cb, nodepath.Join(sub, name)); err != nil {
				return err
			}
		} else {
			cb(path, root, subdir, name)
		}
	}
	return nil
}

// Read is grunt.file.read(path, options): the file decoded with options.encoding
// (DefaultEncoding by default; a leading BOM is stripped unless PreserveBOM), or
// its raw bytes ([]byte) with {encoding: null}.
func (f *File) Read(path string, options ...any) (any, error) {
	return f.read(path, optionsObject(options, 0))
}

func (f *File) read(path string, options *js.Object) (any, error) {
	v := f.g.Verbose
	v.Write("Reading " + path + "...")
	contents, err := f.readContents(path, options)
	if err != nil {
		v.Error()
		return nil, utilError(`Unable to read "`+path+`" file (Error code: `+errorCode(err)+`).`, err)
	}
	v.Ok()
	return contents, nil
}

func (f *File) readContents(path string, options *js.Object) (any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) && pe.Op == "read" {
			return nil, nodefs.NewError(err, "read", path)
		}
		return nil, nodefs.NewError(err, "open", path)
	}
	enc := options.Get("encoding")
	if enc == nil {
		return b, nil
	}
	name := f.DefaultEncoding
	if js.ToBoolean(enc) {
		name = js.ToString(enc)
	}
	return iconv.Decode(b, name, !f.PreserveBOM)
}

// ReadJSON is grunt.file.readJSON: Read, then JSON.parse.
func (f *File) ReadJSON(path string, options ...any) (any, error) {
	src, err := f.Read(path, options...)
	if err != nil {
		return nil, err
	}
	return f.parse(path, func() (any, error) { return jsrt.JSONParse(js.ToString(src)) })
}

// ReadYAML is grunt.file.readYAML(path, options, yamlOptions): Read, then
// js-yaml's safeLoad, or load with yamlOptions {unsafeLoad: true}.
func (f *File) ReadYAML(path string, options ...any) (any, error) {
	src, err := f.Read(path, options...)
	if err != nil {
		return nil, err
	}
	yamlOptions := optionsObject(options, 1)
	return f.parse(path, func() (any, error) {
		if js.ToBoolean(yamlOptions.Get("unsafeLoad")) {
			return yaml.Load(js.ToString(src))
		}
		return yaml.SafeLoad(js.ToString(src))
	})
}

// parse runs a parser for ReadJSON or ReadYAML, wrapping its error as grunt does.
func (f *File) parse(path string, parser func() (any, error)) (any, error) {
	v := f.g.Verbose
	v.Write("Parsing " + path + "...")
	result, err := parser()
	if err != nil {
		v.Error()
		msg, _ := message(err)
		return nil, utilError(`Unable to parse "`+path+`" file (`+js.ToString(msg)+`).`, err)
	}
	v.Ok()
	return result, nil
}

// Write is grunt.file.write(path, contents, options): it creates missing
// directories and writes contents ([]byte as is, anything else as a string encoded
// with options.encoding or DefaultEncoding), with options.mode for a new file.
// With --no-write it only logs.
func (f *File) Write(path string, contents any, options ...any) error {
	return f.write(path, js.Normalize(contents), optionsObject(options, 0))
}

func (f *File) write(path string, contents any, options *js.Object) error {
	nowrite := f.g.option.truthy("no-write")
	v := f.g.Verbose
	if nowrite {
		v.Write("Not actually writing " + path + "...")
	} else {
		v.Write("Writing " + path + "...")
	}
	if err := f.Mkdir(nodepath.Dirname(path)); err != nil {
		return err
	}
	if err := f.writeContents(path, contents, options, nowrite); err != nil {
		v.Error()
		return utilError(`Unable to write "`+path+`" file (Error code: `+errorCode(err)+`).`, err)
	}
	v.Ok()
	return nil
}

func (f *File) writeContents(path string, contents any, options *js.Object, nowrite bool) error {
	data, ok := contents.([]byte)
	if !ok {
		// iconv.encode turns its input into a string with "" + (str || "").
		s := ""
		if js.ToBoolean(contents) {
			s = js.ToString(contents)
		}
		name := f.DefaultEncoding
		if enc := options.Get("encoding"); js.ToBoolean(enc) {
			name = js.ToString(enc)
		}
		var err error
		if data, err = iconv.Encode(s, name, false); err != nil {
			return err
		}
	}
	if nowrite {
		return nil
	}
	perm := fs.FileMode(0o666)
	if m := options.Get("mode"); !js.IsNullish(m) {
		perm = fileMode(m)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return nodefs.NewError(err, "open", path)
	}
	_, err = file.Write(data)
	if cerr := file.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nodefs.NewError(err, "write", path)
	}
	return nil
}

// Copy is grunt.file.copy(src, dest, options): it copies a file, a directory
// recursively, or a symbolic link (as a new link to the source). options.process
// (a function of the contents, the source and the destination path) transforms
// files unless options.noProcess is true or matches the source; it returning
// false skips the file. options.encoding applies to processed files; others are
// copied byte for byte.
func (f *File) Copy(src, dest string, options ...any) (err error) {
	opts := optionsObject(options, 0)
	// A process function or a noProcess pattern may throw.
	if thrown := throws(func() { err = f.copy(src, dest, opts) }); thrown != nil {
		return thrown
	}
	return err
}

func (f *File) copy(src, dest string, opts *js.Object) error {
	isLink, err := f.IsLink(src)
	if err != nil {
		return err
	}
	switch {
	case isLink:
		return f.copySymbolicLink(src, dest)
	case f.IsDir(src):
		if err := f.Mkdir(dest); err != nil {
			return err
		}
		names, err := nodefs.Readdir(src)
		if err != nil {
			return err
		}
		for _, name := range names {
			if err := f.copy(nodepath.Join(src, name), nodepath.Join(dest, name), opts); err != nil {
				return err
			}
		}
		return nil
	}
	return f.copyFile(src, dest, opts)
}

// copyFile is file._copy.
func (f *File) copyFile(src, dest string, options *js.Object) error {
	v := f.g.Verbose
	noProcess := options.Get("noProcess")
	process := js.ToBoolean(options.Get("process")) && noProcess != true &&
		!(js.ToBoolean(noProcess) && f.IsMatch(noProcess, src))
	rwOptions := options
	if !process {
		// Copy without processing byte for byte.
		rwOptions = js.ObjectOf("encoding", nil)
	}
	contents, err := f.read(src, rwOptions)
	if err != nil {
		return err
	}
	if process {
		v.Write("Processing source...")
		fn, ok := options.Get("process").(*js.Function)
		if !ok {
			err = js.NewTypeError("options.process is not a function")
		} else {
			contents, err = fn.Call(options, []any{contents, src, dest})
		}
		if err != nil {
			v.Error()
			return utilError(`Error while processing "`+src+`" file.`, err)
		}
		v.Ok()
	}
	destIsLink, err := f.IsLink(dest)
	if err != nil {
		return err
	}
	if contents == false || destIsLink {
		v.Writeln("Write aborted. Either the process function returned false or the destination is a symlink")
		return nil
	}
	return f.write(dest, js.Normalize(contents), rwOptions)
}

// copySymbolicLink is file._copySymbolicLink: the new link points at the source
// link, relative to the destination's directory; an existing destination is kept.
func (f *File) copySymbolicLink(src, dest string) error {
	destDir := nodepath.Join(dest, "..")
	if !nodepath.IsAbsolute(src) {
		src = nodepath.Relative(cwd(), destDir, src)
		if src == "" {
			src = "."
		}
	}
	if err := f.Mkdir(destDir); err != nil {
		return err
	}
	if _, err := os.Stat(dest); err == nil {
		return nil
	}
	if err := os.Symlink(src, dest); err != nil {
		e := nodefs.NewError(err, "symlink", src)
		e.Dest = dest
		return e
	}
	return nil
}

// Delete is grunt.file.delete(path, options): it removes a file or a directory
// tree, and reports whether it did. Deleting the working directory or anything
// outside it is refused with a warning unless options.force (by default --force) is
// set; a missing path logs a warning. With --no-write it only logs.
func (f *File) Delete(path string, options ...any) (bool, error) {
	g := f.g
	nowrite := g.option.truthy("no-write")
	var opts *js.Object
	if len(options) > 0 && js.ToBoolean(options[0]) {
		opts = optionsObject(options, 0)
	} else {
		opts = js.ObjectOf("force", g.option.truthy("force"))
	}
	v := g.Verbose
	if nowrite {
		v.Write("Not actually deleting " + path + "...")
	} else {
		v.Write("Deleting " + path + "...")
	}
	if !f.Exists(path) {
		v.Error()
		g.Log.Warn("Cannot delete nonexistent file.")
		return false, nil
	}
	if !js.ToBoolean(opts.Get("force")) {
		if f.IsPathCwd(path) {
			v.Error()
			g.Fail.Warn("Cannot delete the current working directory.")
			return false, nil
		} else if !f.IsPathInCwd(path) {
			v.Error()
			g.Fail.Warn("Cannot delete files outside the current working directory.")
			return false, nil
		}
	}
	if !nowrite {
		if err := nodefs.RemoveAll(path); err != nil {
			v.Error()
			return false, utilError(`Unable to delete "`+path+`" file (`+err.Error()+`).`, err)
		}
	}
	v.Ok()
	return true, nil
}

// Exists reports whether the joined path exists (following symbolic links).
func (f *File) Exists(parts ...string) bool {
	_, err := os.Stat(nodepath.Join(parts...))
	return err == nil
}

// IsLink reports whether the joined path is a symbolic link. A missing path is
// not; other errors are reported as `Unable to read "path" file (Error code: X).`
func (f *File) IsLink(parts ...string) (bool, error) {
	path := nodepath.Join(parts...)
	fi, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return false, nil
		}
		return false, utilError(`Unable to read "`+path+`" file (Error code: `+errorCode(nodefs.NewError(err, "lstat", path))+`).`, err)
	}
	return fi.Mode()&fs.ModeSymlink != 0, nil
}

// IsDir reports whether the joined path is a directory (following links).
func (f *File) IsDir(parts ...string) bool {
	fi, err := os.Stat(nodepath.Join(parts...))
	return err == nil && fi.IsDir()
}

// IsFile reports whether the joined path is a regular file (following links).
func (f *File) IsFile(parts ...string) bool {
	fi, err := os.Stat(nodepath.Join(parts...))
	return err == nil && fi.Mode().IsRegular()
}

// IsPathAbsolute reports whether the joined path is absolute.
func (f *File) IsPathAbsolute(parts ...string) bool {
	return nodepath.IsAbsolute(nodepath.Join(parts...))
}

// ArePathsEquivalent reports whether all the paths resolve to the same path.
func (f *File) ArePathsEquivalent(first string, rest ...string) bool {
	dir := cwd()
	first = nodepath.Resolve(dir, first)
	for _, p := range rest {
		if nodepath.Resolve(dir, p) != first {
			return false
		}
	}
	return true
}

// DoesPathContain reports whether every descendant path lies strictly inside
// ancestor (the paths need not exist).
func (f *File) DoesPathContain(ancestor string, descendants ...string) bool {
	dir := cwd()
	ancestor = nodepath.Resolve(dir, ancestor)
	for _, d := range descendants {
		relative := nodepath.Relative(dir, nodepath.Resolve(dir, d), ancestor)
		// relative === '' || /\w+/.test(relative)
		if relative == "" || strings.ContainsFunc(relative, isWordChar) {
			return false
		}
	}
	return true
}

// isWordChar reports whether r is in the regular expression class \w.
func isWordChar(r rune) bool {
	return r == '_' || '0' <= r && r <= '9' || 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z'
}

// realpaths resolves the working directory and the joined path with
// fs.realpathSync.
func realpaths(parts []string) (dir, path string, err error) {
	c := cwd()
	if dir, err = nodefs.Realpath(c, c); err != nil {
		return "", "", err
	}
	path, err = nodefs.Realpath(c, nodepath.Join(parts...))
	return dir, path, err
}

// IsPathCwd reports whether the joined path is the working directory.
func (f *File) IsPathCwd(parts ...string) bool {
	dir, path, err := realpaths(parts)
	return err == nil && f.ArePathsEquivalent(dir, path)
}

// IsPathInCwd reports whether the joined path lies inside the working directory.
func (f *File) IsPathInCwd(parts ...string) bool {
	dir, path, err := realpaths(parts)
	return err == nil && f.DoesPathContain(dir, path)
}
