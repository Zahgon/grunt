# JavaScript in templates

grunt renders `<%= %>`, `<%- %>` and `<% %>` tags with lodash 4.18.1's `_.template`,
which turns the template into JavaScript source and runs it with the `Function`
constructor. The Go port builds the same source text and runs it on an interpreter
for the JavaScript subset described here. Inside that subset, rendered output and
error messages are byte-identical to Node 24 (V8): a template that throws produces
the same `name` and `message`, including V8's syntax error texts.

## How a template runs

- Text outside tags is copied. `<%= expr %>` appends `expr` (nothing for `null`
  and `undefined`), `<%- expr %>` appends `_.escape(expr)`, and `<% code %>` runs
  statements; code may span several tags with text in between
  (`<% if (x) { %>yes<% } %>`).
- `print(a, b, ...)` appends its arguments joined with no separator.
- The code runs in sloppy mode inside `with (obj) { ... }`, where `obj` is the data
  object: grunt passes `Object.create(configData)` with a `grunt` property.
  A free name is looked up on the data object and its prototype chain (including
  `Object.prototype`, so `toString` and `hasOwnProperty` resolve there), then in
  the template's imports (`_`), then among the globals. A name found nowhere
  throws `ReferenceError: x is not defined`.
- Assigning an undeclared name creates a global, which later templates in the same
  run see, as in Node.
- `obj`, `__p`, `__t`, `__e`, `__j` and `print` are lodash's own variables and are
  visible to template code; `this` is the global object.
- A template may `return` a value from evaluate code; the rendered result is then
  `String(value)`.
- Syntax errors are reported for the whole generated function, exactly where V8
  reports them, e.g. `<%= a b %>` gives `SyntaxError: Unexpected identifier 'b'`
  and an unclosed `<% if (x) { %>` gives `SyntaxError: Unexpected token ')'`.

## Syntax

Supported:

- Literals: numbers (decimal, `.5`, exponents, `0x`, `0o`, `0b`, legacy octal
  `017`, `08`, numeric separators `1_000`), strings with every escape (`\n`, `\x41`,
  `A`, `\u{1F600}`, legacy octal, line continuations), template literals with
  `${}` and tagged templates, regular expression literals, array literals (holes,
  spread), object literals (shorthand, computed keys, methods, spread,
  `__proto__: value`).
- Operators: member access `.` `[]` and optional chaining `?.` (including
  `?.()` and `?.[]`), calls with the correct `this`, `new`, unary
  `! - + ~ typeof void delete`, `++` `--`, `** * / % + -`, `<< >> >>>`,
  `< > <= >= in instanceof`, `== != === !==`, `& ^ |`, `&& || ??`, `? :`, the comma
  operator, and every assignment operator including `&&= ||= ??=`.
- Destructuring in declarations, parameters, `for` heads, `catch` and assignment,
  with defaults and rest elements.
- Statements: `var`, `let`, `const` (block scoping, temporal dead zone, a fresh
  binding per `for (let ...)` iteration), `if`/`else`, `for`, `for...in`, `for...of`,
  `while`, `do...while`, `break`/`continue` with labels, `switch`, `try`/`catch`/
  `finally` (with or without a catch binding), `throw`, `return`, `with`,
  `debugger` (no effect), function declarations (hoisted; block-level functions
  follow Annex B), function expressions, arrow functions, default and rest
  parameters, closures and `arguments`.
- Calls nest up to 10450 frames; deeper recursion throws
  `RangeError: Maximum call stack size exceeded` (Node's limit for a simple
  recursive function; Node's real limit depends on frame size).

Not supported (they are rejected as SyntaxErrors whose message says so, where Node
would accept them): `class`, generators and `yield`, `async` functions and
`await`, getters and setters in object literals, `new.target`, `import()`. BigInt
literals parse but throw `TypeError: BigInt values are not supported` when
evaluated. Symbols, `Map`, `Set`, `Proxy`, `Reflect`, typed arrays and `Intl` do
not exist (`Symbol` is not defined). A `"use strict"` directive has no effect: all
code runs in sloppy mode.

## Values

- Arrays have no holes: `[, 1]`, `new Array(3)`, `delete a[0]` and growing `length`
  produce `undefined` elements, so methods that skip holes in JavaScript visit them.
- Dates are immutable values: the `set*` methods do not exist, and two Date
  objects are never `===`, not even a Date compared with itself.
- Primitive wrapper objects do not exist: `new String('a')`, `new Number(1)`,
  `new Boolean(true)` and `Object('a')` give the primitive.
- Property descriptors, getters and setters are not modelled
  (`Object.defineProperty` does not exist); `Object.freeze` makes later writes to a
  plain object's properties do nothing.
- A thrown non-Error value (`throw 'x'`) reaches Go as `*jsrt.Thrown`; Error
  objects reach Go as `*js.Error`. An error's `stack` is `"Name: message"` with no
  frames.

## Builtins

Globals: `globalThis`, `undefined`, `NaN`, `Infinity`, `parseInt`, `parseFloat`,
`isNaN`, `isFinite`, `encodeURIComponent`, `encodeURI`, `decodeURIComponent`,
`decodeURI` (`URIError: URI malformed`), `Object`, `Function`, `Array`, `String`,
`Number`, `Boolean`, `Date`, `RegExp`, `Math`, `JSON`, `Error`, `TypeError`,
`RangeError`, `SyntaxError`, `ReferenceError`, `URIError`, `EvalError`.

- `Object`: `Object(v)`, `Object.keys`, `values`, `entries`, `assign`,
  `fromEntries`, `create` (one argument), `getPrototypeOf`,
  `getOwnPropertyNames`, `freeze`, `isFrozen`, `is`.
  `Object.prototype`: `hasOwnProperty`, `isPrototypeOf`, `propertyIsEnumerable`,
  `toString` (`[object Array]`, `[object Null]`, `[object Arguments]`, ...),
  `toLocaleString`, `valueOf`.
- `Function`: `Function(...params, body)` / `new Function(...)`;
  `Function.prototype`: `call`, `apply`, `bind`, `toString` (the source text of a
  function written in the template, `function name() { [native code] }` for a
  builtin); `name` and `length`; `prototype` for constructors (`new F()` on
  template functions works).
- `Array`: `Array(n)` / `new Array(...)` (`RangeError: Invalid array length`),
  `Array.isArray`, `Array.of`, `Array.from` (iterables and array-likes, with a map
  function). `Array.prototype`: `at`, `concat`, `entries`, `every`, `fill`,
  `filter`, `find`, `findIndex`, `findLast`, `findLastIndex`, `flat`, `flatMap`,
  `forEach`, `includes`, `indexOf`, `join`, `keys`, `lastIndexOf`, `map`, `pop`,
  `push`, `reduce`, `reduceRight`, `reverse`, `shift`, `slice`, `some`, `sort`,
  `splice`, `toLocaleString`, `toReversed`, `toSorted`, `toString`, `unshift`,
  `values`, `with`, and `length` (readable and writable). `sort` is V8's TimSort,
  so even inconsistent comparators give Node's order; without a comparator it
  compares `String(x)` by UTF-16 code units and puts `undefined` last. Applied to
  array-likes through `call`, the methods read a copy, so mutating methods do not
  write back to a non-array.
- `String(v)`, `String.fromCharCode`, `String.fromCodePoint`, `String.raw`.
  `String.prototype`: `at`, `charAt`, `charCodeAt`, `codePointAt`, `concat`,
  `endsWith`, `includes`, `indexOf`, `lastIndexOf`, `match`, `matchAll`, `padEnd`,
  `padStart`, `repeat`, `replace`, `replaceAll`, `search`, `slice`, `split`,
  `startsWith`, `substr`, `substring`, `toLowerCase`, `toUpperCase`,
  `toLocaleLowerCase`, `toLocaleUpperCase` (full Unicode case mapping, no locale),
  `trim`, `trimStart`, `trimEnd`, `trimLeft`, `trimRight`, `toString`, `valueOf`,
  `length`, and indexing. Strings are sequences of UTF-16 code units, as in
  JavaScript. Not provided: `localeCompare`, `normalize`, `isWellFormed`,
  `toWellFormed`, the HTML methods (`anchor`, `bold`, ...).
- Replacement strings support `$$`, `$&`, `` $` ``, `$'`, `$1`–`$99` and `$<name>`;
  replacement functions receive the match, the captures, the offset, the string
  and the groups object.
- `Number(v)`, `Number.isInteger`, `isSafeInteger`, `isFinite`, `isNaN`,
  `parseFloat`, `parseInt`, `MAX_SAFE_INTEGER`, `MIN_SAFE_INTEGER`, `EPSILON`,
  `MAX_VALUE`, `MIN_VALUE`, `NaN`, `POSITIVE_INFINITY`, `NEGATIVE_INFINITY`.
  `Number.prototype`: `toFixed`, `toPrecision`, `toExponential`, `toString(radix)`
  (V8's digits and RangeError texts), `valueOf`. `toLocaleString` is the
  `Object.prototype` one, so it gives `String(n)` without grouping.
- `Boolean(v)`, `Boolean.prototype.toString`, `valueOf`.
- `Math`: `E`, `LN10`, `LN2`, `LOG10E`, `LOG2E`, `PI`, `SQRT1_2`, `SQRT2`, `abs`,
  `acos`, `acosh`, `asin`, `asinh`, `atan`, `atanh`, `atan2`, `cbrt`, `ceil`,
  `clz32`, `cos`, `cosh`, `exp`, `expm1`, `floor`, `fround`, `hypot`, `imul`,
  `log`, `log1p`, `log10`, `log2`, `max`, `min`, `pow`, `random`, `round`, `sign`,
  `sin`, `sinh`, `sqrt`, `tan`, `tanh`, `trunc`. `abs`, `ceil`, `floor`, `round`,
  `trunc`, `sign`, `max`, `min`, `sqrt`, `fround`, `imul`, `clz32`, `cbrt` and
  `hypot` give V8's exact results. `pow` and `**` are correctly rounded; V8 (the
  C library's `pow`) differs in the last bit for about 1 in 2000 random operand
  pairs. The other transcendental functions use Go's `math` package and may differ
  from V8 in the last bit.
- `JSON.stringify(value, replacer, space)` (replacer function or key array,
  numeric or string indent, `toJSON`, `NaN`/`Infinity` as `null`, `undefined` and
  functions skipped, V8's `Converting circular structure to JSON` message with its
  `-->`/`|`/`---` lines) and `JSON.parse(text, reviver)` with Node 24's messages
  (`Unexpected token 'x', "..." is not valid JSON`, `Unexpected end of JSON input`,
  `Expected property name or '}' in JSON at position 1 (line 1 column 2)`, ...).
- `Date`: `Date()` (a string), `new Date()`, `new Date(ms)`, `new Date(string)`
  (V8's parser, including its legacy formats), `new Date(date)`,
  `new Date(y, m, d, h, mi, s, ms)`, `Date.now`, `Date.parse`, `Date.UTC`.
  `Date.prototype`: `getTime`, `valueOf`, `getFullYear`, `getMonth`, `getDate`,
  `getDay`, `getHours`, `getMinutes`, `getSeconds`, `getMilliseconds`, the
  `getUTC*` forms, `getYear`, `getTimezoneOffset`, `toISOString`, `toJSON`,
  `toString`, `toDateString`, `toTimeString`, `toUTCString`, `toGMTString`, and the
  en-US `toLocaleString`, `toLocaleDateString`, `toLocaleTimeString` without
  arguments. Local time is the host's time zone, read from `TZ` (or the system
  setting) as Node reads it. The current time comes from the host's clock
  setting (without one, `Date.now()` and `new Date()` throw).
- `RegExp(pattern, flags)` / `new RegExp`, `RegExp.prototype.exec`, `test`,
  `toString`, `lastIndex`, `source`, `flags`, `global`, `ignoreCase`,
  `multiline`, `dotAll`, `unicode`, `unicodeSets`, `sticky`, `hasIndices`. Match
  arrays carry `index`, `input` and `groups`. Regular expressions follow Node 24
  exactly, including Annex B syntax without u/v, lookbehind, named and
  duplicate-named groups, modifier groups `(?ims-ims:...)`, `\p{...}`/`\P{...}`
  (General_Category, Script, Script_Extensions, all ECMAScript binary properties)
  and the flags d g i m s u v y. Not supported: the v-flag properties of strings
  (`Basic_Emoji`, `Emoji_Keycap_Sequence`, `RGI_Emoji`, `RGI_Emoji_Flag_Sequence`,
  `RGI_Emoji_Modifier_Sequence`, `RGI_Emoji_Tag_Sequence`,
  `RGI_Emoji_ZWJ_Sequence`), which throw a SyntaxError starting `unsupported:`.
- Errors: `Error(message)` and the native error constructors with or without
  `new`; `name`, `message`, `Error.prototype.toString`, `instanceof`.
- Iterators: `keys()`, `values()`, `entries()` and `matchAll()` return iterators
  (`[object Array Iterator]`) with `next()`; they work with `for...of`, spread and
  `Array.from`. Their values are computed when the iterator is created.

## Error messages

Runtime errors use V8's texts, rendering the failing expression the way V8 does:
`a.b is not a function`, `a[1] is not a function`, `foo(...) is not a function`,
`(intermediate value).x is not a function`, `x is not iterable`,
`Cannot read properties of undefined (reading 'x')`,
`Cannot set properties of null (setting '0')`,
`Cannot destructure property 'x' of 'null' as it is null.`,
`Assignment to constant variable.`, `Cannot access 'x' before initialization`,
`toFixed() digits argument must be between 0 and 100`, `Invalid array length`,
`Invalid count value: -1`, `Maximum call stack size exceeded`.
