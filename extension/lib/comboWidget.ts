// How a custom combobox widget is recognised and read — its listbox, its option nodes,
// the values it displays — split out of form.ts.
import type { Fillable } from './form';

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
export function comboOptions(el: Element): string[] {
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
 * The ways a widget declares itself a combobox. One constant because anything that
 * reads *around* a widget (`displayedValues`) must recognise the same set as
 * `isComboWidget`: a guard counting only `[role=combobox]` would walk straight past
 * a neighbour that says `aria-haspopup="listbox"` instead, and report that
 * neighbour's value.
 */
const COMBO_WIDGET = '[role="combobox"], [aria-autocomplete], [aria-haspopup="listbox"]';

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
export function displayedValues(widget: Element): string[] {
  for (let el = widget.parentElement; el; el = el.parentElement) {
    if (el.querySelectorAll(COMBO_WIDGET).length > 1) return [];
    const shown = Array.from(el.querySelectorAll(VALUE_NODE));
    if (shown.length) return shown.map(shownText).filter(Boolean);
    // A visible placeholder means the widget is showing "Select…", not an answer.
    if (el.querySelector(PLACEHOLDER_NODE)) return [];
  }
  return [];
}

/** An element's text as a person reads it: whitespace collapsed, ends trimmed. */
export function shownText(el: Element): string {
  return (el.textContent ?? '').replace(/\s+/g, ' ').trim();
}

export function collapse(text: string | null | undefined): string {
  return (text ?? '').replace(/\s+/g, ' ').trim();
}
