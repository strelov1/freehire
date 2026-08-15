// Global controller for the pre-flight "confirm tailor" dialog, the same singleton
// shape as cvRefreshDialog.svelte.ts: askConfirmTailor() is a plain async function
// (called from script logic in JobDrawer/MatchSummary, not a template) and needs to
// await a yes/no from a dialog that must be mounted somewhere in the tree exactly
// once — see ConfirmTailorDialog.svelte, mounted in the root layout beside
// CvRefreshDialog.

import { api } from './api';
import type { JobMatchResult } from './types';

let open = $state(false);
let jobLabel = $state('');
let match = $state.raw<JobMatchResult | null>(null);
let loading = $state(false);
let resolver: ((value: boolean) => void) | null = null;

export const confirmTailorDialog = {
  get open() {
    return open;
  },
  get jobLabel() {
    return jobLabel;
  },
  get match() {
    return match;
  },
  get loading() {
    return loading;
  },
};

/** Ask the candidate to confirm tailoring their CV for `slug`, after showing the
 *  deterministic (no-LLM) skill/requirement check for that job. Resolves true/false
 *  with their choice. `label` is an optional "Title at Company" string for a nicer
 *  title where the caller already has it cheaply — omitted, the dialog falls back to
 *  generic copy rather than forcing every call site to fetch the job just for a
 *  heading. */
export function askConfirmTailor(slug: string, label?: string): Promise<boolean> {
  jobLabel = label ?? '';
  match = null;
  loading = true;
  open = true;
  api
    .getJobMatch(slug)
    .then((m) => {
      match = m;
    })
    .catch(() => {
      // No match to show (e.g. no profile yet) — the dialog still lets the candidate
      // proceed, it just has nothing deterministic to report.
    })
    .finally(() => {
      loading = false;
    });
  return new Promise((resolve) => {
    resolver = resolve;
  });
}

/** Settles the pending ask. Called both by the dialog's own confirm/cancel and by
 *  Escape/backdrop dismissal, which is why it — not the dialog's `open` binding alone —
 *  is what resolves the promise. */
export function settleConfirmTailorDialog(value: boolean) {
  open = false;
  resolver?.(value);
  resolver = null;
}
