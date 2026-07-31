// A counted rule that cannot be satisfied everywhere yet, held at exactly its
// current number.
//
// Any difference from the baseline fails, in both directions. A rise past the
// baseline is the violation; a fall below it is red too, and asks to be recorded.
// That second half is the load-bearing one: a ratchet that absorbs improvements
// silently keeps a baseline of 250 while reality is 40, and the regression from
// 40 back to 250 passes. It would still be green, and it would be asserting
// nothing — the failure mode of a guarantee written as prose.
//
// Nothing here knows what is being counted. `direction` says which way is better
// and only chooses the words.
import { existsSync, readFileSync, writeFileSync } from 'node:fs';

export function ratchet({ counts, baselinePath, direction, update = false }) {
  const baseline = existsSync(baselinePath)
    ? JSON.parse(readFileSync(baselinePath, 'utf-8'))
    : {};

  if (update) {
    const sorted = Object.fromEntries(byKey(Object.entries(counts)));
    writeFileSync(baselinePath, `${JSON.stringify(sorted, null, 2)}\n`);
    return { ok: true, lines: [`baseline rewritten: ${baselinePath}`] };
  }

  const better = direction === 'up' ? (a, b) => a > b : (a, b) => a < b;
  const lines = [];
  const moved = [];

  for (const key of Object.keys(baseline).sort()) {
    if (key in counts) continue;
    lines.push(`${key}: in the baseline, measured by nothing — drop the entry or restore what counted it`);
  }

  for (const [key, actual] of byKey(Object.entries(counts))) {
    const from = baseline[key] ?? 0;
    if (actual === from) continue;
    const improved = better(actual, from);
    moved.push({ key, from, to: actual, improved });
    lines.push(
      improved
        ? `${key}: ${from} → ${actual} — improved; rerun with --update to record it`
        : `${key}: ${from} → ${actual} — regression`,
    );
  }

  // `moved` alongside `lines` so a caller that wants to say more about a key —
  // check-token-coverage prints the offending lines of the file — has the key
  // itself rather than having to parse it back out of a formatted string.
  return { ok: lines.length === 0, lines, moved };
}

// By key, so a baseline diffs by the entry that moved rather than by insertion
// order, and a report reads in the same order every run.
function byKey(entries) {
  return entries.sort(([a], [b]) => a.localeCompare(b));
}
