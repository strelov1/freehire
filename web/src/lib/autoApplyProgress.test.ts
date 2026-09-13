import { describe, it, expect } from 'vitest';
import { autoApplyProgressSteps } from './autoApplyProgress';

describe('autoApplyProgressSteps', () => {
  it('shows the Tailoring stage active while a CV is being prepared', () => {
    expect(autoApplyProgressSteps('tailoring', false)).toEqual([
      { id: 'tailoring', state: 'active' },
      { id: 'review', state: 'pending' },
      { id: 'submitted', state: 'pending' },
    ]);
  });

  it('marks the Tailoring stage as stopped when tailoring itself failed', () => {
    expect(autoApplyProgressSteps('tailor_failed', false)).toEqual([
      { id: 'tailoring', state: 'error' },
      { id: 'review', state: 'pending' },
      { id: 'submitted', state: 'pending' },
    ]);
  });

  it('shows the Review stage active while awaiting the candidate decision', () => {
    expect(autoApplyProgressSteps('pending_review', false)).toEqual([
      { id: 'tailoring', state: 'done' },
      { id: 'review', state: 'active' },
      { id: 'submitted', state: 'pending' },
    ]);
  });

  it('marks the Review stage as stopped when the candidate declined', () => {
    expect(autoApplyProgressSteps('declined', false)).toEqual([
      { id: 'tailoring', state: 'done' },
      { id: 'review', state: 'error' },
      { id: 'submitted', state: 'pending' },
    ]);
  });

  it('shows the Submitted stage queued once approved', () => {
    expect(autoApplyProgressSteps('approved', false)).toEqual([
      { id: 'tailoring', state: 'done' },
      { id: 'review', state: 'done' },
      { id: 'submitted', state: 'queued' },
    ]);
  });

  it('marks the Submitted stage as stopped when blocked', () => {
    expect(autoApplyProgressSteps('blocked', false)).toEqual([
      { id: 'tailoring', state: 'done' },
      { id: 'review', state: 'done' },
      { id: 'submitted', state: 'error' },
    ]);
  });

  it('marks the Submitted stage as stopped when the submission failed', () => {
    expect(autoApplyProgressSteps('failed', false)).toEqual([
      { id: 'tailoring', state: 'done' },
      { id: 'review', state: 'done' },
      { id: 'submitted', state: 'error' },
    ]);
  });

  it('shows every stage complete once the ledger confirms submission, even with no live attempt', () => {
    expect(autoApplyProgressSteps(null, true)).toEqual([
      { id: 'tailoring', state: 'done' },
      { id: 'review', state: 'done' },
      { id: 'submitted', state: 'done' },
    ]);
  });

  it('returns null when there is nothing to show', () => {
    expect(autoApplyProgressSteps(null, false)).toBeNull();
    expect(autoApplyProgressSteps(undefined, false)).toBeNull();
  });
});
