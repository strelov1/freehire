/**
 * The panel's account of the application form in front of the user: the questions
 * the page asks, which of them are answered, and how far along the required ones
 * are.
 *
 * Pure over the fields the page reported — no DOM, no messaging — so the counter
 * arithmetic and the answered rule are testable on their own, the same discipline
 * `scraper.ts` and `form.ts` keep. The unit is the QUESTION the page reported, not
 * the control: a legend over 29 country checkboxes arrives as one field and stays
 * one line here.
 */

import type { FramedField } from './protocol';

/**
 * Which question, exactly. The label alone does not say: an application and a
 * job-alert signup on the same page each have their own "Email", and an ATS
 * serves the application from its own frame — so both narrowings travel with it.
 */
export interface FieldAddress {
  label: string;
  frame: number;
  form: number;
}

/** One question in the checklist, addressed the way a fill addresses it. */
export interface PlanItem extends FieldAddress {
  required: boolean;
  answered: boolean;
}

/** How far along the questions that gate submission are. */
export interface RequiredProgress {
  answered: number;
  total: number;
  /** Rounded to whole percent — the panel shows a figure, not a measurement. */
  percent: number;
}

export interface ApplyPlan {
  items: PlanItem[];
  /** Null when the form marks nothing required: a percentage over zero required
   *  questions states nothing, so there is no counter to show. */
  required: RequiredProgress | null;
}

/**
 * The plan with one question ticked off, by label.
 *
 * A walk knows what it just wrote, so it says so directly rather than re-reading
 * the whole page per step — the counter moves with the value, not 400ms later
 * when the page's own change notice arrives. A label the plan does not carry
 * changes nothing.
 */
export function markAnswered(plan: ApplyPlan, at: FieldAddress): ApplyPlan {
  const isTarget = (i: PlanItem) => i.label === at.label && i.frame === at.frame && i.form === at.form;
  if (!plan.items.some((i) => isTarget(i) && !i.answered)) return plan;
  return recount(plan.items.map((i) => (isTarget(i) ? { ...i, answered: true } : i)));
}

/** The plan for the questions the page reported, in the order it asks them. */
export function buildPlan(fields: FramedField[]): ApplyPlan {
  const items: PlanItem[] = fields
    // A question with no label cannot be addressed by label, which is how both the
    // fill and the reveal reach one. Counting it would promise a row the panel
    // cannot act on.
    .filter((f) => f.label.trim() !== '')
    .map((f) => ({
      label: f.label,
      required: f.required,
      answered: f.value.trim() !== '',
      frame: f.frame,
      form: f.form,
    }));

  return recount(items);
}

/** The counter, derived from the items. The one place the arithmetic lives, so a
 *  plan built from a fresh read and one ticked off in place agree. */
function recount(items: PlanItem[]): ApplyPlan {
  const required = items.filter((i) => i.required);
  if (required.length === 0) return { items, required: null };
  const answered = required.filter((i) => i.answered).length;
  return {
    items,
    required: {
      answered,
      total: required.length,
      percent: Math.round((answered / required.length) * 100),
    },
  };
}
