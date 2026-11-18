// Records differential fixtures for internal/events from EventEmitter2 0.4.14 with
// wildcard: true. Run with Node 24 and grunt's node_modules mounted at /g/node_modules:
//
//   docker run --rm -e TZ=UTC -v <grunt>/node_modules:/g/node_modules:ro -v $PWD:/w -w /w \
//     node:24-bookworm node gen.js > cases.json
'use strict';

const {EventEmitter2} = require('/g/node_modules/eventemitter2');

let seed = process.env.SEED ? Number(process.env.SEED) : 0xe7e275;
function rand() {
  seed |= 0; seed = seed + 0x6D2B79F5 | 0;
  let t = Math.imul(seed ^ seed >>> 15, 1 | seed);
  t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t;
  return ((t ^ t >>> 14) >>> 0) / 4294967296;
}
const int = (n) => Math.floor(rand() * n);
const pick = (a) => a[int(a.length)];
const chance = (p) => rand() < p;

const segPool = ['a', 'b', 'c', '*', '**', '1', '2', 'error', 'newListener', '', 'undefined'];
const emitPool = ['a', 'b', 'c', '1', '2', 'x', '*', '**', 'error', 'undefined'];
function name(pool, delim) {
  const n = 1 + int(chance(0.7) ? 3 : 4);
  const parts = [];
  for (let i = 0; i < n; i++) parts.push(pick(pool));
  return parts.join(delim);
}

// Listener behaviours by name: T throws, A clears the catch-all listeners, every
// listener records its call first.
const listenerNames = ['L0', 'L1', 'L2', 'L3', 'L4', 'T', 'A'];
const showArg = (a) => (typeof a === 'function' ? 'fn:' + a.name : a);

const cases = [];
for (let c = 0; c < 500; c++) {
  const conf = {wildcard: true};
  const delim = chance(0.2) ? ':' : '.';
  if (delim !== '.') conf.delimiter = delim;
  if (chance(0.3)) conf.maxListeners = pick([1, 2, 3]);
  if (chance(0.2)) conf.newListener = true;
  // setMaxListeners writes into the conf object, so record it before any op runs.
  const recordedConf = {delimiter: conf.delimiter || '', maxListeners: conf.maxListeners || 0, newListener: !!conf.newListener};
  const em = new EventEmitter2(conf);
  const calls = [];
  let stderr = '';
  const origError = console.error, origTrace = console.trace;
  console.error = (...a) => { stderr += require('util').format(...a) + '\n'; };
  console.trace = () => { stderr += 'Trace\n'; };

  const fns = {};
  listenerNames.forEach((n) => {
    fns[n] = {[n]: function (...args) {
      calls.push(n + '|' + this.event + '|' + JSON.stringify(args.map(showArg)));
      if (n === 'T') throw new Error('thrown by T on ' + this.event);
      if (n === 'A') this.offAny();
    }}[n];
  });

  const ops = [];
  const nops = 1 + int(25);
  for (let i = 0; i < nops; i++) {
    const r = rand();
    const op = {};
    try {
      if (r < 0.3) {
        op.op = pick(['on', 'on', 'once', 'many']);
        op.event = chance(0.4) ? pick(['a', 'a' + delim + 'b', '*']) : name(segPool, delim);
        op.fn = pick(listenerNames);
        if (op.op === 'many') { op.ttl = pick([0, 1, 2, 3]); em.many(op.event, op.ttl, fns[op.fn]); }
        else em[op.op](op.event, fns[op.fn]);
      } else if (r < 0.6) {
        op.op = 'emit';
        op.event = chance(0.85) ? name(emitPool, delim) : pick(['error', 'newListener']);
        op.args = [1, 'two', 3].slice(0, int(4));
        if (op.event === 'error' && chance(0.5)) op.err = 'bad thing';
        const args = op.err ? [new Error(op.err)].concat(op.args) : op.args;
        op.ret = em.emit(op.event, ...args);
      } else if (r < 0.7) {
        op.op = 'off';
        op.event = name(segPool, delim);
        op.fn = pick(listenerNames);
        em.off(op.event, fns[op.fn]);
      } else if (r < 0.76) {
        op.op = 'onAny';
        op.fn = pick(listenerNames);
        em.onAny(fns[op.fn]);
      } else if (r < 0.8) {
        op.op = 'offAny';
        op.fn = chance(0.6) ? pick(listenerNames) : '';
        em.offAny(op.fn ? fns[op.fn] : undefined);
      } else if (r < 0.85) {
        op.op = 'removeAllListeners';
        if (chance(0.6)) { op.event = name(segPool, delim); em.removeAllListeners(op.event); }
        else em.removeAllListeners();
      } else if (r < 0.95) {
        op.op = 'listeners';
        op.event = name(emitPool, delim);
        op.ret = em.listeners(op.event).map((f) => f.name);
      } else {
        op.op = 'setMaxListeners';
        op.n = pick([0, 1, 2]);
        em.setMaxListeners(op.n);
      }
    } catch (e) {
      op.throws = e.message;
    }
    op.calls = calls.splice(0);
    ops.push(op);
  }
  console.error = origError;
  console.trace = origTrace;
  cases.push({conf: recordedConf, ops, stderr});
}
process.stdout.write(JSON.stringify(cases));
