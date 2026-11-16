import { execFileSync } from 'node:child_process';
import { readdirSync, readFileSync } from 'node:fs';
import assert from 'node:assert/strict';

// Retain protection for artifacts still present on deployment hosts.
const retired = {
  'backfill-company-info': 'Replaced by focused company backfills.',
  'backfill-engagement': 'Removed engagement backfill.',
  'backfill-justjoin': 'Removed source backfill.',
  'backfill-requirements': 'Requirements now use the queue.',
  'discord-register-commands': 'Removed registration worker.',
  'hire-api': 'Previous server binary name.',
  prune: 'Removed hard-deletion worker.',
  reslug: 'Completed company slug migration.',
  'similar-backfill': 'Removed similarity backfill.',
};
function validate(patterns, targets, ignored) {
  const errors = [];
  const entries = patterns.filter((line) => /^\/[\w-]+$/.test(line)).map((line) => line.slice(1));
  for (const name of new Set(entries)) {
    if (entries.filter((entry) => entry === name).length > 1) errors.push(`Duplicate /${name}`);
    if (!targets.includes(name) && !(name in retired)) errors.push(`Unexpected binary /${name}`);
  }
  for (const name of targets) if (!entries.includes(name)) errors.push(`Missing /${name}`);
  if (ignored.trim()) errors.push(`Tracked files are ignored:\n${ignored}`);
  return errors;
}
if (process.argv.includes('--self-test')) {
  assert.deepEqual(validate(['/server'], ['server'], ''), []);
  assert.equal(validate(['/server', '/server'], ['server'], '').length, 1);
  assert.equal(validate([], ['server'], '').length, 1);
  assert.equal(validate(['/unknown'], [], '').length, 1);
  assert.equal(validate([], [], 'tracked.txt').length, 1);
  assert.deepEqual(validate(['/prune'], [], ''), []);
} else {
  const targets = readdirSync('cmd', { withFileTypes: true }).filter((entry) => entry.isDirectory()).map((entry) => entry.name);
  const errors = validate(readFileSync('.gitignore', 'utf8').split(/\r?\n/), targets,
    execFileSync('git', ['ls-files', '-ci', '--exclude-standard'], { encoding: 'utf8' }));
  if (errors.length) { console.error(errors.join('\n')); process.exitCode = 1; }
}
