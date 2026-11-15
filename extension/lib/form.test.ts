import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
  revealField,
  extractForm,
  extractUploads,
  planLabelFills,
  fillsForFrame,
  fillByLabel,
  looksLikeApplication,
  scopeToApplication,
} from './form';
import { matchFieldKey, formatAuthorizedCountries } from './fieldKeys';
import { labeledInput, must, reset } from './test-utils';

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

  it('reports wrong_form rather than fall back to an out-of-form match', () => {
    // The target form (0) exists, but only the other form (1) carries this
    // label — the fill must not silently land there. `wrong_form` and not
    // `not_found`: the label IS on the page, so the harness's next step is to
    // correct the index, not to give up on the question.
    formWith('application', ['First Name']);
    const signup = formWith('signup', ['Email']);
    const signupInput = must(signup.querySelector<HTMLInputElement>('input'));

    const outcomes = fillByLabel(document, [{ label: 'Email', value: 'ilya@example.com', form: 0 }]);

    expect(signupInput.value).toBe('');
    expect(outcomes).toEqual([{ label: 'Email', status: 'wrong_form' }]);
  });

  // `collectFillable` drops a disabled or hidden control before anything matches
  // against it, so such a question was indistinguishable from one the page never
  // asked — and a harness reading `not_found` retries a fill that cannot land
  // until the page itself changes.
  it('reports not_fillable for a control that is present but disabled', () => {
    const application = formWith('application', ['Email']);
    must(application.querySelector<HTMLInputElement>('input')).disabled = true;

    const outcomes = fillByLabel(document, [{ label: 'Email', value: 'ilya@example.com' }]);

    expect(outcomes).toEqual([{ label: 'Email', status: 'not_fillable' }]);
  });

  it('still reports not_found for a label the page does not carry at all', () => {
    formWith('application', ['Email']);

    const outcomes = fillByLabel(document, [{ label: 'Favourite colour', value: 'blue' }]);

    expect(outcomes).toEqual([{ label: 'Favourite colour', status: 'not_found' }]);
  });

  // The same two forms as the test above, but with the fill naming neither. Taking
  // the first match here is a coin flip between the application and a job-alert
  // signup, and losing it writes the candidate's address into a form they did not
  // choose — reported as `filled`, so nothing downstream can tell.
  it('refuses a fill whose label matches more than one question and names no form', () => {
    const application = formWith('application', ['Email']);
    const signup = formWith('signup', ['Email']);
    const applicationInput = must(application.querySelector<HTMLInputElement>('input'));
    const signupInput = must(signup.querySelector<HTMLInputElement>('input'));

    const outcomes = fillByLabel(document, [{ label: 'Email', value: 'ilya@example.com' }]);

    expect(applicationInput.value).toBe('');
    expect(signupInput.value).toBe('');
    expect(outcomes).toEqual([{ label: 'Email', status: 'ambiguous' }]);
  });

  // The refusal is not a dead end: the index the harness read off `read_form` is
  // exactly what resolves it.
  it('fills the named form on a page it would otherwise refuse', () => {
    formWith('application', ['Email']);
    const signup = formWith('signup', ['Email']);
    const signupInput = must(signup.querySelector<HTMLInputElement>('input'));

    const outcomes = fillByLabel(document, [{ label: 'Email', value: 'ilya@example.com', form: 1 }]);

    expect(signupInput.value).toBe('ilya@example.com');
    expect(outcomes).toEqual([{ label: 'Email', status: 'filled' }]);
  });

  // Telling `not_fillable` from `not_found` means looking at the controls
  // `collectFillable` dropped, and that scan is over the WHOLE document. Doing it
  // per miss puts an ATS form's several hundred controls through `extractLabel` —
  // which walks `getElementById` for `aria-labelledby` — once for every unanswered
  // question, synchronously, in the content script. One pass per call is enough,
  // and a call whose fills all land should not pay for it at all.
  it('scans the document for unfillable controls at most once, however many fills miss', () => {
    formWith('application', ['Email']);
    const seen = vi.spyOn(document, 'querySelectorAll');

    fillByLabel(document, [
      { label: 'Nothing A', value: 'x' },
      { label: 'Nothing B', value: 'y' },
      { label: 'Nothing C', value: 'z' },
    ]);

    const scans = seen.mock.calls.filter(([sel]) => sel === 'input, select, textarea').length;
    seen.mockRestore();

    // One for collectQuestions, at most one for the miss path — not one per miss.
    expect(scans).toBeLessThanOrEqual(2);
  });

  it('does not scan for unfillable controls when every fill lands', () => {
    formWith('application', ['Email']);
    const seen = vi.spyOn(document, 'querySelectorAll');

    fillByLabel(document, [{ label: 'Email', value: 'ilya@example.com' }]);

    const scans = seen.mock.calls.filter(([sel]) => sel === 'input, select, textarea').length;
    seen.mockRestore();

    // Only collectQuestions'. Nothing missed, so nothing to explain.
    expect(scans).toBe(1);
  });

  // Ashby renders its application outside any <form>, which formIndex reports as -1.
  // That is a scope like any other and must resolve — otherwise the one platform the
  // shape was documented for is exactly the one the addressing cannot reach.
  it('resolves form -1 to the question standing outside any form', () => {
    const loose = document.createElement('input');
    loose.type = 'text';
    loose.id = 'loose-email';
    const looseLabel = document.createElement('label');
    looseLabel.setAttribute('for', loose.id);
    looseLabel.textContent = 'Email';
    document.body.append(looseLabel, loose);
    const signup = formWith('signup', ['Email']);
    const signupInput = must(signup.querySelector<HTMLInputElement>('input'));

    const outcomes = fillByLabel(document, [{ label: 'Email', value: 'ilya@example.com', form: -1 }]);

    expect(loose.value).toBe('ilya@example.com');
    expect(signupInput.value).toBe('');
    expect(outcomes).toEqual([{ label: 'Email', status: 'filled' }]);
  });

  // A label carried once is not ambiguous, so the refusal cannot reach the ordinary
  // page — which is every page with one form on it.
  it('still fills an unscoped fill whose label is carried once', () => {
    const application = formWith('application', ['Email']);
    const applicationInput = must(application.querySelector<HTMLInputElement>('input'));

    const outcomes = fillByLabel(document, [{ label: 'Email', value: 'ilya@example.com' }]);

    expect(applicationInput.value).toBe('ilya@example.com');
    expect(outcomes).toEqual([{ label: 'Email', status: 'filled' }]);
  });
});

describe('formatAuthorizedCountries', () => {
  it('turns ISO codes into the country names a checkbox group carries as options', () => {
    expect(formatAuthorizedCountries('US, DE')).toBe('United States, Germany');
  });

  it('is empty for an empty value, rather than an empty-string entry', () => {
    expect(formatAuthorizedCountries('')).toBe('');
  });
});

describe('matchFieldKey', () => {
  it('maps common labels to canonical profile keys', () => {
    expect(matchFieldKey('First Name')).toBe('firstName');
    expect(matchFieldKey('Legal Last Name *')).toBe('lastName');
    expect(matchFieldKey('E-mail Address')).toBe('email');
    expect(matchFieldKey('LinkedIn URL')).toBe('linkedin');
  });

  it('maps German labels to canonical profile keys', () => {
    expect(matchFieldKey('Vorname')).toBe('firstName');
    expect(matchFieldKey('Vorname (erforderlich)')).toBe('firstName');
    expect(matchFieldKey('Nachname *')).toBe('lastName');
    expect(matchFieldKey('E-Mail-Adresse')).toBe('email');
    expect(matchFieldKey('Telefonnummer *')).toBe('phone');
    expect(matchFieldKey('PLZ')).toBe('postalCode');
    expect(matchFieldKey('Wohnort')).toBe('city');
    expect(matchFieldKey('Gehaltsvorstellung (brutto/Jahr)')).toBe('desiredSalary');
    expect(matchFieldKey('Kündigungsfrist')).toBe('noticePeriod');
  });

  it('does not let a German word claim a longer German word', () => {
    expect(matchFieldKey('Ortsteil')).toBeNull();
    expect(matchFieldKey('Landkreis')).toBeNull();
  });

  it('does not let a German word mid-sentence claim a key', () => {
    expect(matchFieldKey('In welchem Land haben Sie studiert?')).toBeNull();
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
      matchFieldKey('Are you authorized to lawfully work for Roku in the country to which you are applying?'),
    ).toBeNull();
    expect(matchFieldKey('Which city office would your manager be based in?')).toBeNull();
  });

  it('does not match a longer word that merely starts the same way', () => {
    expect(matchFieldKey('Citywide preference')).toBeNull();
  });

  it('maps screening-answer questions to their canonical keys', () => {
    expect(matchFieldKey('Which countries are you authorized to work in?')).toBe('authorizedCountries');
    expect(matchFieldKey('Will you now or in the future require sponsorship to work in this country?')).toBe(
      'visaSponsorshipNeeded',
    );
    expect(matchFieldKey('Desired salary')).toBe('desiredSalary');
    expect(matchFieldKey('What is your notice period?')).toBe('noticePeriod');
    expect(matchFieldKey('Are you willing to relocate?')).toBe('willingToRelocate');
    expect(matchFieldKey('Are you at least 18 years of age?')).toBe('age18OrOlder');
  });

  it('leaves a plain work-authorization Yes/No question unmatched', () => {
    // That question has no boolean field in the profile — only a list of
    // authorized countries — so answering it would put the wrong shape of
    // value into a Yes/No control. See the Roku case above.
    expect(matchFieldKey('Are you authorized to work in this country?')).toBeNull();
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
