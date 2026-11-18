// A single-line <input> does not scroll its own text to a selection made from code, so a slot
// past the visible width (most bullets are longer than the panel) would be selected out of
// sight. Measure the text in front of it in the field's own font and scroll there, leaving a
// third of the width for context. A <textarea> wraps, so its selection is already in view.
export function revealSelection(field: HTMLInputElement | HTMLTextAreaElement, at: number) {
  if (field instanceof HTMLTextAreaElement) return;
  const ctx = document.createElement('canvas').getContext('2d');
  if (!ctx) return;
  ctx.font = getComputedStyle(field).font;
  const before = ctx.measureText(field.value.slice(0, at)).width;
  field.scrollLeft = Math.max(0, before - field.clientWidth / 3);
}
