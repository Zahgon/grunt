// Records differential fixtures for internal/nopt from nopt 5.0.0. Run with Node 24,
// grunt's node_modules mounted at /g/node_modules, the working directory /w/sub and
// HOME=/home/tester:
//
//   docker run --rm -e TZ=UTC -e HOME=/home/tester \
//     -v <grunt>/node_modules:/g/node_modules:ro -v $PWD:/w -w /w/sub \
//     node:24-bookworm node ../gen.js > cases.json
'use strict';

const path = require('path');
const nopt = require('/g/node_modules/nopt');
const knownOptions = require('/g/node_modules/grunt-known-options');

let seed = process.env.SEED ? Number(process.env.SEED) : 0x0b7105;
function rand() {
  seed |= 0; seed = seed + 0x6D2B79F5 | 0;
  let t = Math.imul(seed ^ seed >>> 15, 1 | seed);
  t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t;
  return ((t ^ t >>> 14) >>> 0) / 4294967296;
}
const int = (n) => Math.floor(rand() * n);
const pick = (a) => a[int(a.length)];
const chance = (p) => rand() < p;

// Type tables are encoded for the Go side as names: "Boolean", "String", "Number",
// "path", "Array", {"lit": value} and {"list": [...]}.
function encType(t) {
  if (t === Boolean) return 'Boolean';
  if (t === String) return 'String';
  if (t === Number) return 'Number';
  if (t === path) return 'path';
  if (t === Array) return 'Array';
  if (Array.isArray(t)) return {list: t.map(encType)};
  return {lit: encValue(t)};
}
function encValue(v) {
  if (v === undefined) return {u: 1};
  if (typeof v === 'number') return {n: Object.is(v, -0) ? '-0' : String(v)};
  if (Array.isArray(v)) return {a: v.map(encValue)};
  return v;
}

const tables = [];

// grunt's own table, built the way lib/grunt/cli.js and grunt-cli build it.
{
  const known = {}, aliases = {};
  Object.keys(knownOptions).forEach((key) => {
    const short = knownOptions[key].short;
    if (short) aliases[short] = '--' + key;
    known[key] = knownOptions[key].type;
  });
  tables.push({name: 'grunt', known, aliases});
}

tables.push({
  name: 'custom',
  known: {
    str: String, num: Number, bool: Boolean, dir: path, arr: Array, numarr: [Number, Array], strarr: [String, Array],
    lit: ['a', 'b', null], union: [String, Boolean], nullonly: null, boolnull: [Boolean, null], nan: NaN,
    one: ['x'], single: [Number], patharr: [path, Array], color: ['always', false, true], level: [Number, 'auto'],
    numnull: [null, Number], strnull: [null, String], zero: 0, bnot: [false, 'yes'],
  },
  aliases: {
    s: ['--str', 'x'], n: ['--num', '5'], b: ['--bool'], x: ['--no-bool'], abc: ['--arr', 'a'], z: ['--other'],
    q: '--quiet', loud: ['--no-quiet'], d: '--dir', m: ['--numarr'], e: [], self: ['--self'], 'l': '--level',
  },
});

const longNames = {
  grunt: ['help', 'base', 'color', 'gruntfile', 'debug', 'stack', 'force', 'tasks', 'npm', 'write', 'verbose', 'version',
    'completion', 'preload', 'he', 'ba', 'col', 'gr', 'd', 'de', 'st', 'f', 'fo', 'ta', 'np', 'wr', 've', 'verb', 'vers',
    'c', 'co', 'p', 'pre', 'unknown', 'foo', 'x', 'require'],
  custom: ['str', 'num', 'bool', 'dir', 'arr', 'numarr', 'strarr', 'lit', 'union', 'nullonly', 'boolnull', 'nan', 'one',
    'single', 'patharr', 'color', 'level', 'numnull', 'strnull', 'zero', 'bnot', 'st', 'nu', 'num', 'boo', 'un', 'ar',
    'numa', 'stra', 'pat', 'quiet', 'other', 'self', 'le', 'ze'],
};
const shortNames = {
  grunt: ['h', 'b', 'd', 'f', 'v', 'V', 'p', 'hv', 'vf', 'dv', 'vd', 'x', 'vx', 'hp'],
  custom: ['s', 'n', 'b', 'x', 'abc', 'ab', 'a', 'z', 'q', 'loud', 'lo', 'd', 'm', 'e', 'sb', 'bx', 'nb', 'self', 'se', 'l', 'qs'],
};
const values = ['true', 'false', 'null', '1', '0', '-1', '1e3', ' 5 ', '0x10', 'abc', 'a', 'b', 'x', 'auto', 'always',
  'yes', '', '~/x', '~/', '~', '../y', '/abs/p', 'rel/../z', '.', 'a b', '-', '--', '---', '-x', '--x=y', '-5', '--5',
  '---z', 'NaN', 'Infinity', 'True', ' null'];
const positional = ['build', 'test', 'default', 'a:b', 'x y'];

function randArgv(table) {
  const argv = [];
  const n = int(8);
  for (let i = 0; i < n; i++) {
    const r = rand();
    if (r < 0.45) {
      const prefix = pick(['--', '--', '--no-', '--NO-', '--no-no-', '-', '---']);
      let tok = prefix + pick(longNames[table]);
      if (chance(0.3)) tok += '=' + pick(values);
      argv.push(tok);
    } else if (r < 0.65) {
      let tok = pick(['-', '-', '--']) + pick(shortNames[table]);
      if (chance(0.2)) tok += '=' + pick(values);
      argv.push(tok);
    } else if (r < 0.9) {
      argv.push(pick(values));
    } else {
      argv.push(pick(positional));
    }
  }
  return argv;
}

const cases = [];
function record(table, argv, slice) {
  const t = tables.find((x) => x.name === table);
  // nopt splits string shorthands in place; give it a fresh copy every time.
  const aliases = JSON.parse(JSON.stringify(t.aliases));
  const original = argv.slice();
  const parsed = nopt(t.known, aliases, argv.slice(), slice);
  const argvRec = parsed.argv;
  delete parsed.argv;
  cases.push({
    table, slice, args: original,
    options: Object.keys(parsed).map((k) => [k, encValue(parsed[k])]),
    remain: argvRec.remain, cooked: argvRec.cooked, original: argvRec.original,
  });
}

for (let i = 0; i < 1500; i++) {
  const table = chance(0.5) ? 'grunt' : 'custom';
  const slice = chance(0.8) ? 2 : int(4);
  const argv = ['node', 'grunt'].concat(randArgv(table));
  record(table, argv, slice);
}
// Hand-picked cases.
[
  ['grunt', ['--verbose', '--gruntfile=/x/Gruntfile.js']], ['grunt', ['--debug']], ['grunt', ['--no-debug']],
  ['grunt', ['--debug', '5']], ['grunt', ['--debug=', 'x']], ['grunt', ['-d', 'false']], ['grunt', ['--no-color']],
  ['grunt', ['--tasks', 'a', '--tasks=b', '--npm']], ['grunt', ['-hv']], ['grunt', ['--hv']], ['grunt', ['-=x', 'y']],
  ['grunt', ['--base', '~/proj']], ['grunt', ['--base']], ['grunt', ['--no-base']], ['grunt', ['--preload', '--verbose']],
  ['grunt', ['--completion']], ['grunt', ['--completion=bash']], ['grunt', ['--foo', '--foo=bar', '--foo']],
  ['grunt', ['--', '--verbose', 'x']], ['grunt', ['---', '-v']], ['grunt', ['--verb', 'build', '--', 'x']],
  ['custom', ['--argv', 'x']], ['custom', ['--argv=1', '--str=a']], ['custom', ['-e', 'x']], ['custom', ['--self']],
].forEach(([table, args]) => record(table, ['node', 'grunt'].concat(args), 2));

process.stdout.write(JSON.stringify({
  cwd: process.cwd(), home: require('os').homedir(),
  tables: tables.map((t) => ({
    name: t.name,
    known: Object.keys(t.known).map((k) => [k, encType(t.known[k])]),
    aliases: Object.keys(t.aliases).map((k) => [k, Array.isArray(t.aliases[k]) ? t.aliases[k] : t.aliases[k].split(/\s+/)]),
  })),
  cases,
}));
