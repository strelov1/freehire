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
  label: string;
  required: boolean;
  answered: boolean;
  /** The frame and form the question was read from — a fill and a reveal need both
   *  to reach the same question when a page carries more than one form. */
  frame: number;
  form: number;
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
