import { describe, expect, it } from 'vitest';
import {
  decisionLabel,
  decisionNotePlaceholder,
  decisionNotePrompt,
  decisionOutcome,
  isEvidenceReason,
  reportReasons,
  reportReasonLabel,
  type DecisionKind,
} from './reports';
import type { ReportReason } from './types';

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

describe('decisionNotePrompt', () => {
  it('tells the moderator the reporter reads what they write', () => {
    for (const kind of ['close', 'resolve', 'dismiss'] as DecisionKind[]) {
      expect(decisionNotePrompt(kind)).toMatch(/reporter/i);
    }
  });

  it('shapes the example note to the decision', () => {
    expect(decisionNotePlaceholder('dismiss')).not.toBe(decisionNotePlaceholder('resolve'));
    // A dismissal fixed nothing, so its example must not claim a fix.
    expect(decisionNotePlaceholder('dismiss')).not.toMatch(/fixed/i);
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

  it('names who was not reached, since the row leaves the queue either way', () => {
    const warning = decisionOutcome({
      notifyRequested: true,
      notified: false,
      reporterEmail: 'lina@example.test',
      jobTitle: 'Senior Web Designer',
    });
    expect(warning).toContain('lina@example.test');
    expect(warning).toContain('Senior Web Designer');
  });

  it('never claims a notice was sent that was not', () => {
    const warning = decisionOutcome({ notifyRequested: true, notified: false });
    expect(warning).not.toMatch(/\bsent\b(?!.*(not|n't))/i);
  });
});

describe('isEvidenceReason', () => {
  it('routes no_response to the evidence channel', () => {
    expect(isEvidenceReason('no_response')).toBe(true);
  });

  // The moderation queue's only verdict is closing the job. A reason routed there
  // by mistake reaches a reviewer who cannot express "noted, counted".
  it.each<ReportReason>(['not_relevant', 'spam', 'fraud', 'other'])(
    'keeps %s on the moderation path',
    (reason) => {
      expect(isEvidenceReason(reason)).toBe(false);
    },
  );

  it('classifies every reason in the vocabulary', () => {
    const evidence = reportReasons.filter((r) => isEvidenceReason(r.value));
    expect(evidence).toHaveLength(1);
  });
});
