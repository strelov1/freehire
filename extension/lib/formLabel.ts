// How a form control is named for a person: the label text the autofill matches questions by.
import { collapse } from './comboWidget';
import type { Fillable } from './form';

/** Best-effort human label for a control, in decreasing reliability. */
export function extractLabel(el: Fillable): string {
  const fromLabels = Array.from(el.labels ?? [])
    .map((l) => l.textContent?.trim() ?? '')
    .filter(Boolean)
    .join(' ');
  if (fromLabels) return fromLabels;

  const placeholder = collapse(el.getAttribute('placeholder'));
  const ariaLabel = collapse(el.getAttribute('aria-label'));
  return (
    textFromIds(el.ownerDocument, el.getAttribute('aria-labelledby')) ||
    // LinkedIn names a radio after its QUESTION; the option's own words sit beside it.
    (isChoice(el) ? visibleTextBeside(el) : '') ||
    // A typeahead often repeats its placeholder as aria-label; the visible label is the question.
    (ariaLabel === placeholder ? '' : ariaLabel) ||
    visibleTextBeside(el, 'label') ||
    placeholder ||
    ariaLabel ||
    collapse(el.getAttribute('name'))
  );
}

function isChoice(el: Fillable): boolean {
  return el instanceof HTMLInputElement && (el.type === 'radio' || el.type === 'checkbox');
}

/**
 * The text a person reads next to a control the page did not tie to a `<label>`: that of the
 * nearest ancestor (three levels at most) holding no other control. `only` narrows it to a
 * descendant matching that selector; without it the ancestor's whole text counts, capped so a
 * container full of prose is not mistaken for an option.
 */
function visibleTextBeside(el: Fillable, only?: string): string {
  let box = el.parentElement;
  for (let depth = 0; box && depth < 3; depth++, box = box.parentElement) {
    if (box.querySelectorAll('input, select, textarea').length > 1) return '';
    const text = collapse(only ? box.querySelector(only)?.textContent : box.textContent);
    if (text && (only || text.length <= 80)) return text;
  }
  return '';
}

/** The joined text of the elements an `aria-labelledby`-style id list names. */
export function textFromIds(doc: Document, ids: string | null): string {
  if (!ids) return '';
  return ids
    .split(/\s+/)
    .map((id) => collapse(doc.getElementById(id)?.textContent))
    .filter(Boolean)
    .join(' ');
}
