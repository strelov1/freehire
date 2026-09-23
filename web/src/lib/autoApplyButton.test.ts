import { describe, it, expect } from 'vitest';
import {
  autoApplyButtonState,
  jobCtaPlan,
  type AutoApplyButtonState,
  type JobCtaPlan,
} from './autoApplyButton';

describe('autoApplyButtonState', () => {
  it('hides the button for an ATS auto-apply cannot queue against at all', () => {
    expect(autoApplyButtonState('linkedin', null, false, true)).toEqual({ kind: 'hidden' });
    expect(autoApplyButtonState('smartrecruiters', 'queued', false, true)).toEqual({ kind: 'hidden' });
  });

  it('is idle for every ATS auto-apply can queue against, not just Greenhouse', () => {
    for (const source of ['greenhouse', 'ashby', 'workable', 'lever', 'recruitee']) {
      expect(autoApplyButtonState(source, null, false, true)).toEqual({ kind: 'idle' });
    }
  });

  it('is idle for a Greenhouse posting with no attempt', () => {
    expect(autoApplyButtonState('greenhouse', null, false, true)).toEqual({ kind: 'idle' });
    expect(autoApplyButtonState('greenhouse', undefined, false, true)).toEqual({ kind: 'idle' });
  });

  it('is queued when the caller already has a live attempt', () => {
    expect(autoApplyButtonState('greenhouse', 'queued', false, true)).toEqual({ kind: 'queued' });
  });

  it('is declined when the caller already declined this attempt', () => {
    expect(autoApplyButtonState('greenhouse', 'declined', false, true)).toEqual({ kind: 'declined' });
  });

  it('is applied when the caller already applied, regardless of status', () => {
    expect(autoApplyButtonState('greenhouse', null, true, true)).toEqual({ kind: 'applied' });
    expect(autoApplyButtonState('greenhouse', 'queued', true, true)).toEqual({ kind: 'applied' });
  });

  it('is failed when cmd/auto-apply gave up on the attempt', () => {
    expect(autoApplyButtonState('greenhouse', 'failed', false, true)).toEqual({ kind: 'failed' });
  });

  // The gate this test exists for: a non-Pro caller never sees the button at all, for any
  // ATS or attempt state — not idle-only. Even a caller with a live queued/failed/declined
  // attempt from when they WERE Pro sees nothing once they lapse, matching the product
  // decision that this is a Pro feature end to end, not just to start one.
  it('is hidden for a non-Pro caller regardless of source, status, or a standing attempt', () => {
    expect(autoApplyButtonState('greenhouse', null, false, false)).toEqual({ kind: 'hidden' });
    expect(autoApplyButtonState('greenhouse', 'queued', false, false)).toEqual({ kind: 'hidden' });
    expect(autoApplyButtonState('greenhouse', 'failed', false, false)).toEqual({ kind: 'hidden' });
    expect(autoApplyButtonState('greenhouse', 'declined', false, false)).toEqual({ kind: 'hidden' });
    expect(autoApplyButtonState('greenhouse', null, true, false)).toEqual({ kind: 'hidden' });
  });
});

const kinds = ['hidden', 'idle', 'queued', 'applied', 'declined', 'failed'] as const;
const plan = (kind: AutoApplyButtonState['kind']): JobCtaPlan => jobCtaPlan({ kind });

describe('jobCtaPlan', () => {
  it('offers only the apply button where auto-apply cannot drive the ATS', () => {
    expect(plan('hidden')).toEqual({
      autoApply: null,
      external: { label: 'Apply' },
    });
  });

  it('lets a startable auto-apply lead the applying, and names the plan it needs', () => {
    expect(plan('idle')).toEqual({
      autoApply: { label: 'Auto-apply', leads: true, pro: true, disabled: false },
      external: { label: 'Show origin' },
    });
  });

  it('keeps a standing attempt quiet and the apply link relabelled', () => {
    expect(plan('queued')).toEqual({
      autoApply: { label: 'Auto-apply queued', leads: false, pro: false, disabled: true },
      external: { label: 'Show origin' },
    });
  });

  // Not relabelled: `applied` is true of a posting from any source, and relabelling on it
  // would make a Greenhouse posting read differently from an identical Lever one for a
  // reader in the identical situation.
  it('leaves the apply button alone for a reader who already applied by hand', () => {
    expect(plan('applied')).toEqual({
      autoApply: { label: 'Already applied', leads: false, pro: false, disabled: true },
      external: { label: 'Apply' },
    });
  });

  it('gives the apply link its word back when auto-apply will not act', () => {
    expect(plan('declined')).toEqual({
      autoApply: { label: 'Auto-apply declined', leads: false, pro: false, disabled: true },
      external: { label: 'Apply' },
    });
    expect(plan('failed')).toEqual({
      autoApply: { label: "Auto-apply couldn't complete", leads: false, pro: false, disabled: true },
      external: { label: 'Apply' },
    });
  });

  // `leads` answers "is auto-apply the offered way to apply", and nothing else reads it: it
  // decides which single control joins Tailor my CV in the phone's sticky bar, and whether
  // the quiet strip has to carry the origin link the bar gave up. Only a startable attempt
  // qualifies — a queued one is in flight and a declined or failed one will never act.
  it('leads exactly in the state auto-apply can be started from', () => {
    for (const kind of kinds) {
      expect(Boolean(plan(kind).autoApply?.leads), `state ${kind}`).toBe(kind === 'idle');
    }
  });

  // The apply link keeps its own word wherever auto-apply is not going to do the applying —
  // `Show origin` reads as a demotion, and a reader whose auto-apply attempt failed is not
  // being offered anything else.
  it('relabels the apply link only while an attempt stands or can be started', () => {
    for (const kind of kinds) {
      const expected = kind === 'idle' || kind === 'queued' ? 'Show origin' : 'Apply';
      expect(plan(kind).external.label, `state ${kind}`).toBe(expected);
    }
  });

  // The brand fill belongs to Tailor my CV in every state, so this plan ranks nothing by
  // colour any more. A `primary` key reappearing on either control would be a half-done
  // rename — and it would read as true where `leads` is, silently restoring the old
  // hierarchy on the one state that renders three buttons.
  it('ranks nothing by colour', () => {
    for (const kind of kinds) {
      const p = plan(kind);
      expect(p.external, `state ${kind}`).not.toHaveProperty('primary');
      if (p.autoApply) expect(p.autoApply, `state ${kind}`).not.toHaveProperty('primary');
    }
  });

  // The Pro marker states a requirement of the action. On a button nobody can press it
  // would state it about nothing.
  it('marks Pro only on a button that can be pressed', () => {
    for (const kind of kinds) {
      const { autoApply } = plan(kind);
      if (autoApply?.pro) expect(autoApply.disabled, `state ${kind}`).toBe(false);
    }
  });
});
