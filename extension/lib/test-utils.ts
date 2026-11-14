/** Narrows a possibly-null/undefined value, throwing if it is missing instead
 *  of silently trusting it the way a bare `!` would. */
export function must<T>(value: T | null | undefined, what = 'value'): T {
  if (value == null) throw new Error(`expected ${what} to be non-null`);
  return value;
}

/** Empties the document between DOM tests. */
export function reset() {
  document.body.replaceChildren();
}

/** A labelled text input appended to the document body. */
export function labeledInput(id: string, labelText: string, attrs: Record<string, string> = {}) {
  const label = document.createElement('label');
  label.setAttribute('for', id);
  label.textContent = labelText;
  const input = document.createElement('input');
  input.id = id;
  for (const [k, v] of Object.entries(attrs)) input.setAttribute(k, v);
  document.body.append(label, input);
  return input;
}
