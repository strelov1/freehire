import { describe, expect, it } from 'vitest';
import { EMAIL_STATUS_SIGNAL_VALUES } from './generated/contracts';
import { STATUS_LABELS } from './emailStatus';
import { INBOX_STATUS_GUIDE } from './inboxStatusGuide';

describe('INBOX_STATUS_GUIDE', () => {
  // The landing page claims to list what the classifier can tag. If a signal is
  // added to the vocabulary and not explained here, the page quietly becomes a
  // partial list that reads as a complete one.
  //
  // The list is walked from the GENERATED vocabulary rather than from the label
  // map's own keys, so this closes the loop back to Go: `make gen-contracts`
  // widens the vocabulary, the label map fails to typecheck, and this fails if the
  // page never explained the new signal.
  it('explains every signal the product labels', () => {
    const labelled = EMAIL_STATUS_SIGNAL_VALUES.filter((s) => STATUS_LABELS[s] !== '');
    expect(INBOX_STATUS_GUIDE.map((s) => s.signal).sort()).toEqual([...labelled].sort());
  });

  it('describes each signal', () => {
    for (const { signal, description } of INBOX_STATUS_GUIDE) {
      expect(description.trim(), `description for: ${signal}`).not.toBe('');
    }
  });
});
