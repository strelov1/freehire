// Writes the copy of the compiled contract that ogen generates internal/api/apigen from:
// the same document without its security requirements.
//
// The contract states who may call each operation (`@useAuth`) so the API reference can say
// it, but the gates are the Fiber middleware in front of every route (mw.cookie, mw.key).
// Given a security requirement, ogen would generate a SecurityHandler and check credentials
// itself, a second gate that disagrees with the first. Runs in `make api-contract`.
//
//   node scripts/openapi-ogen-input.mjs <in> <out>
import { readFileSync, writeFileSync } from 'node:fs';

const [, , input, output] = process.argv;
const spec = JSON.parse(readFileSync(input, 'utf8'));
delete spec.security;
delete spec.components?.securitySchemes;
for (const item of Object.values(spec.paths)) for (const op of Object.values(item)) delete op.security;
writeFileSync(output, JSON.stringify(spec));
