/**
 * The extension's "eyes + hands" for a page's form: a serialisable observation
 * (extractForm) the agent can reason over, and an executor (fillByLabel) that
 * writes values back, dispatching native input/change events so React/Angular
 * ATS forms register the change. Both work in *questions* (collectQuestions),
 * not raw controls, so a question rendered as 29 checkboxes stays one field; a
 * question is addressed by its label, which survives both a re-render between
 * the observation and the fill and the fan-out across a tab's frames, neither of
 * which a position in the list does.
 */

import { collapse, comboOptions, displayedValues, isComboWidget } from './comboWidget';
import type { FieldTag, FormField, LabelFill, FillOutcome, FillStatus, Upload } from './protocol';
import type { AutofillProfile } from './freehire';
import { easyApplyDialog, easyApplyFieldError, isStoredResumeChoice } from './easyApply';
import { extractLabel, textFromIds } from './formLabel';
import { formatAuthorizedCountries, matchFieldKey, normalizeLabel } from './fieldKeys';

export type Fillable = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;

// Input types that are not free-fill targets.
const SKIP_TYPES = new Set(['hidden', 'submit', 'button', 'reset', 'image', 'file']);

/**
 * The ordered list of fillable controls on the page. An open Easy Apply dialog is
 * the whole application, so only its controls count: LinkedIn's search box and
 * message composer sit on the same page.
 */
function collectFillable(doc: Document): Fillable[] {
  const easyApply = easyApplyDialog(doc);
  const all = Array.from((easyApply ?? doc).querySelectorAll<Fillable>('input, select, textarea'));
  return all.filter((el) => {
    if (el.disabled) return false;
    if (easyApply && isStoredResumeChoice(el)) return false;
    if (el instanceof HTMLInputElement && SKIP_TYPES.has(el.type)) return false;
    if (isHidden(el)) return false;
    return true;
  });
}

/**
 * One question the page asks. Usually a single control, but a question can be
 * rendered as several — "which countries do you anticipate working in" is 29
 * checkboxes under one `legend`. Treating those as 29 fields both swamps the
 * report and hides the actual question from the agent, so the question, not the
 * control, is the unit the rest of this module works in.
 */
export interface Question {
  /** The question's own text: a control's label, or the group's legend. */
  label: string;
  /**
   * One control, or the group's controls in document order. Never empty —
   * every construction site starts it from a single control — encoded as a
   * non-empty tuple so `controls[0]` and single-element destructuring stay
   * typed as `Fillable`, not `Fillable | undefined`.
   */
  controls: [Fillable, ...Fillable[]];
}

/** The ordered list of questions — the index space for observe + act. */
export function collectQuestions(doc: Document): Question[] {
  const questions: Question[] = [];
  const groups: Question[] = [];
  const byContainer = new Map<Element, Map<string, Question>>();

  for (const el of collectFillable(doc)) {
    const group = groupOf(el);
    if (!group) {
      questions.push({ label: extractLabel(el), controls: [el] });
      continue;
    }
    let inContainer = byContainer.get(group.container);
    if (!inContainer) {
      inContainer = new Map();
      byContainer.set(group.container, inContainer);
    }
    const open = inContainer.get(group.key);
    if (open) {
      open.controls.push(el);
      continue;
    }
    const question: Question = { label: group.question, controls: [el] };
    inContainer.set(group.key, question);
    questions.push(question);
    groups.push(question);
  }

  // A group of one is not a group: a lone "I agree to the terms" checkbox under
  // a `Consent` legend is already its own question, and carrying the legend
  // instead of its label would lose what the user is agreeing to.
  for (const group of groups) {
    if (group.controls.length === 1) group.label = extractLabel(group.controls[0]);
  }
  return questions;
}

/**
 * The group a control would answer within, or null when it is its own question.
 *
 * Deliberately narrow, because over-grouping hides fields from the agent while
 * under-grouping only makes the report long. The control must be a checkbox or
 * radio — an `Address` fieldset's text inputs are separate questions despite the
 * shared legend — and its container must be labelled, since without a label
 * there is no question text to report.
 *
 * `key` splits a container that holds more than one question: a
 * "Demographic Questions" fieldset wrapping both a Yes/No radio pair and a
 * country checklist asks two things, and merging them would offer all the options
 * at once and leave the second question unanswerable. Controls answering together
 * share a `name` and a type, which is how the live forms mark them.
 */
function groupOf(el: Fillable): { container: Element; question: string; key: string } | null {
  if (!(el instanceof HTMLInputElement) || (el.type !== 'checkbox' && el.type !== 'radio')) return null;
  // No name, no evidence: a React-controlled form may omit it entirely, and then
  // nothing distinguishes one question in a container from two. A long report is
  // the safer failure, so such controls stay ungrouped.
  if (!el.name) return null;
  const labelled = labelledContainer(el) ?? namedByControls(el);
  return labelled ? { ...labelled, key: `${el.type} ${el.name}` } : null;
}

/**
 * A radio group whose own container states no question, but whose radios are each named after
 * it (LinkedIn Easy Apply's Yes/No: the aria-label is the question, the option text sits beside).
 */
function namedByControls(el: HTMLInputElement): { container: Element; question: string } | null {
  const container = el.closest('fieldset, [role="radiogroup"]');
  const question = collapse(el.getAttribute('aria-label'));
  return container && el.type === 'radio' && question ? { container, question } : null;
}

/** The nearest container that states the question its controls answer. */
function labelledContainer(el: Element): { container: Element; question: string } | null {
  const fieldset = el.closest('fieldset');
  if (fieldset) {
    const legend = Array.from(fieldset.children).find((c) => c.tagName === 'LEGEND');
    const question = collapse(legend?.textContent);
    if (question) return { container: fieldset, question };
  }
  // The same structure without a fieldset: ARIA's own way of saying "these
  // controls answer one question".
  const group = el.closest('[role="group"], [role="radiogroup"]');
  if (group) {
    const question =
      textFromIds(el.ownerDocument, group.getAttribute('aria-labelledby')) ||
      collapse(group.getAttribute('aria-label'));
    if (question) return { container: group, question };
  }
  return null;
}

/**
 * True when the control is not on screen for the user — a display:none recaptcha
 * textarea, a collapsed section, a widget's stashed input. `type=hidden` is
 * already excluded above; this catches the CSS-hidden ones, which a real ATS
 * form is full of and which the agent has no business reading or writing.
 */
function isHidden(el: Fillable): boolean {
  if (el.hidden || el.closest('[hidden]')) return true;
  // checkVisibility walks the ancestors for us. Where it is missing (an older
  // browser), fall back to treating the control as visible rather than dropping
  // fields we cannot judge.
  return typeof el.checkVisibility === 'function' && !el.checkVisibility();
}

/** Serialises the page's form into indexed FormFields. Pure over the document. */
export function extractForm(doc: Document): FormField[] {
  const forms = Array.from(doc.querySelectorAll('form'));
  return collectQuestions(doc).map((q, i) => describeQuestion(q, i, forms));
}

/**
 * The resume/CV uploads the page is offering, each tagged with the form that
 * owns it.
 *
 * A file input is never a fill target — `SKIP_TYPES` drops it from the question
 * list, and its value cannot be set from script anyway — but it is the one thing
 * that reliably marks a form as an *application*. Measured across Greenhouse
 * (board, embed and site-with-iframe), Lever and Ashby: every open application
 * offered one, and no page that was not showing an application did. That is what
 * lets a job-alert signup be told from a short application form, which counting
 * fields or required markers cannot do.
 */
export function extractUploads(doc: Document): Upload[] {
  const forms = Array.from(doc.querySelectorAll('form'));
  return Array.from(doc.querySelectorAll<HTMLInputElement>('input[type="file"]'))
    .filter((el) => !el.disabled && !isHidden(el))
    .map((el) => ({ form: formIndex(el, forms) }));
}

/**
 * The index of the form a control answers within, or -1 when it sits outside
 * one.
 *
 * Asked of the control itself rather than of the tree around it. `.form` is the
 * form owner HTML defines, and a control carrying `form="signup"` answers that
 * form from wherever it is rendered — a distinction `closest('form')` cannot
 * make. (happy-dom does not implement the attribute, so no test here covers
 * that case; browsers do, which is where the extension runs.)
 *
 * It is also the only identity check happy-dom agrees with itself about across
 * versions. Under 20.11.6 `closest` hands back a different object than the
 * query list holds, so `===` fails and `isSameNode` is what matches; under
 * 20.11.13 that reversed — `===` matches and `isSameNode` returns false for a
 * node against itself. `.form` returns the very object `querySelectorAll`
 * collected under both, so `indexOf` is enough.
 */
function formIndex(el: Fillable, forms: HTMLFormElement[]): number {
  const owner = el.form;
  return owner ? forms.indexOf(owner) : -1;
}

function describeQuestion({ label, controls }: Question, index: number, forms: HTMLFormElement[]): FormField {
  const [el] = controls;
  const tag = el.tagName.toLowerCase() as FieldTag;
  const field: FormField = {
    index,
    form: formIndex(el, forms),
    tag,
    type: el instanceof HTMLInputElement ? el.type : tag,
    label,
    name: el.getAttribute('name') ?? '',
    // A React-rendered ATS form typically validates in JS and marks the
    // requirement with ARIA rather than the native attribute.
    // Plenty of forms (LinkedIn Easy Apply's radio groups) mark it only as a trailing
    // asterisk in the question's text.
    required: controls.some((c) => c.required || c.getAttribute('aria-required') === 'true') || /\*\s*$/.test(label),
    value: el.value ?? '',
    combo: isComboWidget(el),
  };
  const invalid = fieldError(el);
  if (invalid) field.invalid = invalid;

  if (controls.length > 1) {
    // A group: the options are the controls' own labels, and its value is
    // whichever of them are already chosen.
    field.options = controls.map(extractLabel);
    field.value = controls
      .filter((c) => c instanceof HTMLInputElement && c.checked)
      .map(extractLabel)
      .join(', ');
  } else if (el instanceof HTMLInputElement && (el.type === 'checkbox' || el.type === 'radio')) {
    // A lone box's value attribute is "on" whether or not it is ticked; the answer is
    // its state, read back the way `fillField` writes it.
    field.value = el.checked ? 'Yes' : '';
  } else if (el instanceof HTMLSelectElement) {
    field.options = Array.from(el.options).map((o) => (o.textContent ?? '').trim());
    // The answer is the chosen option's text; its value attribute is the ATS's own
    // code ("4417"), which means nothing to the person or to anyone reading it back.
    // An empty value stays empty: that is the "Select..." placeholder, not an answer.
    if (field.value) field.value = (el.selectedOptions[0]?.textContent ?? '').trim();
  } else if (field.combo) {
    const offered = comboOptions(el);
    if (offered.length) field.options = offered;
    // The widget's input is empty even after a commit; the answer is what it displays.
    if (!field.value) field.value = displayedValues(el).join(', ');
  }
  return field;
}

export function fillByLabel(doc: Document, fills: LabelFill[]): FillOutcome[] {
  const questions = collectQuestions(doc);
  const forms = Array.from(doc.querySelectorAll('form'));
  const unfillable = unfillableLabels(doc);
  return fills.map(({ label, value, form }) => {
    const resolved = findQuestion(unfillable, questions, forms, label, form);
    if ('miss' in resolved) return { label, status: resolved.miss };
    const question = resolved.found;
    if (question.controls.length === 1 && isComboWidget(question.controls[0])) {
      return { label, status: 'deferred_combobox' as const };
    }
    const answer = needsNumber(question, label) ? (numericAnswer(value) ?? value) : value;
    return { label, status: answerQuestion(question, answer) ? ('filled' as const) : ('no_option' as const) };
  });
}

/** The page's own verdict that a control's answer is wrong, as its message; '' when none. */
function fieldError(el: Fillable): string {
  return easyApplyFieldError(el) || (el.getAttribute('aria-invalid') === 'true' ? 'Invalid' : '');
}

/**
 * Whether a single text control takes a bare number. The markup says so for a
 * number input or a numeric keyboard; Easy Apply's salary question says nothing yet
 * rejects anything but digits ("5000 USD/month" is "Invalid input").
 */
function needsNumber({ controls }: Question, label: string): boolean {
  if (controls.length !== 1) return false;
  const [el] = controls;
  if (!(el instanceof HTMLInputElement)) return false;
  if (el.type === 'number' || /^(numeric|decimal)$/.test(el.inputMode)) return true;
  return matchFieldKey(label) === 'desiredSalary' && easyApplyDialog(el.ownerDocument)?.contains(el) === true;
}

/**
 * The first amount in a free-text answer as plain digits: "5000 USD/month" → "5000",
 * "€60,000" → "60000", "60k" → "60000". Null when the answer holds no number, so the
 * caller keeps the text rather than writing nothing.
 */
export function numericAnswer(value: string): string | null {
  const m = /(\d[\d.,\s]*\d|\d)\s*(k\b)?/i.exec(value);
  if (!m?.[1]) return null;
  // A separator followed by exactly three digits groups thousands; any other is a decimal point.
  const n = Number(
    m[1]
      .replace(/[.,\s](?=\d{3}(?!\d))/g, '')
      .replace(',', '.')
      .replace(/\s/g, ''),
  );
  if (!Number.isFinite(n)) return null;
  return String(Math.round(m[2] ? n * 1000 : n));
}

/**
 * The one question a fill addresses, or why the label did not name one.
 *
 * The misses are kept apart because each implies a different next step for the
 * harness, and collapsing them into "not found" — which is what this did — invites
 * a retry in the two cases where a retry cannot succeed.
 */
type Resolution =
  | { found: Question }
  // Extract, not a bare union of the four names: the outcome vocabulary is
  // FillStatus's, so a status renamed there must fail to compile here rather than
  // leave this listing a name the wire no longer uses.
  | { miss: Extract<FillStatus, 'ambiguous' | 'wrong_form' | 'not_fillable' | 'not_found'> };

/**
 * Resolves a fill's `label`, narrowed to `form` when the fill names one.
 *
 * An unscoped fill matching several questions is `ambiguous` rather than the first
 * of them: on a page carrying an application form beside a job-alert signup the two
 * are indistinguishable from here, and writing into the wrong one is worse than
 * writing into neither.
 */
export function findQuestion(
  unfillable: () => Set<string>,
  questions: Question[],
  forms: HTMLFormElement[],
  label: string,
  form: number | undefined,
): Resolution {
  const target = normalizeLabel(label);
  const matches = questions.filter((q) => normalizeLabel(q.label) === target);

  if (form === undefined) {
    const [first, ...rest] = matches;
    if (rest.length > 0) return { miss: 'ambiguous' };
    if (first) return { found: first };
    return { miss: absentOrUnfillable(unfillable, target) };
  }

  // `find`, so a label repeated INSIDE one form resolves to the first of them —
  // knowingly. `(label, frame, form)` is not a key: a multi-entry section asking
  // "Employer" once per job carries the label several times in one form, and each
  // of the fills the agent fans out lands on control #1 while the rest stay empty.
  // Refusing here instead would take that page from partly filled to not filled at
  // all, which is the worse answer while nothing on the wire can name a control
  // more precisely. What closes it is an addressing scheme with a per-control id,
  // not a refusal.
  const scoped = matches.find((q) => formIndex(q.controls[0], forms) === form);
  if (scoped) return { found: scoped };
  // The label is asked somewhere in this frame, just not in the form named — the
  // index is what is wrong, not the question.
  if (matches.length > 0) return { miss: 'wrong_form' };
  return { miss: absentOrUnfillable(unfillable, target) };
}

/**
 * The labels the page carries on a control `collectFillable` dropped, read at most
 * once and only if something actually misses.
 *
 * Lazy because the scan is over the whole document and `extractLabel` walks
 * `getElementById` for every `aria-labelledby` it meets. Per miss, on an ATS form
 * with several hundred controls and thirty unanswered questions, that is thirty
 * full passes synchronously in the content script — and a call whose fills all land
 * should not pay for it at all.
 */
export function unfillableLabels(doc: Document): () => Set<string> {
  let read: Set<string> | undefined;
  return () => (read ??= readUnfillableLabels(doc));
}

function readUnfillableLabels(doc: Document): Set<string> {
  const labels = new Set<string>();
  for (const el of doc.querySelectorAll<Fillable>('input, select, textarea')) {
    // Never a fill target in the first place, so its absence is not a refusal.
    if (el instanceof HTMLInputElement && SKIP_TYPES.has(el.type)) continue;
    const droppedByCollectFillable = el.disabled || isHidden(el);
    if (!droppedByCollectFillable) continue;
    labels.add(normalizeLabel(extractLabel(el)));
  }
  return labels;
}

/**
 * Tells a label the page never asks from one it asks through a control nothing can
 * write to.
 *
 * `collectFillable` drops a disabled or invisible control before any of this runs,
 * which is right — keeping one in the match set would risk writing into it — but it
 * left the two indistinguishable downstream. So the check happens here, on the miss
 * path only, against the controls that were filtered out.
 *
 * Only individually-labelled controls are examined; a disabled member of a grouped
 * question (a legend over 29 checkboxes) still reads as `not_found`. The group's
 * label lives on the legend rather than on any one control, and a partially
 * disabled group is not the case this exists for.
 */
function absentOrUnfillable(unfillable: () => Set<string>, target: string): 'not_fillable' | 'not_found' {
  return unfillable().has(target) ? 'not_fillable' : 'not_found';
}

/**
 * Answers one question. A group is answered by ticking the controls whose own
 * labels are the chosen options; if any of them is not on offer, nothing is
 * ticked, so a wrong choice is reported rather than half-applied.
 *
 * Ticking only, never clearing: a group may already carry an answer the user
 * chose by hand, and autofill has no business undoing it.
 */
function answerQuestion({ controls }: Question, value: string): boolean {
  if (controls.length === 1) return fillField(controls[0], value);

  const chosen = chosenOptions(controls, value);
  if (!chosen) return false;
  for (const control of chosen) fillField(control, 'true');
  return true;
}

/**
 * The controls a group's requested value names, or null when it names one the
 * group does not offer.
 *
 * A group reports every chosen option as one comma-joined value and must accept
 * that value back — but an option may carry a comma of its own, so splitting on
 * commas would turn "Germany, Korea, Republic of" into three countries the group
 * has never heard of. Instead the value is consumed from the front, taking the
 * *longest* option that matches at each step, which recovers both readings
 * unambiguously.
 */
function chosenOptions(controls: Fillable[], value: string): Fillable[] | null {
  const byLongestLabel = controls.toSorted((a, b) => extractLabel(b).length - extractLabel(a).length);
  const chosen: Fillable[] = [];
  let rest = value.trim();

  while (rest) {
    const next = byLongestLabel.find((c) => startsWithOption(rest, extractLabel(c)));
    if (!next) return null;
    chosen.push(next);
    rest = rest
      .slice(extractLabel(next).trim().length)
      .replace(/^\s*,\s*/, '')
      .trim();
  }
  return chosen.length ? chosen : null;
}

/** True when `value` opens with exactly this option, not merely with its prefix. */
function startsWithOption(value: string, option: string): boolean {
  const label = option.trim();
  if (!label) return false;
  const head = value.slice(0, label.length);
  if (normalizeLabel(head) !== normalizeLabel(label)) return false;
  // "Germany" must not match inside "Germanywest": what follows has to end the
  // option, either at the string's end or at the separator.
  const tail = value.slice(label.length);
  return tail === '' || /^\s*,/.test(tail);
}

/** Writes one control, dispatching native events. Returns false if unfillable. */
function fillField(el: Fillable, value: string): boolean {
  if (el instanceof HTMLInputElement && (el.type === 'checkbox' || el.type === 'radio')) {
    el.checked = value === 'true' || value === '1' || normalizeLabel(value) === 'yes';
    dispatchNative(el);
    return true;
  }
  if (el instanceof HTMLSelectElement) {
    const match = Array.from(el.options).find(
      (o) =>
        (o.textContent ?? '').trim().toLowerCase() === value.toLowerCase() ||
        o.value.toLowerCase() === value.toLowerCase(),
    );
    if (!match) return false;
    el.value = match.value;
    dispatchNative(el);
    return true;
  }
  setNativeValue(el, value);
  return true;
}

// React/Angular track the input's value via a native setter; calling it (rather
// than el.value =) plus bubbling input/change is what makes them notice the fill.
function setNativeValue(el: HTMLInputElement | HTMLTextAreaElement, value: string): void {
  const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
  if (setter) setter.call(el, value);
  else el.value = value;
  dispatchNative(el);
}

function dispatchNative(el: Fillable): void {
  el.dispatchEvent(new Event('input', { bubbles: true }));
  el.dispatchEvent(new Event('change', { bubbles: true }));
}

/** The autofill profile as the label-keyed values `planLabelFills` fills from. */
export function profileToValues(p: AutofillProfile): Record<string, string> {
  return {
    fullName: p.full_name,
    firstName: p.first_name,
    lastName: p.last_name,
    email: p.email,
    phone: p.phone,
    city: p.location,
    linkedin: p.linkedin,
    github: p.github,
    portfolio: p.portfolio,
    authorizedCountries: formatAuthorizedCountries(p.authorized_countries),
    visaSponsorshipNeeded: p.visa_sponsorship_needed,
    desiredSalary: p.desired_salary,
    noticePeriod: p.notice_period,
    willingToRelocate: p.willing_to_relocate,
    age18OrOlder: p.age_18_or_older,
  };
}
