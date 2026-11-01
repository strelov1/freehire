import { INDUSTRY_ALIASES, INDUSTRY_LABELS, INDUSTRY_VALUES, type Industry } from '#lib/generated/contracts.js';

const industryAliases: Partial<Record<Industry, readonly string[]>> = INDUSTRY_ALIASES;
const industryLabels: Record<Industry, string> = INDUSTRY_LABELS;

export type IndustryLookup =
  | { type: 'canonical' | 'alias'; slug: string; label: string }
  | { type: 'miss' };

export interface IndustryOption {
  slug: string;
  label: string;
}

export function seedDictionaryReturnParams(
  params: URLSearchParams,
  kind: 'specialization' | 'skill' | 'industry',
  term: string,
): void {
  params.set(kind, term);
  if (kind === 'industry') params.set('tab', 'industry');
}

// Keep the same ASCII folding as industrytag.normalize in Go. Alias strings and
// canonical labels are generated from that package, avoiding a second vocabulary.
function normalize(term: string): string {
  // Go's strings.ToLower uses simple mappings; JavaScript expands U+0130 into i
  // plus a combining dot, which would become a separator in the ASCII fold below.
  const lower = term.trim().replaceAll('\u0130', 'i').toLowerCase();
  return lower
    .replaceAll('&', ' and ')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '');
}

export function lookupIndustry(term: string): IndustryLookup {
  const key = normalize(term);
  if (!key) return { type: 'miss' };
  for (const slug of INDUSTRY_VALUES) {
    if (industryAliases[slug]?.includes(key) && slug !== key) {
      return { type: 'alias', slug, label: industryLabels[slug] ?? slug };
    }
  }
  for (const slug of INDUSTRY_VALUES) {
    if (key === slug || key === normalize(industryLabels[slug] ?? slug)) {
      return { type: 'canonical', slug, label: industryLabels[slug] ?? slug };
    }
  }
  return { type: 'miss' };
}

export function isIndustryMiss(term: string): boolean {
  return term.trim().length > 0 && lookupIndustry(term).type === 'miss';
}

export function searchIndustries(term: string): IndustryOption[] {
  const query = normalize(term);
  // A term that folds to nothing ("!!!", non-Latin script) matches nothing; only a
  // blank search lists the whole dictionary.
  if (!query && term.trim()) return [];
  return INDUSTRY_VALUES
    .filter((slug) => {
      if (!query) return true;
      return slug.includes(query)
        || normalize(industryLabels[slug] ?? slug).includes(query)
        || (industryAliases[slug] ?? []).some((alias) => alias.includes(query));
    })
    .map((slug) => ({
      slug,
      label: industryLabels[slug] ?? slug,
    }))
    .sort((a, b) => a.label.localeCompare(b.label));
}
