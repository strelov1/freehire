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

/** One question in the checklist, addressed the way a fill addresses it. */
export interface PlanItem {
  /**
   * Where the question is, for a fill or a reveal to reach it. The label alone
   * does not say: an application and a job-alert signup on the same page each
   * have their own "Email", and an ATS serves the application from its own frame
   * — so both narrowings travel with it.
   */
  label: string;
  frame: number;
  form: number;
  /**
   * Identity for rendering. NOT the address: two questions on one form can carry
   * the same label — an ATS repeats "Years" under several headings, and a page
   * that labels nothing falls back to the same text for each — and a list keyed
   * by a duplicate is a hard render error in Svelte, which takes the whole card
   * down with it. The page's own question index, scoped by frame, is unique by
   * construction and stable between reads of an unchanged form.
   */
  key: string;
  required: boolean;
  answered: boolean;
}

/** How far along the questions that gate submission are. */
interface RequiredProgress {
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
 * The plan with one question ticked off, named by its key.
 *
 * A walk knows what it just wrote, so it says so directly rather than re-reading
 * the whole page per step — the counter moves with the value, not 400ms later
 * when the page's own change notice arrives. A key the plan does not carry
 * changes nothing.
 */
export function markAnswered(plan: ApplyPlan, key: string): ApplyPlan {
  if (!plan.items.some((i) => i.key === key && !i.answered)) return plan;
  return recount(plan.items.map((i) => (i.key === key ? { ...i, answered: true } : i)));
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
 * So the length of the questionnaire counts too — on its own, without asking for
 * `required` in the markup. Plenty of ATS forms mark a question required with an
 * asterisk in its label and nothing a parser can read, and demanding the
 * attribute is why a real application of eleven fields, filled successfully,
 * showed no checklist at all.
 *
 * This decides only what to SHOW; nothing is written on the strength of it, which
 * is what makes the looser test safe here and wrong in the filler.
 */
export function showsApplicationForm(
  fields: FramedField[],
  uploads: unknown[],
  /** `filled` says the panel has just written into this form. Whatever its markup
   *  claims, a page that accepted an autofill is asking an application. */
  evidence: { filled?: boolean } = {},
): boolean {
  if (uploads.length > 0 || evidence.filled) return true;
  return fields.filter((f) => f.label.trim() !== '').length >= QUESTIONNAIRE_SIZE;
}

/** The plan for the questions the page reported, in the order it asks them. */
export function buildPlan(fields: FramedField[]): ApplyPlan {
  const items: PlanItem[] = fields
    // A question with no label cannot be addressed by label, which is how both the
    // fill and the reveal reach one. Counting it would promise a row the panel
    // cannot act on.
    .filter((f) => f.label.trim() !== '')
    .map((f) => ({
      key: `${f.frame}:${f.index}`,
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
