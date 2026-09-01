import { skillLabel } from '$lib/facets';
import { loadSkillDescriptions } from '$lib/skillDescriptions';
import { isGlossaryPublished } from '$lib/skillGlossary';
import type { PageServerLoad } from './$types';

// The glossary index: every skill that has an entry, grouped by the first character of
// its label so the page can be scanned rather than read.
//
// Server-rendered from the catalog module, so the whole list is in the HTML and nothing
// about this page depends on the client fetching a chunk. It is also the crawl path
// that does not depend on a reveal only interaction opens — the sitemap is the other.
export const load: PageServerLoad = async ({ setHeaders }) => {
  const catalog = await loadSkillDescriptions();

  const slugs = Object.keys(catalog);
  const groups = new Map<string, { slug: string; label: string }[]>();
  for (const slug of slugs) {
    const label = skillLabel(slug);
    // Digits and punctuation share one bucket rather than each opening their own: "1C",
    // ".NET" and "C++" are three headings of one entry apiece otherwise. It sorts first,
    // which is where a reader looks for them.
    const first = label[0]?.toUpperCase() ?? '';
    const key = first >= 'A' && first <= 'Z' ? first : '#';
    const bucket = groups.get(key);
    if (bucket) bucket.push({ slug, label });
    else groups.set(key, [{ slug, label }]);
  }

  setHeaders({ 'cache-control': 'public, max-age=0, s-maxage=3600' });

  return {
    total: slugs.length,
    published: isGlossaryPublished(slugs.length),
    groups: [...groups.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([letter, skills]) => ({
        letter,
        skills: skills.sort((a, b) => a.label.localeCompare(b.label)),
      })),
  };
};
