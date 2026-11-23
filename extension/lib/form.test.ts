import { describe, it, expect, beforeEach } from 'vitest';
import { extractForm, extractUploads } from './form';
import { revealField } from './revealField';
import { planLabelFills, fillsForFrame, looksLikeApplication, scopeToApplication } from './fillPlan';
import { labeledInput, must, reset, formWith } from './test-utils';

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
    expect(scopeToApplication(framed, [{ frame: 18, form: 0 }])).toEqual([{ frame: 18, form: 0, label: 'First Name' }]);
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

  it("reports a native select's chosen option as its text, not its code", () => {
    // The option's value attribute is the ATS's internal id; the text is the answer.
    const select = document.createElement('select');
    select.setAttribute('aria-label', 'Veteran status');
    const choices: [string, string][] = [
      ['', 'Select...'],
      ['4417', 'I am not a veteran'],
    ];
    for (const [value, text] of choices) {
      const option = document.createElement('option');
      option.value = value;
      option.textContent = text;
      select.append(option);
    }
    document.body.append(select);
    expect(must(extractForm(document)[0]).value).toBe('');

    select.value = '4417';
    expect(must(extractForm(document)[0]).value).toBe('I am not a veteran');
  });

  it('reports a lone checkbox as Yes when ticked and empty when not', () => {
    // Its value attribute is "on" either way, so reading that made an unticked
    // consent box look answered.
    const box = labeledInput('arb', 'Agreement to Arbitrate', { type: 'checkbox' });
    expect(must(extractForm(document)[0]).value).toBe('');

    (box as HTMLInputElement).checked = true;
    expect(must(extractForm(document)[0]).value).toBe('Yes');
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

  it('reports the value a widget displays, since its input stays empty after a commit', () => {
    // A react-select keeps the chosen answer in its own markup, never in the
    // input. Reading the input alone made every answered dropdown look unanswered
    // — the checklist never ticked it and the answers could not be offered for saving.
    const control = document.createElement('div');
    const chosen = document.createElement('div');
    chosen.className = 'css-1dimb5e-singleValue';
    chosen.textContent = 'No';
    const label = document.createElement('label');
    label.setAttribute('for', 'iv');
    label.textContent = 'Have you interviewed with us before?';
    const input = document.createElement('input');
    input.id = 'iv';
    input.setAttribute('role', 'combobox');
    control.append(chosen, input);
    document.body.append(label, control);

    expect(must(extractForm(document)[0])).toMatchObject({ combo: true, value: 'No' });
  });

  it('reports no value for a widget showing its placeholder', () => {
    const control = document.createElement('div');
    const placeholder = document.createElement('div');
    placeholder.className = 'css-1jqq78o-placeholder';
    placeholder.textContent = 'Select...';
    const input = document.createElement('input');
    input.setAttribute('role', 'combobox');
    input.setAttribute('aria-label', 'Gender');
    control.append(placeholder, input);
    document.body.append(control);

    expect(must(extractForm(document)[0]).value).toBe('');
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

describe('revealField', () => {
  beforeEach(reset);

  /** A form whose control carries an inline style of its own, so a reveal that
   *  fails to restore it is visible. */
  function labelled(label: string, style = 'color: rebeccapurple') {
    const form = document.createElement('form');
    const el = document.createElement('input');
    el.type = 'text';
    el.id = 'reveal-target';
    el.setAttribute('style', style);
    const lab = document.createElement('label');
    lab.setAttribute('for', el.id);
    lab.textContent = label;
    form.append(lab, el);
    document.body.append(form);
    return el;
  }

  it('scrolls the question it names into view', () => {
    const el = labelled('Email');
    let scrolled: ScrollIntoViewOptions | undefined;
    el.scrollIntoView = (opts?: boolean | ScrollIntoViewOptions) => {
      scrolled = opts as ScrollIntoViewOptions;
    };

    expect(revealField(document, { label: 'Email' })).toBe(true);
    expect(scrolled?.block).toBe('center');
  });

  it('reports a label the page does not carry, and touches nothing', () => {
    labelled('Email');

    expect(revealField(document, { label: 'Favourite colour' })).toBe(false);
  });

  // The outline is written onto a page freehire does not own, so it has to leave
  // the element exactly as it found it — including an inline style of its own.
  it('restores the inline style it borrowed', async () => {
    const el = labelled('Email', 'color: rebeccapurple');
    el.scrollIntoView = () => {};

    revealField(document, { label: 'Email', outlineMs: 5 });
    expect(el.getAttribute('style')).not.toBe('color: rebeccapurple');

    await new Promise((r) => setTimeout(r, 20));
    expect(el.getAttribute('style')).toBe('color: rebeccapurple');
  });

  it('focuses the control only when asked', () => {
    const el = labelled('Email');
    el.scrollIntoView = () => {};

    revealField(document, { label: 'Email' });
    expect(document.activeElement).not.toBe(el);

    revealField(document, { label: 'Email', focus: true });
    expect(document.activeElement).toBe(el);
  });
});

describe('revealField, revealed twice', () => {
  beforeEach(reset);

  // Two reveals of the same control inside the outline's lifetime: without a
  // per-control record of what was borrowed, the second call saves the FIRST
  // call's outline as the original and restores it for good.
  it('still gives the page its own style back', async () => {
    const form = document.createElement('form');
    const el = document.createElement('input');
    el.type = 'text';
    el.id = 'twice';
    el.setAttribute('style', 'color: rebeccapurple');
    const lab = document.createElement('label');
    lab.setAttribute('for', el.id);
    lab.textContent = 'Email';
    form.append(lab, el);
    document.body.append(form);
    el.scrollIntoView = () => {};

    revealField(document, { label: 'Email', outlineMs: 30 });
    revealField(document, { label: 'Email', outlineMs: 30 });

    await new Promise((r) => setTimeout(r, 80));
    expect(el.getAttribute('style')).toBe('color: rebeccapurple');
  });
});

describe('LinkedIn-style radios named after their question', () => {
  const q = 'Will you now or in the future require sponsorship?*';
  const radio = (id: string, option: string) =>
    `<div><div><input id="${id}" type="radio" name="g" aria-label="${q}"><label for="${id}"></label></div><p>${option}</p></div>`;

  it('lists the question once, with the visible option text as options, and reads the * as required', () => {
    document.body.innerHTML = `<fieldset role="radiogroup">${radio('a', 'Yes')}${radio('b', 'No')}</fieldset>`;
    const fields = extractForm(document);
    expect(fields).toHaveLength(1);
    expect(fields[0]).toMatchObject({ label: q, options: ['Yes', 'No'], required: true });
  });
});

describe('control naming', () => {
  it('prefers the visible label over the placeholder', () => {
    document.body.innerHTML =
      '<div><label>Location (city)*</label><input type="text" placeholder="Enter city or location"></div>';
    expect(extractForm(document)[0]?.label).toBe('Location (city)*');
  });

  it('prefers the visible label over an aria-label that repeats the placeholder', () => {
    document.body.innerHTML =
      '<div><label>Location (city)*</label><input type="text" aria-label="Enter city" placeholder="Enter city"></div>';
    expect(extractForm(document)[0]?.label).toBe('Location (city)*');
  });
});
