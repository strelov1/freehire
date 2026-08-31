#!/usr/bin/env node
// Lints the migrations a change ADDS, with squawk, in two passes.
//
// Read .squawk.toml first — it holds the argument for why this script exists at all
// instead of a bare `squawk migrations/*.sql`. In short: whether a migration runs inside
// a transaction is a property of the FILE, not of the repository, and squawk has one
// global flag for it; and the applied history carries 1322 findings that are never going
// to be fixed, because rewriting an applied migration is a worse hazard than any of them.
//
// Usage:
//   node scripts/check-migrations.mjs                  # files added/changed vs origin/main
//   node scripts/check-migrations.mjs migrations/*.sql # exactly these
//   node scripts/check-migrations.mjs --all            # the whole history, for an audit

import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

// Resolved relative to THIS FILE, not to the caller's PATH. The lefthook command runs
// `node scripts/check-migrations.mjs` directly — no `pnpm run` wrapper — so
// node_modules/.bin is not on PATH and a bare `squawk` fails for everyone with the hook
// installed. Falls back to PATH for a global install.
const LOCAL_SQUAWK = fileURLToPath(new URL('../node_modules/.bin/squawk', import.meta.url));
const SQUAWK = existsSync(LOCAL_SQUAWK) ? LOCAL_SQUAWK : 'squawk';

const MIGRATIONS_DIR = 'migrations';
const RUNNER_SOURCE = 'internal/platform/migrate/migrate.go';

/**
 * The marker is read out of the runner rather than copied, so the two cannot disagree
 * about its spelling. The *rule* around it still lives in both places and is stated
 * below; a rename of the constant's value stays in one.
 */
function noTxMarker() {
  const src = readFileSync(RUNNER_SOURCE, 'utf8');
  const m = src.match(/const\s+noTxMarker\s*=\s*"([^"]+)"/);
  if (!m) {
    throw new Error(
      `Could not find noTxMarker in ${RUNNER_SOURCE}. The runner decides which migrations ` +
        `run in a transaction; without the marker this check would lint every file under ` +
        `the wrong assumption, and quietly.`,
    );
  }
  return m[1];
}

/**
 * Mirrors hasNoTxMarker in the runner: the marker counts only in the LEADING comment
 * block — blank lines and `--` comments before the first statement. A marker further
 * down is prose about the mechanism, which several migrations contain, and treating it
 * as the opt-out would lint a transactional file as though it were not one.
 */
function isNoTx(file, marker) {
  for (const raw of readFileSync(file, 'utf8').split('\n')) {
    const line = raw.trim();
    if (line === '') continue;
    if (!line.startsWith('--')) return false;
    if (line.includes(marker)) return true;
  }
  return false;
}

/**
 * The merge base to compare against.
 *
 * A base that cannot be resolved is a BROKEN ASSUMPTION, not "nothing changed" — a
 * shallow clone, a detached checkout or a missing remote would otherwise produce an
 * empty file list and a green check that examined nothing, which is the exact failure
 * mode a linter is supposed to remove. So this throws rather than returning [].
 */
function mergeBase() {
  const candidates = ['origin/main'];
  // GitHub sets this on a pull request. Not interpolated into a shell — execFileSync
  // takes an argv array, and git treats an unknown ref as a failed lookup either way.
  if (process.env.GITHUB_BASE_REF) candidates.push(`origin/${process.env.GITHUB_BASE_REF}`);
  candidates.push('main');

  for (const ref of candidates) {
    try {
      return execFileSync('git', ['merge-base', ref, 'HEAD'], {
        encoding: 'utf8',
        stdio: ['ignore', 'pipe', 'ignore'],
      }).trim();
    } catch {
      // Try the next one.
    }
  }
  throw new Error(
    `Could not resolve a merge base against any of ${candidates.join(', ')}. Fetch the ` +
      `main branch (CI needs fetch-depth: 0), or pass the migration files explicitly.`,
  );
}

function changedMigrations() {
  const base = mergeBase();
  // ADDED ONLY. A modified migration is a rule violation in its own right — "never edit
  // an applied migration" — but linting its CONTENT is the wrong response to it: fixing
  // a typo in the comment on 0012 would dump every historical finding in that file into
  // a change that touched a comment, and the useful signal would be the one line nobody
  // reads by then. What this check owes a new migration is a verdict on what it does to
  // a live table; what an edited one needs is a reviewer.
  const out = execFileSync(
    'git',
    ['diff', '--name-only', '--diff-filter=A', base, 'HEAD', '--', `${MIGRATIONS_DIR}/*.sql`],
    { encoding: 'utf8' },
  );
  return out.split('\n').filter(Boolean);
}

function allMigrations() {
  return readdirSync(MIGRATIONS_DIR)
    .filter((f) => f.endsWith('.sql'))
    .sort()
    .map((f) => join(MIGRATIONS_DIR, f));
}

/** Runs one pass and reports whether it found anything. Output goes straight through. */
function squawk(files, { inTransaction }) {
  if (files.length === 0) return false;
  const args = [...files];
  if (inTransaction) args.push('--assume-in-transaction');
  try {
    execFileSync(SQUAWK, args, { stdio: 'inherit' });
    return false;
  } catch (err) {
    // A missing binary FAILS rather than passing quietly. A migration check that
    // reports nothing because the linter was absent is worse than no check: it makes
    // the commit look examined.
    if (err.code === 'ENOENT') {
      console.error('squawk is not installed. Run `pnpm install` at the repository root.');
      process.exit(1);
    }
    return true;
  }
}

const argv = process.argv.slice(2);
const explicit = argv.filter((a) => !a.startsWith('--'));

let files;
if (argv.includes('--all')) {
  files = allMigrations();
} else if (explicit.length > 0) {
  // The hook passes staged files, which include everything else in the commit.
  files = explicit.filter((f) => f.startsWith(`${MIGRATIONS_DIR}/`) && f.endsWith('.sql'));
} else {
  files = changedMigrations();
}

// A path that no longer exists is a rename or a delete carried in by the hook's staged
// file list, not something to lint — and reading it would end the run in a stack trace
// rather than in a verdict.
files = files.filter((f) => existsSync(f));

if (files.length === 0) {
  console.log('check-migrations: no migrations to check.');
  process.exit(0);
}

const marker = noTxMarker();
const noTx = files.filter((f) => isNoTx(f, marker));
const inTx = files.filter((f) => !noTx.includes(f));

console.log(
  `check-migrations: ${inTx.length} transactional, ${noTx.length} outside a transaction ` +
    `(marker: "${marker}").`,
);

// Both passes always run — an early exit on the first would hide the second's findings
// behind a fix for the first's.
const failedInTx = squawk(inTx, { inTransaction: true });
const failedNoTx = squawk(noTx, { inTransaction: false });

if (failedInTx || failedNoTx) {
  console.error(
    '\nA finding above is about the migration this change ADDS, not about the history.\n' +
      'Fix the statement, or put `-- squawk-ignore <rule-name>` on the line before it with\n' +
      'the argument in a comment beside it. Never edit an applied migration to satisfy this.',
  );
  process.exit(1);
}
