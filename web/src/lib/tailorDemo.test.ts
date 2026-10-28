import { describe, expect, it } from 'vitest';
import { DEMO_ASKED, DEMO_BULLETS, DEMO_REQUIREMENTS, DEMO_SCORES, highlight } from './tailorDemo';

describe('highlight', () => {
  it('marks every occurrence of each word, case-insensitively', () => {
    expect(highlight('Built the Kafka pipeline; kafka at scale', ['kafka'])).toEqual([
      { text: 'Built the ', hit: false },
      { text: 'Kafka', hit: true },
      { text: ' pipeline; ', hit: false },
      { text: 'kafka', hit: true },
      { text: ' at scale', hit: false },
    ]);
  });

  it('returns the text whole when nothing matches', () => {
    expect(highlight('Code reviews', ['Rust'])).toEqual([{ text: 'Code reviews', hit: false }]);
  });

  it('treats regex characters in a word literally', () => {
    expect(highlight('C++ and C', ['C++'])).toEqual([
      { text: 'C++', hit: true },
      { text: ' and C', hit: false },
    ]);
  });
});

describe('demo copy', () => {
  // The page argues that tailoring adds nothing you did not do. The example must not contradict it.
  it('has one score per step, rising', () => {
    expect(DEMO_SCORES).toHaveLength(DEMO_BULLETS.length + 1);
    for (let i = 1; i < DEMO_SCORES.length; i++) expect(DEMO_SCORES[i]).toBeGreaterThan(DEMO_SCORES[i - 1]);
  });

  it('ties every bullet and the asked requirement to a listed requirement', () => {
    const ids = new Set(DEMO_REQUIREMENTS.map((r) => r.id));
    for (const b of DEMO_BULLETS) expect(ids.has(b.req), b.req).toBe(true);
    expect(ids.has(DEMO_ASKED)).toBe(true);
    expect(DEMO_BULLETS.some((b) => b.req === DEMO_ASKED)).toBe(false);
  });

  it('highlights only words the after line actually contains', () => {
    for (const b of DEMO_BULLETS) {
      for (const k of b.keywords) expect(b.after.toLowerCase(), k).toContain(k.toLowerCase());
    }
  });
});
