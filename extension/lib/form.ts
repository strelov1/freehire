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

import type { FieldTag, FormField, LabelFill, FillOutcome, RevealRequest, Upload } from './protocol';
import { countryLabel } from './labels';

type Fillable = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;

// Input types that are not free-fill targets.
const SKIP_TYPES = new Set(['hidden', 'submit', 'button', 'reset', 'image', 'file']);

/** The ordered list of fillable controls on the page. */
function collectFillable(doc: Document): Fillable[] {
  const all = Array.from(doc.querySelectorAll<Fillable>('input, select, textarea'));
  return all.filter((el) => {
    if (el.disabled) return false;
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
  const labelled = labelledContainer(el);
  return labelled ? { ...labelled, key: `${el.type} ${el.name}` } : null;
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
 * one. Matched with `isSameNode` rather than `indexOf`, because that is the
 * identity check that holds however the two references were obtained — the
 * reference `closest` hands back need not be the very object the query list
 * holds.
 */
function formIndex(el: Element, forms: HTMLFormElement[]): number {
  const owner = el.closest('form');
  return owner ? forms.findIndex((f) => f.isSameNode(owner)) : -1;
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
    required: controls.some((c) => c.required || c.getAttribute('aria-required') === 'true'),
    value: el.value ?? '',
    combo: isComboWidget(el),
  };

  if (controls.length > 1) {
    // A group: the options are the controls' own labels, and its value is
    // whichever of them are already chosen.
    field.options = controls.map(extractLabel);
    field.value = controls
      .filter((c) => c instanceof HTMLInputElement && c.checked)
      .map(extractLabel)
      .join(', ');
  } else if (el instanceof HTMLSelectElement) {
    field.options = Array.from(el.options).map((o) => (o.textContent ?? '').trim());
  } else if (field.combo) {
    const offered = comboOptions(el);
    if (offered.length) field.options = offered;
  }
  return field;
}

/**
 * The options a custom-widget combobox is offering right now, read from the
 * listbox the widget itself points at. Addressing it by `aria-controls`/
 * `aria-owns` rather than sweeping the page for `[role=option]` is what keeps a
 * form of 27 widgets honest: a closed widget owns no listbox, so it reports
 * nothing instead of inheriting an open neighbour's countries.
 *
 * A react-select renders its listbox only once opened, so this is empty for one
 * that is closed — the agent reaches for `combobox.open` in that case.
 */
function comboOptions(el: Element): string[] {
  return comboOptionNodes(el).map(collapseText).filter(Boolean);
}

/**
 * The option elements a widget is offering, for a caller that must click one.
 * Options the user cannot see are left out, on the same grounds as hidden
 * controls: a widget keeping a stale list behind `display:none` is not offering
 * it, and neither reading nor clicking it would be honest.
 */
export function comboOptionNodes(el: Element): Element[] {
  const listbox = comboListbox(el);
  if (!listbox) return [];
  return Array.from(listbox.querySelectorAll('[role="option"]')).filter(isOnScreen);
}

/**
 * The listbox a widget declares as its own, or null when it names none.
 *
 * `aria-controls` is an id *list*, and a widget may well point at a live-region
 * status node alongside its listbox — so the ids are resolved individually and
 * the one that actually looks like a listbox wins. Resolving the whole attribute
 * as a single id finds nothing, which would report an open widget as offering
 * no options.
 */
export function comboListbox(el: Element): Element | null {
  const ids = (el.getAttribute('aria-controls') || el.getAttribute('aria-owns') || '').split(/\s+/).filter(Boolean);
  const named = ids.map((id) => el.ownerDocument.getElementById(id)).filter((node): node is HTMLElement => !!node);
  const listbox = named.find((n) => n.getAttribute('role') === 'listbox' || n.querySelector('[role="option"]'));
  return listbox ?? named[0] ?? null;
}

function collapseText(el: Element): string {
  return collapse(el.textContent);
}

/**
 * True when the element is on screen for the user; see `isHidden`. A widget's
 * menu is routinely hidden with `visibility` or `opacity` rather than `display`,
 * which `checkVisibility` ignores unless asked, so those are checked explicitly.
 */
export function isOnScreen(el: Element): boolean {
  if (el.closest('[hidden]')) return false;
  if (typeof el.checkVisibility === 'function' && !el.checkVisibility()) return false;

  const view = el.ownerDocument.defaultView;
  if (!view) return true;
  for (let node: Element | null = el; node; node = node.parentElement) {
    const style = view.getComputedStyle(node);
    if (style.visibility === 'hidden' || style.visibility === 'collapse') return false;
    if (style.opacity === '0') return false;
  }
  return true;
}

/**
 * True when the control is the text input of a custom dropdown widget
 * (react-select and friends) rather than a plain field. Such a widget ignores a
 * written value and commits whatever its own listbox highlights — the spike's
 * "Norfolk Island instead of No" failure — so the simple filler must skip it.
 * Detected by the ARIA the widgets expose; a native <select> is never one.
 */
export function isComboWidget(el: Fillable): boolean {
  if (el instanceof HTMLSelectElement) return false;
  return el.matches(COMBO_WIDGET);
}

/**
 * The ways a widget declares itself a combobox. Exported because anything that
 * reads *around* a widget must recognise the same set: a guard counting only
 * `[role=combobox]` would walk straight past a neighbour that says
 * `aria-haspopup="listbox"` instead, and report that neighbour's value.
 */
export const COMBO_WIDGET = '[role="combobox"], [aria-autocomplete], [aria-haspopup="listbox"]';

/** Best-effort human label for a control, in decreasing reliability. */
function extractLabel(el: Fillable): string {
  const fromLabels = Array.from(el.labels ?? [])
    .map((l) => l.textContent?.trim() ?? '')
    .filter(Boolean)
    .join(' ');
  if (fromLabels) return fromLabels;

  return (
    textFromIds(el.ownerDocument, el.getAttribute('aria-labelledby')) ||
    collapse(el.getAttribute('aria-label')) ||
    collapse(el.getAttribute('placeholder')) ||
    collapse(el.getAttribute('name'))
  );
}

/** The joined text of the elements an `aria-labelledby`-style id list names. */
function textFromIds(doc: Document, ids: string | null): string {
  if (!ids) return '';
  return ids
    .split(/\s+/)
    .map((id) => collapse(doc.getElementById(id)?.textContent))
    .filter(Boolean)
    .join(' ');
}

function collapse(text: string | null | undefined): string {
  return (text ?? '').replace(/\s+/g, ' ').trim();
}

/** Canonical profile keys and the label synonyms that map to them. */
const FIELD_SYNONYMS: Record<string, string[]> = {
  fullName: ['full name', 'your name', 'candidate name', 'applicant name'],
  firstName: ['first name', 'given name', 'legal first name', 'preferred first name'],
  lastName: ['last name', 'family name', 'surname', 'legal last name'],
  email: ['email', 'e-mail', 'e-mail address', 'email address'],
  phone: ['phone', 'mobile', 'telephone', 'contact number', 'cell'],
  city: ['city', 'town', 'current location', 'location (city)'],
  state: ['state', 'province', 'region'],
  country: ['country'],
  postalCode: ['postal code', 'zip', 'zip code', 'pincode', 'pin code'],
  linkedin: ['linkedin'],
  github: ['github'],
  portfolio: ['portfolio', 'website', 'personal site'],
  // The candidate's own screening answers (internal/screeninganswers), not a
  // boolean "are you authorized to work" question — that asks something the
  // profile does not carry an answer to, so it stays unmatched rather than
  // being fed a country list where a Yes/No answer belongs.
  authorizedCountries: [
    'which countries are you authorized to work in',
    'in which countries are you authorized to work',
    'in which countries are you legally authorized to work',
    'countries you are authorized to work in',
    'list the countries you are authorized to work in',
  ],
  visaSponsorshipNeeded: [
    'will you now or in the future require sponsorship',
    'do you now or will you in the future require sponsorship',
    'will you require sponsorship',
    'do you require visa sponsorship',
    'do you need visa sponsorship',
    'will you need visa sponsorship',
  ],
  desiredSalary: [
    'desired salary',
    'desired compensation',
    'expected salary',
    'salary expectation',
    'salary expectations',
    'what are your salary expectations',
    'compensation expectations',
  ],
  noticePeriod: [
    'notice period',
    'current notice period',
    'what is your notice period',
    'how much notice',
  ],
  willingToRelocate: [
    'are you willing to relocate',
    'would you be willing to relocate',
    'willing to relocate',
    'are you open to relocating',
    'open to relocation',
  ],
  age18OrOlder: [
    'are you at least 18 years',
    'are you 18 years of age or older',
    'are you over the age of 18',
    'are you 18 or older',
    'will you be 18 years of age',
  ],
};

/**
 * The profile's authorized-countries field arrives as comma-joined ISO codes ("US, DE" —
 * screeninganswers.AutofillFields' wire format), but the question it answers is almost
 * always a checkbox group whose options are full country names ("United States"), matched
 * option-by-option against this value by `chosenOptions` below. Left as codes, the value
 * would never match any option and the group would silently stay unchecked.
 */
export function formatAuthorizedCountries(codes: string): string {
  if (!codes) return '';
  return codes
    .split(',')
    .map((c) => c.trim())
    .filter(Boolean)
    .map(countryLabel)
    .join(', ');
}

/**
 * Comparison form of a label: case-, whitespace- and required-marker-insensitive,
 * so "First Name *" and "first name" address the same control.
 */
export function normalizeLabel(label: string): string {
  return label.toLowerCase().replace(/\*/g, '').replace(/\s+/g, ' ').trim();
}

/**
 * Maps a control's label to a canonical profile key, or null when unknown.
 *
 * The synonym has to *open* the label, not merely appear in it. A real form asks
 * "Are you authorized to lawfully work for Roku in the country to which you are
 * applying?" — a substring match hands that Yes/No question the `country` key,
 * and the profile's country goes into a radio group. Anchoring at the front
 * still reads every label the live ATS forms produce, decorations and all
 * ("First Name (required) e85441b6"), because those trail the question rather
 * than lead it.
 */
export function matchFieldKey(label: string): string | null {
  const normalized = normalizeLabel(label);
  if (!normalized) return null;
  for (const [key, synonyms] of Object.entries(FIELD_SYNONYMS)) {
    if (synonyms.some((s) => opensWith(normalized, s))) return key;
  }
  return null;
}

/** True when the label opens with this synonym and ends it at a word boundary. */
function opensWith(label: string, synonym: string): boolean {
  if (!label.startsWith(synonym)) return false;
  // "Citywide preference" is not a `city` question.
  return !/[a-z0-9]/.test(label.charAt(synonym.length));
}

/**
 * Whether the page is showing an application form at all.
 *
 * A careers page routinely keeps the application behind an "Apply" button and
 * shows a job-alert signup meanwhile — on Roku that signup asks for a first
 * name, a last name and an email, all required, which no count of fields can
 * distinguish from a short application. The resume upload can: see
 * `extractUploads`.
 */
export function looksLikeApplication(uploads: { form: number }[]): boolean {
  return uploads.length > 0;
}

/**
 * Narrows an observation to the one form the application is asking, identified
 * by the upload sitting in it. Without this a page carrying both an application
 * and a job-alert signup gets both written, since each has its own "Email".
 *
 * Both the frame and the form identify the group: an ATS iframe numbers its own
 * forms from zero, so the frame alone would merge two unrelated first forms.
 * When the upload names a group holding no questions — a page rendering them
 * outside its form element — every field is kept, because filling nothing at all
 * is the worse answer.
 */
export function scopeToApplication<T extends { frame: number; form: number }>(
  fields: T[],
  uploads: { frame: number; form: number }[],
): T[] {
  const [target] = uploads;
  if (!target) return fields;
  const scoped = fields.filter((f) => f.frame === target.frame && f.form === target.form);
  return scoped.length > 0 ? scoped : fields;
}

/**
 * The fills a profile justifies against the questions a page asks: each
 * recognised label paired with the value it maps to. A label the profile knows
 * nothing about, or knows only as a blank, is left for the user rather than
 * written over with an empty string.
 *
 * A repeated label is asked for once. A careers page routinely carries two forms
 * — the application and a job-alert signup — and `fillByLabel` answers the first
 * question carrying a label, so the repeats only pad the wire. Pure over its
 * input; the page is not touched here.
 */
export function planLabelFills(
  fields: { label: string; frame?: number; form?: number }[],
  values: Record<string, string>,
): LabelFill[] {
  const asked = new Set<string>();
  const fills: LabelFill[] = [];
  for (const { label, frame, form } of fields) {
    const key = matchFieldKey(label);
    const value = key ? (values[key] ?? '') : '';
    if (!value || asked.has(normalizeLabel(label))) continue;
    asked.add(normalizeLabel(label));
    fills.push({ label, value, frame, form });
  }
  return fills;
}

/**
 * The fills a given frame should act on: those addressed to it by
 * `deterministicAutofill`, plus any fill naming no frame at all — a
 * `fill_simple` tool call, which is still offered to every frame as before
 * frame scoping existed. Used by the background relay to stop broadcasting a
 * frame-addressed fill to frames that do not hold the target control, which is
 * what let a same-labeled field in another frame's form get filled instead.
 */
export function fillsForFrame(fills: LabelFill[], frame: number): LabelFill[] {
  return fills.filter((f) => f.frame === undefined || f.frame === frame);
}

/**
 * Writes values into the questions carrying the given labels, in one pass:
 * the page is read and written inside a single synchronous walk, so a re-render
 * between an agent's observation and its fills cannot drift the target the way a
 * positional index does. Every requested fill gets an outcome; custom-widget
 * comboboxes are reported as deferred rather than written into.
 *
 * A fill naming a `form` only matches a question inside that form — the frame's
 * own signup form sharing a label ("Email") with the application form must not
 * absorb a fill meant for the other. A fill naming none matches the first
 * question carrying the label, as `fillByLabel` always has.
 */
/** How long the borrowed outline stays on a revealed control. Long enough to
 *  follow by eye, short enough that a walk's next step does not overlap it. */
const REVEAL_OUTLINE_MS = 600;

/**
 * Brings one question into view: scrolls its first control to the middle of the
 * viewport, outlines it briefly, and focuses it when asked. Reports whether the
 * question was there at all — the panel says so rather than doing nothing.
 *
 * The outline is written as inline style and the previous inline style is put
 * back: this runs on a page freehire does not own, where an injected class could
 * collide with the ATS's own and a stylesheet would outlive our interest in the
 * element.
 */
export function revealField(
  doc: Document,
  { label, form, focus = false, outlineMs = REVEAL_OUTLINE_MS }: RevealRequest,
): boolean {
  const question = findQuestion(collectQuestions(doc), Array.from(doc.querySelectorAll('form')), label, form);
  if (!question) return false;

  const el = question.controls[0];
  el.scrollIntoView({ block: 'center', behavior: 'smooth' });

  // Revealing the same control twice inside the outline's lifetime must not make
  // the outline the style we give back: the first reveal's record wins, and its
  // timer is replaced rather than left to fire against a control the second
  // reveal is still outlining.
  const open = outlined.get(el);
  if (open) clearTimeout(open.timer);
  const borrowed = open ? open.borrowed : el.getAttribute('style');
  el.style.outline = '2px solid #4f46e5';
  el.style.outlineOffset = '2px';
  const timer = setTimeout(() => {
    outlined.delete(el);
    if (borrowed === null) el.removeAttribute('style');
    else el.setAttribute('style', borrowed);
  }, outlineMs);
  outlined.set(el, { borrowed, timer });

  if (focus) el.focus({ preventScroll: true });
  return true;
}

/** Controls currently wearing a borrowed outline, and the style each one had
 *  before it. Weak so a control the page discards is not held alive by it. */
const outlined = new WeakMap<Element, { borrowed: string | null; timer: ReturnType<typeof setTimeout> }>();

export function fillByLabel(doc: Document, fills: LabelFill[]): FillOutcome[] {
  const questions = collectQuestions(doc);
  const forms = Array.from(doc.querySelectorAll('form'));
  return fills.map(({ label, value, form }) => {
    const question = findQuestion(questions, forms, label, form);
    if (!question) return { label, status: 'not_found' as const };
    if (question.controls.length === 1 && isComboWidget(question.controls[0])) {
      return { label, status: 'deferred_combobox' as const };
    }
    return { label, status: answerQuestion(question, value) ? ('filled' as const) : ('no_option' as const) };
  });
}

/** The question a fill addresses, narrowed to `form` when the fill names one. */
function findQuestion(
  questions: Question[],
  forms: HTMLFormElement[],
  label: string,
  form: number | undefined,
): Question | undefined {
  const target = normalizeLabel(label);
  const matches = questions.filter((q) => normalizeLabel(q.label) === target);
  if (form === undefined) return matches[0];
  return matches.find((q) => formIndex(q.controls[0], forms) === form);
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
  const byLongestLabel = [...controls].sort((a, b) => extractLabel(b).length - extractLabel(a).length);
  const chosen: Fillable[] = [];
  let rest = value.trim();

  while (rest) {
    const next = byLongestLabel.find((c) => startsWithOption(rest, extractLabel(c)));
    if (!next) return null;
    chosen.push(next);
    rest = rest.slice(extractLabel(next).trim().length).replace(/^\s*,\s*/, '').trim();
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
