import { describe, it, expect } from 'vitest';
import { autoApplyButtonState } from './autoApplyButton';

describe('autoApplyButtonState', () => {
  it('hides the button for a non-Greenhouse posting', () => {
    expect(autoApplyButtonState('lever', null)).toEqual({ kind: 'hidden' });
    expect(autoApplyButtonState('workable', 'queued')).toEqual({ kind: 'hidden' });
  });

  it('is idle for a Greenhouse posting with no attempt', () => {
    expect(autoApplyButtonState('greenhouse', null)).toEqual({ kind: 'idle' });
    expect(autoApplyButtonState('greenhouse', undefined)).toEqual({ kind: 'idle' });
  });

  it('is queued when the caller already has a live attempt', () => {
    expect(autoApplyButtonState('greenhouse', 'queued')).toEqual({ kind: 'queued' });
  });

  it('is declined when the caller already declined this attempt', () => {
    expect(autoApplyButtonState('greenhouse', 'declined')).toEqual({ kind: 'declined' });
  });

  it('is applied when the caller already applied, regardless of status', () => {
    expect(autoApplyButtonState('greenhouse', null, true)).toEqual({ kind: 'applied' });
    expect(autoApplyButtonState('greenhouse', 'queued', true)).toEqual({ kind: 'applied' });
  });

  it('is failed when cmd/auto-apply gave up on the attempt', () => {
    expect(autoApplyButtonState('greenhouse', 'failed')).toEqual({ kind: 'failed' });
  });
});
