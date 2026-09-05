import { describe, it, expect } from 'vitest';
import { autoApplyNeedsReviewBadge, autoApplyReviewBanner } from './autoApplyReview';

describe('autoApplyNeedsReviewBadge', () => {
  it('shows for pending_review and blocked', () => {
    expect(autoApplyNeedsReviewBadge('pending_review')).toBe(true);
    expect(autoApplyNeedsReviewBadge('blocked')).toBe(true);
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

  it('is null for tailoring, approved, and no attempt', () => {
    for (const status of ['tailoring', 'approved', null, undefined]) {
      expect(autoApplyReviewBanner(status)).toBeNull();
    }
  });
});
