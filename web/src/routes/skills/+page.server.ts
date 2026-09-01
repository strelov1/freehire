import { skillLabel } from '$lib/facets';
import { loadSkillDescriptions } from '$lib/skillDescriptions';
import type { PageServerLoad } from './$types';

// The glossary index: every skill that has an entry, grouped by the first character of
// its label so the page can be scanned rather than read.
//
// Server-rendered from the catalog module, so the whole list is in the HTML and nothing
// about this page depends on the client fetching a chunk. It is also the crawl path
// that does not depend on a reveal only interaction opens — the sitemap is the other.
export const load: PageServerLoad = async ({ setHeaders }) => {
  const catalog = await loadSkillDescriptions();

  const groups = new Map<string, { slug: string; label: string }[]>();
  for (const slug of Object.keys(catalog)) {
    const label = skillLabel(slug);
    // Digits and punctuation share one bucket rather than each opening their own: "1C",
    // ".NET" and "C++" are three headings of one entry apiece otherwise.
    const first = label[0]?.toUpperCase() ?? '';
    const key = first >= 'A' && first <= 'Z' ? first : '#';
    groups.set(key, [...(groups.get(key) ?? []), { slug, label }]);
  }

  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });

  return {
    total: Object.keys(catalog).length,
    groups: [...groups.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([letter, skills]) => ({
        letter,
        skills: skills.sort((a, b) => a.label.localeCompare(b.label)),
      })),
  };
};
