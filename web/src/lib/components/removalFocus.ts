// Where focus goes after an experience-bank removal. Each function reads the page BEFORE the
// removal (the row is about to go) and returns what to run once the confirmation dialog has
// closed.
import type { ExperienceAtom, ExperienceEmployment } from '#lib/types.js';

/** The first of `selectors` still in the page, focused. */
function focusFirst(...selectors: (string | null)[]) {
  for (const selector of selectors) {
    if (!selector) continue;
    const el = document.querySelector<HTMLElement>(selector);
    if (el) {
      el.focus();
      return;
    }
  }
}

/** The neighbour a removed item hands focus to: the next one, else the previous. */
function neighbours(el: Element | null, attr: string): [string | null, string | null] {
  const read = (n: Element | null | undefined) => n?.getAttribute(attr) ?? null;
  return [read(el?.nextElementSibling), read(el?.previousElementSibling)];
}

/** A removed job or project card: the next card's edit button, else the previous, else the
 *  section's heading. */
export function employmentRemovalFocus(employment: ExperienceEmployment): () => void {
  const card = document.querySelector(`[data-employment-id="${employment.id}"]`);
  const [next, prev] = neighbours(card, 'data-employment-id');
  const heading = `#experience-section-${employment.kind}`;
  return () =>
    focusFirst(
      next && `[data-employment-id="${next}"] [data-row-action="edit"]`,
      prev && `[data-employment-id="${prev}"] [data-row-action="edit"]`,
      heading,
    );
}

/** A removed achievement: the next row's edit button, else the previous, else a heading. */
export function atomRemovalFocus(atom: ExperienceAtom): () => void {
  const row = document.querySelector(`[data-atom-id="${atom.id}"]`);
  const [next, prev] = neighbours(row, 'data-atom-id');
  // The list the row sat in is gone with its last achievement, so the fallback is the
  // heading above it: the card's own, or for an untied achievement the Projects section
  // it would be saved into.
  const heading = atom.employment_id
    ? `[data-employment-id="${atom.employment_id}"] [data-row-heading]`
    : '#experience-section-unplaced';
  return () =>
    focusFirst(
      next && `[data-atom-id="${next}"] [data-row-action="edit"]`,
      prev && `[data-atom-id="${prev}"] [data-row-action="edit"]`,
      heading,
      '#experience-section-project',
    );
}
