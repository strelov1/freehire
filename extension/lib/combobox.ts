/**
 * The four primitives that drive a custom-widget combobox the way a person does:
 * open it, read what it actually offers, choose one, confirm what got committed.
 * Writing text into such a widget commits whatever its listbox happens to
 * highlight — the spike's "Norfolk Island" for a question wanting "No" — so
 * `fill_simple` refuses them and these primitives replace it.
 *
 * Pure over the Document, like `form.ts`, but asynchronous: a widget re-renders
 * when its framework decides to, not when the event is dispatched, so opening and
 * selecting must be *awaited* rather than dispatched-and-read.
 */

import type { ComboboxReply, ComboboxStep } from './protocol';
import {
  COMBO_WIDGET,
  collectQuestions,
  comboListbox,
  comboOptionNodes,
  isComboWidget,
  isOnScreen,
  normalizeLabel,
} from './form';

/**
 * `ambiguous` is shared by all four: a label that addresses several widgets
 * cannot say which one is meant. Greenhouse asks School / Degree / Discipline
 * once per education row, so answering for the first would report row 1's value
 * as row 2's — a success claimed for a field still empty.
 */
export type OpenStatus = 'opened' | 'did_not_open' | 'not_found' | 'ambiguous';
export type OptionsStatus = 'open' | 'not_open' | 'not_found' | 'ambiguous';
export type SelectStatus = 'selected' | 'did_not_commit' | 'no_option' | 'not_open' | 'not_found' | 'ambiguous';
export type VerifyStatus = 'verified' | 'mismatch' | 'empty' | 'not_found' | 'ambiguous';

/** How long a widget gets to re-render before it counts as unresponsive. */
const SETTLE_MS = 1000;
const POLL_MS = 25;

/**
 * Reveals the widget's listbox. Reports `did_not_open` rather than assuming, so
 * the agent stops instead of acting blind on a widget it cannot drive.
 */
export async function open(doc: Document, label: string): Promise<{ status: OpenStatus }> {
  const found = findWidget(doc, label);
  if (!found.widget) return { status: found.status };
  const widget = found.widget;
  if (isOpen(widget)) return { status: 'opened' };

  press(widget);
  return { status: (await settle(() => isOpen(widget))) ? 'opened' : 'did_not_open' };
}

/**
 * What the open widget currently offers — the evidence the agent chooses from.
 * A closed widget and an open one offering nothing are reported apart, because
 * their remedies differ: the first needs opening, while the second is a typeahead
 * that fetches its options over the network and is out of scope.
 */
export function options(doc: Document, label: string): { status: OptionsStatus; options: string[] } {
  const { widget, status } = findWidget(doc, label);
  if (!widget) return { status, options: [] };
  if (!isOpen(widget)) return { status: 'not_open', options: [] };
  return { status: 'open', options: comboOptionNodes(widget).map(text).filter(Boolean) };
}

/**
 * Commits an offered option by acting on the option itself. A value the widget
 * does not offer commits nothing: approximating it is how a question wanting
 * "No" ends up holding "Norfolk Island".
 */
export async function select(doc: Document, label: string, value: string): Promise<{ status: SelectStatus }> {
  const found = findWidget(doc, label);
  if (!found.widget) return { status: found.status };
  const widget = found.widget;
  if (!isOpen(widget)) return { status: 'not_open' };

  const option = comboOptionNodes(widget).find((o) => normalizeLabel(text(o)) === normalizeLabel(value));
  if (!option) return { status: 'no_option' };

  press(option);
  // Waiting for the widget to settle is what makes a following `verify` read the
  // committed value rather than the state before the commit landed.
  const closed = await settle(() => !isOpen(widget));
  // A widget still open and displaying nothing did not take the click — most
  // likely it wants trusted input. Saying `selected` would hand the caller a
  // commit that never happened; *which* value arrived is still `verify`'s call.
  if (!closed && !displayedValues(widget).length) return { status: 'did_not_commit' };
  return { status: 'selected' };
}

/**
 * Reads back what the widget now holds, as the widget itself displays it, and
 * says whether that is what was asked for.
 *
 * The comparison is containment in one direction — the committed text must sit
 * inside the chosen option — because a widget may legitimately display a
 * shortened form: the live phone-code widget commits "+358" for the option
 * "Åland Islands +358". The other direction looks equally reasonable and is
 * quietly wrong, since "No" sits inside "Norfolk Island" and would reinstate the
 * exact failure these primitives exist to prevent.
 */
export function verify(
  doc: Document,
  label: string,
  expected: string,
): { status: VerifyStatus; committed: string } {
  const { widget, status } = findWidget(doc, label);
  if (!widget) return { status, committed: '' };

  const committed = displayedValues(widget);
  if (!committed.length) return { status: 'empty', committed: '' };
  // A multi-select widget renders one chip per choice, so the pick is confirmed
  // when any of them accounts for it; reading only the first would report a
  // correct second pick as a failure.
  const wanted = normalizeLabel(expected);
  const matches = committed.some((shown) => wanted.includes(normalizeLabel(shown)));
  return { status: matches ? 'verified' : 'mismatch', committed: committed.join(', ') };
}

/**
 * Runs one step against this document. The content script is a relay, so the
 * mapping from a wire step to a primitive lives here where it is tested; a frame
 * that does not hold the widget answers `not_found` and the background relay
 * keeps whichever frame does.
 */
export function runStep(doc: Document, step: ComboboxStep): Promise<ComboboxReply> {
  switch (step.action) {
    case 'open':
      return open(doc, step.label);
    case 'options':
      return Promise.resolve(options(doc, step.label));
    case 'select':
      return select(doc, step.label, step.value);
    case 'verify':
      return Promise.resolve(verify(doc, step.label, step.value));
  }
}

/** The custom-widget combobox carrying `label`, or null when none does. */
function findWidget(doc: Document, label: string): { widget: Element | null; status: 'not_found' | 'ambiguous' } {
  const widgets = collectQuestions(doc)
    .filter((q) => normalizeLabel(q.label) === normalizeLabel(label))
    .map((q) => q.controls[0])
    .filter(isComboWidget);

  // Guarded by the length check above: index 0 exists.
  if (widgets.length === 1) return { widget: widgets[0]!, status: 'not_found' };
  return { widget: null, status: widgets.length > 1 ? 'ambiguous' : 'not_found' };
}

/**
 * Whether the widget is showing its listbox.
 *
 * A widget that states `aria-expanded` is taken at its word: several libraries
 * keep the listbox in the DOM and hide it, so its mere presence means nothing —
 * treating that as open would report a full option list for a widget nobody
 * opened. Only where the widget makes no such claim does the listbox's own
 * visibility decide.
 */
function isOpen(widget: Element): boolean {
  const expanded = expandedState(widget);
  if (expanded !== null) return expanded === 'true';
  const listbox = comboListbox(widget);
  return listbox !== null && isOnScreen(listbox);
}

/**
 * The widget's own `aria-expanded`, wherever it puts it. ARIA 1.1's combobox
 * pattern places the role and the state on a wrapper around the input, so
 * reading the input alone misses it. Only a combobox ancestor is consulted — a
 * collapsible form section also carries `aria-expanded` and says nothing about
 * this widget.
 */
function expandedState(widget: Element): string | null {
  if (widget.hasAttribute('aria-expanded')) return widget.getAttribute('aria-expanded');
  const owner = widget.closest(
    '[role="combobox"][aria-expanded], [aria-haspopup="listbox"][aria-expanded], [aria-autocomplete][aria-expanded]',
  );
  return owner?.getAttribute('aria-expanded') ?? null;
}

// react-select's rendered value and placeholder, by the emotion class names it
// generates. Only the *reading* path guesses at class names; see displayedValue.
const VALUE_NODE = '[class*="singleValue"], [class*="single-value"], [class*="multiValue"], [class*="multi-value"]';
const PLACEHOLDER_NODE = '[class*="placeholder"]';

/**
 * The values the widget displays — one per chip on a multi-select, empty when it
 * displays none.
 *
 * A react-select keeps its committed value in its own markup — the input's
 * `value` is empty even after a successful commit — so this walks up from the
 * widget looking for the rendered value. It stops the moment the subtree holds a
 * second combobox, which is what makes reporting a *neighbour's* value
 * impossible: on a form of 27 widgets that would be a success reported for a
 * write that never happened. The count uses `COMBO_WIDGET`, the same set
 * `isComboWidget` accepts, so a neighbour declaring itself by `aria-haspopup`
 * rather than `role` still stops the walk.
 */
function displayedValues(widget: Element): string[] {
  for (let el = widget.parentElement; el; el = el.parentElement) {
    if (el.querySelectorAll(COMBO_WIDGET).length > 1) return [];
    const shown = Array.from(el.querySelectorAll(VALUE_NODE));
    if (shown.length) return shown.map(text).filter(Boolean);
    // A visible placeholder means the widget is showing "Select…", not an answer.
    if (el.querySelector(PLACEHOLDER_NODE)) return [];
  }
  return [];
}

/**
 * The pointer sequence a real click produces, dispatched on the element itself:
 * a widget's handler sits on an ancestor and events bubble, so this reaches it
 * without having to guess which ancestor is the widget's "control" by class name.
 */
function press(el: Element): void {
  if (typeof PointerEvent === 'function') {
    el.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, cancelable: true, composed: true }));
  }
  for (const type of ['mousedown', 'mouseup', 'click']) {
    el.dispatchEvent(new MouseEvent(type, { bubbles: true, cancelable: true, composed: true, button: 0 }));
  }
}

/** Waits for a condition the widget's framework may only satisfy on a later tick. */
async function settle(holds: () => boolean): Promise<boolean> {
  for (let waited = 0; waited < SETTLE_MS; waited += POLL_MS) {
    if (holds()) return true;
    await new Promise((resolve) => setTimeout(resolve, POLL_MS));
  }
  return holds();
}

function text(el: Element): string {
  return (el.textContent ?? '').replace(/\s+/g, ' ').trim();
}
