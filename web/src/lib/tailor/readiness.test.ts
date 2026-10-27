import { describe, it, expect } from 'vitest';
import { findPlaceholders } from './readiness';
import type { Document } from '#lib/generated/contracts.js';

const base: Document = {
  margins: { top: 0.5, right: 0.5, bottom: 0.5, left: 0.5 },
  style: { font_family: '', font_size: 0, line_height: 0 },
  header: { full_name: 'Ada Lovelace' },
} as Document;

describe('findPlaceholders', () => {
  it('finds a template slot the agent left in a bullet, at the address the preview lights', () => {
    const doc = {
      ...base,
      experience: [
        { company: 'Solvd', role: 'Engineer', bullets: ['Shipped search'] },
        { company: 'Acme', role: 'Engineer', bullets: ['Real one', 'Built an app for [purpose], delivering [result].'] },
      ],
    } as Document;
    expect(findPlaceholders(doc)).toEqual([
      { path: 'experience[1].bullets[1]', token: '[purpose]' },
      { path: 'experience[1].bullets[1]', token: '[result]' },
    ]);
  });

  it('reads the header and summary too', () => {
    const doc = { ...base, header: { full_name: '[Your Name]' }, summary: 'Engineer with [X] years' } as Document;
    expect(findPlaceholders(doc).map((p) => p.path)).toEqual(['header.full_name', 'summary']);
  });

  // Margins and typography are numbers the candidate never types into; walking them would
  // only ever cost time.
  it('ignores the layout blocks', () => {
    expect(findPlaceholders(base)).toEqual([]);
  });

  it('does not mistake a citation, a markdown link or an empty pair for a slot', () => {
    const doc = { ...base, summary: 'Cited [1] and [2023]; see [docs](https://x.dev); empty [] and [ ].' } as Document;
    expect(findPlaceholders(doc)).toEqual([]);
  });

  // A long bracketed aside is prose, not a slot someone forgot to fill.
  it('leaves a long bracketed aside alone', () => {
    const doc = { ...base, summary: 'Led it [together with two other teams across three time zones and a vendor].' } as Document;
    expect(findPlaceholders(doc)).toEqual([]);
  });
});
