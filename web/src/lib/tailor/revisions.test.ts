import { describe, it, expect } from 'vitest';
import { keepIndex, makeHighlighter, groupByBatch, actorLabel } from './revisions';
import type { RevisionView } from '$lib/generated/contracts';

describe('keepIndex', () => {
  it('carries the original position through a filter', () => {
    const bullets = ['Shipped it', '', 'Twice'];
    expect(keepIndex(bullets, (b) => b.trim() !== '')).toEqual([
      { index: 0, item: 'Shipped it' },
      { index: 2, item: 'Twice' },
    ]);
  });

  // This is the whole point: an empty bullet between two filled ones shifts the numbering,
  // and a highlight for experience[0].bullets[2] would land silently on the wrong line.
  it('does not renumber what survives the filter', () => {
    const kept = keepIndex(['', 'Twice'], (b) => b.trim() !== '');
    expect(kept[0]?.index).toBe(1);
  });

  it('returns nothing for a list that is entirely empty', () => {
    expect(keepIndex(['', '  '], (b) => b.trim() !== '')).toEqual([]);
  });
});

describe('makeHighlighter', () => {
  it('matches the exact address a revision touched', () => {
    const lit = makeHighlighter(['experience[0].bullets[1]']);
    expect(lit('experience[0].bullets[1]')).toBe(true);
    expect(lit('experience[0].bullets[0]')).toBe(false);
    expect(lit('experience[1].bullets[1]')).toBe(false);
  });

  // A revision that replaced a whole entry lights everything inside it: the candidate asked
  // "what changed", and every line of that entry is the answer.
  it('lights what sits inside a changed container', () => {
    const lit = makeHighlighter(['experience[0]']);
    expect(lit('experience[0]')).toBe(true);
    expect(lit('experience[0].bullets[0]')).toBe(true);
    expect(lit('experience[0].company')).toBe(true);
    expect(lit('experience[1].bullets[0]')).toBe(false);
  });

  // The container of a changed field is NOT lit: underlining the entire entry because one
  // bullet in it moved would say less than saying nothing.
  it('does not light the container of a changed field', () => {
    const lit = makeHighlighter(['experience[0].bullets[1]']);
    expect(lit('experience[0]')).toBe(false);
  });

  it('is not fooled by a shared prefix', () => {
    const lit = makeHighlighter(['experience[1]']);
    expect(lit('experience[10].bullets[0]')).toBe(false);
  });

  it('lights nothing when no revision is selected', () => {
    const lit = makeHighlighter([]);
    expect(lit('experience[0].bullets[1]')).toBe(false);
    expect(lit('summary')).toBe(false);
  });
});

function revision(over: Partial<RevisionView> = {}): RevisionView {
  return {
    id: crypto.randomUUID(),
    actor: 'candidate',
    origin: 'editor',
    title: 'Rewrote a bullet',
    paths: ['summary'],
    reverted: false,
    undoable: true,
    created_at: '2026-07-31T09:00:00Z',
    ...over,
  } as RevisionView;
}

describe('groupByBatch', () => {
  it('keeps standalone entries as themselves, in order', () => {
    const a = revision({ title: 'One' });
    const b = revision({ title: 'Two' });
    expect(groupByBatch([a, b])).toEqual([
      { kind: 'single', revision: a },
      { kind: 'single', revision: b },
    ]);
  });

  it('folds a run into one group', () => {
    const batch = crypto.randomUUID();
    const first = revision({ actor: 'agent', batch_id: batch, title: 'One' });
    const second = revision({ actor: 'agent', batch_id: batch, title: 'Two' });
    const own = revision({ title: 'Mine' });

    const groups = groupByBatch([first, second, own]);

    expect(groups).toHaveLength(2);
    const run = groups[0];
    if (run?.kind !== 'batch') throw new Error('the run did not fold into one entry');
    expect(run.batchId).toBe(batch);
    expect(run.revisions).toHaveLength(2);
    expect(groups[1]).toEqual({ kind: 'single', revision: own });
  });

  // A run whose edits have all been undone offers no control: there is nothing left to undo.
  it('reports a run as spent when every edit in it is reverted', () => {
    const batch = crypto.randomUUID();
    const [run] = groupByBatch([
      revision({ actor: 'agent', batch_id: batch, reverted: true, undoable: false }),
      revision({ actor: 'agent', batch_id: batch, reverted: true, undoable: false }),
    ]);
    if (run?.kind !== 'batch') throw new Error('the run did not fold into one entry');
    expect(run.undoable).toBe(false);
  });

  it('reports a run as undoable while any edit still stands', () => {
    const batch = crypto.randomUUID();
    const [run] = groupByBatch([
      revision({ actor: 'agent', batch_id: batch, reverted: true, undoable: false }),
      revision({ actor: 'agent', batch_id: batch, reverted: false }),
    ]);
    if (run?.kind !== 'batch') throw new Error('the run did not fold into one entry');
    expect(run.undoable).toBe(true);
  });

  // Two runs are two groups even when their entries arrive next to each other.
  it('does not merge separate runs', () => {
    const groups = groupByBatch([
      revision({ actor: 'agent', batch_id: crypto.randomUUID() }),
      revision({ actor: 'agent', batch_id: crypto.randomUUID() }),
    ]);
    expect(groups).toHaveLength(2);
  });
});

describe('actorLabel', () => {
  it('names each hand in the candidate’s own terms', () => {
    expect(actorLabel('candidate')).toBe('You');
    expect(actorLabel('agent')).toBe('Assistant');
    expect(actorLabel('system')).toBe('freehire');
  });

  it('falls back to something readable for an actor it has not met', () => {
    expect(actorLabel('robot')).toBe('robot');
  });
});
