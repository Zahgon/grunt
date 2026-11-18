// dump.js prints iconv-lite's merged encoding table (encodings/index.js) as
// JSON for gen/main.go. It is fed to `node -` on stdin; process.argv[2] is the
// iconv-lite package directory.
//
// Each key maps to one of:
//   {"alias": "<target>"}  string entries (aliases, possibly chained)
//   {"sbcs": "<chars>"}    {type: "_sbcs", chars} single-byte definitions
//   {"codec": true}        everything else (codec functions and option
//                          objects), which the Go package implements by hand
"use strict";

var path = require("path");
var encodings = require(path.resolve(process.argv[2], "encodings"));

var out = {};
Object.keys(encodings).forEach(function (name) {
    var def = encodings[name];
    if (typeof def === "string") {
        out[name] = { alias: def };
    } else if (typeof def === "object" && def.type === "_sbcs") {
        var extra = Object.keys(def).filter(function (k) { return k !== "type" && k !== "chars"; });
        if (extra.length > 0)
            throw new Error("sbcs entry '" + name + "' has unsupported options: " + extra.join(", "));
        out[name] = { sbcs: def.chars };
    } else {
        out[name] = { codec: true };
    }
});
process.stdout.write(JSON.stringify(out));
