// glob.js records glob 7.1.7's glob.sync on randomly generated, seeded
// directory trees, and String#localeCompare(…, 'en') on random strings, for
// the Go differential tests.
//
//   node glob.js <node_modules dir> <scratch dir> <output dir>
//
// Run it as a non-root user so that unreadable directories fail. It writes:
//
//   glob.json      [{tree, cases: [{pattern, options, result | error,
//                   warnings, caseSensitive, nonRoot}]}]
//   collation.json {pairs: [[a, b, sign]], lists: [[sorted strings]]}
//
// A tree is a list of {path, type, target?, mode?} entries (type is file,
// dir, symlink or fifo; mode is an octal string). Every absolute path in
// patterns, options, results and messages is written with the tree's
// directory replaced by @ROOT@ (see unplace). Glob runs with the tree as process.cwd().
// caseSensitive marks a case whose result depends on a case-sensitive file
// system; nonRoot marks one that relies on permissions root ignores.
//
// The fixtures were recorded with Node 24 (node:24-bookworm, user node).
"use strict";

var fs = require("fs");
var path = require("path");
var childProcess = require("child_process");
var modules = process.argv[2];
var scratch = process.argv[3];
var out = process.argv[4];
var glob = require(path.join(modules, "glob"));
var minimatch = require(path.join(modules, "minimatch"));

var seed = 0x61060b;
function rand() {
  seed = (seed + 0x6d2b79f5) | 0;
  var t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
  return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
}
function pick(a) { return a[Math.floor(rand() * a.length)]; }
function chance(p) { return rand() < p; }

var names = ["a", "b", "c", "abc", "x.js", "y.js", "z.txt", "README.md", "file.txt", "sub", "dir",
  "lib", "src", ".hidden", ".git", ".x.js", "with space", "é", "Äb", "日本", "1", "2", "10", "01",
  "-dash", "_under", "~tilde", "+plus", "@at", "a.b.c", "#hash", "[ab]", "{a,b}", "!bang", "*",
  "?q", "B", "Zed", "zed2", "ç.js", "Ω", "яблоко", "é", "node_modules", "test.min.js",
  "a-b", "a_b", "a.b", "A.c", "ab", "aB2"];
var linkNames = ["link", "lnk", "ln-dir", "ln-file", "broken", "loop1", "loop2", "self"];

function lower(s) { return s.toLowerCase(); }

// randomTree builds a tree whose names never differ from a sibling's only
// by case, so it can be recreated on a case-insensitive file system.
function randomTree() {
  var entries = [];
  var dirs = [""];
  function addDir(dir, depth) {
    var count = 1 + Math.floor(rand() * 5);
    var used = {};
    for (var i = 0; i < count; i++) {
      var name = pick(names);
      // APFS also treats canonically equivalent names as the same.
      var key = lower(name.normalize("NFC"));
      if (used[key]) continue;
      used[key] = true;
      var p = dir ? dir + "/" + name : name;
      if (depth < 3 && chance(0.4)) {
        entries.push({ path: p, type: "dir" });
        dirs.push(p);
        addDir(p, depth + 1);
      } else if (chance(0.03)) {
        entries.push({ path: p, type: "fifo" });
      } else {
        entries.push({ path: p, type: "file" });
      }
    }
  }
  addDir("", 0);
  var all = entries.map(function (e) { return e.path; });
  var linkCount = Math.floor(rand() * 3);
  for (var l = 0; l < linkCount; l++) {
    var dir = pick(dirs);
    var name = pick(linkNames);
    var p = dir ? dir + "/" + name : name;
    if (all.some(function (q) { return lower(q) === lower(p); })) continue;
    var target;
    if (name === "broken") target = "does-not-exist";
    else if (name === "self") target = "self";
    else if (name === "loop1") target = "loop2";
    else if (name === "loop2") target = "loop1";
    else {
      var to = pick(all.concat(dirs.filter(Boolean)));
      if (!to) continue;
      target = path.relative(dir || ".", to) || ".";
    }
    entries.push({ path: p, type: "symlink", target: target });
    all.push(p);
  }
  if (chance(0.1)) {
    var locked = pick(dirs.filter(Boolean));
    if (locked) {
      entries.forEach(function (e) {
        if (e.path === locked) e.mode = pick(["0", "300", "600"]);
      });
    }
  }
  return entries;
}

function createTree(root, entries) {
  fs.mkdirSync(root, { recursive: true });
  entries.forEach(function (e) {
    var p = path.join(root, e.path);
    if (e.type === "dir") fs.mkdirSync(p);
    else if (e.type === "file") fs.writeFileSync(p, "");
    else if (e.type === "fifo") childProcess.execFileSync("mkfifo", [p]);
    else fs.symlinkSync(e.target, p);
  });
  // Permissions last, so the tree can be filled first.
  entries.forEach(function (e) {
    if (e.mode) fs.chmodSync(path.join(root, e.path), parseInt(e.mode, 8));
  });
}

function removeTree(root, entries) {
  entries.forEach(function (e) {
    if (e.mode) fs.chmodSync(path.join(root, e.path), 0o755);
  });
  fs.rmSync(root, { recursive: true, force: true });
}

// randomPattern turns an existing path into a pattern.
function randomPattern(entries) {
  var base = pick(entries).path.split("/");
  var segs = base.map(function (s) {
    var r = rand();
    if (r < 0.25) return s;
    if (r < 0.4) return "*";
    if (r < 0.5) return "**";
    if (r < 0.55) return s.replace(/^./, "?");
    if (r < 0.6) return "*" + s.slice(-2);
    if (r < 0.65) return "[a-c]*";
    if (r < 0.7) return "{" + s + "," + pick(names) + "}";
    if (r < 0.74) return ".*";
    if (r < 0.78) return "+(a|b|" + s + ")";
    if (r < 0.82) return "!(" + s + ")";
    if (r < 0.85) return "@(" + s + "|x.js)";
    if (r < 0.88) return pick(names);
    if (r < 0.91) return "*.js";
    if (r < 0.94) return s.toUpperCase();
    return s + "*";
  });
  if (chance(0.15)) segs.splice(Math.floor(rand() * segs.length), 0, "**");
  var p = segs.join("/");
  var r = rand();
  if (r < 0.06) p = "./" + p;
  else if (r < 0.1) p = "@ROOT@/" + p;
  else if (r < 0.13) p = "/" + p;
  else if (r < 0.16) p = "sub/../" + p;
  else if (r < 0.2) p += "/";
  else if (r < 0.23) p = pick(["*", "**", "**/*", "*/", "**/", ".*", "**/.*", "*/**/*.js", "./*", "**/*.{js,md}"]);
  return p;
}

// hasCycle reports a symlink to a directory that contains it: with follow,
// glob would walk it until the system's symlink limit, which differs between
// Linux and macOS (and takes exponential time).
function hasCycle(entries) {
  return entries.some(function (e) {
    if (e.type !== "symlink") return false;
    var dir = path.dirname(e.path);
    var to = path.normalize(path.join(dir, e.target));
    return to === "." || (dir + "/").indexOf(to + "/") === 0;
  });
}

function randomOptions(entries) {
  var o = {};
  ["dot", "mark", "nosort", "nounique", "nonull", "nodir", "follow", "realpath", "absolute",
    "matchBase", "nocase", "noext", "nobrace", "noglobstar", "stat", "nomount", "silent"].forEach(function (k) {
    if (chance(0.1)) o[k] = true;
  });
  if (hasCycle(entries)) delete o.follow;
  if (chance(0.08)) o.strict = false;
  if (chance(0.15)) {
    var dirs = entries.filter(function (e) { return e.type === "dir"; });
    o.cwd = dirs.length && chance(0.8) ? pick(dirs).path : pick(["missing", "x.js", "."]);
  }
  if (chance(0.12)) {
    o.ignore = [];
    for (var i = 0; i < 1 + Math.floor(rand() * 2); i++) {
      o.ignore.push(pick(["**/*.js", "sub/**", "*/**", "a", "**/.git/**", "**/node_modules/**", "dir/*", pick(entries).path, pick(entries).path + "/**"]));
    }
    if (o.ignore.length === 1 && chance(0.5)) o.ignore = o.ignore[0];
  }
  if (chance(0.05)) o.root = "@ROOT@/" + pick(["sub", "dir", "a"]);
  if (chance(0.03)) o.maxDepth = 1;
  return o;
}

function placeholders(v, root) {
  if (typeof v === "string") return v.split("@ROOT@").join(root);
  if (Array.isArray(v)) return v.map(function (x) { return placeholders(x, root); });
  if (v && typeof v === "object") {
    var o = {};
    Object.keys(v).forEach(function (k) { o[k] = placeholders(v[k], root); });
    return o;
  }
  return v;
}

// unplace writes root as @ROOT@. An absolute match under the root option is
// mounted on root again, giving root twice; realpath of such a path fails at
// root's first component, written @TOP@ so it does not depend on where the
// tree lives.
function unplace(s, root) {
  var top = root + "/" + root.split("/")[1] + "'";
  return s.split(root + root).join("@ROOT@@ROOT@").split(top).join("@ROOT@/@TOP@'").split(root).join("@ROOT@");
}

// caseSensitive reports a literal segment that names an entry only
// case-insensitively.
function caseSensitive(pattern, options, entries) {
  var mm = new minimatch.Minimatch(pattern, { nobrace: options.nobrace, noglobstar: options.noglobstar,
    noext: options.noext, nonegate: true, nocomment: true });
  var parts = [];
  mm.set.forEach(function (s) { s.forEach(function (p) { if (typeof p === "string") parts.push(p); }); });
  if (typeof options.cwd === "string") parts = parts.concat(options.cwd.split("/"));
  var entryNames = entries.map(function (e) { return e.path.split("/").pop(); });
  return parts.some(function (p) {
    return entryNames.some(function (n) { return lower(n) === lower(p) && n !== p; });
  });
}

// A fixed tree with hand-picked cases for paths the random ones rarely take.
var fixedTree = [
  { path: "x.js", type: "file" }, { path: "2", type: "file" }, { path: "10", type: "file" },
  { path: "b", type: "file" }, { path: "01", type: "file" }, { path: "a", type: "dir" },
  { path: "a/c.js", type: "file" }, { path: "a/.d", type: "file" }, { path: "lib", type: "dir" },
  { path: "lib/m.js", type: "file" }, { path: "lnk", type: "symlink", target: "lib" },
  { path: "locked", type: "dir", mode: "0" }, { path: "dead", type: "symlink", target: "nowhere" },
];
var fixedCases = [
  ["", {}], ["*.js", { matchBase: true, noglobstar: true }], ["[\\w-a]*", {}], ["*", { cwd: "x.js" }],
  ["*", { cwd: "missing" }], ["*", { nosort: true }], ["*", { nosort: true, nounique: true }],
  ["a/**/**/**", {}], ["**", {}], ["**", { strict: false }], ["**", { strict: false, silent: true }],
  ["**", { follow: true, strict: false }], ["**/*.js", { realpath: true, strict: false }],
  ["dead", { realpath: true }], ["dead", { mark: true, stat: true }], ["*", { mark: true, strict: false }],
  ["*/", {}], ["**/", { strict: false }], ["*", { nodir: true }], ["a/*", { dot: true, mark: true }],
  ["{b,nothing,{x,y}.js}", { nonull: true }], ["{b,nothing}", { nonull: true, nounique: true }],
  ["/x.js", { root: "@ROOT@" }], ["/x.js", { root: "@ROOT@", nomount: true }], ["@ROOT@/a/*.js", {}],
  ["@ROOT@/a/*.js", { absolute: true }], ["./*.js", {}], ["../*/x.js", { cwd: "a" }],
  ["*", { ignore: ["a/**", "1*"] }], ["**", { ignore: "lib/**", strict: false }], ["*.JS", { nocase: true }],
  ["+(a|b)", { noext: true }], ["{a,b}", { nobrace: true }], ["a/**", { noglobstar: true }],
  ["x.js", { maxLength: 3 }], ["x.js", { maxLength: 4 }], ["*", { maxDepth: 0 }],
  ["x".repeat(65537), {}], ["  x.js  ", {}], ["!x.js", {}], ["#x.js", {}],
];

var trees = [];
var caseCount = 0;
for (var t = 0; t <= 140; t++) {
  var fixed = t === 140;
  var entries = fixed ? fixedTree : randomTree();
  var root = path.join(scratch, "tree" + t);
  createTree(root, entries);
  process.chdir(root);
  var cases = [];
  for (var c = 0; c < (fixed ? fixedCases.length : 10); c++) {
    var pattern = fixed ? fixedCases[c][0] : randomPattern(entries);
    var options = fixed ? fixedCases[c][1] : randomOptions(entries);
    // A pattern starting with "/" is mounted on root; keep it inside the tree.
    if (!fixed && pattern[0] === "/") {
      delete options.nomount;
      if (!options.root) options.root = "@ROOT@";
    }
    var rec = { pattern: pattern, options: options };
    var warnings = [];
    var origError = console.error;
    console.error = function (label, er) { warnings.push(unplace(er.message, root)); };
    try {
      var res = glob.sync(placeholders(pattern, root), placeholders(options, root));
      rec.result = res.map(function (r) { return unplace(r, root); });
    } catch (e) {
      rec.error = unplace(e.message, root);
    }
    console.error = origError;
    if (warnings.length) rec.warnings = warnings;
    try {
      if (caseSensitive(pattern, options, entries)) rec.caseSensitive = true;
    } catch (e) {
      // The pattern is too odd to inspect; the run above recorded what glob did.
    }
    if (entries.some(function (e) { return e.mode; })) rec.nonRoot = true;
    cases.push(rec);
    caseCount++;
  }
  process.chdir(scratch);
  removeTree(root, entries);
  trees.push({ tree: entries, cases: cases });
}

// Collation: strings from the scripts and symbols file names use.
var blocks = [[0x20, 0x7e], [0x20, 0x7e], [0x30, 0x39], [0x41, 0x5a], [0x61, 0x7a], [0xa0, 0xff],
  [0x100, 0x17f], [0x180, 0x24f], [0x300, 0x36f], [0x370, 0x3ff], [0x400, 0x4ff], [0x5d0, 0x5ea],
  [0x620, 0x64a], [0x1e00, 0x1eff], [0x2010, 0x2027], [0x20ac, 0x20ac], [0x3041, 0x30ff],
  [0xac00, 0xd7a3], [0xff01, 0xff5e], [0x1f600, 0x1f64f]];
function randomChar() {
  var b = pick(blocks);
  return String.fromCodePoint(b[0] + Math.floor(rand() * (b[1] - b[0] + 1)));
}
function randomString() {
  var s = "";
  var n = Math.floor(rand() * 6);
  for (var i = 0; i < n; i++) s += randomChar();
  return s;
}
var pairs = [];
for (var i = 0; i < 6000; i++) {
  var a = randomString();
  var b = chance(0.5) ? randomString() : a.slice(0, Math.floor(rand() * (a.length + 1))) + randomChar() + a.slice(Math.floor(rand() * (a.length + 1)));
  if (chance(0.1)) b = a.toUpperCase();
  if (chance(0.05)) b = a + pick(["l·", "L·", "й", "آ"]);
  pairs.push([a, b, Math.sign(a.localeCompare(b, "en"))]);
}
var lists = [];
for (var j = 0; j < 40; j++) {
  var list = [];
  for (var k = 0; k < 25; k++) list.push(chance(0.5) ? randomString() : pick(names) + randomString());
  lists.push(list.sort(function (x, y) { return x.localeCompare(y, "en"); }));
}

fs.writeFileSync(path.join(out, "glob.json"), JSON.stringify(trees));
fs.writeFileSync(path.join(out, "collation.json"), JSON.stringify({ pairs: pairs, lists: lists }));
console.log("trees", trees.length, "cases", caseCount, "pairs", pairs.length);
