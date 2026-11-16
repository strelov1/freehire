// Scrolling a form field into view and outlining it for a moment, on the side panel's request.

import { collectQuestions, findQuestion, unfillableLabels } from './form';
import type { RevealRequest } from './protocol';

/**
 * Writes values into the questions carrying the given labels, in one pass:
 * the page is read and written inside a single synchronous walk, so a re-render
 * between an agent's observation and its fills cannot drift the target the way a
 * positional index does. Every requested fill gets an outcome; custom-widget
 * comboboxes are reported as deferred rather than written into.
 *
 * A fill naming a `form` only matches a question inside that form — the frame's
 * own signup form sharing a label ("Email") with the application form must not
 * absorb a fill meant for the other. A fill naming NONE matches only where the
 * label is carried once; where it is carried by several questions the fill is
 * refused as `ambiguous` and nothing is written, because from here the application
 * and the signup are indistinguishable and the wrong write is silent.
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
  // Reveal reports one boolean — the panel either scrolled to the question or says
  // it was not there — so every miss reads the same here, ambiguity included. That
  // is right for a reveal: with nothing written, there is nothing to be wrong about,
  // and highlighting one of two same-labeled questions would assert a choice this
  // has no basis for.
  const resolved = findQuestion(
    unfillableLabels(doc),
    collectQuestions(doc),
    Array.from(doc.querySelectorAll('form')),
    label,
    form,
  );
  if ('miss' in resolved) return false;

  const el = resolved.found.controls[0];
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
