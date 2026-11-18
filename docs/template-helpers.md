# Template helpers: `_` and `grunt.template.date`

Templates (`<%= ... %>`, `<% ... %>`, `<%- ... %>`) and `grunt.util._` see the same
`_` object as grunt 1.6.3: lodash 4.18.1 (from grunt-legacy-util 2.0.2) with
underscore.string 3.3.6 mixed in. The mixin runs last, so where both libraries
define a name, **the underscore.string function wins**. Those names are marked
*(u.s.)* below. Every function behaves exactly as in Node, including coercion of
`null`, `undefined`, numbers and arrays, and the errors it throws.

Only the functions listed here exist on `_`. `_.VERSION` is `"4.18.1"`.

## Conventions

- **Iteratee shorthands** (lodash collection functions):
  - a function is called with `(value, index|key, collection)`;
  - `null` or `undefined` means identity;
  - a string or number is a property path, such as `'a.b[0]'` (`_.property`);
  - `[path, value]` matches that property's value (`_.matchesProperty`);
  - any other object is a partial deep match (`_.matches`).
- **Collections**:
  - arrays and array-like objects are walked by index;
  - strings are walked by UTF-16 code unit;
  - other objects are walked by their own keys in JavaScript key order: integer keys ascending, then insertion order.
- **Strings**:
  - Lengths and indexes count UTF-16 code units, as in JavaScript.
  - underscore.string functions turn `null` and `undefined` into `""`.
- **Regular expression arguments**:
  - A RegExp argument (a `/.../flags` literal in a template, or `*js.RegExp`) is matched with full ECMAScript semantics. This covers lookbehind, named groups, `\p{...}` and the `u`, `v` and `y` flags.
  - Several underscore.string functions build a RegExp from a *string* argument, exactly as the original does: `replaceAll`'s `find`, `trim`/`ltrim`/`rtrim` characters, and `dedent`'s pattern. Special characters in such strings therefore behave as in Node; for example `_.trim('a-z', 'a-z')` trims the range `[a-z]`.

## underscore.string 3.3.6

| Function | Result |
|---|---|
| `isBlank(s)` | `true` if `s` is empty or white space only |
| `stripTags(s)` | `s` without `<...>` tags |
| `capitalize(s, lowerRest)` *(u.s.)* | first character upper-cased; the rest lower-cased if `lowerRest` is truthy |
| `decapitalize(s)` | first character lower-cased |
| `chop(s, step)` | array of `step`-sized pieces (`[s]` if `step` < 1) |
| `trim(s, chars)` / `strip` *(u.s.)* | `s` trimmed of white space or of `chars` |
| `ltrim(s, chars)` / `lstrip`, `rtrim(s, chars)` / `rstrip` | left or right trim |
| `clean(s)` | trimmed, with runs of white space collapsed to one space |
| `cleanDiacritics(s)` | accents removed (`'ąàá…ß'`; `ß` becomes `ss`) |
| `count(s, sub)` | number of occurrences of `sub` |
| `chars(s)` | array of code units |
| `swapCase(s)` | case of each character swapped |
| `escapeHTML(s)` | `& < > " '` and `¢ £ ¥ € © ®` as named entities (`'` as `&#39;`) |
| `unescapeHTML(s)` | named, `&#NN;` and `&#xHH;` entities decoded |
| `splice(s, i, howmany, sub)` | `s` with `howmany` characters at `i` replaced by `sub` |
| `insert(s, i, sub)` | `sub` inserted at `i` |
| `replaceAll(s, find, replace, ignorecase)` | every match of `new RegExp(find, 'g'+(ignorecase===true?'i':''))` replaced |
| `lines(s)` | array of lines split on `\r\n`, `\r` and `\n` |
| `dedent(s, pattern)` | common indentation (or `^pattern`) removed from every line |
| `startsWith(s, starts, pos)` *(u.s.)*, `endsWith(s, ends, pos)` *(u.s.)* | boolean |
| `pred(s)`, `succ(s)` | last character's code decremented or incremented |
| `titleize(s)` | lower-cased, each word's first letter upper-cased |
| `camelize(s, decap)` / `camelcase` | `'foo-bar_baz qux'` becomes `'fooBarBazQux'`; with `decap === true`, first letter lower-cased |
| `classify(s)` | `'some_class name'` becomes `'SomeClassName'` |
| `underscored(s)` | `'fooBar'` becomes `'foo_bar'` |
| `dasherize(s)` | `'fooBar'` becomes `'foo-bar'` (and `'FooBar'` becomes `'-foo-bar'`) |
| `humanize(s)` | `'some_id_name_id'` becomes `'Some id name'` |
| `truncate(s, length, truncateStr)` *(u.s.)* | first `length` characters plus `truncateStr` (default `'...'`) when longer |
| `prune(s, length, pruneStr)` | like `truncate` but never cuts a word |
| `words(s, delimiter)` *(u.s.)* | array of words split on white space, or on `delimiter` (a string or RegExp) |
| `pad(s, length, padStr, type)` *(u.s.)* | padded to `length` with the first character of `padStr` (default `' '`); `type` is `'left'` (default), `'right'` or `'both'` |
| `lpad` / `rjust`, `rpad` / `ljust`, `lrpad` / `center` | `pad` with type left, right or both |
| `sprintf(format, ...args)`, `vsprintf(format, args)` | sprintf-js 1.1.3: `%s %d %i %f %e %g %b %c %o %x %X %u %j %t %T %v %%`, flags `+ - 0 ' '`, `'c` padding, width, `.precision`, positional `%1$s`, named `%(key.path)s` |
| `toNumber(n, decimals)` *(u.s.)* | `n` rounded to `decimals` places |
| `numberFormat(n, decimals, decimalSep, thousandsSep)` | `1234.5` becomes `'1,234.50'` with `decimals` 2; `''` for `NaN`/`null` |
| `strRight(s, sep)`, `strRightBack(s, sep)` | text after the first or last `sep` |
| `strLeft(s, sep)`, `strLeftBack(s, sep)` | text before the first or last `sep` |
| `toSentence(array, sep, lastSep, serial)` | `['a','b','c']` becomes `'a, b and c'` |
| `toSentenceSerial(array, sep, lastSep)` | `'a, b, and c'` |
| `slugify(s)` | `'Un éléphant à l'orée'` becomes `'un-elephant-a-l-oree'` |
| `surround(s, wrap)`, `quote(s, q)` / `q`, `unquote(s, q)` | wrap in, or strip, `wrap` or `q` (default `"`) |
| `repeat(s, n, sep)` *(u.s.)* | `s` repeated `n` times, joined by `sep` |
| `naturalCmp(a, b)` | -1, 0 or 1 in natural order (`'a2' < 'a10'`) |
| `levenshtein(a, b)` | edit distance |
| `toBoolean(s, trueValues, falseValues)` / `toBool` | `true` for `'true'`/`'1'`, `false` for `'false'`/`'0'`, `undefined` otherwise (custom strings or RegExps allowed) |
| `escapeRegExp(s)` *(u.s.)* | regex metacharacters backslash-escaped |
| `mapChars(s, fn)` | each character replaced by `fn(char, index)` |

## lodash 4.18.1

**Strings.**
- `escape(s)` escapes `& < > " '` as `&amp; &lt; &gt; &quot; &#39;`.
- `unescape(s)` reverses `escape`.
- `camelCase(s)`, `kebabCase(s)`, `snakeCase(s)`, `startCase(s)`, `lowerCase(s)` and `upperCase(s)` split `s` into words, so `'XMLHttpRequest'`, `'foo_bar'` and `'Foo Bar'` all work, and accents are removed.
- `upperFirst(s)` and `lowerFirst(s)` change the case of the first character.
- `padStart(s, length, chars)` and `padEnd(s, length, chars)` pad with `chars` (default `' '`).
- `deburr(s)` removes accents.
- `toUpper(s)` and `toLower(s)` change the case of the whole string.
- `trimStart(s, chars)` and `trimEnd(s, chars)` trim white space, or the characters of `chars`.
- `split(s, separator, limit)` takes a string or RegExp separator.
- `replace(s, pattern, replacement)`:
  - A string pattern replaces only its first occurrence.
  - A RegExp pattern follows the RegExp's flags.
  - The replacement can be a `$`-template or a function.
- `toString(v)` gives `''` for `null`/`undefined` and keeps `-0` as `'-0'`.

**Arrays.**
- `join(array, sep)`.
- `uniq(array)`.
- `union(...arrays)`.
- `difference(array, ...values)`.
- `intersection(...arrays)`.
- `without(array, ...values)`.
- `flatten(array)` flattens one level; `flattenDeep(array)` flattens all levels.
- `compact(array)` drops falsy items.
- `head(array)` / `first(array)`, `last(array)`, `initial(array)` and `tail(array)`.
- `take(array, n=1)`, `drop(array, n=1)` and `chunk(array, size=1)`.
- `indexOf(array, value, fromIndex)` finds `NaN`.
- `reverse(array)` reverses in place.
- `concat(array, ...values)`.
- `range(start, end, step)`.

**Collections** (arrays, strings and objects):
- `map`, `filter`, `reject`, `find(c, pred, fromIndex)`, `every` and `some`.
- `reduce(c, fn, acc)`.
- `groupBy`, `countBy` and `keyBy`.
- `sortBy(c, ...iteratees)` is stable.
- `orderBy(c, iteratees, orders)`, where `orders` are `'asc'` or `'desc'`.
- `size(c)`.
- `includes(c, value, fromIndex)` tests for a substring when `c` is a string.

**Objects.**
- `keys(o)` and `values(o)`.
- `get(o, path, default)`.
- `has(o, path)` checks own properties only.
- `pick(o, ...paths)` and `omit(o, ...paths)`.
- `merge(o, ...sources)` merges deeply. Undefined source values don't overwrite, and `__proto__` and `constructor` are guarded.
- `assign(o, ...sources)`, and `extend` / `assignIn`.
- `defaults(o, ...sources)`.
- `clone(v)` and `cloneDeep(v)`.

**Lang.**
- `isArray`, `isString`, `isNumber` (true for `NaN`), `isBoolean`, `isEmpty`, `isNil`, `isNull`, `isUndefined` and `isPlainObject`.
- `isEqual(a, b)` compares deeply.
- `toArray(v)`.
- `toNumber(v)` *(u.s., see above)*.
- `toInteger(v)`.
- `parseInt(s, radix)`.

**Math.**
- `max(array)`, `min(array)` and `sum(array)` (strings concatenate).
- `mean(array)`.
- `round(n, precision)`, `floor(n, precision)` and `ceil(n, precision)`.
- `clamp(n, lower, upper)`.
- `inRange(n, start, end)`.

**Utility.**
- `times(n, fn)`, `identity(v)`, `constant(v)`, `noop()` and `once(fn)`.

## `grunt.template.date(date, mask)` and `grunt.template.today(mask)`

These use dateformat 4.6.3, in the local time zone unless the mask starts with `UTC:` or `GMT:`. `date` can be a Date, a millisecond number or a date string. A string without digits is taken as the mask, and the current time is used instead.

**Named masks** (each is shown with its expansion):

| Mask | Format |
|---|---|
| `default` | `ddd mmm dd yyyy HH:MM:ss` |
| `shortDate` | `m/d/yy` |
| `paddedShortDate` | `mm/dd/yyyy` |
| `mediumDate` | `mmm d, yyyy` |
| `longDate` | `mmmm d, yyyy` |
| `fullDate` | `dddd, mmmm d, yyyy` |
| `shortTime` | `h:MM TT` |
| `mediumTime` | `h:MM:ss TT` |
| `longTime` | `h:MM:ss TT Z` |
| `isoDate` | `yyyy-mm-dd` |
| `isoTime` | `HH:MM:ss` |
| `isoDateTime` | `yyyy-mm-dd'T'HH:MM:sso` |
| `isoUtcDateTime` | `UTC:yyyy-mm-dd'T'HH:MM:ss'Z'` |
| `expiresHeaderFormat` | `ddd, dd mmm yyyy HH:MM:ss Z` |

A mask may start with `UTC:` or `GMT:`. With `GMT:`, the `Z` token prints `GMT`.

**Tokens:**

| Token | Meaning |
|---|---|
| `d`, `dd` | day of the month |
| `ddd`, `dddd` | `Sun` / `Sunday` |
| `DDD`, `DDDD` | `Tdy`/`Ysd`/`Tmw` or `Today`/`Yesterday`/`Tomorrow` near the current date, otherwise the day name |
| `m`, `mm` | month number |
| `mmm`, `mmmm` | `Jan` / `January` |
| `yy`, `yyyy` | year |
| `h`, `hh` | 12-hour hour |
| `H`, `HH` | 24-hour hour |
| `M`, `MM` | minutes |
| `s`, `ss` | seconds |
| `l` | milliseconds (3 digits) |
| `L` | centiseconds |
| `t`, `tt`, `T`, `TT` | `a`/`am`/`A`/`AM` and the `p` forms |
| `Z` | the zone of `String(date)`: `UTC` for a zero offset, `EST`-style abbreviations for the US zones, otherwise `GMT+0100`-style; `UTC` (or `GMT`) with a `UTC:` (`GMT:`) mask |
| `o` | offset from UTC, `+0100` (`+0000` with `UTC:`) |
| `p` | offset from UTC, `+01:00` (`+00:00` with `UTC:`) |
| `S` | ordinal suffix: `st`, `nd`, `rd`, `th` |
| `W`, `WW` | ISO week |
| `N` | ISO weekday, 1–7 |
| `'...'`, `"..."` | literal text |

An invalid date throws `TypeError: Invalid date`.
