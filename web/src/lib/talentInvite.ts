// Talent Network invitation: dismissal persistence, kept free of Svelte runes and
// SvelteKit imports so it unit-tests in the plain-node vitest environment. The markup
// lives in components/TalentNetworkInvite.svelte — the same seam cliPromo.ts and
// cvRefreshOffer.ts draw for the banners beside it.
//
// The card is mounted by the account shell, so it is on screen for every `my/*` page.
// That is exactly why the storage access below is guarded: unreachable localStorage
// (a blocked origin, storage switched off) throws on ACCESS, and an unguarded read in
// the card's `onMount` would fail the mount of the whole account section rather than
// one banner.

/** localStorage key holding the candidate's dismissal of the invitation.
 *
 *  Deliberately NOT the camelCase `hire.talentInviteDismissed` this shipped as for a
 *  few hours: that spelling files a dismissal under the convention the PREFERENCE keys
 *  use (`hire.myNavCollapsed`, `hire.jobFilters`), while every sibling dismissal is
 *  kebab (`hire.cli-banner-dismissed`, `hire.support-toast-dismissed`). Abandoning the
 *  old key shows the card once more to whoever closed it in that window — the smallest
 *  possible cost, paid once, for keys that read as one family from here on. */
export const TALENT_INVITE_DISMISSED_KEY = 'hire.talent-invite-dismissed';

/** Whether the candidate has closed the invitation. Absent or unreadable storage reads
 *  as "not dismissed" — showing the card to someone who closed it is a smaller harm
 *  than a thrown error in the account shell. */
export function isTalentInviteDismissed(): boolean {
  try {
    return localStorage.getItem(TALENT_INVITE_DISMISSED_KEY) === '1';
  } catch {
    return false;
  }
}

/** Record that the candidate closed the invitation. Silently a no-op when storage is
 *  unavailable — the card then simply returns on the next page load. */
export function dismissTalentInvite(): void {
  try {
    localStorage.setItem(TALENT_INVITE_DISMISSED_KEY, '1');
  } catch {
    /* storage unavailable; the dismissal lasts for this page only */
  }
}
