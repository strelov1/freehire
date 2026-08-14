/** Session-scoped dismiss after the candidate declines a CV refresh offer so a burst
 *  of bank edits does not become a confirm storm. Survives in-page mutations; clears
 *  when the tab closes. */
export const CV_REFRESH_DISMISSED_KEY = 'fh.cv-refresh-offer.dismissed';

export const TAILOR_REFRESH_MESSAGE =
  'Update this tailored CV from your experience bank and résumé? Template and typography stay; content edits can be undone from History.';

export const BASE_REFRESH_MESSAGE =
  'Update your base CV from your experience bank and résumé? Template and typography stay. Tailored copies for specific jobs are not changed.';

export function isCvRefreshDismissed(storage: Pick<Storage, 'getItem'> | null = defaultSession()): boolean {
  if (!storage) return false;
  try {
    return storage.getItem(CV_REFRESH_DISMISSED_KEY) === '1';
  } catch {
    return false;
  }
}

export function dismissCvRefreshOffer(storage: Pick<Storage, 'setItem'> | null = defaultSession()): void {
  if (!storage) return;
  try {
    storage.setItem(CV_REFRESH_DISMISSED_KEY, '1');
  } catch {
    // private mode / blocked storage — treat as in-memory only (next call may re-ask)
  }
}

function defaultSession(): Storage | null {
  if (typeof sessionStorage === 'undefined') return null;
  return sessionStorage;
}

/** Ask whether to rebuild a CV from the current seed after a bank edit.
 *  Decline is a no-op on the document and dismisses further offers this tab session.
 *
 *  `confirm` is required rather than defaulted to a real dialog on purpose: a default
 *  would need to import cvRefreshDialog.svelte.ts, which uses runes, and this module
 *  is exercised by cvRefreshOffer.test.ts under the plain-Node vitest project that
 *  loads no Svelte plugin — see that config's comment. Real call sites pass
 *  `confirm: askCvRefresh` themselves. */
export async function offerCvRefresh(opts: {
  message: string;
  apply: () => Promise<void>;
  confirm: (message: string) => boolean | Promise<boolean>;
  dismissed?: boolean;
  onDismiss?: () => void;
}): Promise<'applied' | 'declined' | 'skipped'> {
  if (opts.dismissed ?? isCvRefreshDismissed()) return 'skipped';
  const ok = await opts.confirm(opts.message);
  if (!ok) {
    (opts.onDismiss ?? dismissCvRefreshOffer)();
    return 'declined';
  }
  await opts.apply();
  return 'applied';
}
