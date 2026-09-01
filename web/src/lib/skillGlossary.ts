/** Pure helpers behind the /skills glossary — the definition side of the skills facet.
 *
 *  Kept free of fetch and Svelte so it is unit-testable in isolation; the routes fetch
 *  and feed these, the way roleLandings.ts sits behind /roles.
 *
 *  Two of the functions here exist because a block that rendered unconditionally would
 *  misdescribe the catalogue or pad the page. The measurements that put them there are
 *  named at each one — they are properties of how sparse the data is, not style choices. */

/** A skill's postings block renders only above this many open postings. Below it a
 *  median-shaped claim over a handful of rows reads exactly like one over thousands.
 *  The /insights floor, which roleLandings cites for the same reason.
 *
 *  The page itself is NOT gated on this, unlike a /roles pair: a glossary page is about
 *  the definition and the postings illustrate it. Gating the page would take the "what
 *  is X?" link away from precisely the obscure skills a reader does not recognise. */
export const MIN_SKILL_OPEN = 25;

export function showsPostings(openPostings: number): boolean {
  return openPostings >= MIN_SKILL_OPEN;
}

/** Cyrillic letters that are drawn like Latin ones. The dictionary accepts both
 *  spellings because postings are written both ways — `1c` carries a Latin and a
 *  Cyrillic `с` — but they render identically, so a page printing both looks broken.
 *
 *  Only the letters that actually collide. A general confusables table is thousands of
 *  entries and would fold spellings that a reader can in fact tell apart. */
const LOOKALIKES: Readonly<Record<string, string>> = {
  а: 'a',
  в: 'b',
  е: 'e',
  к: 'k',
  м: 'm',
  н: 'h',
  о: 'o',
  р: 'p',
  с: 'c',
  т: 't',
  у: 'y',
  х: 'x',
};

/** The key two spellings share when only an invisible difference separates them. */
function fold(spelling: string): string {
  return [...spelling.toLowerCase()].map((c) => LOOKALIKES[c] ?? c).join('');
}

/** The spellings worth showing a reader: what the parser accepts, minus the skill's own
 *  slug and label, minus anything that only looks different from a spelling already in
 *  the list.
 *
 *  Empty for roughly two canonicals in three — measured, 552 of 863 — which is why the
 *  block that renders this is conditional rather than always present. "Also written as:
 *  javascript" beside a heading reading JavaScript is filler, and filler on two pages in
 *  three is the thin-content failure this glossary set out to avoid.
 *
 *  Order is the dictionary's, which is sorted; nothing here re-ranks. */
export function displayAliases(
  aliases: readonly string[],
  slug: string,
  label: string
): string[] {
  const seen = new Set([fold(slug), fold(label)]);
  const out: string[] = [];
  for (const alias of aliases) {
    const key = fold(alias);
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(alias);
  }
  return out;
}

/** The skills most often named alongside this one, from the skills distribution of the
 *  postings already filtered to it. The skill itself tops that distribution by
 *  construction — it is on every posting counted — so it is dropped.
 *
 *  Ties break on the slug: the page is server-rendered on every request, and two skills
 *  at the same count must not trade places between one render and the next. */
export function topNeighbours(
  distribution: Readonly<Record<string, number>>,
  slug: string,
  limit: number
): string[] {
  return Object.entries(distribution)
    .filter(([neighbour]) => neighbour !== slug)
    .sort(([aSlug, aCount], [bSlug, bCount]) => bCount - aCount || aSlug.localeCompare(bSlug))
    .slice(0, limit)
    .map(([neighbour]) => neighbour);
}
