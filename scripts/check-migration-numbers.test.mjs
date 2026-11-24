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

// On a push to main the base is the previous tip, so the result is the file the merge
// added and the one to rename. `git ls-tree` without -r names the `migrations` DIRECTORY,
// which makes every file look new; this pins the recursive form. An unusable base must
// yield an empty list rather than throw: the verdict does not depend on it.
test('addedMigrations lists the files a base does not hold, and nothing for a bad base', (t) => {
  const dir = mkdtempSync(join(tmpdir(), 'migration-numbers-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const git = (...args) =>
    execFileSync('git', args, { cwd: dir, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] });
  mkdirSync(join(dir, 'migrations'));
  git('init', '-q');
  git('config', 'user.email', 't@t');
  git('config', 'user.name', 't');

  writeFileSync(join(dir, 'migrations', '0001_init.sql'), 'select 1;');
  git('add', '.');
  git('commit', '-qm', 'init');
  const base = git('rev-parse', 'HEAD').trim();

  writeFileSync(join(dir, 'migrations', '0002_a.sql'), 'select 1;');
  writeFileSync(join(dir, 'migrations', '0002_notes.txt'), 'not a migration');

  const cwd = process.cwd();
  process.chdir(dir);
  try {
    assert.deepEqual(addedMigrations(base), ['0002_a.sql']);
    assert.deepEqual(addedMigrations(''), []);
    assert.deepEqual(addedMigrations('0'.repeat(40)), []);
    assert.deepEqual(addedMigrations('not-a-ref'), []);
  } finally {
    process.chdir(cwd);
  }
});
