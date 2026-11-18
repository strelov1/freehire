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

/** A <form> holding a labelled control, and optionally a resume upload. */
export function formWith(id: string, labels: string[], upload = false) {
  const form = document.createElement('form');
  form.id = id;
  for (const text of labels) {
    const label = document.createElement('label');
    const control = document.createElement('input');
    control.type = 'text';
    control.id = `${id}-${text}`;
    label.setAttribute('for', control.id);
    label.textContent = text;
    form.append(label, control);
  }
  if (upload) {
    const file = document.createElement('input');
    file.type = 'file';
    form.append(file);
  }
  document.body.append(form);
  return form;
}
