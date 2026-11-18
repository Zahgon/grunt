// minimatch.js records the behaviour of minimatch 3.1.5 and brace-expansion
// 1.1.22 on randomly generated, seeded inputs, for the Go differential tests.
//
//   node minimatch.js <node_modules dir> <output dir>
//
// It writes three fixtures to the output directory:
//
//   braces.json    [{pattern, expansions}]
//   regexp.json    [{source, flags, valid, inputs, results}] for the RegExp
//                  engine, results being a string of 0/1 per input
//   minimatch.json [{pattern, options, error?, negate, comment, empty,
//                   globSet, set, makeRe, paths, results, list}] where set
//                  entries are "G" (GLOBSTAR), "L:<literal>", "R:<source>" or
//                  "X" (the /$./ fallback), makeRe is the source or null, and
//                  results holds one of 0, 1 or E (thrown) per path.
//
// The fixtures were recorded with Node 24 (node:24-bookworm).
"use strict";

var fs = require("fs");
var path = require("path");
var modules = process.argv[2];
var out = process.argv[3];
var minimatch = require(path.join(modules, "minimatch"));
var expand = require(path.join(modules, "brace-expansion"));

// mulberry32, so the fixtures are reproducible.
var seed = 0x5eed1234;
function rand() {
  seed = (seed + 0x6d2b79f5) | 0;
  var t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
}
function pick(a) { return a[Math.floor(rand() * a.length)]; }
function chance(p) { return rand() < p; }
function build(tokens, min, max) {
  var n = min + Math.floor(rand() * (max - min + 1));
  var s = "";
  for (var i = 0; i < n; i++) s += pick(tokens);
  return s;
}

// --- brace expansion -------------------------------------------------------

var braceTokens = ["{", "}", ",", "a", "b", "c", "..", "1", "2", "0", "-", "\\", "$",
  "x", "{a,b}", "{1..3}", "{a..e}", "{1..10..3}", "{-2..2}", "{05..10}", "{a..E}",
  "{Z..a}", "{}", "{,}", "{a,}", "{,b}", "\\{", "\\,", "\\}", "é", ".", "{x}", "{a},b}",
  "${", "{3..1}", "{a..z..5}", "{10..1..-3}", "{0..0}", "{-05..5}", "{1..2..0}"];
var braces = [];
var seenBraces = {};
while (braces.length < 2000) {
  var bp = build(braceTokens, 1, 6);
  if (seenBraces[bp]) continue;
  seenBraces[bp] = true;
  var ex = expand(bp);
  if (ex.length > 24) continue;
  braces.push({ pattern: bp, expansions: ex });
}
["", "{}", "{},a}b", "a{},b}c", "{a,b}{c,d}", "{{a,b}}", "x{{a,b}}y", "{a..\\}", "{0..-0}",
  "{1..3}{a,b}", "a{b,c{d,e}f}g", "{a,b{,c}}",
  "\\\\{a,b}", "{a,\\\\,b}", "{1e2..1e3}", "{1.5..3}", "{a,b}\n{c,d}", "{a,b\n}"].forEach(function (p) {
  braces.push({ pattern: p, expansions: expand(p) });
});

// --- regular expressions ---------------------------------------------------

var reTokens = ["a", "b", "ab", ".", "*", "+", "?", "*?", "+?", "??", "{2}", "{1,}", "{1,2}",
  "{2,1}", "{", "}", "[", "]", "[ab]", "[^a]", "[a-c]", "[c-a]", "[\\d]", "[\\w-a]", "(", ")",
  "(?:", "(?=", "(?!", "(?<=", "(?<!", "(?<n>", "\\k<n>", "\\1", "\\2", "|", "^", "$", "\\b",
  "\\B", "\\d", "\\w", "\\s", "\\.", "\\", "\\x41", "\\u0062", "\\0", "\\12", "\\c", "\\cA",
  "[\\cA]", "[\\c1]", "\\/", "/", "-", "[]", "[^]", "(?i:", "(?-i:", "A", "É", "é", "[à-é]", "\\k"];
var reInputs = ["", "a", "b", "ab", "aab", "ba", "abab", "A", "AB", "\n", "a\nb", "é", "É", "1", "12",
  "_", " ", "a.b", "/", "c", "abc", "\u0001", "\u000a", "-", "ǅ", "ß", "ſ", "K"];
var regexps = [];
while (regexps.length < 1500) {
  var src = build(reTokens, 1, 6);
  var flags = chance(0.3) ? "i" : "";
  var entry = { source: src, flags: flags, valid: true };
  var re;
  try {
    re = new RegExp(src, flags);
  } catch (e) {
    entry.valid = false;
    regexps.push(entry);
    continue;
  }
  entry.inputs = [];
  var res = "";
  for (var i = 0; i < 8; i++) {
    var input = chance(0.5) ? pick(reInputs) : build(reInputs, 1, 3);
    entry.inputs.push(input);
    res += re.test(input) ? "1" : "0";
  }
  entry.results = res;
  regexps.push(entry);
}

// --- minimatch -------------------------------------------------------------

var patTokens = ["a", "b", "c", "abc", "foo", "x.js", ".", "..", "/", "/", "/", "*", "*", "**",
  "**", "?", "[a-c]", "[!a]", "[^b]", "[[:alpha:]]", "[]", "]", "[", "[]a]", "[a-]", "+(", "@(",
  "!(", "?(", "*(", "|", ")", "(", "{a,b}", "{1..3}", "{", "}", ",", "\\", "\\*", "\\[", "#",
  "!", " ", "é", "Ä", "$", "^", "{a..c}", "-", ".js", "x", "A", "[\\w-a]", "[z-a]", "\\\\",
  "*.!(x)", "!(*.js)", "+(a|b)", "@(a|.b)", "{.x,y}", "[.]", "\t", "**/", "/**", "\\!", "\\#"];
var segVocab = ["a", "b", "c", "abc", "foo", "x.js", ".x", ".", "..", "", "A", "B", "é", "É", "Ä",
  "ä", "ab", "ba", "aa", "x", "y", "1", "2", "3", "a.b", "b.js", "[a]", "]", "*", "?", "-", " ",
  ".js", "a b", "+(a|b)", "(", ")", "|", "\\", "#a", "!a", "[undefined", "a]xb", "_", "z"];
var optionNames = ["dot", "nocase", "nocomment", "nonegate", "flipNegate", "noext", "nobrace",
  "noglobstar", "matchBase", "nonull", "partial"];

function randomPath() {
  var n = 1 + Math.floor(rand() * 4);
  var segs = [];
  for (var i = 0; i < n; i++) segs.push(pick(segVocab));
  var p = segs.join("/");
  if (chance(0.1)) p = "/" + p;
  if (chance(0.1)) p += "/";
  return p;
}

// derivedPath fills in an expansion of the pattern, so that many paths match.
function derivedPath(pattern) {
  var set = minimatch.braceExpand(pattern);
  if (!set.length) return randomPath();
  var p = pick(set);
  p = p.replace(/\*\*/g, function () { return pick(["", "a", "a/b", ".x", "b/c/d"]); });
  p = p.replace(/[+@!?*]\(([^)]*)\)/g, function (_, body) { return pick(body.split("|").concat(["", "zz"])); });
  p = p.replace(/\*/g, function () { return pick(["", "x", "ab", ".", "é"]); });
  p = p.replace(/\?/g, function () { return pick(["a", "b", ".", "Z"]); });
  p = p.replace(/\[[^\]]*\]/g, function () { return pick(["a", "b", "z", ":", "-", "."]); });
  p = p.replace(/\\(.)/g, "$1");
  if (chance(0.2)) p = p.toUpperCase();
  return p;
}

function describePart(x) {
  if (x === minimatch.GLOBSTAR) return "G";
  if (typeof x === "string") return "L:" + x;
  if (x._src === undefined) return "X";
  return "R:" + x.source;
}

var cases = [];
var seenCases = {};
function addCase(pattern, options) {
  var key = JSON.stringify([pattern, options]);
  if (seenCases[key]) return;
  seenCases[key] = true;
  var c = { pattern: pattern, options: options };
  var mm;
  try {
    mm = new minimatch.Minimatch(pattern, Object.assign({}, options));
  } catch (e) {
    c.error = e.message;
    cases.push(c);
    return;
  }
  c.negate = mm.negate;
  c.comment = mm.comment;
  c.empty = mm.empty;
  c.globSet = mm.globSet || null;
  c.set = mm.set.map(function (s) { return s.map(describePart); });
  var re = mm.makeRe();
  c.makeRe = re ? re.source : null;
  c.paths = [];
  var results = "";
  for (var i = 0; i < 14; i++) c.paths.push(i < 7 ? derivedPath(pattern) : randomPath());
  c.paths.forEach(function (p) {
    try {
      results += minimatch(p, pattern, Object.assign({}, options)) ? "1" : "0";
    } catch (e) {
      results += "E";
    }
  });
  c.results = results;
  try {
    c.list = minimatch.match(c.paths, pattern, Object.assign({}, options));
  } catch (e) {
    c.list = null;
  }
  cases.push(c);
}

while (cases.length < 2200) {
  var pattern = build(patTokens, 1, 6);
  if (chance(0.15)) pattern = "!" + pattern;
  if (chance(0.05)) pattern = " " + pattern + " ";
  var options = {};
  optionNames.forEach(function (name) {
    if (chance(0.12)) options[name] = true;
  });
  addCase(pattern, options);
}
// Hand-picked cases for quirks the random ones may miss.
[["*[**", {}], ["[**", {}], ["*[", {}], ["x[", {}], ["a*]b", {}], ["[]-a]", {}], ["[\\w-a]", {}],
  ["+(a|b", {}], ["+(a\\\\|b", {}], ["*.!(x).!(y|z)", {}], ["a/**/**/**", {}], ["a/**/**/**", { dot: true }],
  ["**/**/**", {}], ["a/**/b/**/c", {}], ["**", { dot: true }], ["**", { noglobstar: true }],
  ["#comment", {}], ["#comment", { nocomment: true }], ["!!a", {}], ["!a", { flipNegate: true }],
  ["", {}], ["   ", {}], ["*.js", { matchBase: true }], ["a/*", { partial: true }],
  ["[[:alpha:]]", {}], ["*(*.js|!(*.json))", {}], ["!(a|b)c", {}], ["@(a)*", { nocase: true }],
  ["{a,b}/{c,d}", {}], ["a/{,b}", {}], ["{,}", {}], ["ǅ*", { nocase: true }], ["ß", { nocase: true }],
  ["[ſ]", { nocase: true }], ["*K*", { nocase: true }], ["[😀-😂]", {}], ["😀*", {}],
  ["x".repeat(65537), {}], ["\\", {}], ["a\\", {}], ["*\\", {}]].forEach(function (c) {
  addCase(c[0], c[1]);
});
[["a", "a/**/**/**"], ["a/", "a/**/**/**"], ["a/b/", "a/**/*"], ["/", "*"], ["/", "a/*"]].forEach(function (c) {
  var key = JSON.stringify([c[1], {}]);
  cases.forEach(function (k) {
    if (JSON.stringify([k.pattern, k.options]) === key && k.paths) {
      k.paths.push(c[0]);
      try {
        k.results += minimatch(c[0], c[1]) ? "1" : "0";
      } catch (e) {
        k.results += "E";
      }
      try {
        k.list = minimatch.match(k.paths, k.pattern, Object.assign({}, k.options));
      } catch (e) {
        k.list = null;
      }
    }
  });
});

fs.writeFileSync(path.join(out, "braces.json"), JSON.stringify(braces));
fs.writeFileSync(path.join(out, "regexp.json"), JSON.stringify(regexps));
fs.writeFileSync(path.join(out, "minimatch.json"), JSON.stringify(cases));
console.log("braces", braces.length, "regexps", regexps.length, "minimatch", cases.length);
