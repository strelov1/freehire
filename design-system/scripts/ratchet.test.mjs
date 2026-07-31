import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { beforeEach, describe, expect, it } from 'vitest';
import { ratchet } from './ratchet.mjs';

let baselinePath;

beforeEach(() => {
  baselinePath = join(mkdtempSync(join(tmpdir(), 'ratchet-')), 'baseline.json');
});

function baseline(contents) {
  writeFileSync(baselinePath, JSON.stringify(contents, null, 2));
}

function read() {
  return JSON.parse(readFileSync(baselinePath, 'utf-8'));
}

describe('ratchet', () => {
  it('passes when every count equals its baseline', () => {
    baseline({ button: 47, dialog: 0 });

    const { ok } = ratchet({ counts: { button: 47, dialog: 0 }, baselinePath, direction: 'up' });

    expect(ok).toBe(true);
  });

  // The whole point of the direction: for adoption a fall is the regression, and
  // for violations a rise is. Both are the same comparison with the words swapped.
  it('fails a fall when higher is better, naming both numbers', () => {
    baseline({ button: 47 });

    const { ok, lines } = ratchet({ counts: { button: 46 }, baselinePath, direction: 'up' });

    expect(ok).toBe(false);
    expect(lines.join('\n')).toMatch(/button.*47.*46/);
  });

  it('fails a rise when lower is better', () => {
    baseline({ 'colour literal': 64 });

    const { ok, lines } = ratchet({
      counts: { 'colour literal': 65 },
      baselinePath,
      direction: 'down',
    });

    expect(ok).toBe(false);
    expect(lines.join('\n')).toMatch(/colour literal.*64.*65/);
  });

  // A ratchet that absorbs improvements silently drifts until it asserts nothing:
  // the baseline sits at 250 while reality is 40, and a regression back to 250 is
  // green. So an improvement is red too — it just asks to be recorded.
  it('fails an improvement and asks for the baseline to be rewritten', () => {
    baseline({ dialog: 0 });

    const { ok, lines } = ratchet({ counts: { dialog: 3 }, baselinePath, direction: 'up' });

    expect(ok).toBe(false);
    expect(lines.join('\n')).toMatch(/--update/);
  });

  it('never writes the baseline without the update flag', () => {
    baseline({ dialog: 0 });

    ratchet({ counts: { dialog: 3 }, baselinePath, direction: 'up' });

    expect(read()).toEqual({ dialog: 0 });
  });

  it('writes exactly the measured counts with the update flag, and passes', () => {
    baseline({ dialog: 0, button: 47 });

    const { ok } = ratchet({
      counts: { dialog: 3, button: 47 },
      baselinePath,
      direction: 'up',
      update: true,
    });

    expect(ok).toBe(true);
    expect(read()).toEqual({ dialog: 3, button: 47 });
  });

  // A primitive added to the package has no baseline entry yet. Zero is the
  // honest starting point — it is genuinely used nowhere — so a new one at zero
  // is not news, and one that arrives already used is.
  it('treats a key missing from the baseline as zero', () => {
    baseline({});

    expect(ratchet({ counts: { chip: 0 }, baselinePath, direction: 'up' }).ok).toBe(true);
    expect(ratchet({ counts: { chip: 2 }, baselinePath, direction: 'up' }).ok).toBe(false);
  });

  // The mirror of the allowlist assertion already in check-token-coverage: an
  // entry nobody measures any more is an allowance outliving its reason.
  it('fails a baseline entry that nothing measures', () => {
    baseline({ button: 47, retired: 3 });

    const { ok, lines } = ratchet({ counts: { button: 47 }, baselinePath, direction: 'up' });

    expect(ok).toBe(false);
    expect(lines.join('\n')).toMatch(/retired/);
  });

  it('starts from empty when the baseline file does not exist', () => {
    const missing = join(mkdtempSync(join(tmpdir(), 'ratchet-')), 'absent.json');

    const { ok } = ratchet({ counts: { button: 0 }, baselinePath: missing, direction: 'up' });

    expect(ok).toBe(true);
  });
});
