// Which of a page's forms is the application, and which label fills go to which frame.

import { matchFieldKey, normalizeLabel } from './fieldKeys';
import type { LabelFill } from './protocol';

/**
 * Whether the page is showing an application form at all.
 *
 * A careers page routinely keeps the application behind an "Apply" button and
 * shows a job-alert signup meanwhile — on Roku that signup asks for a first
 * name, a last name and an email, all required, which no count of fields can
 * distinguish from a short application. The resume upload can: see
 * `extractUploads`.
 */
export function looksLikeApplication(uploads: { form: number }[]): boolean {
  return uploads.length > 0;
}

/**
 * Narrows an observation to the one form the application is asking, identified
 * by the upload sitting in it. Without this a page carrying both an application
 * and a job-alert signup gets both written, since each has its own "Email".
 *
 * Both the frame and the form identify the group: an ATS iframe numbers its own
 * forms from zero, so the frame alone would merge two unrelated first forms.
 * When the upload names a group holding no questions — a page rendering them
 * outside its form element — every field is kept, because filling nothing at all
 * is the worse answer.
 */
export function scopeToApplication<T extends { frame: number; form: number }>(
  fields: T[],
  uploads: { frame: number; form: number }[],
): T[] {
  const [target] = uploads;
  if (!target) return fields;
  const scoped = fields.filter((f) => f.frame === target.frame && f.form === target.form);
  return scoped.length > 0 ? scoped : fields;
}

/**
 * The fills a profile justifies against the questions a page asks: each
 * recognised label paired with the value it maps to. A label the profile knows
 * nothing about, or knows only as a blank, is left for the user rather than
 * written over with an empty string.
 *
 * A repeated label is asked for once, and the one fill kept carries the frame and
 * form of the FIRST field that offered it — which is the application's, because
 * `scopeToApplication` has already narrowed the input to it. Keeping the repeats
 * would send a second fill addressed at whatever else carries the label. Pure over
 * its input; the page is not touched here.
 */
export function planLabelFills(
  fields: { label: string; frame?: number; form?: number }[],
  values: Record<string, string>,
): LabelFill[] {
  const asked = new Set<string>();
  const fills: LabelFill[] = [];
  for (const { label, frame, form } of fields) {
    const key = matchFieldKey(label);
    const value = key ? (values[key] ?? '') : '';
    if (!value || asked.has(normalizeLabel(label))) continue;
    asked.add(normalizeLabel(label));
    fills.push({ label, value, frame, form });
  }
  return fills;
}

/**
 * The fills a given frame should act on: those addressed to it by
 * `deterministicAutofill`, plus any fill naming no frame at all — a
 * `fill_simple` tool call, which is still offered to every frame as before
 * frame scoping existed. Used by the background relay to stop broadcasting a
 * frame-addressed fill to frames that do not hold the target control, which is
 * what let a same-labeled field in another frame's form get filled instead.
 */
export function fillsForFrame(fills: LabelFill[], frame: number): LabelFill[] {
  return fills.filter((f) => f.frame === undefined || f.frame === frame);
}
