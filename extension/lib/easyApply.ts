// LinkedIn Easy Apply: the application is a native <dialog> rendered by LinkedIn's
// server-driven UI. Its class names are generated hashes, so everything here keys on
// the test ids and the SDUI screen name, which describe what the screen is.

/** The open Easy Apply dialog, or null when the page is not showing one. */
export function easyApplyDialog(doc: Document): HTMLDialogElement | null {
  for (const dialog of doc.querySelectorAll<HTMLDialogElement>('dialog[data-testid="dialog"][open]')) {
    if (dialog.querySelector('[data-sdui-screen$=".easyapply.EasyApply"]')) return dialog;
  }
  return null;
}

/**
 * The resume picker: one radio per CV already stored on LinkedIn, labelled with its
 * file name, one of them chosen in advance. It is not a question the candidate is
 * asked, and listing each file as a field buries the real ones.
 */
export function isStoredResumeChoice(el: Element): boolean {
  return (
    el instanceof HTMLInputElement &&
    el.type === 'radio' &&
    /\.(pdf|docx?|rtf|txt)$/i.test(el.getAttribute('aria-label') ?? '')
  );
}

/**
 * The message Easy Apply shows under a field it rejected, or ''. The field names its
 * helper through `aria-describedby`; the helper always holds the live character
 * counter, and a rejection adds a `<p>` of its own beside it ("Invalid input").
 */
export function easyApplyFieldError(el: Element): string {
  for (const id of (el.getAttribute('aria-describedby') ?? '').split(/\s+/).filter(Boolean)) {
    const helper = el.ownerDocument.getElementById(id);
    if (helper?.getAttribute('data-testid') !== 'text-input-helper-text') continue;
    const message = Array.from(helper.children).find((c) => c.tagName === 'P');
    const text = message?.textContent?.trim();
    if (text) return text;
  }
  return '';
}

export type EasyApplyActionKind = 'advance' | 'submit' | 'unknown';

/**
 * The button that moves the dialog on: the last one in its footer (Back sits before
 * it). Found by position, so a step whose button reads in another language is still
 * located. Its kind comes from the text, and any text not recognised is `unknown`:
 * the caller advances only on `advance` and never presses `submit`, so an
 * unrecognised button is left to the candidate rather than guessed.
 */
export function easyApplyAction(dialog: Element): { button: HTMLButtonElement; kind: EasyApplyActionKind } | null {
  const buttons = dialog.querySelectorAll<HTMLButtonElement>('footer button');
  const button = buttons[buttons.length - 1];
  if (!button) return null;
  const text = (button.textContent ?? '').trim();
  let kind: EasyApplyActionKind = 'unknown';
  if (/submit/i.test(text)) kind = 'submit';
  else if (/^(next|continue|review)$/i.test(text)) kind = 'advance';
  return { button, kind };
}
