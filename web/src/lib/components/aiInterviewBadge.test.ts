import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// A SOURCE-TEXT AUDIT, deliberately not a mounted-component test — the same choice
// jobActionStrip.test.ts makes and for the same reason: what is worth pinning here is
// not what the component renders for one prop, it is two editorial decisions that a
// later "improvement" would quietly undo without failing anything.
//
// 1. The badge is NEUTRAL. Plenty of candidates prefer an AI screen; the badge exists so
//    somebody can recognise the practice before they meet it, not so the platform can
//    rule on it. A warning colour or an alert icon turns a fact into an accusation, and
//    a label that reads as an accusation gets discounted by the people it is for.
// 2. The badge NEVER renders without a count. One report is enough to show the label,
//    so a reader is entitled to weigh 1 differently from 40; a badge with no number
//    beside it hides exactly the difference that makes it fair.
const SOURCE = readFileSync(join(process.cwd(), 'src/lib/components/AIInterviewBadge.svelte'), 'utf8');

// The tone assertions read the MARKUP only. Prose that argues for the neutral tone
// necessarily names the tones it rejects, and a test that failed on its own rationale
// would push the reasoning out of the file.
//
// Split on the literal closing tag rather than a regex: an HTML-stripping pattern here
// reads to a static analyser as a sanitizer for untrusted input, and it flags the
// incomplete ones. This is a file reading itself, so a plain split says what is meant
// and leaves nothing to misread. The badge's own rationale lives in that script block,
// which is exactly why it has to come off before the assertions run.
const MARKUP = SOURCE.split('</script>').pop() ?? '';

describe('AIInterviewBadge', () => {
  it('carries no warning or destructive styling', () => {
    for (const tone of ['warning', 'destructive', 'danger', 'text-red', 'bg-red']) {
      expect(MARKUP).not.toContain(tone);
    }
  });

  it('uses the neutral muted tone the unremarkable chips use', () => {
    expect(MARKUP).toContain('text-muted-foreground');
    expect(MARKUP).toContain('border-border');
  });

  it('guards on a positive count before rendering anything', () => {
    // The guard must be on the derived value, so a zero and an absent count take the
    // same path — a serialized 0 would otherwise render a badge claiming a practice was
    // reported when nobody reported it.
    expect(SOURCE).toContain('count > 0');
    expect(SOURCE).toMatch(/\{#if shown !== null\}/);
  });

  it('renders the count beside the label', () => {
    expect(SOURCE).toContain('{shown}');
  });
});
