# grunt 1.6.3 in Go: what changed

The JavaScript package (`lib/`, `bin/grunt`, `internal-tasks/`, `test/`, `Gruntfile.js`,
`package.json` and the ESLint configuration) is retired. The module `github.com/gruntjs/grunt-go`
reimplements grunt 1.6.3 together with the behaviour it inherits from its dependencies, at the
versions the 1.6.3 release installs: grunt-cli 1.5.0, grunt-known-options 2.0.0,
grunt-legacy-log 3.0.1 and grunt-legacy-log-utils 2.1.3, grunt-legacy-util 2.0.2 (lodash 4.18.1 with
the underscore.string 3.3.6 mixins), glob 7.1.7, minimatch 3.1.5, js-yaml 3.15.2, iconv-lite 0.6.3,
dateformat 4.6.3, nopt 5.0.0 and eventemitter2 0.4.14.

## 1. The gruntfile is a Go program

`grunt.Main(func(g *grunt.Grunt) { … })` is both grunt-cli and the gruntfile: it parses the command
line as grunt-cli does, runs the function as "loading the Gruntfile", then runs the tasks and exits
with grunt's exit code. The gruntfile's path is reported as `Gruntfile.go` in the working directory
(or `--base`), where grunt reports the `Gruntfile.js` it found. `--version --verbose` reports the
directory holding the running executable as the install path.

Consequences:

- `grunt.loadTasks` and `grunt.loadNpmTasks` cannot load JavaScript task files. They report the same
  "not found" errors as grunt; a task file that exists is reported as a load error.
- `--require`, `--preload` and CoffeeScript gruntfiles have no meaning and are ignored; the options
  are still parsed and listed by `--help`.
- `--completion=bash|zsh` prints grunt-cli's completion scripts, embedded from `completion/`.

## 2. Values

Configuration data, template data and the values grunt's helpers return follow the JavaScript
value model in package `js` (undefined, null, numbers, strings, booleans, `*js.Array`, ordered
`*js.Object`, `js.Date`, `*js.Function`, buffers as `[]byte`). Calls that throw in JavaScript return
an error in Go. A task function returns what the JavaScript function returns (`false`, an error, or
any other value, which counts as success), and a panic inside it is a throw.

Strings hold UTF-8. Where JavaScript can produce a lone UTF-16 surrogate (a decoder given broken
UTF-16, a template slicing a surrogate pair, a YAML `"\uD800"` escape), the Go value carries U+FFFD
when it leaves the template runtime, which is what Node prints for it.

## 3. Templates

lodash compiles template code into a JavaScript function. The Go module evaluates the JavaScript
subset in [docs/template-js.md](docs/template-js.md) instead, with the `_` helpers in
[docs/template-helpers.md](docs/template-helpers.md); output and error messages (V8's texts) are the
same for that subset. Outside it: classes, generators, async functions, getters and setters, Symbol,
Map, Set, Intl, `localeCompare` and `normalize` raise a SyntaxError or are absent; arrays have no
holes; trigonometric and logarithmic `Math` functions can differ from V8 in the last bit. With no
accessors in the value model, `Object.prototype` has no `__defineGetter__`, `__defineSetter__`,
`__lookupGetter__` or `__lookupSetter__`, nor the `__proto__` accessor (`obj.__proto__` reads
undefined in a template; `Object.getPrototypeOf` works), and `Object.create` with a second (property
descriptors) argument throws a TypeError. Regular expressions with the v flag cannot use the
properties of strings (`\p{RGI_Emoji}` and the like), which throw a SyntaxError starting
`unsupported:`.

## 4. Library differences

- js-yaml: `!!js/function` (load with `unsafeLoad`) is not supported and is reported as an
  unsupported tag; very deep nesting that overflows V8's stack parses in Go.
- glob: names sort with the ICU root collation of Node 24 (`localeCompare(b, 'en')`) for Latin,
  Greek, Cyrillic, Armenian, Hebrew, Arabic, Hangul, kana, punctuation, symbols and common emoji;
  Han ideographs sort by code point and scripts outside that list after them.
- glob: with `strict: false`, a directory that cannot be read is reported on stderr as
  `glob error [Error: EACCES: …] { errno, code, syscall, path }`; Node prints the error's stack
  trace where this prints `[Error: …]`.
- lodash `sortBy`/`orderBy` sort with the same port of V8's TimSort as the template runtime's
  `Array.prototype.sort`, so even inconsistent comparisons order items as in Node.
- `util.inspect` of a function prints `[Function: name]`, and `String(fn)` prints a native-code body,
  since Go functions have no source text.
- What a value inherits, where lodash's path helpers (`_.get`, `_.hasIn`, `_.property`, `_.pick`, …),
  underscore.string's `unescapeHTML`, dateformat's mask names and `grunt.util.namespace` (so
  `grunt.config.get`) read it — `constructor`, `toString`, `hasOwnProperty`, `'abc'.toUpperCase` and
  the like — is the template runtime's builtin function, which behaves as Node's. These builtins
  come from a realm of their own, so they are not `===` to the globals a template sees. Following the
  runtime: a Buffer inherits `Object.prototype` (its `constructor` is `Object`, not `Buffer`) and the
  four legacy accessor methods above are absent. Outside templates `__proto__` reads as the enumerable
  data of the prototype, which is none: an empty array for an array (`Array.prototype` is an array) and an empty
  object otherwise. That is a new value each time, not the prototype itself, so writing to it changes
  nothing else.
- The `sprintf()` deprecation warning that Node prints on stderr is not printed.

## 5. Dates and the local time zone

Local time is the host's time zone, as in Node: Go's `time.Local` reads `TZ` (or
`/etc/localtime`), and the local getters, `new Date(y, m, ...)`, date-time strings without an
offset, `getTimezoneOffset`, `toString`, dateformat's masks without `UTC:` and YAML dates used as
mapping keys follow it with V8's rules (the offset in force at each instant; a wall-clock time in
a daylight-saving gap or overlap takes the offset from before the transition).

`Date.prototype.toString` ends with ICU's long name of the zone, as in
`GMT-0500 (Eastern Standard Time)`. The names are recorded from Node 24's ICU for every zone in
the IANA database (`internal/jsdate/zonenames_gen.go`, written by `internal/jsdate/gen/zonenames.js`);
as in V8, dates before 1970 or after 2037 take the standard or daylight name of the equivalent
year in 2008–2037. Remaining differences:

- `TZ` must name a zone file. Node also accepts a POSIX rule such as `TZ=EST5EDT,M3.2.0,M11.1.0`;
  Go's `time.Local` falls back to UTC for it.
- A zone missing from the table (one added to the IANA database after it was recorded) is named in
  ICU's fallback form, `GMT-05:00`, where ICU may know a long name.
- The offsets come from the system's time-zone database and ICU's from its own copy; where the
  two disagree (a rule change one of them has not picked up yet) the offsets differ too.
- The names are ICU's English ones, which is what Node prints under `LANG=C.UTF-8` or an English
  locale.
