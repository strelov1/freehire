import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import {
  ALLOWED_DUPLICATE_COUNTS,
  addedMigrations,
  checkMigrationNumbers,
} from './check-migration-numbers.mjs';

test('the migration tree matches the allowlist', () => {
  const files = readdirSync('migrations').filter((f) => f.endsWith('.sql'));
  const result = checkMigrationNumbers(files, ALLOWED_DUPLICATE_COUNTS);
  assert.equal(result.ok, true, result.errors.join('\n'));
});

test('two new files sharing a fresh number fail', () => {
  const result = checkMigrationNumbers(['0264_a.sql', '0264_b.sql'], {});
  assert.equal(result.ok, false);
  assert.match(result.errors.join('\n'), /0264 is used 2 times and is not allowlisted/);
});

test('one more file on an allowlisted prefix fails', () => {
  const result = checkMigrationNumbers(
    ['0144_a.sql', '0144_b.sql', '0144_c.sql'],
    { '0144': 2 },
  );
  assert.equal(result.ok, false);
  assert.match(result.errors.join('\n'), /0144 is used 3 times; the allowlist permits 2/);
});

test('a unique new prefix passes beside an allowlisted duplicate', () => {
  const result = checkMigrationNumbers(
    ['0144_a.sql', '0144_b.sql', '0264_cascade_fk_indexes.sql'],
    { '0144': 2 },
  );
  assert.equal(result.ok, true, result.errors.join('\n'));
});

test('a stale allowlist entry fails', () => {
  const result = checkMigrationNumbers(['0144_a.sql', '0144_b.sql'], { '0144': 2, '0999': 2 });
  assert.equal(result.ok, false);
  assert.match(result.errors.join('\n'), /0999 is allowlisted \(2\) but no migration uses it/);
});

test('a lower count than the allowlist fails', () => {
  const result = checkMigrationNumbers(['0144_a.sql', '0144_b.sql'], { '0144': 7 });
  assert.equal(result.ok, false);
  assert.match(result.errors.join('\n'), /0144 is used 2 times; the allowlist permits 7/);
});

test('a filename without a numeric prefix fails', () => {
  const result = checkMigrationNumbers(['notes.sql'], {});
  assert.equal(result.ok, false);
  assert.match(result.errors.join('\n'), /notes\.sql has no numeric prefix/);
});

// On a push to main the base is the previous tip, so the result is what the merge added,
// restricted to the numbers that failed. `git ls-tree` without -r names the `migrations`
// DIRECTORY, which would make every file look new. An unusable base yields an empty list
// rather than throwing: the verdict does not depend on it.
test('addedMigrations names what a change added on the failing numbers, nothing else', (t) => {
  const dir = mkdtempSync(join(tmpdir(), 'migration-numbers-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const git = (...args) =>
    execFileSync('git', args, {
      cwd: dir,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
      env: { ...process.env, GIT_CONFIG_GLOBAL: '/dev/null', GIT_CONFIG_SYSTEM: '/dev/null' },
    });
  mkdirSync(join(dir, 'migrations'));
  git('init', '-q');
  git('config', 'user.email', 't@t');
  git('config', 'user.name', 't');
  git('config', 'core.hooksPath', '/dev/null');
  const commit = (msg) => {
    git('add', '-A');
    git('commit', '-qm', msg);
    return git('rev-parse', 'HEAD').trim();
  };

  writeFileSync(join(dir, 'migrations', '0009_a.sql'), 'select 1;');
  writeFileSync(join(dir, 'migrations', '0009_notes.txt'), 'not a migration');
  const base = commit('two files share 0009 at the base');

  const cwd = process.cwd();
  process.chdir(dir);
  try {
    // Nothing added: no file to name, whatever the failing numbers are.
    assert.deepEqual(addedMigrations(base, new Set(['0009'])), []);

    // A later change adds a colliding file AND an unrelated one. Only the colliding file
    // is named: while a duplicate sits on main every later push fails for it, and one that
    // merely adds an unrelated migration must not be told to rename that.
    writeFileSync(join(dir, 'migrations', '0009_c.sql'), 'select 1;');
    writeFileSync(join(dir, 'migrations', '0316_unrelated.sql'), 'select 1;');
    const failing = new Set(['0009']);
    assert.deepEqual(addedMigrations(base, failing), ['0009_c.sql']);

    // An unusable base yields nothing rather than throwing.
    assert.deepEqual(addedMigrations('', failing), []);
    assert.deepEqual(addedMigrations('0'.repeat(40), failing), []);
    assert.deepEqual(addedMigrations('not-a-ref', failing), []);
  } finally {
    process.chdir(cwd);
  }
});
