import { describe, it, expect, beforeEach } from 'vitest';
import * as combobox from './combobox';

function reset() {
  document.body.replaceChildren();
}

interface FixtureOptions {
  /**
   * What the widget displays once an option is committed, given the option's
   * text. Defaults to the option itself; a live phone-code widget commits
   * "+358" for the option "Åland Islands +358".
   */
  commitAs?: (option: string) => string;
  /** A widget that ignores dispatched events — the CDP-or-nothing case. */
  deaf?: boolean;
}

/**
 * A react-select-shaped widget, built to the two properties the live probe found
 * on Greenhouse's forms: its listbox does not exist until the control is
 * pressed, and it appears on a *later tick*, because React does not re-render
 * synchronously. A fixture that opened synchronously would let a primitive that
 * merely dispatches-and-reads pass.
 */
function reactSelect(labelText: string, options: string[], { commitAs = (o) => o, deaf = false }: FixtureOptions = {}) {
  const slug = `w${document.body.children.length}`;
  const label = document.createElement('label');
  label.setAttribute('for', slug);
  label.textContent = labelText;

  const control = document.createElement('div');
  control.className = 'select__control css-13cymwt-control';
  const placeholder = document.createElement('div');
  placeholder.className = 'css-1jqq78o-placeholder';
  placeholder.textContent = 'Select...';
  const input = document.createElement('input');
  input.id = slug;
  input.type = 'text';
  input.setAttribute('role', 'combobox');
  input.setAttribute('aria-expanded', 'false');
  control.append(placeholder, input);
  document.body.append(label, control);

  const listboxId = `react-select-${slug}-listbox`;

  function commit(optionText: string) {
    document.getElementById(listboxId)?.remove();
    input.removeAttribute('aria-controls');
    input.setAttribute('aria-expanded', 'false');
    placeholder.remove();
    const chosen = control.querySelector('.css-1dimb5e-singleValue') ?? document.createElement('div');
    chosen.className = 'css-1dimb5e-singleValue';
    chosen.textContent = commitAs(optionText);
    control.prepend(chosen);
  }

  function openMenu() {
    if (document.getElementById(listboxId)) return;
    const listbox = document.createElement('div');
    listbox.id = listboxId;
    listbox.setAttribute('role', 'listbox');
    for (const text of options) {
      const option = document.createElement('div');
      option.setAttribute('role', 'option');
      option.textContent = text;
      // react-select commits on the option's own press, again a tick later.
      option.addEventListener('mousedown', () => setTimeout(() => commit(text), 0));
      listbox.append(option);
    }
    // The menu is portalled to the body, which is why a widget's options must be
    // reached through its aria-controls rather than by looking inside it.
    document.body.append(listbox);
    input.setAttribute('aria-controls', listboxId);
    input.setAttribute('aria-expanded', 'true');
  }

  if (!deaf) control.addEventListener('mousedown', () => setTimeout(openMenu, 0));
  return { input, control };
}

describe('combobox.open', () => {
  beforeEach(reset);

  it('reveals the listbox of the widget carrying the label', async () => {
    const { input } = reactSelect('Country', ['Germany', 'Poland']);

    expect(await combobox.open(document, 'Country')).toEqual({ status: 'opened' });
    expect(input.getAttribute('aria-expanded')).toBe('true');
  });

  it('reports a widget that ignores the events it can dispatch', async () => {
    reactSelect('Country', ['Germany'], { deaf: true });

    expect(await combobox.open(document, 'Country')).toEqual({ status: 'did_not_open' });
  });

  it('reports not-found for a label that addresses no widget', async () => {
    reactSelect('Country', ['Germany']);

    expect(await combobox.open(document, 'Favourite colour')).toEqual({ status: 'not_found' });
  });

  it('opens the widget the label names, not its neighbour', async () => {
    const { input: country } = reactSelect('Country', ['Germany']);
    const { input: sponsorship } = reactSelect('Do you require sponsorship', ['Yes', 'No']);

    await combobox.open(document, 'Do you require sponsorship');

    expect(sponsorship.getAttribute('aria-expanded')).toBe('true');
    expect(country.getAttribute('aria-expanded')).toBe('false');
  });
});

describe('combobox.options', () => {
  beforeEach(reset);

  it('returns the option texts an open widget offers', async () => {
    reactSelect('Country', ['Germany', 'Poland', 'Spain']);
    await combobox.open(document, 'Country');

    expect(combobox.options(document, 'Country')).toEqual({
      status: 'open',
      options: ['Germany', 'Poland', 'Spain'],
    });
  });

  it('reports a closed widget as closed rather than as offering nothing', () => {
    reactSelect('Country', ['Germany', 'Poland']);

    // The remedy differs from an open-but-empty widget's, so the two must not
    // collapse into the same answer.
    expect(combobox.options(document, 'Country')).toEqual({ status: 'not_open', options: [] });
  });

  it('distinguishes an open widget that offers nothing — the typeahead case', async () => {
    // Stripe's "Location (City)" opens and fetches its options over the network,
    // so it is open and empty. Out of scope, and it must say so.
    reactSelect('Location (City)', []);
    await combobox.open(document, 'Location (City)');

    expect(combobox.options(document, 'Location (City)')).toEqual({ status: 'open', options: [] });
  });

  it('reports not-found for an unknown label', () => {
    expect(combobox.options(document, 'Favourite colour')).toEqual({ status: 'not_found', options: [] });
  });

  it('trusts the widget over the mere presence of a listbox', () => {
    // A Downshift-style widget keeps its listbox in the DOM and hides it, saying
    // so with aria-expanded. Reading the list anyway would report options the
    // user cannot see for a widget nobody opened.
    const { input } = reactSelect('Country', ['Germany', 'Poland']);
    const listbox = document.createElement('div');
    listbox.id = 'persistent-listbox';
    listbox.setAttribute('role', 'listbox');
    listbox.style.display = 'none';
    for (const text of ['Germany', 'Poland']) {
      const option = document.createElement('div');
      option.setAttribute('role', 'option');
      option.textContent = text;
      listbox.append(option);
    }
    document.body.append(listbox);
    input.setAttribute('aria-controls', 'persistent-listbox');

    expect(combobox.options(document, 'Country')).toEqual({ status: 'not_open', options: [] });
  });

  it('reads the listbox when aria-controls names several elements', async () => {
    // ARIA allows an id *list*, and getElementById on the whole string finds
    // nothing — which would silently report an open widget as offering nothing.
    const { input } = reactSelect('Country', ['Germany', 'Poland']);
    await combobox.open(document, 'Country');
    const status = document.createElement('div');
    status.id = 'live-status';
    document.body.append(status);
    input.setAttribute('aria-controls', `live-status ${input.getAttribute('aria-controls')}`);

    expect(combobox.options(document, 'Country')).toEqual({ status: 'open', options: ['Germany', 'Poland'] });
  });
});

describe('combobox.select', () => {
  beforeEach(reset);

  it('commits an offered option by acting on it, and the widget closes', async () => {
    const { input } = reactSelect('Country', ['Germany', 'Poland', 'Spain']);
    await combobox.open(document, 'Country');

    expect(await combobox.select(document, 'Country', 'Poland')).toEqual({ status: 'selected' });
    expect(input.getAttribute('aria-expanded')).toBe('false');
  });

  it('refuses a value the widget does not offer, committing nothing', async () => {
    reactSelect('Do you require sponsorship', ['Yes', 'No']);
    await combobox.open(document, 'Do you require sponsorship');

    expect(await combobox.select(document, 'Do you require sponsorship', 'Norfolk Island')).toEqual({
      status: 'no_option',
    });
    expect(combobox.verify(document, 'Do you require sponsorship', 'Norfolk Island').committed).toBe('');
  });

  it('refuses to act on a widget that is not open', async () => {
    reactSelect('Country', ['Germany']);

    expect(await combobox.select(document, 'Country', 'Germany')).toEqual({ status: 'not_open' });
  });
});

describe('combobox.verify', () => {
  beforeEach(reset);

  it("reports the widget's committed value after a successful select", async () => {
    reactSelect('Country', ['Germany', 'Poland']);
    await combobox.open(document, 'Country');
    await combobox.select(document, 'Country', 'Poland');

    expect(combobox.verify(document, 'Country', 'Poland')).toEqual({ status: 'verified', committed: 'Poland' });
  });

  it('accepts a commit the widget displays in shortened form', async () => {
    // The live phone-code widget commits "+358" for "Åland Islands +358".
    // Comparing for equality would report every such fill as a failure.
    reactSelect('Phone country code', ['United States +1', 'Åland Islands +358'], {
      commitAs: (option) => option.slice(option.indexOf('+')),
    });
    await combobox.open(document, 'Phone country code');
    await combobox.select(document, 'Phone country code', 'Åland Islands +358');

    expect(combobox.verify(document, 'Phone country code', 'Åland Islands +358')).toEqual({
      status: 'verified',
      committed: '+358',
    });
  });

  it('reports a mismatch when the widget committed something else', async () => {
    // The spike's original failure: a question wanting "No" ends up holding
    // "Norfolk Island". Note that "No" is a substring of "Norfolk Island", so a
    // containment test in the wrong direction would call this a success.
    reactSelect('Are you authorized to work', ['Yes', 'No'], { commitAs: () => 'Norfolk Island' });
    await combobox.open(document, 'Are you authorized to work');
    await combobox.select(document, 'Are you authorized to work', 'No');

    expect(combobox.verify(document, 'Are you authorized to work', 'No')).toEqual({
      status: 'mismatch',
      committed: 'Norfolk Island',
    });
  });

  it('reports an untouched widget as holding nothing, not as verified', () => {
    reactSelect('Country', ['Germany']);

    expect(combobox.verify(document, 'Country', 'Germany')).toEqual({ status: 'empty', committed: '' });
  });

  it('reports not-found for an unknown label', () => {
    expect(combobox.verify(document, 'Favourite colour', 'blue')).toEqual({ status: 'not_found', committed: '' });
  });
});

let committedCount = 0;

/**
 * A widget already holding committed values, with no listbox of its own.
 * `declareAs` picks which attribute marks it a combobox — `isComboWidget`
 * accepts several, and anything reading widgets must accept the same set.
 */
function committedWidget(
  labelText: string,
  values: string[],
  { declareAs = 'role', parent = document.body }: { declareAs?: 'role' | 'aria-haspopup'; parent?: Element } = {},
) {
  const slug = `c${committedCount++}`;
  const label = document.createElement('label');
  label.setAttribute('for', slug);
  label.textContent = labelText;

  const control = document.createElement('div');
  control.className = 'select__control';
  for (const value of values) {
    const shown = document.createElement('div');
    shown.className = values.length > 1 ? 'css-1p3m7a8-multiValue' : 'css-1dimb5e-singleValue';
    shown.textContent = value;
    control.append(shown);
  }
  const input = document.createElement('input');
  input.id = slug;
  if (declareAs === 'role') input.setAttribute('role', 'combobox');
  else input.setAttribute('aria-haspopup', 'listbox');
  control.append(input);

  const wrap = document.createElement('div');
  wrap.append(label, control);
  parent.append(wrap);
  return { input, control };
}

describe('combobox.verify cannot claim a write that did not happen', () => {
  beforeEach(reset);

  it("never reports a neighbour's value, however that neighbour declares itself", () => {
    // Two widgets in one row. The neighbour says it is a combobox with
    // aria-haspopup rather than role, so a guard counting only [role=combobox]
    // would walk straight past it and read its "Germany".
    const row = document.createElement('div');
    row.className = 'form-row';
    document.body.append(row);
    const { control: mine } = committedWidget('Country of residence', [], { parent: row });
    committedWidget('Country of citizenship', ['Germany'], { declareAs: 'aria-haspopup', parent: row });

    expect(mine.textContent).toBe('');
    expect(combobox.verify(document, 'Country of residence', 'Germany')).toEqual({
      status: 'empty',
      committed: '',
    });
  });

  it('refuses a label that addresses more than one widget', () => {
    // A repeated education row: Greenhouse asks School/Degree/Discipline once
    // per row, so the label alone cannot say which row is meant. Answering for
    // the first would report row 1's value as row 2's.
    committedWidget('School', ['MIT']);
    committedWidget('School', []);

    expect(combobox.verify(document, 'School', 'MIT')).toEqual({ status: 'ambiguous', committed: '' });
    expect(combobox.options(document, 'School')).toEqual({ status: 'ambiguous', options: [] });
  });

  it('accepts a pick that is one of several committed values', () => {
    // A multi-select widget renders one chip per choice. Reading only the first
    // would report a correct second pick as a mismatch.
    committedWidget('Which countries?', ['Germany', 'Poland']);

    expect(combobox.verify(document, 'Which countries?', 'Poland')).toEqual({
      status: 'verified',
      committed: 'Germany, Poland',
    });
  });
});

describe('combobox.open trusts the widget wherever it states itself', () => {
  beforeEach(reset);

  it('reads aria-expanded from the wrapper that carries it', async () => {
    // ARIA 1.1 puts role=combobox and aria-expanded on a wrapper, not the input,
    // and the listbox stays in the DOM hidden with visibility/opacity rather
    // than display:none.
    const label = document.createElement('label');
    label.setAttribute('for', 'd');
    label.textContent = 'Country';
    const input = document.createElement('input');
    input.id = 'd';
    input.setAttribute('aria-autocomplete', 'list');
    input.setAttribute('aria-controls', 'lb');
    const wrapper = document.createElement('div');
    wrapper.setAttribute('role', 'combobox');
    wrapper.setAttribute('aria-expanded', 'false');
    wrapper.append(input);
    const listbox = document.createElement('div');
    listbox.id = 'lb';
    listbox.setAttribute('role', 'listbox');
    listbox.style.visibility = 'hidden';
    for (const t of ['Germany', 'Poland']) {
      const option = document.createElement('div');
      option.setAttribute('role', 'option');
      option.textContent = t;
      listbox.append(option);
    }
    document.body.append(label, wrapper, listbox);

    expect(combobox.options(document, 'Country')).toEqual({ status: 'not_open', options: [] });
  });
});

describe('combobox.select does not claim a commit it cannot see', () => {
  beforeEach(reset);

  it('reports a widget that stayed open and committed nothing', async () => {
    const label = document.createElement('label');
    label.setAttribute('for', 'g');
    label.textContent = 'Country';
    const input = document.createElement('input');
    input.id = 'g';
    input.setAttribute('role', 'combobox');
    input.setAttribute('aria-expanded', 'true');
    input.setAttribute('aria-controls', 'glb');
    const control = document.createElement('div');
    control.className = 'select__control';
    control.append(input);
    const listbox = document.createElement('div');
    listbox.id = 'glb';
    listbox.setAttribute('role', 'listbox');
    const option = document.createElement('div');
    option.setAttribute('role', 'option');
    option.textContent = 'Germany';
    listbox.append(option);
    // Nothing listens: a widget that would need trusted input.
    document.body.append(label, control, listbox);

    expect(await combobox.select(document, 'Country', 'Germany')).toEqual({ status: 'did_not_commit' });
  });
});

describe('combobox.runStep', () => {
  beforeEach(reset);

  it('drives a widget end to end through the four steps', async () => {
    reactSelect('Are you authorized to work', ['Yes', 'No']);
    const step = (action: 'open' | 'options' | 'select' | 'verify', value = '') =>
      combobox.runStep(document, { action, label: 'Are you authorized to work', value });

    expect(await step('open')).toEqual({ status: 'opened' });
    expect(await step('options')).toEqual({ status: 'open', options: ['Yes', 'No'] });
    expect(await step('select', 'No')).toEqual({ status: 'selected' });
    expect(await step('verify', 'No')).toEqual({ status: 'verified', committed: 'No' });
  });

  it('reports not-found from a frame that does not hold the widget', async () => {
    expect(await combobox.runStep(document, { action: 'open', label: 'Country', value: '' })).toEqual({
      status: 'not_found',
    });
  });
});
