import { describe, expect, it } from 'vitest';

import { applyFormModel } from './applyFormGroups';
import type { Question } from './generated/contracts';

const ask = (text: string, answer?: string, required = true): Question => ({
  text,
  required,
  ...(answer === undefined ? {} : { answer }),
});

describe('apply form groups', () => {
  // The `answer` vocabulary is closed and lives in internal/ingest/applyform/display.go.
  // Every word in it has to land somewhere, or a question silently vanishes from a
  // block whose whole job is to say what the employer will ask.
  it('sorts every answer kind in the served vocabulary into a group', () => {
    const model = applyFormModel([
      ask('Website', undefined),
      ask('English level?', 'choose one'),
      ask('Which stacks?', 'choose any'),
      ask('Worked in fintech?', 'yes / no'),
      ask('Why this role?', 'written answer'),
      ask('Portfolio', 'upload'),
    ]);

    expect(model.groups.map((g) => [g.key, g.questions.map((q) => q.text)])).toEqual([
      ['short', ['Website']],
      ['choice', ['English level?', 'Which stacks?', 'Worked in fintech?']],
      ['written', ['Why this role?']],
      ['upload', ['Portfolio']],
    ]);
  });

  // The order is the feature. A reader who meets the essays first has already been
  // told the form is expensive before learning most of it is not.
  it('runs the groups from the cheapest kind to the most expensive', () => {
    const model = applyFormModel([
      ask('Portfolio', 'upload'),
      ask('Why this role?', 'written answer'),
      ask('English level?', 'choose one'),
      ask('Website', undefined),
    ]);

    expect(model.groups.map((g) => g.key)).toEqual(['short', 'choice', 'written', 'upload']);
  });

  // An empty group is not a group. Rendered, it would be a heading announcing
  // nothing — worse than silence, because the reader stops to check.
  it('omits a group no question falls into', () => {
    const model = applyFormModel([ask('Why this role?', 'written answer')]);

    expect(model.groups.map((g) => g.key)).toEqual(['written']);
  });

  // `display.go` withholds the word when it cannot normalize a control, and states
  // why: an unqualified question is one anyone assumes is answerable in a line.
  // Reading it as anything else would put a question the reader was never warned
  // about under a heading promising cheap ones.
  it('reads a question of unknown kind as answerable in a line', () => {
    const model = applyFormModel([ask('Notice period', 'signature')]);

    expect(model.groups.map((g) => [g.key, g.questions.map((q) => q.text)])).toEqual([
      ['short', ['Notice period']],
    ]);
  });

  it('has no groups when the form asks nothing', () => {
    expect(applyFormModel([]).groups).toEqual([]);
  });
});

describe('apply form summary', () => {
  it('counts the questions and the written answers among them', () => {
    const model = applyFormModel([
      ask('Website', undefined),
      ask('Why this role?', 'written answer'),
      ask('Why us?', 'written answer'),
    ]);

    expect(model.total).toBe(3);
    expect(model.written).toBe(2);
  });

  // Counted, not withheld. The rule that a zero is never PRINTED — "0 written
  // answers" states a cost that does not exist — belongs to the page, which can
  // ask `written` for it. A model returning undefined here would instead conflate
  // "none demand one" with "we do not know", and only one of those is ever true.
  it('counts no written answers when nothing demands one', () => {
    const model = applyFormModel([ask('Website', undefined), ask('English?', 'choose one')]);

    expect(model.total).toBe(2);
    expect(model.written).toBe(0);
  });
});

describe('apply form headings', () => {
  it('heads the groups when there is more than one', () => {
    const model = applyFormModel([ask('Website', undefined), ask('Why us?', 'written answer')]);

    expect(model.headed).toBe(true);
  });

  // A lone heading reading "Written answers (5)" under a summary reading
  // "5 questions · 5 written answers" says the same thing twice in two lines.
  it('drops the headings when every question is of one kind', () => {
    const model = applyFormModel([ask('Why us?', 'written answer'), ask('Why this role?', 'written answer')]);

    expect(model.headed).toBe(false);
  });

  it('drops the headings when the form asks nothing', () => {
    expect(applyFormModel([]).headed).toBe(false);
  });
});
