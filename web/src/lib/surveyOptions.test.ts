import { describe, expect, it } from 'vitest';
import { JOB_CHALLENGE_OPTIONS, JOB_CHALLENGE_OTHER } from './surveyOptions';
import { JOB_CHALLENGE_VALUES } from './generated/contracts';

// These values are validated server-side against internal/dict/vocab, and anything outside
// it is a 400. A drifted value is therefore not a cosmetic bug — it is an answer the
// candidate can never save. Locked against the list generated from the server's own.
describe('the vocabulary matches the server', () => {
  it('offers exactly the challenges the API accepts', () => {
    expect(JOB_CHALLENGE_OPTIONS.map((o) => o.value).sort()).toEqual([...JOB_CHALLENGE_VALUES].sort());
  });
});

describe('the option lists are well-formed', () => {
  it('gives every option a label and a unique value', () => {
    const values = JOB_CHALLENGE_OPTIONS.map((o) => o.value);
    expect(new Set(values).size).toBe(values.length);
    for (const o of JOB_CHALLENGE_OPTIONS) expect(o.label.trim()).not.toBe('');
  });

  // The note field is revealed only for this member, and the server rejects a note sent
  // with any other — so an options list without it makes the free-text answer unreachable.
  it('includes the member the free-text note hangs off', () => {
    expect(JOB_CHALLENGE_OPTIONS.map((o) => o.value)).toContain(JOB_CHALLENGE_OTHER);
  });
});
