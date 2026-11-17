import { describe, expect, it, vi } from 'vitest';
import { TailorOutcomes } from './outcome';

function setup(opts: { slug?: string; cvId?: string } = {}) {
  const data = new Map<string, string>();
  const store = { getItem: (k: string) => data.get(k) ?? null, setItem: (k: string, v: string) => void data.set(k, v) };
  const emit = vi.fn();
  const cv = { doc: { summary: 'seed' }, id: opts.cvId ?? 'cv-1' };
  const outcomes = new TailorOutcomes({
    cvId: () => cv.id,
    slug: () => ('slug' in opts ? opts.slug : 'acme-engineer'),
    document: () => cv.doc,
    store,
    emit,
  });
  return { outcomes, emit, cv, store };
}

describe('TailorOutcomes tailoring result', () => {
  it('emits once when a completed turn persisted a changed document', () => {
    const { outcomes, emit, cv } = setup();
    outcomes.turnStarted();
    outcomes.documentEdited();
    cv.doc = { summary: 'tailored to the vacancy' };
    outcomes.turnEnded(true);
    expect(emit).toHaveBeenCalledExactlyOnceWith('tailor_result', { slug: 'acme-engineer' });
  });

  it('stays silent for a turn that made no edit (a seed, a plain answer)', () => {
    const { outcomes, emit } = setup();
    outcomes.turnStarted();
    outcomes.turnEnded(true);
    expect(emit).not.toHaveBeenCalled();
  });

  it('stays silent when a cv_edit succeeded but left the document unchanged', () => {
    const { outcomes, emit } = setup();
    outcomes.turnStarted();
    outcomes.documentEdited();
    outcomes.turnEnded(true);
    expect(emit).not.toHaveBeenCalled();
  });

  it('stays silent when the turn did not complete, even though edits were persisted', () => {
    const { outcomes, emit, cv } = setup();
    outcomes.turnStarted();
    outcomes.documentEdited();
    cv.doc = { summary: 'half done' };
    outcomes.turnEnded(false);
    expect(emit).not.toHaveBeenCalled();
  });

  it('stays silent when the document changed without any successful cv_edit', () => {
    const { outcomes, emit, cv } = setup();
    outcomes.turnStarted();
    cv.doc = { summary: 'typed by hand' };
    outcomes.turnEnded(true);
    expect(emit).not.toHaveBeenCalled();
  });

  it('stays silent for a turn end that never saw its start (catch-up after a reload)', () => {
    const { outcomes, emit, cv } = setup();
    outcomes.documentEdited();
    cv.doc = { summary: 'changed' };
    outcomes.turnEnded(true);
    expect(emit).not.toHaveBeenCalled();
  });

  it('ignores a duplicate end callback and later turns on the same CV', () => {
    const { outcomes, emit, cv } = setup();
    outcomes.turnStarted();
    outcomes.documentEdited();
    cv.doc = { summary: 'one' };
    outcomes.turnEnded(true);
    outcomes.turnEnded(true);
    outcomes.turnStarted();
    outcomes.documentEdited();
    cv.doc = { summary: 'two' };
    outcomes.turnEnded(true);
    expect(emit).toHaveBeenCalledTimes(1);
  });

  it('is not repeated by a remounted workspace for the same CV, but counts another CV', () => {
    const first = setup();
    first.outcomes.turnStarted();
    first.outcomes.documentEdited();
    first.cv.doc = { summary: 'one' };
    first.outcomes.turnEnded(true);

    const turn = (cvId: string) => {
      const doc = { current: { summary: 'before' } };
      const remount = new TailorOutcomes({
        cvId: () => cvId,
        slug: () => 'acme-engineer',
        document: () => doc.current,
        store: first.store,
        emit: first.emit,
      });
      remount.turnStarted();
      remount.documentEdited();
      doc.current = { summary: 'after' };
      remount.turnEnded(true);
    };
    turn('cv-1');
    expect(first.emit).toHaveBeenCalledTimes(1);
    turn('cv-2');
    expect(first.emit).toHaveBeenCalledTimes(2);
  });

  it('never counts a base CV edit as tailoring', () => {
    const { outcomes, emit, cv } = setup({ slug: undefined });
    outcomes.turnStarted();
    outcomes.documentEdited();
    cv.doc = { summary: 'base edit' };
    outcomes.turnEnded(true);
    expect(emit).not.toHaveBeenCalled();
  });

  it('emits nothing when storage is unavailable rather than risk double counting', () => {
    const emit = vi.fn();
    const cv = { doc: { a: 1 } };
    const outcomes = new TailorOutcomes({
      cvId: () => 'cv-1',
      slug: () => 's',
      document: () => cv.doc,
      store: undefined,
      emit,
    });
    outcomes.turnStarted();
    outcomes.documentEdited();
    cv.doc = { a: 2 };
    outcomes.turnEnded(true);
    expect(emit).not.toHaveBeenCalled();
  });
});

describe('TailorOutcomes pdf export', () => {
  it('emits a tailored export with the vacancy slug', () => {
    const { outcomes, emit } = setup();
    outcomes.pdfExported();
    expect(emit).toHaveBeenCalledExactlyOnceWith('cv_pdf_export', { kind: 'tailored', slug: 'acme-engineer' });
  });

  it('labels a base CV export without a slug', () => {
    const { outcomes, emit } = setup({ slug: undefined });
    outcomes.pdfExported();
    expect(emit).toHaveBeenCalledExactlyOnceWith('cv_pdf_export', { kind: 'base' });
  });

  it('does not let an export claim the tailoring result', () => {
    const { outcomes, emit, cv } = setup();
    outcomes.pdfExported();
    outcomes.turnStarted();
    outcomes.documentEdited();
    cv.doc = { summary: 'tailored' };
    outcomes.turnEnded(true);
    expect(emit.mock.calls.map((c) => c[0])).toEqual(['cv_pdf_export', 'tailor_result']);
  });
});
