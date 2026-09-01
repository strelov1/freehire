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

/** How many skills must carry a definition before the glossary is ADVERTISED — linked
 *  from the footer and listed in the sitemap.
 *
 *  The pages themselves exist from the first entry, because the chip's reveal links to
 *  one and a link to a page that does not exist is worse than no link. What waits is the
 *  promise: a footer entry reading "Skills glossary" and a sitemap shard offering one,
 *  both delivering a handful of words, describe something this is not yet. The waves
 *  land over weeks and this flips itself when they do — no second deploy, no flag. */
export const MIN_GLOSSARY_PUBLISHED = 25;

export function isGlossaryPublished(describedSkills: number): boolean {
  return describedSkills >= MIN_GLOSSARY_PUBLISHED;
}

/** Cyrillic letters drawn identically to a Latin one AT THIS CASE. The dictionary
 *  accepts both spellings because postings are written both ways — `1c` carries a Latin
 *  and a Cyrillic `с` — but they render the same, so a page printing both looks broken.
 *
 *  Lowercase only, because `fold` lowercases first. The uppercase pairs (В/B, К/K, М/M,
 *  Н/H, Т/T) do NOT survive that: lowercase Cyrillic `в`, `к`, `м`, `н`, `т` look
 *  nothing like `b`, `k`, `m`, `h`, `t`, and folding them would quietly merge two
 *  spellings a reader can tell apart.
 *
 *  Deliberately not a general confusables table: that is thousands of entries and errs
 *  the same wrong way, dropping distinct spellings to catch collisions nobody has. */
const LOOKALIKES: Readonly<Record<string, string>> = {
  а: 'a',
  е: 'e',
  о: 'o',
  р: 'p',
  с: 'c',
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
 *  `isDescribed` drops the rest of the dead links. Every neighbour is rendered as a link
 *  to /skills/<slug>, which 404s on a skill with no entry — so while coverage is thin an
 *  unfiltered list is a block of broken links on a page whose entire claim is that it is
 *  worth linking to. Filtering before the slice, not after, so a thin patch of coverage
 *  shortens the block only when there is genuinely nothing left to put in it.
 *
 *  Ties break on the slug: the page is server-rendered on every request, and two skills
 *  at the same count must not trade places between one render and the next. */
export function topNeighbours(
  distribution: Readonly<Record<string, number>>,
  slug: string,
  limit: number,
  isDescribed: (slug: string) => boolean
): string[] {
  return Object.entries(distribution)
    .filter(([neighbour]) => neighbour !== slug && isDescribed(neighbour))
    .sort(([aSlug, aCount], [bSlug, bCount]) => bCount - aCount || aSlug.localeCompare(bSlug))
    .slice(0, limit)
    .map(([neighbour]) => neighbour);
}
