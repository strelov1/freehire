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
//   node scripts/check-migrations.mjs                  # files added vs origin/main, and
//                                                      # no applied one edited
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
 * block — blank lines and `--` comments before the first statement — and only when it is
 * the WHOLE content of its comment line. A marker further down, or named inside a
 * sentence, is prose about the mechanism, which several migrations contain, and treating
 * it as the opt-out would lint a transactional file as though it were not one.
 *
 * The whole-line rule is not a refinement: migrations/0126 opens by explaining that it
 * carries "No `migrate: no-transaction` marker, so the two statements are one
 * transaction", and a substring test read that sentence as the opt-out itself.
 */
function isNoTx(file, marker) {
  for (const raw of readFileSync(file, 'utf8').split('\n')) {
    const line = raw.trim();
    if (line === '') continue;
    if (!line.startsWith('--')) return false;
    if (line.slice(2).trim() === marker) return true;
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

function changedMigrations(base) {
  // ADDED ONLY. A modified migration is a rule violation in its own right — "never edit
  // an applied migration" — but linting its CONTENT is the wrong response to it: fixing
  // a typo in the comment on 0012 would dump every historical finding in that file into
  // a change that touched a comment, and the useful signal would be the one line nobody
  // reads by then. What this check owes a new migration is a verdict on what it does to
  // a live table; an edited one is refused outright by checkImmutability.
  const out = execFileSync(
    'git',
    ['diff', '--name-only', '--diff-filter=A', base, 'HEAD', '--', `${MIGRATIONS_DIR}/*.sql`],
    { encoding: 'utf8' },
  );
  return out.split('\n').filter(Boolean);
}

/**
 * The migrations this change MODIFIES, DELETES or RENAMES that already exist at the merge
 * base — the "never edit an applied migration" rule, which until now only a reviewer
 * enforced. A file the branch itself added is new at the base, so editing it as often as
 * the branch likes is not caught here, which is the point: only what main already holds
 * can have been applied somewhere.
 */
function editedMigrations(base) {
  const out = execFileSync(
    'git',
    ['diff', '--name-status', '--diff-filter=MDR', base, 'HEAD', '--', `${MIGRATIONS_DIR}/*.sql`],
    { encoding: 'utf8' },
  );
  return out.split('\n').filter(Boolean);
}

/**
 * The escape hatch: a `Migration-Edit: <reason>` trailer on any commit in the change. A
 * trailer rather than a pull-request label because it works the same in CI and on a
 * laptop, and because the reason then lives in the history next to the edit instead of
 * in a UI field nobody reads again. Some edits are right — a comment that points at the
 * wrong follow-up, a migration that never ran anywhere because it failed on deploy.
 */
function migrationEditReasons(base) {
  const out = execFileSync(
    'git',
    ['log', '--format=%(trailers:key=Migration-Edit,valueonly,separator=%x0A)', `${base}..HEAD`],
    { encoding: 'utf8' },
  );
  return out.split('\n').map((l) => l.trim()).filter(Boolean);
}

function checkImmutability(base) {
  const edited = editedMigrations(base);
  if (edited.length === 0) return true;

  const reasons = migrationEditReasons(base);
  console.log(`check-migrations: ${edited.length} existing migration(s) changed:`);
  for (const line of edited) console.log(`  ${line.replaceAll('\t', '  ')}`);
  if (reasons.length > 0) {
    console.log('Allowed by a Migration-Edit trailer:');
    for (const r of reasons) console.log(`  ${r}`);
    return true;
  }
  console.error(
    '\nA migration that main already holds may have run on production, and editing it\n' +
      'changes nothing there — the runner records it as applied and never reads it again.\n' +
      'Add a NEW migration instead. If the edit is genuinely right (a comment, a file that\n' +
      'never ran anywhere), say why in a commit trailer:\n\n' +
      '  Migration-Edit: <reason>\n',
  );
  return false;
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
// Immutability is a property of the CHANGE against main, so only the default mode asks
// it. The hook passes staged paths, and a staged edit to a migration this branch added is
// indistinguishable there from an edit to an applied one.
let immutable = true;
if (argv.includes('--all')) {
  files = allMigrations();
} else if (explicit.length > 0) {
  // The hook passes staged files, which include everything else in the commit.
  files = explicit.filter((f) => f.startsWith(`${MIGRATIONS_DIR}/`) && f.endsWith('.sql'));
} else {
  const base = mergeBase();
  immutable = checkImmutability(base);
  files = changedMigrations(base);
}

// A path that no longer exists is a rename or a delete carried in by the hook's staged
// file list, not something to lint — and reading it would end the run in a stack trace
// rather than in a verdict.
files = files.filter((f) => existsSync(f));

if (files.length === 0) {
  console.log('check-migrations: no new migrations to lint.');
  process.exit(immutable ? 0 : 1);
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

if (!immutable) process.exitCode = 1;

if (failedInTx || failedNoTx) {
  console.error(
    '\nA finding above is about the migration this change ADDS, not about the history.\n' +
      'Fix the statement, or put `-- squawk-ignore <rule-name>` on the line before it with\n' +
      'the argument in a comment beside it. Never edit an applied migration to satisfy this.',
  );
  process.exit(1);
}
