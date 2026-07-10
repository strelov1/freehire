// Pure presentation logic for the AI fit-analysis block. Kept out of the component so
// it is unit-testable (vitest) without a DOM: the verdict/score → colour tone, and the
// ATS requirement-status → label + tone. The wire shapes come from the Go contract.

/** A colour tone bucket the component maps to Tailwind classes. */
export type Tone = 'strong' | 'good' | 'moderate' | 'weak' | 'poor';

/** verdictTone buckets a 0-100 overall score into a colour tone, on the same
 *  thresholds the backend uses to derive the verdict label (kept in sync by hand —
 *  the label is authoritative, this only drives colour). */
export function verdictTone(overall: number): Tone {
  if (overall >= 75) return 'strong';
  if (overall >= 60) return 'good';
  if (overall >= 45) return 'moderate';
  if (overall >= 30) return 'weak';
  return 'poor';
}

/** Display metadata for one ATS requirement-match status: a short human label and a
 *  tone. `covered`/`synonym-only` are positive, `missing-have` is a fixable near-miss,
 *  `missing-gap` is a genuine gap. An unknown status falls back to a neutral label. */
export function requirementStatusMeta(status: string): { label: string; tone: Tone } {
  switch (status) {
    case 'covered':
      return { label: 'Covered', tone: 'strong' };
    case 'synonym-only':
      return { label: 'Synonym', tone: 'good' };
    case 'missing-have':
      return { label: 'Add it', tone: 'moderate' };
    case 'missing-gap':
      return { label: 'Gap', tone: 'poor' };
    default:
      return { label: status, tone: 'moderate' };
  }
}
