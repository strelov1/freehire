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
      external: { label: 'Apply', primary: true },
    });
  });

  it('makes a startable auto-apply the primary CTA', () => {
    expect(plan('idle')).toEqual({
      autoApply: { label: 'Auto-apply', primary: true, disabled: false },
      external: { label: 'Show origin', primary: false },
    });
  });

  it('keeps a standing attempt quiet and the apply button demoted', () => {
    expect(plan('queued')).toEqual({
      autoApply: { label: 'Auto-apply queued', primary: false, disabled: true },
      external: { label: 'Show origin', primary: false },
    });
  });

  // Not demoted: `applied` is true of a posting from any source, and demoting on it would
  // make a Greenhouse posting read differently from an identical Lever one for a reader in
  // the identical situation.
  it('leaves the apply button alone for a reader who already applied by hand', () => {
    expect(plan('applied')).toEqual({
      autoApply: { label: 'Already applied', primary: false, disabled: true },
      external: { label: 'Apply', primary: true },
    });
  });

  it('promotes the apply button back when auto-apply will not act', () => {
    expect(plan('declined')).toEqual({
      autoApply: { label: 'Auto-apply declined', primary: false, disabled: true },
      external: { label: 'Apply', primary: true },
    });
    expect(plan('failed')).toEqual({
      autoApply: { label: "Auto-apply couldn't complete", primary: false, disabled: true },
      external: { label: 'Apply', primary: true },
    });
  });

  // The rule the table exists to protect: never two loud buttons competing for the same
  // click.
  it('never offers two primary CTAs at once', () => {
    for (const kind of kinds) {
      const p = plan(kind);
      const primaries = [p.autoApply?.primary, p.external.primary].filter(Boolean).length;
      expect(primaries, `state ${kind}`).toBeLessThanOrEqual(1);
    }
  });

  // And the other half of it: a state where the reader still HAS something to do gets a
  // loud button for it. `queued` is the one state where they do not — a submission is in
  // flight, and a loud button there would only invite a second one.
  it('offers a primary CTA in every state that still has an action left', () => {
    for (const kind of kinds) {
      const p = plan(kind);
      const hasPrimary = Boolean(p.autoApply?.primary) || p.external.primary;
      expect(hasPrimary, `state ${kind}`).toBe(kind !== 'queued');
    }
  });
});
