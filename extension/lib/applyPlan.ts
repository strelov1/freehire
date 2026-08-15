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

/**
 * How many questions a page has to ask before it counts as an application on the
 * strength of the questions alone. Below it sit the forms every site has — a
 * sign-in, a search box, a newsletter field.
 */
const QUESTIONNAIRE_SIZE = 4;

/**
 * Whether the page is showing an application worth accounting for.
 *
 * A CV upload settles it: that is what `looksLikeApplication` keys on, and it is
 * what lets the FILLER tell an application from a job-alert signup before writing
 * anything. But the upload sits on the FIRST step of a multi-step ATS form, and
 * the steps after it — the ones full of screening questions the candidate most
 * needs an account of — offer no file field at all. Requiring one there is why a
 * form full of questions showed no checklist.
 *
 * So a long questionnaire that requires answers counts too. This decides only
 * what to SHOW; nothing is written on the strength of it, which is what makes the
 * looser test safe here and wrong in the filler.
 */
export function showsApplicationForm(fields: FramedField[], uploads: unknown[]): boolean {
  if (uploads.length > 0) return true;
  const questions = fields.filter((f) => f.label.trim() !== '');
  return questions.length >= QUESTIONNAIRE_SIZE && questions.some((f) => f.required);
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
