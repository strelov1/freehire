// Global controller for the pre-flight "confirm tailor" dialog, the same singleton
// shape as cvRefreshDialog.svelte.ts: askConfirmTailor() is a plain async function
// (called from script logic in JobView/JobDrawer, not a template) and needs to
// await a yes/no from a dialog that must be mounted somewhere in the tree exactly
// once — see ConfirmTailorDialog.svelte, mounted in the root layout beside
// CvRefreshDialog.

import { api } from './api';
import type { CvBaseItem } from './cv';
import type { Allowance, JobMatchResult } from './types';

let open = $state(false);
let jobLabel = $state('');
let match = $state.raw<JobMatchResult | null>(null);
let allowance = $state.raw<Allowance | null>(null);
let loading = $state(false);
let bases = $state.raw<CvBaseItem[]>([]);
let baseCvId = $state('');
let resolver: ((value: TailorChoice | null) => void) | null = null;

/** What the candidate confirmed. `baseCvId` is set only when they had a choice to make (two or
 *  more bases); with one base the server's default is the only answer and nothing is sent. */
export interface TailorChoice {
  baseCvId?: string;
}

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
  /** Where the candidate stands on today's tailoring sessions, or null when it could not
   *  be read. The dialog says what confirming will cost before they commit — and when
   *  there is nothing left, says so rather than offering a button that answers 402. */
  get allowance() {
    return allowance;
  },
  get loading() {
    return loading;
  },
  /** The candidate's base CVs. The picker shows only for two or more. */
  get bases() {
    return bases;
  },
  /** The base picked to tailor from, preselected to the default. */
  get baseCvId() {
    return baseCvId;
  },
};

/** Ask the candidate to confirm tailoring their CV for `slug`, after showing the
 *  deterministic (no-LLM) skill/requirement check for that job. Resolves the confirmed
 *  choice (with the base to tailor from when they had more than one), or null when they
 *  cancel. `label` is an optional "Title at Company" string for a nicer
 *  title where the caller already has it cheaply — omitted, the dialog falls back to
 *  generic copy rather than forcing every call site to fetch the job just for a
 *  heading. */
export function askConfirmTailor(slug: string, label?: string): Promise<TailorChoice | null> {
  jobLabel = label ?? '';
  bases = [];
  baseCvId = '';
  match = null;
  allowance = null;
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
  // Which bases there are to pick from. Its own request and failure again: when it cannot be
  // read the candidate still proceeds, on the default base, rather than losing the dialog.
  api
    .listBaseCvs()
    .then((items) => {
      bases = items;
      baseCvId = items.find((b) => b.is_default)?.id ?? items[0]?.id ?? '';
    })
    .catch(() => {
      bases = [];
    });
  // Its own request and its own failure: what the plan allows is a separate question from
  // how well the candidate fits, and a plan read that fails must not take the fit check
  // down with it. The dialog simply says nothing about cost in that case.
  refreshConfirmTailorAllowance();
  return new Promise((resolve) => {
    resolver = resolve;
  });
}

/** Re-reads the tailoring allowance. Called on open, and again when the candidate comes
 *  back to the tab — the upgrade link opens /my/plan in a new one, so a purchase made
 *  there should turn this dialog's refusal into a working confirm without a reload. */
export function refreshConfirmTailorAllowance() {
  api
    .myPlan()
    .then((p) => {
      allowance = p.allowances.find((a) => a.feature === 'tailor') ?? null;
    })
    .catch(() => {
      allowance = null;
    });
}

/** Settles the pending ask. Called both by the dialog's own confirm/cancel and by
 *  Escape/backdrop dismissal, which is why it — not the dialog's `open` binding alone —
 *  is what resolves the promise. */
export function settleConfirmTailorDialog(value: boolean) {
  open = false;
  resolver?.(value ? (bases.length >= 2 && baseCvId ? { baseCvId } : {}) : null);
  resolver = null;
}

/** The picker's selection. */
export function chooseConfirmTailorBase(id: string) {
  baseCvId = id;
}
