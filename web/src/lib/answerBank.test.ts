import { describe, it, expect } from 'vitest';
import { answerableQuestions, hasVisibleRows, pendingRows } from './answerBank';

describe('answerableQuestions', () => {
  it('offers an input for a question that has readable text', () => {
    const pending = [
      { label: 'What is your desired salary?', will_draft_at_submission: false },
      { label: 'Which state do you currently reside in?', will_draft_at_submission: false }
    ];
    expect(answerableQuestions(pending).map((q) => q.label)).toEqual([
      'What is your desired salary?',
      'Which state do you currently reside in?'
    ]);
  });

  // A question the model will fill needs no input from the candidate — offering one asks
  // them to do work that is already handled.
  it('skips a question that will be drafted at submission', () => {
    const pending = [{ label: 'Why do you want to work here?', will_draft_at_submission: true }];
    expect(answerableQuestions(pending)).toEqual([]);
  });

  // A guard, not the labelless-field case. The server titles a DOM-only field with its own
  // id before this list is built (internal/api/atsapply's questionText — Greenhouse's
  // `country` arrives here reading "country" and IS answerable), so what survives to be
  // filtered here is a field with neither a label nor an id. Rendering a blank input would
  // invite an answer to a question nobody can see.
  it('skips a question with no readable text at all', () => {
    const pending = [
      { label: '', will_draft_at_submission: false },
      { label: '   ', will_draft_at_submission: false }
    ];
    expect(answerableQuestions(pending)).toEqual([]);
  });

  // The server refuses to recall a work-authorization answer (atsapply's
  // matchBankAnswerKey), because one topic covers every posting worded that way and a "Yes"
  // banked from a US posting would fill a Brazilian one. Offering an input here would
  // collect exactly that answer, and the candidate would never see it used.
  it('does not offer an input for a work-authorization question', () => {
    const pending = [
      {
        label: 'Are you legally authorized to work in the country in which this position is located?',
        will_draft_at_submission: false
      },
      { label: 'Do you require visa sponsorship?', will_draft_at_submission: false },
      { label: 'Do you have the right to work in the UK?', will_draft_at_submission: false }
    ];
    expect(answerableQuestions(pending)).toEqual([]);
  });

  // Only the authorization subset. Salary is sensitive too and is the case the bank exists
  // for; a demographic question is the candidate's own answer to give.
  it('still offers an input for the other sensitive questions', () => {
    const pending = [
      { label: 'What is your desired salary?', will_draft_at_submission: false },
      { label: 'Are you a protected veteran?', will_draft_at_submission: false }
    ];
    expect(answerableQuestions(pending)).toHaveLength(2);
  });

  it('is empty for no pending questions at all', () => {
    expect(answerableQuestions(undefined)).toEqual([]);
    expect(answerableQuestions([])).toEqual([]);
  });
});

// pendingRows pairs every raw pending entry with a stable identity (its position) and a
// `kind` saying how it renders. Nothing here may key on label text: two distinct questions
// can share a label, and an entry can carry no text at all — either would collide if the
// identity came from the text itself.
describe('pendingRows', () => {
  it('gives two entries with the same label their own distinct identity', () => {
    const pending = [
      { label: 'Additional information', will_draft_at_submission: false },
      { label: 'Additional information', will_draft_at_submission: false }
    ];
    const rows = pendingRows(pending);
    expect(rows).toHaveLength(2);
    expect(rows.map((r) => r.key)).toEqual([0, 1]);
    // Distinct identity, not just distinct array position: a Set built from the keys must
    // hold two members, not collapse to one the way it would if the key were the label.
    expect(new Set(rows.map((r) => r.key)).size).toBe(2);
    expect(rows.every((r) => r.kind === 'answerable')).toBe(true);
  });

  it('gives several textless entries their own distinct identity too', () => {
    const pending = [
      { label: '', will_draft_at_submission: false },
      { label: '', will_draft_at_submission: false },
      { label: '   ', will_draft_at_submission: false },
      { label: '', will_draft_at_submission: false }
    ];
    const rows = pendingRows(pending);
    expect(new Set(rows.map((r) => r.key)).size).toBe(4);
    // Empty-label rows draw nothing — there is genuinely nothing to show, unlike a
    // work-authorization question, which has a label but still cannot be answered.
    expect(rows.every((r) => r.kind === 'empty')).toBe(true);
  });

  it('marks a question drafted at submission unanswerable without dropping it from the list', () => {
    const pending = [{ label: 'Why do you want to work here?', will_draft_at_submission: true }];
    const rows = pendingRows(pending);
    expect(rows).toHaveLength(1);
    const [row] = rows;
    expect(row?.kind).toBe('draft');
    expect(row?.pending.will_draft_at_submission).toBe(true);
  });

  // The regression this branch introduced: a work-authorization question has a real label
  // and is neither drafted nor answerable, so it must still produce a row — one the
  // candidate can see names what is blocking the application, distinct from both an
  // answerable question and an empty-label one that genuinely has nothing to show.
  it('marks a work-authorization question blocked, not silently dropped', () => {
    const pending = [
      {
        label: 'Are you legally authorized to work in the country in which this position is located?',
        will_draft_at_submission: false
      }
    ];
    const rows = pendingRows(pending);
    expect(rows).toHaveLength(1);
    expect(rows[0]?.kind).toBe('blocked');
  });

  it('is empty for no pending questions at all', () => {
    expect(pendingRows(undefined)).toEqual([]);
    expect(pendingRows([])).toEqual([]);
  });
});

// hasVisibleRows is what JobDrawer.svelte gates the whole pending block on — it must not
// re-derive which kinds render, or it can drift from pendingRows and reintroduce the bug
// this fixes (a blocked-only list rendering an empty-looking block, or nothing at all).
describe('hasVisibleRows', () => {
  it('is true when the only pending row is a blocked work-authorization question', () => {
    const pending = [{ label: 'Do you require visa sponsorship?', will_draft_at_submission: false }];
    expect(hasVisibleRows(pendingRows(pending))).toBe(true);
  });

  it('is true when the only pending row is answerable', () => {
    const pending = [{ label: 'What is your desired salary?', will_draft_at_submission: false }];
    expect(hasVisibleRows(pendingRows(pending))).toBe(true);
  });

  it('is true when the only pending row will be drafted at submission', () => {
    const pending = [{ label: 'Why do you want to work here?', will_draft_at_submission: true }];
    expect(hasVisibleRows(pendingRows(pending))).toBe(true);
  });

  it('is false when every row is empty-label', () => {
    const pending = [
      { label: '', will_draft_at_submission: false },
      { label: '   ', will_draft_at_submission: false }
    ];
    expect(hasVisibleRows(pendingRows(pending))).toBe(false);
  });

  it('is false for no pending questions at all', () => {
    expect(hasVisibleRows(pendingRows(undefined))).toBe(false);
  });
});
