// Writes the copy of the compiled contract that ogen generates internal/api/apigen from:
// the same document without its security requirements.
//
// The contract states who may call each operation (`@useAuth`) so the API reference can say
// it, but the gates are the Fiber middleware in front of every route (mw.cookie, mw.key).
// Given a security requirement, ogen would generate a SecurityHandler and check credentials
// itself, a second gate that disagrees with the first. Runs in `make api-contract`.
//
// A schema marked `x-raw-json` is typed for readers but written by the handlers as raw JSON
// (see IgnoredValues in typespec/common.tsp), so every reference to it reaches ogen as `{}`,
// which ogen reads as jx.Raw.
//
//   node scripts/openapi-ogen-input.mjs <in> <out>
import { readFileSync, writeFileSync } from 'node:fs';

const [, , input, output] = process.argv;
const spec = JSON.parse(readFileSync(input, 'utf8'));
delete spec.security;
delete spec.components?.securitySchemes;
for (const item of Object.values(spec.paths)) for (const op of Object.values(item)) delete op.security;
const schemas = spec.components?.schemas ?? {};
const raw = new Set(Object.keys(schemas).filter((name) => schemas[name]['x-raw-json']));
for (const name of raw) delete schemas[name];
(function untype(node) {
  if (!node || typeof node !== 'object') return;
  for (const [k, v] of Object.entries(node)) {
    if (raw.has(v?.$ref?.split('/').pop())) node[k] = {};
    else untype(v);
  }
})(spec);
writeFileSync(output, JSON.stringify(spec));
