import { WORK_MODE_VALUES } from './generated/contracts';
import { WORK_MODE_LABELS, REGION_LABELS, titleCase } from './labels';
import type { FacetOption } from './facets';

export const WORK_MODE_OPTIONS: FacetOption[] = WORK_MODE_VALUES.map((value) => ({
  value,
  label: WORK_MODE_LABELS[value] ?? titleCase(value),
}));
export const REGION_OPTIONS: FacetOption[] = Object.entries(REGION_LABELS).map(([value, label]) => ({ value, label }));

const regionNames = (() => {
  try {
    return new Intl.DisplayNames(['en'], { type: 'region' });
  } catch {
    return null;
  }
})();

export function countryLabel(code: string): string {
  const up = code.toUpperCase();
  try {
    return regionNames?.of(up) ?? up;
  } catch {
    return up;
  }
}
