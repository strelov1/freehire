import { describe, it, expect, vi } from 'vitest';
import { summarizeRun, statusMeta, undoRun, openingActions } from './autopilot';
import type { AutopilotEntry } from '$lib/generated/contracts';

const report: AutopilotEntry[] = [
  { requirement: 'Kafka in production', status: 'closed_bank', note: 'Reframed the payments bullet.' },
  { requirement: 'Python', status: 'closed_candidate', note: 'Their words: migration scripts.' },
  { requirement: 'Team leadership', status: 'open' },
  { requirement: 'Terraform', status: 'open' },
  { requirement: 'Kubernetes', status: 'not_reached' },
];

describe('summarizeRun', () => {
  it('counts what the run closed against what it considered', () => {
    expect(summarizeRun(report)).toEqual({ closed: 2, open: 2, notReached: 1, total: 5 });
  });

  it('reads an absent report as no run at all', () => {
    expect(summarizeRun(undefined)).toEqual({ closed: 0, open: 0, notReached: 0, total: 0 });
    expect(summarizeRun([])).toEqual({ closed: 0, open: 0, notReached: 0, total: 0 });
  });
});

describe('statusMeta', () => {
  it('gives every status of the vocabulary a label and a tone', () => {
    for (const status of ['closed_bank', 'closed_candidate', 'open', 'not_reached'] as const) {
      const meta = statusMeta(status);
      expect(meta.label.length).toBeGreaterThan(0);
      expect(['closed', 'open', 'skipped']).toContain(meta.tone);
    }
  });

  // The vocabulary is fixed server-side, but a row the panel cannot classify must still
  // render as a row rather than crash the workspace around it.
  it('falls back for a status it does not know', () => {
    const meta = statusMeta('something_new' as never);
    expect(meta.label.length).toBeGreaterThan(0);
  });

  it('separates the two ways a requirement gets closed', () => {
    expect(statusMeta('closed_bank').label).not.toBe(statusMeta('closed_candidate').label);
  });
});

describe('undoRun', () => {
  it('flushes the pending save BEFORE undoing, then re-reads', async () => {
    const order: string[] = [];
    const flush = vi.fn(async () => {
      order.push('flush');
    });
    const undo = vi.fn(async () => {
      order.push('undo');
    });
    const refetch = vi.fn(async () => {
      order.push('refetch');
    });

    await undoRun({ flush, undo, refetch });

    // A pending autosave carries the tailored text. Undo it first and the debounce
    // resurrects what was just reverted a second later.
    expect(order).toEqual(['flush', 'undo', 'refetch']);
  });

  it('does not re-read when the undo failed', async () => {
    const refetch = vi.fn(async () => {});
    const failing = async () => {
      throw new Error('there is no autopilot run to undo');
    };

    await expect(undoRun({ flush: async () => {}, undo: failing, refetch })).rejects.toThrow(
      'no autopilot run',
    );
    expect(refetch).not.toHaveBeenCalled();
  });
});

describe('openingActions', () => {
  it('offers both rhythms of the same method', () => {
    const actions = openingActions();
    expect(actions).toHaveLength(2);
    expect(actions.map((a) => a.kind)).toContain('autopilot');
    // The conversational action carries the text it will send; the run carries none, because
    // its brief belongs to the server.
    const walk = actions.find((a) => a.kind === 'message');
    expect(walk && 'text' in walk && walk.text.length).toBeTruthy();
  });

  it('offers them regardless of how the workspace was opened', () => {
    // Keyed on an EMPTY conversation, not on a fresh one: a CV re-opened by id can carry a
    // conversation nobody has spoken to, and that case read as lost history. The chat renders
    // these only while there are no messages, so the decision lives there.
    expect(openingActions()).toHaveLength(2);
  });

  it('never opens a turn by itself', () => {
    // The actions are data, not effects: nothing here sends anything until one is picked.
    for (const action of openingActions()) {
      expect(typeof action.label).toBe('string');
    }
  });
});
