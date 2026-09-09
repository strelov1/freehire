import { describe, it, expect } from 'vitest';
import { autoApplyNeedsReviewBadge, autoApplyReviewBanner } from './autoApplyReview';

describe('autoApplyNeedsReviewBadge', () => {
  it('shows for pending_review, blocked and tailor_failed', () => {
    expect(autoApplyNeedsReviewBadge('pending_review')).toBe(true);
    expect(autoApplyNeedsReviewBadge('blocked')).toBe(true);
    // A run that never produced a CV stopped just as surely as a blocked one did, and the
    // candidate has no other way to notice: nothing on the card changes otherwise.
    expect(autoApplyNeedsReviewBadge('tailor_failed')).toBe(true);
  });

  it('hides for tailoring, approved, declined, failed, and no attempt', () => {
    for (const status of ['tailoring', 'approved', 'declined', 'failed', null, undefined]) {
      expect(autoApplyNeedsReviewBadge(status)).toBe(false);
    }
  });
});

describe('autoApplyReviewBanner', () => {
  it('is the pending_review variant for pending_review', () => {
    expect(autoApplyReviewBanner('pending_review')).toEqual({ kind: 'pending_review' });
  });

  it('is the blocked variant for blocked', () => {
    expect(autoApplyReviewBanner('blocked')).toEqual({ kind: 'blocked' });
  });

  it('is the declined variant for declined', () => {
    expect(autoApplyReviewBanner('declined')).toEqual({ kind: 'declined' });
  });

  it('is the failed variant for failed', () => {
    expect(autoApplyReviewBanner('failed')).toEqual({ kind: 'failed' });
  });

  it('is the tailoring variant for tailoring', () => {
    expect(autoApplyReviewBanner('tailoring')).toEqual({ kind: 'tailoring' });
  });

  it('is the approved variant for approved', () => {
    expect(autoApplyReviewBanner('approved')).toEqual({ kind: 'approved' });
  });

  // A tailoring run that gave up is its own variant, not the "still preparing" one it used
  // to fall through to: the candidate has no CV to look at, so the copy cannot be the
  // failed-submission one either (that one implies there was something to send).
  it('is the tailor_failed variant for tailor_failed', () => {
    expect(autoApplyReviewBanner('tailor_failed')).toEqual({ kind: 'tailor_failed' });
  });

  it('is null for no attempt', () => {
    for (const status of [null, undefined]) {
      expect(autoApplyReviewBanner(status)).toBeNull();
    }
  });
});
