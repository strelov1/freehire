// The job-match score as the workspace panel shows it: rows that read as "earned / weight"
// when they were scored and as a reason when they could not be, an impact label taken from
// the weight the server sent, and the requirement ledger split by what the document
// actually says. Kept out of the component so the wording and the never-render-an-
// unavailable-category-as-zero rule are unit-tested rather than eyeballed in a browser.
import type { CvJobMatch } from '$lib/cv';
import type { LineItem, RequirementCheck } from '$lib/generated/contracts';

export interface JobMatchRow {
  id: string;
  label: string;
  earned: number;
  weight: number;
  available: boolean;
  /** How much this row can move the overall, from the weight the response carried. */
  impact: string;
  /** What the row shows beside its label: 'earned / weight', or nothing when unavailable. */
  text: string;
  /** Why the row could not be scored. Set only when `available` is false. */
  reason?: string;
  items: LineItem[];
}

export interface JobMatchView {
  overall: number;
  /** True when the overall was taken over fewer than all four categories, so the panel can
   *  say so. A score out of three categories is a different claim from one out of four. */
  partial: boolean;
  rows: JobMatchRow[];
  missingSkills: string[];
  matchedSkills: string[];
  requirements: {
    covered: RequirementCheck[];
    missing: RequirementCheck[];
    /** Requirements naming no skill this vacancy states. They carry the earlier analysis's
     *  status and were excluded from the score — the panel must label them, never mix them
     *  in with the checked ones. */
    unverifiable: RequirementCheck[];
  };
}

const ALL_CATEGORIES = 4;

/**
 * viewJobMatch turns the endpoint's response into what the panel renders, or null when
 * there is nothing to show. An unavailable score is an absence, not an error state, so the
 * caller has one falsy case to handle instead of two.
 */
export function viewJobMatch(resp: CvJobMatch | null | undefined): JobMatchView | null {
  const score = resp?.available ? resp.score : undefined;
  if (!score) return null;

  const rows: JobMatchRow[] = (score.categories ?? []).map((c) => ({
    id: c.id,
    label: c.label,
    earned: c.earned,
    weight: c.weight,
    available: c.available,
    impact: impactOf(c.weight),
    // An unavailable category shows its reason and NO number. Rendering it as "0 / 10"
    // would say the CV failed a check that was never run.
    text: c.available ? `${c.earned} / ${c.weight}` : '',
    reason: c.available ? undefined : c.reason,
    items: c.items ?? [],
  }));

  const requirements = { covered: [], missing: [], unverifiable: [] } as JobMatchView['requirements'];
  for (const r of score.requirements ?? []) {
    if (r.coverage === 'covered') requirements.covered.push(r);
    else if (r.coverage === 'missing') requirements.missing.push(r);
    else requirements.unverifiable.push(r);
  }

  return {
    overall: score.overall,
    partial: (score.contributing ?? []).length < ALL_CATEGORIES,
    rows,
    missingSkills: score.missing_skills ?? [],
    matchedSkills: score.matched_skills ?? [],
    requirements,
  };
}

/** impactOf labels how much a category can move the overall, read off the weight the
 *  RESPONSE carried rather than a table the client keeps — so a server-side re-weighting
 *  can never leave the label disagreeing with the score it labels. */
export function impactOf(weight: number): string {
  if (weight >= 30) return 'High impact';
  if (weight >= 15) return 'Medium impact';
  return 'Low impact';
}

export type JobMatchTone = 'strong' | 'moderate' | 'poor';

/** scoreTone grades an overall on the same bands the match verdict uses, so two scores in one
 *  workspace never colour the same number differently. */
export function scoreTone(overall: number): JobMatchTone {
  if (overall >= 75) return 'strong';
  if (overall >= 45) return 'moderate';
  return 'poor';
}
