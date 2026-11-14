#!/usr/bin/env node
// Fails when a migration prefix is shared by more files than the allowlist records.
//
// The number in the filename is how a migration is named, and 69 prefixes were already
// shared when this check landed. Initdb and cmd/migrate order by the whole filename and
// record that filename, so those files stay as they are — renumbering one would make a
// database that already applied it apply a different file under the old name. The
// allowlist is that history: each prefix mapped to how many files already use it. A
// prefix used once is not listed. One more file on 0144, or two new files sharing a
// fresh number, is a count the allowlist does not name, and this fails.
//
// Run from the repository root:
//   node scripts/check-migration-numbers.mjs
//   pnpm check:migration-numbers

import { readdirSync } from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

const MIGRATIONS_DIR = 'migrations';

// Existing duplicate prefixes and how many files already use each one. A new collision
// fails until this map is updated in the same change — the escape hatch for a corrective
// file that has to sort beside an immutable migration (internal/platform/db/AGENTS.md).
export const ALLOWED_DUPLICATE_COUNTS = {
  '0009': 2,
  '0017': 2,
  '0022': 2,
  '0026': 2,
  '0031': 2,
  '0034': 4,
  '0036': 2,
  '0037': 2,
  '0041': 4,
  '0042': 2,
  '0049': 2,
  '0056': 2,
  '0057': 2,
  '0059': 2,
  '0060': 2,
  '0062': 2,
  '0074': 2,
  '0076': 2,
  '0077': 2,
  '0078': 2,
  '0079': 3,
  '0082': 2,
  '0085': 2,
  '0086': 2,
  '0087': 2,
  '0091': 2,
  '0092': 3,
  '0093': 2,
  '0094': 2,
  '0096': 2,
  '0109': 2,
  '0116': 2,
  '0119': 2,
  '0120': 2,
  '0123': 2,
  '0128': 4,
  '0129': 2,
  '0130': 2,
  '0132': 2,
  '0134': 2,
  '0135': 5,
  '0139': 2,
  '0140': 2,
  '0144': 7,
  '0152': 2,
  '0154': 3,
  '0157': 2,
  '0160': 3,
  '0169': 2,
  '0171': 2,
  '0174': 2,
  '0177': 2,
  '0183': 2,
  '0196': 2,
  '0207': 2,
  '0212': 2,
  '0214': 2,
  '0220': 2,
  '0221': 4,
  '0222': 2,
  '0228': 2,
  '0229': 2,
  '0231': 2,
  '0233': 2,
  '0243': 2,
  '0248': 3,
  '0251': 3,
  '0258': 2,
  '0259': 3,
};

export function checkMigrationNumbers(filenames, allowlist) {
  const groups = new Map();
  const errors = [];
  for (const name of filenames) {
    const base = name.includes('/') ? name.slice(name.lastIndexOf('/') + 1) : name;
    const match = /^(\d+)_/.exec(base);
    if (!match) {
      errors.push(`${base} has no numeric prefix (expected NNNN_name.sql)`);
      continue;
    }
    const prefix = match[1];
    const files = groups.get(prefix);
    if (files) files.push(base);
    else groups.set(prefix, [base]);
  }

  const seen = new Set();
  for (const prefix of [...groups.keys()].sort()) {
    seen.add(prefix);
    const files = groups.get(prefix).slice().sort();
    const count = files.length;
    const allowed = allowlist[prefix];
    if (count === 1) {
      if (allowed !== undefined) {
        errors.push(
          `${prefix} is used once (${files[0]}) but the allowlist permits ${allowed}; remove the stale entry`,
        );
      }
      continue;
    }
    if (allowed === undefined) {
      errors.push(`${prefix} is used ${count} times and is not allowlisted: ${files.join(', ')}`);
      continue;
    }
    if (allowed !== count) {
      errors.push(
        `${prefix} is used ${count} times; the allowlist permits ${allowed}: ${files.join(', ')}`,
      );
    }
  }

  for (const prefix of Object.keys(allowlist).sort()) {
    if (!/^\d+$/.test(prefix)) {
      errors.push(`allowlist key ${prefix} is not a numeric prefix`);
      continue;
    }
    if (!Number.isInteger(allowlist[prefix]) || allowlist[prefix] < 2) {
      errors.push(`${prefix} allowlist count must be an integer >= 2, got ${allowlist[prefix]}`);
    }
    if (!seen.has(prefix)) {
      errors.push(`${prefix} is allowlisted (${allowlist[prefix]}) but no migration uses it`);
    }
  }

  return { ok: errors.length === 0, errors };
}

function main() {
  let names;
  try {
    names = readdirSync(MIGRATIONS_DIR).filter((f) => f.endsWith('.sql'));
  } catch (err) {
    console.error(`check-migration-numbers: cannot read ${MIGRATIONS_DIR}: ${err.message}`);
    console.error('Run it from the repository root.');
    return 1;
  }
  const { ok, errors } = checkMigrationNumbers(names, ALLOWED_DUPLICATE_COUNTS);
  if (!ok) {
    console.error(`check-migration-numbers: ${errors.length} problem(s):`);
    for (const error of errors) console.error(`  ${error}`);
    console.error(
      '\nA new migration must use a numeric prefix no other file uses. The allowlist is\n' +
        'the duplicate prefixes already applied; do not renumber those. To permit a\n' +
        'corrective file that has to sort beside an immutable migration, update\n' +
        'ALLOWED_DUPLICATE_COUNTS in this script and say why.',
    );
    return 1;
  }
  console.log(
    `check-migration-numbers: ${names.length} files, ${Object.keys(ALLOWED_DUPLICATE_COUNTS).length} allowlisted duplicate prefixes.`,
  );
  return 0;
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  process.exit(main());
}
