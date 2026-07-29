import { describe, expect, it } from 'vitest';
import { decisionLabel, decisionOutcome, reportReasonLabel, type DecisionKind } from './reports';

describe('reportReasonLabel', () => {
  it('labels a known reason and falls back to the raw value', () => {
    expect(reportReasonLabel('fraud')).toBe('Fraud');
    expect(reportReasonLabel('nonsense' as never)).toBe('nonsense');
  });
});

describe('decisionLabel', () => {
  it('names each decision the way the button reads', () => {
    const labels = (['close', 'resolve', 'dismiss'] as DecisionKind[]).map(decisionLabel);
    expect(new Set(labels).size).toBe(3);
    expect(decisionLabel('close')).toMatch(/close/i);
  });
});

describe('decisionOutcome', () => {
  it('is silent when the decision landed and the notice went out', () => {
    expect(decisionOutcome({ notifyRequested: true, notified: true })).toBeNull();
  });

  it('is silent when no notice was asked for', () => {
    expect(decisionOutcome({ notifyRequested: false, notified: false })).toBeNull();
  });

  it('warns when a requested notice did not go out', () => {
    const warning = decisionOutcome({ notifyRequested: true, notified: false });
    expect(warning).not.toBeNull();
    // The moderator must learn two things: the decision stands, and nobody was told.
    expect(warning).toMatch(/recorded|saved|stands/i);
    expect(warning).toMatch(/email|notif/i);
  });

  it('never claims a notice was sent that was not', () => {
    const warning = decisionOutcome({ notifyRequested: true, notified: false });
    expect(warning).not.toMatch(/\bsent\b(?!.*(not|n't))/i);
  });
});
