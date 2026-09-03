import { readDismissed, writeDismissed } from './cliPromo';

// Whether the CLI strip has stopped asking, as a reactive fact rather than a storage
// read taken once.
//
// It lives in a shared store rather than inside the component because the strip's
// dismissal is a page-wide fact: the component itself drops its node on it, and any
// surface that has to yield to the strip reads the same value as it changes rather than
// a snapshot taken at mount.

let dismissed = $state(false);

/** Reactive: whether the visitor has closed the strip. */
export function cliBannerDismissed(): boolean {
  return dismissed;
}

/** Seed the flag from storage. Called from the strip's `onMount`; safe to call again. */
export function loadCliBannerDismissed(): void {
  dismissed = readDismissed();
}

/** Record the dismissal for this session and the next. */
export function dismissCliBanner(): void {
  dismissed = true;
  writeDismissed();
}
