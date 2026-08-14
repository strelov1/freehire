import { describe, it, expect, beforeEach } from 'vitest';
import {
  extractForm,
  extractUploads,
  matchFieldKey,
  planLabelFills,
  fillsForFrame,
  fillByLabel,
  looksLikeApplication,
  scopeToApplication,
} from './form';
import { must } from './test-utils';

function reset() {
  document.body.replaceChildren();
}

/** A <form> holding a labelled control, and optionally a resume upload. */
function formWith(id: string, labels: string[], upload = false) {
  const form = document.createElement('form');
  form.id = id;
  for (const text of labels) {
    const label = document.createElement('label');
    const control = document.createElement('input');
    control.type = 'text';
    control.id = `${id}-${text}`;
    label.setAttribute('for', control.id);
    label.textContent = text;
    form.append(label, control);
  }
  if (upload) {
    const file = document.createElement('input');
    file.type = 'file';
    form.append(file);
  }
  document.body.append(form);
  return form;
}

function labeledInput(id: string, labelText: string, attrs: Record<string, string> = {}) {
  const label = document.createElement('label');
  label.setAttribute('for', id);
  label.textContent = labelText;
  const input = document.createElement('input');
  input.id = id;
  for (const [k, v] of Object.entries(attrs)) input.setAttribute(k, v);
  document.body.append(label, input);
  return input;
}

describe('extractForm', () => {
  beforeEach(reset);

  it('indexes fillable fields with their label, type and required flag', () => {
    labeledInput('fn', 'First Name', { type: 'text', name: 'first', required: '' });
    labeledInput('em', 'Email Address', { type: 'email', name: 'email' });

    const fields = extractForm(document);

    expect(fields).toHaveLength(2);
    expect(fields[0]).toMatchObject({ index: 0, type: 'text', label: 'First Name', name: 'first', required: true });
    expect(fields[1]).toMatchObject({ index: 1, type: 'email', label: 'Email Address', required: false });
  });

  it('treats aria-required as required, which is how ATS forms mark it', () => {
    labeledInput('a', 'Employer', { type: 'text', 'aria-required': 'true' });
    labeledInput('b', 'Nickname', { type: 'text' });

    expect(extractForm(document).map((f) => [f.label, f.required])).toEqual([
      ['Employer', true],
      ['Nickname', false],
    ]);
  });

  it('skips hidden, submit and disabled controls', () => {
    labeledInput('a', 'Keep', { type: 'text' });
    labeledInput('b', 'Hidden', { type: 'hidden' });
    labeledInput('c', 'Submit', { type: 'submit' });
    labeledInput('d', 'Disabled', { type: 'text', disabled: '' });

    const fields = extractForm(document);
    expect(fields.map((f) => f.label)).toEqual(['Keep']);
  });

  it('skips controls the user cannot see', () => {
    labeledInput('a', 'Keep', { type: 'text' });
    // A real Greenhouse form carries one of these: a hidden recaptcha textarea.
    const recaptcha = document.createElement('textarea');
    recaptcha.setAttribute('name', 'g-recaptcha-response');
    recaptcha.style.display = 'none';
    // …and controls buried in a collapsed section.
    const collapsed = document.createElement('div');
    collapsed.style.display = 'none';
    const buried = document.createElement('input');
    buried.setAttribute('aria-label', 'Buried');
    collapsed.append(buried);
    document.body.append(recaptcha, collapsed);

    expect(extractForm(document).map((f) => f.label)).toEqual(['Keep']);
  });

  it('captures select options', () => {
    const label = document.createElement('label');
    label.setAttribute('for', 'country');
    label.textContent = 'Country';
    const select = document.createElement('select');
    select.id = 'country';
    for (const c of ['United States', 'Canada']) {
      const opt = document.createElement('option');
      opt.textContent = c;
      select.append(opt);
    }
    document.body.append(label, select);

    const [field] = extractForm(document);
    expect(field).toMatchObject({ tag: 'select', label: 'Country', options: ['United States', 'Canada'] });
  });
});

describe('extractUploads', () => {
  beforeEach(reset);

  it('reports a resume upload with the form that owns it', () => {
    formWith('alerts', ['Email']);
    formWith('application', ['First Name'], true);

    expect(extractUploads(document)).toEqual([{ form: 1 }]);
  });

  it('reports an upload standing outside any form, as Ashby renders it', () => {
    const file = document.createElement('input');
    file.type = 'file';
    document.body.append(file);

    expect(extractUploads(document)).toEqual([{ form: -1 }]);
  });

  it('ignores an upload the user cannot see', () => {
    const file = document.createElement('input');
    file.type = 'file';
    file.hidden = true;
    document.body.append(file);

    expect(extractUploads(document)).toEqual([]);
  });
});

describe('looksLikeApplication', () => {
  // Measured across Greenhouse (board, embed, and site-with-iframe), Lever and
  // Ashby: every open application offered a resume upload, and every page that
  // was not showing one offered none — including Roku's job-alert signup, which
  // is otherwise indistinguishable from a small application (5 fields, 3 of them
  // required).
  it('recognises a form offering a resume upload', () => {
    expect(looksLikeApplication([{ form: 0 }])).toBe(true);
  });

  it('rejects a page whose only visible form asks for nothing to upload', () => {
    expect(looksLikeApplication([])).toBe(false);
  });
});

describe('scopeToApplication', () => {
  const fields = [
    { frame: 0, form: 1, label: 'First Name' },
    { frame: 0, form: 1, label: 'Email' },
    { frame: 0, form: 2, label: 'Email' },
  ];

  it('keeps the form the upload sits in, leaving a second form alone', () => {
    // Roku carries both an application and a job-alert signup, each asking for a
    // name and an email; only the application takes a resume.
    expect(scopeToApplication(fields, [{ frame: 0, form: 1 }])).toEqual([
      { frame: 0, form: 1, label: 'First Name' },
      { frame: 0, form: 1, label: 'Email' },
    ]);
  });

  it('scopes to the frame as well, since an ATS iframe carries its own form 0', () => {
    const framed = [
      { frame: 0, form: 0, label: 'Search' },
      { frame: 18, form: 0, label: 'First Name' },
    ];
    expect(scopeToApplication(framed, [{ frame: 18, form: 0 }])).toEqual([
      { frame: 18, form: 0, label: 'First Name' },
    ]);
  });

  it('keeps every field when the upload names a group holding none', () => {
    // Rather than fill nothing at all: a page that renders its questions outside
    // the form the upload sits in is unusual, but silence would be worse.
    expect(scopeToApplication(fields, [{ frame: 3, form: 9 }])).toEqual(fields);
  });
});

describe('planLabelFills', () => {
  const values = { firstName: 'Ilya', email: 'ilya@example.com', github: '' };

  it('addresses each recognised question by its own label', () => {
    expect(planLabelFills([{ label: 'First Name' }, { label: 'Email Address' }], values)).toEqual([
      { label: 'First Name', value: 'Ilya' },
      { label: 'Email Address', value: 'ilya@example.com' },
    ]);
  });

  it('leaves a question the profile knows nothing about alone', () => {
    expect(planLabelFills([{ label: 'Why do you want to work here?' }], values)).toEqual([]);
  });

  it('skips a key the profile carries no value for', () => {
    expect(planLabelFills([{ label: 'GitHub' }], values)).toEqual([]);
  });

  it('asks for a repeated label once', () => {
    // A careers page routinely carries two forms — the application and a
    // job-alert signup — each with its own "Email". `fillByLabel` answers the
    // first question carrying the label, so asking twice only doubles the wire.
    expect(planLabelFills([{ label: 'Email' }, { label: 'email ' }], values)).toEqual([
      { label: 'Email', value: 'ilya@example.com' },
    ]);
  });

  it("carries the field's frame and form onto the fill, so it can be scoped later", () => {
    expect(planLabelFills([{ label: 'First Name', frame: 18, form: 0 }], values)).toEqual([
      { label: 'First Name', value: 'Ilya', frame: 18, form: 0 },
    ]);
  });
});

describe('fillsForFrame', () => {
  const fills = [
    { label: 'First Name', value: 'Ilya', frame: 18, form: 0 },
    { label: 'Email', value: 'ilya@example.com', frame: 0, form: 1 },
    { label: 'Referral code', value: 'ABC' },
  ];

  it('keeps only the fills addressed to the frame, plus the frame-agnostic ones', () => {
    expect(fillsForFrame(fills, 18)).toEqual([
      { label: 'First Name', value: 'Ilya', frame: 18, form: 0 },
      { label: 'Referral code', value: 'ABC' },
    ]);
  });

  it('withholds a frame-addressed fill from every other frame', () => {
    expect(fillsForFrame(fills, 7)).toEqual([{ label: 'Referral code', value: 'ABC' }]);
  });
});

describe('extractForm combo flag', () => {
  beforeEach(reset);

  it('flags custom-widget comboboxes and leaves plain controls alone', () => {
    labeledInput('plain', 'City', { type: 'text' });
    labeledInput('rs', 'Country', { type: 'text', role: 'combobox', 'aria-autocomplete': 'list' });

    const fields = extractForm(document);

    expect(fields.map((f) => [f.label, f.combo])).toEqual([
      ['City', false],
      ['Country', true],
    ]);
  });

  it('does not flag a native select as a custom widget', () => {
    const select = document.createElement('select');
    select.setAttribute('aria-label', 'Country');
    document.body.append(select);

    expect(must(extractForm(document)[0]).combo).toBe(false);
  });

  it("carries a widget's options when it exposes them without being opened", () => {
    const combo = labeledInput('wa', 'Work authorization', {
      type: 'text',
      role: 'combobox',
      'aria-controls': 'wa-listbox',
    });
    const listbox = document.createElement('div');
    listbox.id = 'wa-listbox';
    listbox.setAttribute('role', 'listbox');
    for (const text of ['Yes', 'No']) {
      const option = document.createElement('div');
      option.setAttribute('role', 'option');
      option.textContent = text;
      listbox.append(option);
    }
    document.body.append(listbox);

    const [field] = extractForm(document);
    expect(field).toMatchObject({ label: 'Work authorization', combo: true, options: ['Yes', 'No'] });
    expect(combo.value).toBe('');
  });

  it('reports no options for a widget that only renders them once opened', () => {
    // A react-select while closed: its listbox does not exist yet, so
    // aria-controls is absent. The agent must reach for combobox.open.
    labeledInput('co', 'Country', { type: 'text', role: 'combobox', 'aria-autocomplete': 'list' });

    const field = must(extractForm(document)[0]);
    expect(field.combo).toBe(true);
    expect(field.options).toBeUndefined();
  });

  it("does not lift a neighbouring widget's options onto a closed one", () => {
    // Two widgets, one open. A page-wide [role=option] sweep would hand the
    // open widget's countries to the closed sponsorship question.
    labeledInput('sp', 'Do you require sponsorship', { type: 'text', role: 'combobox' });
    labeledInput('ct', 'Country', { type: 'text', role: 'combobox', 'aria-controls': 'ct-listbox' });
    const listbox = document.createElement('div');
    listbox.id = 'ct-listbox';
    listbox.setAttribute('role', 'listbox');
    const option = document.createElement('div');
    option.setAttribute('role', 'option');
    option.textContent = 'Germany';
    listbox.append(option);
    document.body.append(listbox);

    const fields = extractForm(document);
    expect(must(fields[0]).options).toBeUndefined();
    expect(must(fields[1]).options).toEqual(['Germany']);
  });
});

describe('fillByLabel', () => {
  beforeEach(reset);

  it('writes text, checkbox and native-select values addressed by label', () => {
    const name = labeledInput('fn', 'First Name *', { type: 'text' });
    const agree = labeledInput('ag', 'I agree', { type: 'checkbox' });
    const label = document.createElement('label');
    label.setAttribute('for', 'country');
    label.textContent = 'Country';
    const select = document.createElement('select');
    select.id = 'country';
    for (const c of ['United States', 'Canada']) {
      const opt = document.createElement('option');
      opt.textContent = c;
      select.append(opt);
    }
    document.body.append(label, select);

    const outcomes = fillByLabel(document, [
      { label: 'First name', value: 'Ilya' },
      { label: 'I agree', value: 'true' },
      { label: 'Country', value: 'Canada' },
    ]);

    expect(name.value).toBe('Ilya');
    expect(agree.checked).toBe(true);
    expect(select.value).toBe('Canada');
    expect(outcomes.every((o) => o.status === 'filled')).toBe(true);
  });

  it('lands values on the right field after a re-render changes the control count', () => {
    labeledInput('em', 'Email', { type: 'email' });
    const observed = extractForm(document).map((f) => f.label);
    expect(observed).toEqual(['Email']);

    // The form re-renders: a new control appears *before* the observed one, so
    // any positional addressing would now target the wrong element.
    const extra = document.createElement('input');
    extra.type = 'text';
    extra.setAttribute('aria-label', 'Referral code');
    document.body.prepend(extra);

    fillByLabel(document, [{ label: 'Email', value: 'ilya@example.com' }]);

    expect(extra.value).toBe('');
    expect(must(document.querySelector<HTMLInputElement>('#em')).value).toBe('ilya@example.com');
  });

  it('skips a custom-widget combobox instead of writing stale text into it', () => {
    const combo = labeledInput('co', 'Country', { type: 'text', role: 'combobox' });

    const outcomes = fillByLabel(document, [{ label: 'Country', value: 'Canada' }]);

    expect(combo.value).toBe('');
    expect(outcomes).toEqual([{ label: 'Country', status: 'deferred_combobox' }]);
  });

  it('reports an unmatched label and a select value with no matching option', () => {
    const select = document.createElement('select');
    select.setAttribute('aria-label', 'Country');
    const opt = document.createElement('option');
    opt.textContent = 'Canada';
    select.append(opt);
    document.body.append(select);

    const outcomes = fillByLabel(document, [
      { label: 'Favourite colour', value: 'blue' },
      { label: 'Country', value: 'Norfolk Island' },
    ]);

    expect(outcomes).toEqual([
      { label: 'Favourite colour', status: 'not_found' },
      { label: 'Country', status: 'no_option' },
    ]);
  });

  it('writes only the target form when two forms share a label', () => {
    // Roku carries both an application and a job-alert signup, each asking for
    // an "Email"; a fill scoped to the application form must not spill into
    // the signup's same-labeled control sitting right next to it.
    const application = formWith('application', ['Email']);
    const signup = formWith('signup', ['Email']);
    const applicationInput = must(application.querySelector<HTMLInputElement>('input'));
    const signupInput = must(signup.querySelector<HTMLInputElement>('input'));

    const outcomes = fillByLabel(document, [{ label: 'Email', value: 'ilya@example.com', form: 0 }]);

    expect(applicationInput.value).toBe('ilya@example.com');
    expect(signupInput.value).toBe('');
    expect(outcomes).toEqual([{ label: 'Email', status: 'filled' }]);
  });

  it('reports not_found rather than fall back to an out-of-form match', () => {
    // The target form (0) exists, but only the other form (1) carries this
    // label — the fill must not silently land there.
    formWith('application', ['First Name']);
    const signup = formWith('signup', ['Email']);
    const signupInput = must(signup.querySelector<HTMLInputElement>('input'));

    const outcomes = fillByLabel(document, [{ label: 'Email', value: 'ilya@example.com', form: 0 }]);

    expect(signupInput.value).toBe('');
    expect(outcomes).toEqual([{ label: 'Email', status: 'not_found' }]);
  });
});

let uniqueId = 0;

/** Labelled controls of one kind, appended to `parent`. */
function optionControls(parent: Element, optionLabels: string[], type: string, name: string) {
  return optionLabels.map((text) => {
    const id = `opt-${uniqueId++}`;
    const label = document.createElement('label');
    label.setAttribute('for', id);
    label.textContent = text;
    const input = document.createElement('input');
    input.id = id;
    input.type = type;
    input.name = name;
    parent.append(label, input);
    return input;
  });
}

/** A fieldset asking one question through several controls. */
function checkboxGroup(legendText: string, optionLabels: string[], type = 'checkbox', name = `q${uniqueId}[]`) {
  const fieldset = document.createElement('fieldset');
  const legend = document.createElement('legend');
  legend.textContent = legendText;
  fieldset.append(legend);
  const inputs = optionControls(fieldset, optionLabels, type, name);
  document.body.append(fieldset);
  return inputs;
}

describe('extractForm grouping', () => {
  beforeEach(reset);

  it('reports a checkbox group as one field carrying the question and its options', () => {
    checkboxGroup('Which countries do you anticipate working in? *', ['Germany', 'Poland', 'Spain']);

    const fields = extractForm(document);

    expect(fields).toHaveLength(1);
    expect(fields[0]).toMatchObject({
      label: 'Which countries do you anticipate working in? *',
      options: ['Germany', 'Poland', 'Spain'],
      type: 'checkbox',
      required: false,
    });
  });

  it('groups a radio set the same way', () => {
    checkboxGroup('Do you require sponsorship?', ['Yes', 'No'], 'radio');

    const [field] = extractForm(document);
    expect(field).toMatchObject({ label: 'Do you require sponsorship?', options: ['Yes', 'No'] });
  });

  it('reports which options of a group are already chosen as its value', () => {
    const poland = must(checkboxGroup('Countries', ['Germany', 'Poland', 'Spain'])[1]);
    poland.checked = true;

    expect(must(extractForm(document)[0]).value).toBe('Poland');
  });

  it('does not group text inputs that merely share a fieldset', () => {
    // An address fieldset: each control is its own question despite the shared
    // legend, so grouping them would hide three fields from the agent.
    const fieldset = document.createElement('fieldset');
    const legend = document.createElement('legend');
    legend.textContent = 'Address';
    fieldset.append(legend);
    for (const name of ['Street', 'City', 'Postal code']) {
      const input = document.createElement('input');
      input.type = 'text';
      input.setAttribute('aria-label', name);
      fieldset.append(input);
    }
    document.body.append(fieldset);

    expect(extractForm(document).map((f) => f.label)).toEqual(['Street', 'City', 'Postal code']);
  });

  it('does not group a lone checkbox, whose own label is the question', () => {
    const fieldset = document.createElement('fieldset');
    const legend = document.createElement('legend');
    legend.textContent = 'Consent';
    const label = document.createElement('label');
    label.setAttribute('for', 'tos');
    label.textContent = 'I agree to the terms';
    const input = document.createElement('input');
    input.id = 'tos';
    input.type = 'checkbox';
    fieldset.append(legend, label, input);
    document.body.append(fieldset);

    expect(extractForm(document).map((f) => f.label)).toEqual(['I agree to the terms']);
  });

  it('leaves a legend-less fieldset ungrouped, having no question to carry', () => {
    const fieldset = document.createElement('fieldset');
    for (const text of ['Germany', 'Poland']) {
      const label = document.createElement('label');
      label.setAttribute('for', text);
      label.textContent = text;
      const input = document.createElement('input');
      input.id = text;
      input.type = 'checkbox';
      fieldset.append(label, input);
    }
    document.body.append(fieldset);

    expect(extractForm(document).map((f) => f.label)).toEqual(['Germany', 'Poland']);
  });

  it('keeps two questions apart when one fieldset holds both', () => {
    // A shared "Demographic Questions" legend over two unrelated questions.
    // Grouping on the fieldset alone would merge them into one field offering
    // all four options, and the second question would become unanswerable.
    const fieldset = document.createElement('fieldset');
    const legend = document.createElement('legend');
    legend.textContent = 'Demographic Questions';
    fieldset.append(legend);
    optionControls(fieldset, ['Yes', 'No'], 'radio', 'veteran');
    optionControls(fieldset, ['Germany', 'Poland'], 'checkbox', 'countries[]');
    document.body.append(fieldset);

    const fields = extractForm(document);

    expect(fields).toHaveLength(2);
    expect(fields.map((f) => f.options)).toEqual([
      ['Yes', 'No'],
      ['Germany', 'Poland'],
    ]);
  });

  it('leaves nameless controls ungrouped, having nothing that says they answer together', () => {
    // A React-controlled form omits `name` entirely, so nothing distinguishes one
    // question from two inside a shared legend. Grouping on the container alone
    // would merge them and hide the second; a long report is the safer failure.
    const fieldset = document.createElement('fieldset');
    const legend = document.createElement('legend');
    legend.textContent = 'Demographic Questions';
    fieldset.append(legend);
    optionControls(fieldset, ['Yes', 'No', 'Germany', 'Poland'], 'checkbox', '');
    document.body.append(fieldset);

    expect(extractForm(document).map((f) => f.label)).toEqual(['Yes', 'No', 'Germany', 'Poland']);
  });

  it('groups a labelled container that is not a fieldset', () => {
    const heading = document.createElement('h3');
    heading.id = 'q-visa';
    heading.textContent = 'Do you now or will you require sponsorship?';
    const group = document.createElement('div');
    group.setAttribute('role', 'radiogroup');
    group.setAttribute('aria-labelledby', 'q-visa');
    optionControls(group, ['Yes', 'No'], 'radio', 'visa');
    document.body.append(heading, group);

    const fields = extractForm(document);

    expect(fields).toHaveLength(1);
    expect(fields[0]).toMatchObject({
      label: 'Do you now or will you require sponsorship?',
      options: ['Yes', 'No'],
    });
  });

  it('leaves an unlabelled container ungrouped, having no question to carry', () => {
    const group = document.createElement('div');
    group.setAttribute('role', 'radiogroup');
    optionControls(group, ['Yes', 'No'], 'radio', 'visa');
    document.body.append(group);

    expect(extractForm(document).map((f) => f.label)).toEqual(['Yes', 'No']);
  });

  it('reaches a plain field standing behind a group', () => {
    // The group sits *before* the plain field and collapses three controls into
    // one question, so anything addressing by position has to survive that.
    const germany = must(checkboxGroup('Countries', ['Germany', 'Poland', 'Spain'])[0]);
    const email = labeledInput('em', 'Email', { type: 'email' });

    const emailField = must(extractForm(document).find((f) => f.label === 'Email'));
    fillByLabel(document, [{ label: emailField.label, value: 'ilya@example.com' }]);

    expect(email.value).toBe('ilya@example.com');
    expect(germany.checked).toBe(false);
  });
});

describe('fillByLabel on a group', () => {
  beforeEach(reset);

  it('ticks the control for the chosen option, reporting the outcome for the question', () => {
    const countries = checkboxGroup('Which countries?', ['Germany', 'Poland', 'Spain']);
    const germany = must(countries[0]);
    const poland = must(countries[1]);

    const outcomes = fillByLabel(document, [{ label: 'Which countries?', value: 'Poland' }]);

    expect(poland.checked).toBe(true);
    expect(germany.checked).toBe(false);
    expect(outcomes).toEqual([{ label: 'Which countries?', status: 'filled' }]);
  });

  it('accepts back the joined value it reported, ticking every named option', () => {
    const countries = checkboxGroup('Which countries?', ['Germany', 'Poland', 'Spain']);
    const germany = must(countries[0]);
    const poland = must(countries[1]);
    const spain = must(countries[2]);

    const outcomes = fillByLabel(document, [{ label: 'Which countries?', value: 'Germany, Spain' }]);

    expect([germany.checked, poland.checked, spain.checked]).toEqual([true, false, true]);
    expect(outcomes).toEqual([{ label: 'Which countries?', status: 'filled' }]);
  });

  it("prefers an option's own comma to a list separator", () => {
    // Splitting first would look for "Korea" and "Republic of", find neither,
    // and refuse a question the group does in fact offer.
    const korea = must(checkboxGroup('Which countries?', ['Korea, Republic of', 'Japan'])[0]);

    fillByLabel(document, [{ label: 'Which countries?', value: 'Korea, Republic of' }]);

    expect(korea.checked).toBe(true);
  });

  it('round-trips a value whose options contain commas of their own', () => {
    // Both chosen, so the reported value is "Germany, Korea, Republic of" — which
    // splits into three parts the group has never heard of. Only matching the
    // longest offered option at each step recovers the two real answers.
    const countries = checkboxGroup('Which countries?', ['Germany', 'Korea, Republic of']);
    const germany = must(countries[0]);
    const korea = must(countries[1]);
    germany.checked = true;
    korea.checked = true;
    const reported = must(extractForm(document)[0]).value;
    expect(reported).toBe('Germany, Korea, Republic of');

    germany.checked = false;
    korea.checked = false;
    const outcomes = fillByLabel(document, [{ label: 'Which countries?', value: reported }]);

    expect([germany.checked, korea.checked]).toEqual([true, true]);
    expect(outcomes).toEqual([{ label: 'Which countries?', status: 'filled' }]);
  });

  it('ticks nothing when only some of the named options are offered', () => {
    const inputs = checkboxGroup('Which countries?', ['Germany', 'Poland']);

    const outcomes = fillByLabel(document, [{ label: 'Which countries?', value: 'Germany, Norfolk Island' }]);

    expect(inputs.some((i) => i.checked)).toBe(false);
    expect(outcomes).toEqual([{ label: 'Which countries?', status: 'no_option' }]);
  });

  it("leaves the user's own choice alone rather than clearing it", () => {
    const countries = checkboxGroup('Which countries?', ['Germany', 'Poland']);
    const germany = must(countries[0]);
    const poland = must(countries[1]);
    poland.checked = true;

    fillByLabel(document, [{ label: 'Which countries?', value: 'Germany' }]);

    expect(germany.checked).toBe(true);
    expect(poland.checked).toBe(true);
  });

  it('lets a radio group hold one answer, unchecking the sibling the browser clears', () => {
    const options = checkboxGroup('Do you require sponsorship?', ['Yes', 'No'], 'radio', 'visa');
    const yes = must(options[0]);
    const no = must(options[1]);
    yes.checked = true;

    fillByLabel(document, [{ label: 'Do you require sponsorship?', value: 'No' }]);

    expect(no.checked).toBe(true);
    expect(yes.checked).toBe(false);
  });

  it('leaves the group untouched when the option is not one it offers', () => {
    const inputs = checkboxGroup('Which countries?', ['Germany', 'Poland']);

    const outcomes = fillByLabel(document, [{ label: 'Which countries?', value: 'Norfolk Island' }]);

    expect(inputs.some((i) => i.checked)).toBe(false);
    expect(outcomes).toEqual([{ label: 'Which countries?', status: 'no_option' }]);
  });
});

describe('matchFieldKey', () => {
  it('maps common labels to canonical profile keys', () => {
    expect(matchFieldKey('First Name')).toBe('firstName');
    expect(matchFieldKey('Legal Last Name *')).toBe('lastName');
    expect(matchFieldKey('E-mail Address')).toBe('email');
    expect(matchFieldKey('LinkedIn URL')).toBe('linkedin');
  });

  it('returns null for an unknown label', () => {
    expect(matchFieldKey('Favourite colour')).toBeNull();
  });

  it('still reads a label an ATS has decorated', () => {
    // Greenhouse suffixes each label with a hash of its own.
    expect(matchFieldKey('First Name\n    (required)\n  \n  e85441b6')).toBe('firstName');
    expect(matchFieldKey('LinkedIn Profile 947f0cc4')).toBe('linkedin');
  });

  it('does not let a word buried in a question claim a profile key', () => {
    // Live on Roku. "country" appears mid-sentence in a Yes/No question; taking
    // it would put the user's country into a radio group.
    expect(
      matchFieldKey(
        'Are you authorized to lawfully work for Roku in the country to which you are applying?',
      ),
    ).toBeNull();
    expect(matchFieldKey('Which city office would your manager be based in?')).toBeNull();
  });

  it('does not match a longer word that merely starts the same way', () => {
    expect(matchFieldKey('Citywide preference')).toBeNull();
  });
});
