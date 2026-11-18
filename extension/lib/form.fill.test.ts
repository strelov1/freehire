import { describe, it, expect, beforeEach, vi } from 'vitest';
import { extractForm, fillByLabel } from './form';

import { labeledInput, must, reset, formWith } from './test-utils';

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
