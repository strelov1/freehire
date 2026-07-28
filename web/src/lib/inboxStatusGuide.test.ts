import { describe, expect, it } from 'vitest';
import { STATUS_LABELS } from './emailStatus';
import { INBOX_STATUS_GUIDE } from './inboxStatusGuide';

describe('INBOX_STATUS_GUIDE', () => {
  // The landing page claims to list what the classifier can tag. If a signal is
  // added to the vocabulary and not explained here, the page quietly becomes a
  // partial list that reads as a complete one.
  it('explains every signal the product labels', () => {
    const labelled = Object.keys(STATUS_LABELS).filter((s) => STATUS_LABELS[s] !== '');
    expect(INBOX_STATUS_GUIDE.map((s) => s.signal).sort()).toEqual(labelled.sort());
  });

  it('describes each signal', () => {
    for (const { signal, description } of INBOX_STATUS_GUIDE) {
      expect(description.trim(), `description for: ${signal}`).not.toBe('');
    }
  });
});
