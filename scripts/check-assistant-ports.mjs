#!/usr/bin/env node
// Checks that the extension's verbatim ports of the web assistant are still verbatim
// (extension/AGENTS.md, "Conventions").
//
// Both sides fold the same server stream through the same reducer, so a port that drifts
// is not a style problem: the server began sending `queued`, `attachments` and `system`
// notes and the extension's copy carried none of them, with every test green on both
// sides — each was tested against the file beside it, and nothing compared the two.
//
// The check compares the files after rewriting import specifiers: the web reaches a
// sibling as `#lib/assistant/x.js`, the extension as `./x`, and that is the only
// difference a port is allowed to introduce. A pair that is NOT a verbatim port says so
// below with its reason, and the check fails once that reason stops being true.
//
// Run from the repository root:
//   node scripts/check-assistant-ports.mjs
//   pnpm check:ports
//   node scripts/check-assistant-ports.mjs --self-test

import assert from 'node:assert/strict';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

const WEB = 'web/src/lib/assistant';
const EXTENSION = 'extension/lib/assistant';

/** `name` is the file stem; its test (`<name>.test.ts`) is compared too. */
export const PORTS = [
  { name: 'sse' },
  { name: 'wire' },
  { name: 'chat' },
  { name: 'deck' },
  // Not a port, though extension/AGENTS.md once grouped it with the others: the web labels
  // a call through its i18n messages (#lib/i18n, tool-formatters.messages.ts, a `locale`
  // argument on every function); the extension through a flat English LABELS map, which
  // also carries `read_current_page` — a tool that only exists in a browsing session. The
  // web additionally exports parseConfirmationRequest, for a Yes/No UI the panel lacks.
  // Aligning either would be a feature, not a tidy-up.
  { name: 'tool-formatters', diverges: true },
];

/** Rewrites the one thing the two sides spell differently: how they import a sibling. */
export function normalize(source, side) {
  return side === 'web'
    ? source.replaceAll("'#lib/assistant/", "'./").replaceAll(/'\.\/([\w.-]+)\.js'/g, "'./$1'")
    : source;
}

// Reads from disk. A file that is not there reads as `undefined`, which is what lets
// `driftedPorts` skip a test one side does not have; readFileSync would throw ENOENT and
// bypass that skip. The main port files are asserted present by the caller, so nothing
// silently passes for being missing.
export const readSource = (path) => {
  try {
    return readFileSync(path, 'utf8');
  } catch (e) {
    if (e.code === 'ENOENT') return undefined;
    throw e;
  }
};

/** The pairs that are not what they claim to be. An empty list means every port is aligned. */
export function driftedPorts(pairs, read = readSource) {
  const failed = [];
  for (const pair of pairs) {
    // A port is the source file; it cannot be absent on either side. Only its test is optional.
    for (const side of [WEB, EXTENSION]) {
      if (read(join(side, `${pair.name}.ts`)) === undefined) throw new Error(`${side}/${pair.name}.ts is missing`);
    }
    for (const file of [pair.name, `${pair.name}.test`]) {
      const webPath = join(WEB, `${file}.ts`);
      const extensionPath = join(EXTENSION, `${file}.ts`);
      // The test is compared where both sides have one; a port need not bring its tests.
      if (read(extensionPath) === undefined || read(webPath) === undefined) continue;
      const same = normalize(read(webPath), 'web') === normalize(read(extensionPath), 'extension');
      if (pair.diverges ? same : !same) failed.push({ file, same });
    }
  }
  return failed;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  if (process.argv.includes('--self-test')) {
    assert.equal(normalize("import { x } from '#lib/assistant/wire.js';", 'web'), "import { x } from './wire';");
    assert.equal(normalize("import { x } from '#lib/i18n/t.js';", 'web'), "import { x } from '#lib/i18n/t.js';");
    assert.equal(normalize("import { x } from './wire';", 'extension'), "import { x } from './wire';");

    // A reader over a hand-written tree; a path it does not hold reads as absent.
    const over = (files) => (path) => files[path];
    const both = (web, extension) => ({ [`${WEB}/a.ts`]: web, [`${EXTENSION}/a.ts`]: extension });
    // A port that differs fails, one that matches passes, and a declared divergence that
    // no longer diverges fails too — so the reason beside it cannot outlive the fact.
    assert.deepEqual(driftedPorts([{ name: 'a' }], over(both('a', 'a'))), []);
    assert.deepEqual(driftedPorts([{ name: 'a' }], over(both("from '#lib/assistant/b.js'", "from './b'"))), []);
    assert.equal(driftedPorts([{ name: 'a' }], over(both('a', 'b'))).length, 1);
    assert.deepEqual(driftedPorts([{ name: 'a', diverges: true }], over(both('a', 'b'))), []);
    assert.equal(driftedPorts([{ name: 'a', diverges: true }], over(both('a', 'a'))).length, 1);
    // The test file drifts on its own.
    const testOnly = { ...both('a', 'a'), [`${WEB}/a.test.ts`]: 't', [`${EXTENSION}/a.test.ts`]: 'u' };
    assert.deepEqual(
      driftedPorts([{ name: 'a' }], over(testOnly)).map((f) => f.file),
      ['a.test'],
    );

    // The same function over the real reader and a real directory, because the reader is
    // where this check once lied: readFileSync throws for a missing file, so a test that
    // only exists on one side has to read as absent rather than crash the check.
    const cwd = process.cwd();
    const scratch = mkdtempSync(join(tmpdir(), 'assistant-ports-'));
    try {
      process.chdir(scratch);
      mkdirSync(WEB, { recursive: true });
      mkdirSync(EXTENSION, { recursive: true });
      writeFileSync(`${WEB}/a.ts`, 'a');
      writeFileSync(`${EXTENSION}/a.ts`, 'a');
      // No test on either side: the pair is aligned, nothing is compared.
      assert.deepEqual(driftedPorts([{ name: 'a' }]), []);
      // A test on one side only is skipped, not a crash and not a drift.
      writeFileSync(`${WEB}/a.test.ts`, 't');
      assert.deepEqual(driftedPorts([{ name: 'a' }]), []);
      // A port that is there on one side only is an error with the path in it, not an ENOENT.
      assert.throws(() => driftedPorts([{ name: 'b' }]), /lib\/assistant\/b\.ts is missing/);
    } finally {
      process.chdir(cwd);
      rmSync(scratch, { recursive: true, force: true });
    }
  } else {
    let failed = [];
    try {
      failed = driftedPorts(PORTS);
    } catch (e) {
      // A port file that is not there is reported like a drift, not as a stack trace.
      console.error(e instanceof Error ? e.message : String(e));
      process.exitCode = 1;
    }
    for (const { file, same } of failed) {
      console.error(
        same
          ? `${WEB}/${file}.ts and ${EXTENSION}/${file}.ts are now identical: drop the pair's \`diverges\` entry in scripts/check-assistant-ports.mjs.`
          : `${EXTENSION}/${file}.ts has drifted from ${WEB}/${file}.ts. Re-port it, keeping only the import paths.`,
      );
    }
    if (failed.length) process.exitCode = 1;
  }
}
