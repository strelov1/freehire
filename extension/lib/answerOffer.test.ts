import { describe, it, expect } from 'vitest';
import { answersToOffer } from './answerOffer';
import { normalizeLabel } from './form';
import type { FramedField } from './protocol';

function field(label: string, value: string, extra: Partial<FramedField> = {}): FramedField {
  return {
    index: 0,
    frame: 0,
    form: 0,
    tag: 'input',
    type: 'text',
    label,
    name: '',
    required: false,
    value,
    combo: false,
    ...extra,
  };
}

describe('answersToOffer', () => {
  it('offers the screening questions the candidate answered by hand', () => {
    const offers = answersToOffer(
      [field('Have you interviewed with us before?', 'No', { index: 3 }), field('Gender', '')],
      new Set(),
    );
    expect(offers).toEqual([{ key: '0:3', question: 'Have you interviewed with us before?', answer: 'No' }]);
  });

  // The profile already holds these — name, email, notice period and the rest of the
  // fixed set. Banking them as free-text answers would be a second, drifting copy.
  it('leaves out what the profile already answers', () => {
    const offers = answersToOffer([field('Email', 'a@b.c'), field('Notice period', '30 days')], new Set());
    expect(offers).toEqual([]);
  });

  // What freehire itself wrote came FROM the profile or the bank; offering it back is noise.
  it('leaves out what the autofill wrote', () => {
    const label = 'Have you used our product before?';
    expect(answersToOffer([field(label, 'Yes')], new Set())).toHaveLength(1);
    expect(answersToOffer([field(label, 'Yes')], new Set([normalizeLabel(label)]))).toEqual([]);
  });

  // The bank refuses an answer past its limit; offering one would end in an error.
  it('leaves out an answer longer than the bank stores', () => {
    const offers = answersToOffer([field('Why us?', 'x'.repeat(2001))], new Set());
    expect(offers).toEqual([]);
  });

  it('keys each offer by frame and question index, so a repeated label stays two rows', () => {
    const offers = answersToOffer(
      [field('Years', '3', { index: 1 }), field('Years', '5', { index: 2 })],
      new Set(),
    );
    expect(offers.map((o) => o.key)).toEqual(['0:1', '0:2']);
  });
});
