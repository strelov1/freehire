import { readDismissed, writeDismissed } from './productHunt';

// Whether the Product Hunt strip has stopped asking, as a reactive fact rather than a
// storage read taken once.
//
// The strip used to keep this in its own `$state`, which was enough while it was the only
// reader. The support toast queues behind it and has to notice the moment it happens: a
// snapshot taken at mount would leave the toast invisible for the rest of the session the
// visitor actually closed the strip in — and before the launch day that is the only way
// the toast can appear at all.

let dismissed = $state(false);

/** Reactive: whether the visitor has closed the strip. */
export function phBannerDismissed(): boolean {
  return dismissed;
}

/** Seed the flag from storage. Called from the strip's `onMount`; safe to call again. */
export function loadPhBannerDismissed(): void {
  dismissed = readDismissed();
}

/** Record the dismissal for this session and the next. */
export function dismissPhBanner(): void {
  dismissed = true;
  writeDismissed();
}
