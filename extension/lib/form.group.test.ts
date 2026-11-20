import { describe, it, expect, beforeEach } from 'vitest';
import { extractForm, fillByLabel } from './form';
import { formatAuthorizedCountries } from './fieldKeys';
import { labeledInput, must, reset } from './test-utils';

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

  it('fills a country checkbox group from the profile ISO-code value once formatted', () => {
    // screeninganswers.AutofillFields serves "US, DE" — the group's own options are
    // full country names, so the raw codes must go through formatAuthorizedCountries
    // first or nothing would ever match.
    const countries = checkboxGroup('Which countries are you authorized to work in?', [
      'United States',
      'Germany',
      'Canada',
    ]);
    const us = must(countries[0]);
    const de = must(countries[1]);
    const ca = must(countries[2]);

    const outcomes = fillByLabel(document, [
      {
        label: 'Which countries are you authorized to work in?',
        value: formatAuthorizedCountries('US, DE'),
      },
    ]);

    expect([us.checked, de.checked, ca.checked]).toEqual([true, true, false]);
    expect(outcomes).toEqual([{ label: 'Which countries are you authorized to work in?', status: 'filled' }]);
  });

  it('keeps the control label for a lone checkbox under a legend', () => {
    document.body.innerHTML =
      '<fieldset><legend>Consent</legend><label><input type="checkbox" name="c"> I agree to the privacy policy</label></fieldset>';
    expect(extractForm(document).map((f) => f.label)).toEqual(['I agree to the privacy policy']);
    const [o] = fillByLabel(document, [{ label: 'I agree to the privacy policy', value: 'Yes' }]);
    expect(o?.status).toBe('filled');
    expect(document.querySelector<HTMLInputElement>('input')?.checked).toBe(true);
  });
});
