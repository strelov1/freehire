import { SvelteSet } from 'svelte/reactivity';
import { browser } from 'wxt/browser';
import { buildPlan, markAnswered, showsApplicationForm, type ApplyPlan } from '../../lib/applyPlan';
import { normalizeLabel } from '../../lib/fieldKeys';
import { scopeToApplication } from '../../lib/fillPlan';
import type { FramedField, FramedUpload, RuntimeMessage } from '../../lib/protocol';

/**
 * The application-form plan: the panel's standing account of the form in front of the
 * user — what it asks, what is answered, how far the required questions are along. It
 * exists before any fill and outlives every one, which is the difference from the
 * notices — those describe one action and are gone by the next.
 */
export class FormPlan {
  plan = $state<ApplyPlan | null>(null);
  /** The question a walk is filling right now, by label. */
  fillingLabel = $state<string | null>(null);
  /** How many labelled questions the last read found, whether or not they added
   *  up to an application. It is what the panel says when it shows no checklist,
   *  so "nothing appeared" can be told from "nothing was found". */
  questionsSeen = $state(0);
  /** Why the last read produced nothing, when it failed outright. */
  planError = $state('');
  /** True once a walk has written into this page's form. It outranks every guess
   *  about whether the page is showing an application: it accepted values. Reset
   *  by a page change, with the plan. */
  formFilled = $state(false);
  /** The questions on this page whose answers freehire already holds, by normalised
   *  label: the ones this panel wrote (from the profile or the bank) and the ones just
   *  saved. Neither is offered for saving again. Reset by a page change, with
   *  `formFilled`. */
  readonly autofilled = new SvelteSet<string>();
  /** The last read's questions, scoped as the plan is — with their values, which the
   *  plan does not carry. What the "save my answers" offer is computed from. */
  formFields = $state<FramedField[]>([]);
  /** The upload fields the last read found, across every frame — MatchCard's "Attach
   *  tailored CV" action is gated on at least one being in the TOP frame specifically
   *  (see `pickAttachableUpload`), not merely on this list being non-empty: an upload
   *  that exists only inside an embedded iframe is one this action can never reach. */
  uploads = $state<FramedUpload[]>([]);
  /** A LinkedIn Easy Apply dialog is open: its resume step takes a file through its own picker. */
  easyApply = $state(false);
  /** The page is a LinkedIn job that no longer takes applications. */
  closed = $state(false);

  // Reads of the form are serialised rather than raced. A request id (the
  // `matchRequestId` pattern) was the first attempt and it deadlocked on a real
  // ATS page: the page announces changes continuously, so every read found its id
  // already superseded by the next one and returned before assigning anything —
  // the checklist never appeared at all. One read at a time, with a re-read queued
  // if anything arrived meanwhile, cannot starve and cannot land stale.
  #reading = false;
  #stale = false;
  readonly #afterRead: () => void;

  /** `afterRead` runs after every read that landed, to follow the application. */
  constructor(afterRead: () => void) {
    this.#afterRead = afterRead;
  }

  /** Reads the page's form and rebuilds the plan, or clears it for a page that
   *  is not showing an application. */
  async refresh() {
    if (this.#reading) {
      this.#stale = true;
      return;
    }
    this.#reading = true;
    try {
      do {
        this.#stale = false;
        const reply = (await browser.runtime.sendMessage({
          kind: 'GET_FRAMED_FORM',
        } satisfies RuntimeMessage)) as RuntimeMessage | undefined;
        this.#applyRead(reply);
        this.#afterRead();
      } while (this.#stale);
    } catch (err) {
      // A read that could not happen is not a page without a form: say so rather
      // than clearing a checklist the user is looking at.
      this.planError = err instanceof Error ? err.message : 'could not read the form';
    } finally {
      this.#reading = false;
    }
  }

  /** Turns one form read into the plan, or into the reason there is none. */
  #applyRead(reply: RuntimeMessage | undefined) {
    this.planError = '';
    if (reply?.kind !== 'FRAMED_FORM') {
      this.questionsSeen = 0;
      this.plan = null;
      this.uploads = [];
      this.easyApply = false;
      this.closed = false;
      this.formFields = [];
      return;
    }
    this.uploads = reply.uploads;
    this.easyApply = reply.easyApply === true;
    this.closed = reply.closed === true;
    this.questionsSeen = reply.fields.filter((f) => f.label.trim() !== '').length;
    if (!showsApplicationForm(reply.fields, reply.uploads, { filled: this.formFilled, easyApply: reply.easyApply })) {
      this.plan = null;
      this.formFields = [];
      return;
    }
    // Scoped to the one form the upload identifies, where there is one: an
    // application and a job-alert signup each have their own "Email". With no
    // upload to scope by — every step of an ATS form after the first — the page's
    // questions ARE the form.
    this.formFields = reply.uploads.length > 0 ? scopeToApplication(reply.fields, reply.uploads) : reply.fields;
    this.plan = buildPlan(this.formFields);
  }

  /** Ticks one question off the plan, narrowed by whatever address the caller
   *  has: a fill knows its frame and form, the agent's report knows the label
   *  alone. An address that matches nothing leaves the plan as it was. */
  tickOff(label: string, frame?: number, form?: number) {
    this.autofilled.add(normalizeLabel(label));
    if (!this.plan) return;
    const item = this.plan.items.find(
      (i) => i.label === label && (frame === undefined || i.frame === frame) && (form === undefined || i.form === form),
    );
    if (item) this.plan = markAnswered(this.plan, item.key);
  }
}
