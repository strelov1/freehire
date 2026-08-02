import { describe, expect, it } from 'vitest';

import { EMAIL_STATUS_SIGNAL_VALUES, SIGNAL_STAGE } from './generated/contracts';
import { stageImplication, statusLabel } from './emailStatus';
import { humanizeStage } from './stages';

// A classified message says what it means for the application, or says that it means
// nothing for it. Both are answers; a bare chip is not, and a bare chip is what left
// somebody with seven emails and an unexplained stage.
describe('stageImplication', () => {
  it('names the stage an advancing signal implies', () => {
    expect(stageImplication('acknowledgement')).toBe('→ Applied');
    expect(stageImplication('assessment')).toBe('→ Screening');
  });

  // The chip already carries the signal's own label. Where that label IS the stage name —
  // Screening, Interview, Offer, Rejected — repeating it reads as "Rejected → Rejected",
  // which is noise standing where an explanation should be.
  it('does not repeat the stage when the chip already says it', () => {
    expect(stageImplication('interview_invitation')).toBe('');
    expect(stageImplication('screening')).toBe('');
    expect(stageImplication('offer')).toBe('');
  });

  // The rule this is here to make visible: the message plainly means `rejected`, and
  // deciding an application is settled is still the candidate's call. The stage name is
  // the chip's job; what is worth saying is that nothing moved.
  it('says a rejection moves nothing, without repeating its name', () => {
    expect(stageImplication('rejection')).toBe('does not move the stage');
  });

  it('says so when a signal implies no stage at all', () => {
    expect(stageImplication('info_request')).toBe('does not move the stage');
    expect(stageImplication('incomplete_application')).toBe('does not move the stage');
  });

  it('says nothing for an unclassified message', () => {
    expect(stageImplication(undefined)).toBe('');
    expect(stageImplication('')).toBe('');
  });

  // A server ahead of this build may name a signal it has never heard of; silence beats
  // inventing a meaning for it.
  it('says nothing for a signal it does not know', () => {
    expect(stageImplication('definitely-not-a-signal')).toBe('');
  });

  // Every signal is accounted for: either the chip's own label already says what the stage
  // is, or there is a phrase explaining what the message means for it. A signal with
  // neither would be the bare chip this change exists to replace.
  it('leaves no signal without either a self-explaining chip or a phrase', () => {
    for (const signal of EMAIL_STATUS_SIGNAL_VALUES) {
      if (signal === 'other') continue; // `other` renders no chip at all
      const label = statusLabel(signal);
      expect(label, signal).not.toBe('');
      const implication = stageImplication(signal);
      const selfExplaining = SIGNAL_STAGE[signal].stage
        ? label === humanizeStage(SIGNAL_STAGE[signal].stage)
        : false;
      expect(implication !== '' || selfExplaining, signal).toBe(true);
    }
  });
});
