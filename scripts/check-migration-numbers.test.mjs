import assert from 'node:assert/strict';
import { readdirSync } from 'node:fs';
import { test } from 'node:test';
import { ALLOWED_DUPLICATE_COUNTS, checkMigrationNumbers } from './check-migration-numbers.mjs';

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
