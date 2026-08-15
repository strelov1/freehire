import { describe, it, expect } from 'vitest';
import { buildPlan, markAnswered, showsApplicationForm } from './applyPlan';
import type { FramedField } from './protocol';
import { must } from './test-utils';

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

  // Svelte keys the checklist by this, and a duplicate key is a hard render error
  // — the whole card disappears. Two questions on one form CAN carry the same
  // label (an ATS repeats "Years" under several headings, or leaves labels off
  // and the page falls back to the same text), so the key cannot be the label.
  it('gives every item a key of its own, even when labels repeat', () => {
    const plan = buildPlan([
      field('Years', { index: 0, frame: 0, form: 0 }),
      field('Years', { index: 1, frame: 0, form: 0 }),
      field('Years', { index: 0, frame: 1, form: 0 }),
    ]);

    const keys = plan.items.map((i) => i.key);
    expect(new Set(keys).size).toBe(keys.length);
  });
});

describe('markAnswered', () => {
  it('ticks one item off and moves the counter', () => {
    const plan = buildPlan([
      field('First name', { required: true, value: 'Igor', index: 0 }),
      field('Email', { required: true, index: 1 }),
    ]);

    const after = markAnswered(plan, must(plan.items[1]).key);

    expect(after.items.map((i) => i.answered)).toEqual([true, true]);
    expect(after.required).toEqual({ answered: 2, total: 2, percent: 100 });
  });

  // The walk ticks off what it wrote. A label the plan does not carry (the agent
  // reported a question this page no longer asks) changes nothing rather than
  // inventing a row.
  it('leaves a plan that does not carry the label alone', () => {
    const plan = buildPlan([field('Email', { required: true })]);

    expect(markAnswered(plan, '9:9')).toEqual(plan);
  });

  // Two questions can carry the same label — an application and a job-alert
  // signup each have their own "Email", and one ATS form repeats "Years" under
  // several headings. Ticking one off must not tick the other, or the counter
  // reports progress that is not there.
  it('ticks off that question only, where labels repeat', () => {
    const plan = buildPlan([
      field('Email', { required: true, frame: 0, index: 0 }),
      field('Email', { required: true, frame: 1, index: 0 }),
    ]);

    const after = markAnswered(plan, must(plan.items[1]).key);

    expect(after.items.map((i) => i.answered)).toEqual([false, true]);
    expect(after.required).toEqual({ answered: 1, total: 2, percent: 50 });
  });

  it('does not touch the optional side of the counter', () => {
    const plan = buildPlan([field('Email', { required: true, index: 0 }), field('Website', { index: 1 })]);

    const after = markAnswered(plan, must(plan.items[1]).key);

    expect(after.required).toEqual({ answered: 0, total: 1, percent: 0 });
  });
});

describe('showsApplicationForm', () => {
  const upload = { frame: 0, form: 0 };

  it('is true for a form offering a CV upload, however short', () => {
    expect(showsApplicationForm([field('Email', { required: true })], [upload])).toBe(true);
  });

  // The upload is what marks an application for the FILLER, and it is gone by the
  // second step of a multi-step ATS form — where the questions the panel most
  // needs to account for are. A questionnaire that long is an application whether
  // or not it still offers a file field.
  it('is true for a long questionnaire with no upload', () => {
    const questions = [
      field('First name', { required: true }),
      field('Email', { required: true }),
      field('Years of experience', { required: true }),
      field('Notice period'),
      field('Expected compensation', { required: true }),
    ];

    expect(showsApplicationForm(questions, [])).toBe(true);
  });

  it('is false for a sign-in form', () => {
    const login = [field('Email', { required: true }), field('Password', { required: true })];

    expect(showsApplicationForm(login, [])).toBe(false);
  });

  // The walk has just written into it: whatever the markup says, this page is
  // asking an application's worth of questions.
  it('is true for a form the panel has just filled', () => {
    expect(showsApplicationForm([field('Email')], [], { filled: true })).toBe(true);
  });

  // A search box, a newsletter field, a cookie preference: pages ask short
  // questions everywhere, and a checklist over them is noise.
  it('is false for a page asking nothing in particular', () => {
    expect(showsApplicationForm([field('Search jobs')], [])).toBe(false);
    expect(showsApplicationForm([], [])).toBe(false);
  });

  // Plenty of ATS forms mark a question required with an asterisk in its label
  // and nothing in the markup. Demanding `required` in the DOM is why a real
  // application — eleven fields, filled successfully — showed no checklist at all.
  it('is true for a long questionnaire that marks nothing required in the markup', () => {
    const questions = ['One', 'Two', 'Three', 'Four', 'Five'].map((l) => field(l));

    expect(showsApplicationForm(questions, [])).toBe(true);
  });

  // Three fields is a sign-in, a search box with a filter, a newsletter row.
  it('is false just below the questionnaire threshold', () => {
    const few = ['One', 'Two', 'Three'].map((l) => field(l));

    expect(showsApplicationForm(few, [])).toBe(false);
  });
});
