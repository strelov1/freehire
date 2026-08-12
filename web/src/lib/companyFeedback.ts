// The company feedback category vocabulary, mirroring the report-reason picker's
// shape ($lib/reports.ts). Built off the generated COMPANY_FEEDBACK_TYPE_VALUES
// (internal/vocab's CompanyFeedbackTypeValues via cmd/gen-contracts) so a category
// added in Go can't drift from the picker without a type error here.
import { COMPANY_FEEDBACK_TYPE_VALUES, type CompanyFeedbackType } from './generated/contracts';

interface FeedbackTypeOption {
  value: CompanyFeedbackType;
  label: string;
}

const LABELS: Record<CompanyFeedbackType, string> = {
  interview: 'Interview',
  culture: 'Culture',
  compensation: 'Compensation',
  management: 'Management',
  work_life_balance: 'Work-life balance',
  career_growth: 'Career growth',
  other: 'Other',
};

/** The categories in the order they appear in the picker. */
export const companyFeedbackTypes: FeedbackTypeOption[] = COMPANY_FEEDBACK_TYPE_VALUES.map((value) => ({
  value,
  label: LABELS[value],
}));

/** Label for a stored category value (used by the feedback list). */
export function companyFeedbackTypeLabel(type: string): string {
  return LABELS[type as CompanyFeedbackType] ?? type;
}
