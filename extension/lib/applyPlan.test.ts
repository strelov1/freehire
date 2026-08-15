import { describe, it, expect } from 'vitest';
import { buildPlan } from './applyPlan';
import type { FramedField } from './protocol';

/** A question as the page reported it. Overrides say what the case is about. */
function field(label: string, over: Partial<FramedField> = {}): FramedField {
  return {
    frame: 0,
    index: 0,
    form: 0,
    tag: 'input',
    type: 'text',
    label,
    name: label.toLowerCase().replace(/\s+/g, '_'),
    required: false,
    value: '',
    combo: false,
    ...over,
  };
}

describe('buildPlan', () => {
  it('carries one item per question, in the order the page asks them', () => {
    const plan = buildPlan([field('First name'), field('Email'), field('City')]);

    expect(plan.items.map((i) => i.label)).toEqual(['First name', 'Email', 'City']);
  });

  // The page reports a question, not a control: a legend over 29 country checkboxes
  // arrives as ONE field carrying their labels as options, and must stay one line.
  it('keeps a grouped question as a single item', () => {
    const plan = buildPlan([
      field('Which countries do you anticipate working in?', {
        options: ['Germany', 'Poland', 'Spain'],
        value: 'Germany',
      }),
    ]);

    expect(plan.items).toHaveLength(1);
    expect(plan.items[0]?.answered).toBe(true);
  });

  it('counts a question the page already answered as answered', () => {
    const plan = buildPlan([field('Email', { value: 'a@b.test' }), field('City')]);

    expect(plan.items.map((i) => i.answered)).toEqual([true, false]);
  });

  // Whitespace is not an answer: an ATS that seeds a field with a space would
  // otherwise report a form as further along than it is.
  it('does not count whitespace as an answer', () => {
    const plan = buildPlan([field('City', { value: '   ' })]);

    expect(plan.items[0]?.answered).toBe(false);
  });

  it('counts required questions only', () => {
    const plan = buildPlan([
      field('First name', { required: true, value: 'Igor' }),
      field('Email', { required: true, value: 'a@b.test' }),
      field('City', { required: true }),
      field('Website', { value: 'https://example.test' }),
    ]);

    expect(plan.required).toEqual({ answered: 2, total: 3, percent: 67 });
  });

  // A percentage over zero required questions states nothing, so there is nothing
  // to show — the checklist still is.
  it('reports no counter when the form marks nothing required', () => {
    const plan = buildPlan([field('Website'), field('X (fka Twitter)')]);

    expect(plan.required).toBeNull();
    expect(plan.items).toHaveLength(2);
  });

  it('is empty for a page with no fields', () => {
    const plan = buildPlan([]);

    expect(plan.items).toEqual([]);
    expect(plan.required).toBeNull();
  });

  // The panel addresses a question the way a fill does — by label plus the frame and
  // form it was read from — so an item carries them verbatim.
  it('carries the frame and form each question was read from', () => {
    const plan = buildPlan([field('Email', { frame: 2, form: 1 })]);

    expect(plan.items[0]).toMatchObject({ label: 'Email', frame: 2, form: 1 });
  });

  // A question with no label cannot be addressed by label, and reads as an anonymous
  // row in the checklist. Dropping it is honest: the counter would otherwise promise
  // an item the panel cannot reach.
  it('drops a question the page gave no label', () => {
    const plan = buildPlan([field(''), field('  '), field('Email', { required: true })]);

    expect(plan.items.map((i) => i.label)).toEqual(['Email']);
    expect(plan.required).toEqual({ answered: 0, total: 1, percent: 0 });
  });
});
