// Global controller for the CV-refresh confirm dialog, the same singleton shape as
// auth-dialog.svelte.ts: offerCvRefresh() is a plain async function (called from script
// logic, not a template) and needs to await a yes/no from a dialog that must be mounted
// somewhere in the tree exactly once — see CvRefreshDialog.svelte, mounted in the root
// layout beside CookieConsent and SupportToast.

let open = $state(false);
let message = $state('');
let resolver: ((value: boolean) => void) | null = null;

export const cvRefreshDialog = {
  get open() {
    return open;
  },
  get message() {
    return message;
  },
};

/** Ask the candidate to confirm a CV refresh; resolves true/false with their choice.
 *  The default `confirm` offerCvRefresh() reaches for when the caller doesn't inject one. */
export function askCvRefresh(msg: string): Promise<boolean> {
  message = msg;
  open = true;
  return new Promise((resolve) => {
    resolver = resolve;
  });
}

/** Settles the pending ask. Called both by the dialog's own confirm/cancel and by
 *  Escape/backdrop dismissal, which is why it — not the dialog's `open` binding alone —
 *  is what resolves the promise. */
export function settleCvRefreshDialog(value: boolean) {
  open = false;
  resolver?.(value);
  resolver = null;
}
