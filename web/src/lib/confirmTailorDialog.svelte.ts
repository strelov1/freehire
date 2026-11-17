// Global controller for the pre-flight "confirm tailor" dialog, the same singleton
// shape as cvRefreshDialog.svelte.ts: askConfirmTailor() is a plain async function
// (called from script logic in JobView/JobDrawer, not a template) and needs to
// await a yes/no from a dialog that must be mounted somewhere in the tree exactly
// once — see ConfirmTailorDialog.svelte, mounted in the root layout beside
// CvRefreshDialog.

import { notInPlan } from './allowance';
import { api } from './api';
import type { CvBaseItem } from './cv';
import { proMonthlyPrice } from './proPrice';
import type { Allowance, JobMatchResult, PublicPrice } from './types';

let open = $state(false);
let slug = $state('');
let jobLabel = $state('');
let match = $state.raw<JobMatchResult | null>(null);
let allowance = $state.raw<Allowance | null>(null);
let loading = $state(false);
let bases = $state.raw<CvBaseItem[]>([]);
let baseCvId = $state('');
let proOffer = $state.raw<PublicPrice | null>(null);
// Bumped by every ask, so a plan or price read that outlives its ask cannot offer a
// purchase to the next one.
let askId = 0;
// Bumped by every plan read, so only the newest one may set the allowance or the offer.
let refreshSeq = 0;
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
  /** The vacancy being tailored for — what a purchase made from here must come back to. */
  get slug() {
    return slug;
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
  /** The monthly Pro price, offered only to an account KNOWN to be on Free whose plan has no
   *  tailoring sessions; null for a paid account, an unread plan or a price not on sale. */
  get proOffer() {
    return proOffer;
  },
};

/** Ask the candidate to confirm tailoring their CV for `slug`, after showing the
 *  deterministic (no-LLM) skill/requirement check for that job. Resolves the confirmed
 *  choice (with the base to tailor from when they had more than one), or null when they
 *  cancel. `label` is an optional "Title at Company" string for a nicer
 *  title where the caller already has it cheaply — omitted, the dialog falls back to
 *  generic copy rather than forcing every call site to fetch the job just for a
 *  heading. */
export function askConfirmTailor(forSlug: string, label?: string): Promise<TailorChoice | null> {
  askId++;
  slug = forSlug;
  jobLabel = label ?? '';
  bases = [];
  baseCvId = '';
  match = null;
  allowance = null;
  proOffer = null;
  loading = true;
  open = true;
  api
    .getJobMatch(forSlug)
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
  const mine = askId;
  const seq = ++refreshSeq;
  const latest = () => seq === refreshSeq && mine === askId;
  api
    .myPlan()
    .then(async (p) => {
      if (!latest()) return;
      allowance = p.allowances.find((a) => a.feature === 'tailor') ?? null;
      // The server's own answer decides who may be offered Pro, never the cached session tier.
      if (p.plan !== 'free' || !notInPlan(allowance)) {
        proOffer = null;
        return;
      }
      if (proOffer) return;
      const price = proMonthlyPrice(await api.plans().catch(() => null));
      if (latest()) proOffer = price;
    })
    .catch(() => {
      if (latest()) allowance = null;
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
