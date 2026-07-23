// Reactive cookie-consent state: the single source of truth for "may trackers
// start" and "should the banner show". Pure classification and persistence live in
// consent.ts (unit-tested); this module adds the $state runes on top, so it is
// verified visually rather than in the plain-node vitest environment.
import { browser } from '$app/environment';
import { type ConsentChoice, needsBanner, readConsent, regionNeedsConsent, writeConsent } from './consent';

function resolveTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone ?? '';
  } catch {
    return '';
  }
}

// The region is fixed for the session (a visitor's timezone does not change
// mid-visit), so it is a plain const. The choice and the forced-open flag are
// reactive so the banner and the tracker gate react to Accept/Reject and to a
// footer-driven re-open. On the server nothing is consent-required and no choice
// exists — the banner and gate resolve entirely on the client.
const regionRequired = browser ? regionNeedsConsent(resolveTimezone()) : false;

let choice = $state<ConsentChoice | null>(browser ? readConsent() : null);
let forcedOpen = $state(false);

/** Whether non-essential trackers may run now. An explicit choice is authoritative
 *  for everyone: `denied` always blocks (so a rejection is honoured even outside the
 *  consent-required region), `granted` always allows. With no choice yet, trackers
 *  run only for a visitor who is not consent-required. */
export function trackersAllowed(): boolean {
  if (choice === 'denied') return false;
  if (choice === 'granted') return true;
  return !regionRequired;
}

/** Whether the consent banner should render — a consent-required visitor with no
 *  choice, or one who re-opened it from the footer to change a prior choice. */
export function bannerVisible(): boolean {
  return forcedOpen || needsBanner(regionRequired, choice);
}

/** Record consent and close the banner. The caller starts the trackers. */
export function grant(): void {
  choice = 'granted';
  forcedOpen = false;
  writeConsent('granted');
}

/** Record refusal and close the banner. No tracker starts. */
export function deny(): void {
  choice = 'denied';
  forcedOpen = false;
  writeConsent('denied');
}

/** Re-open the banner so a previously recorded choice can be changed
 *  (withdrawal as easy as granting). */
export function reopen(): void {
  forcedOpen = true;
}
