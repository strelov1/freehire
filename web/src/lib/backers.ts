// Presentation for backer collections — company tags naming the accelerator or fund
// that selected the company.
//
// Membership and the editorial/credential/backer split come from the generated
// registry (cmd/gen-contracts ← internal/collections). What lives here is only what
// a badge needs to render: which mark to draw, and the sentence that keeps the badge
// from reading as a claim about the role being viewed. A backer added upstream with
// no mark below renders nothing, which a test catches.
import { COLLECTIONS } from './generated/contracts';

export type BackerBadge = {
  slug: string;
  /** The brand's name, e.g. "Y Combinator". */
  label: string;
  /** The brand's own square mark, committed under web/static/brands. */
  mark: string;
  /** Accessible name — the badge is an image, so this is its entire meaning. */
  alt: string;
};

// Each brand's own mark, taken from the brand's own site. Not resolved at runtime
// from the logo proxy: it matches by company name, and "a16z" sent it to an
// unrelated Substack page — a plausible-looking wrong mark is worse than none.
const MARKS: Record<string, string> = {
  yc: '/brands/yc.png',
  techstars: '/brands/techstars.png',
  'a16z-portfolio': '/brands/a16z.png',
  'a16z-speedrun': '/brands/a16z.png',
};

/**
 * Badges for the backer tags in a company's or job's collection list, in registry
 * order. Editorial collections, credentials, unknown slugs and an absent list all
 * yield nothing — a mark we do not have is better shown as nothing than guessed at.
 */
export function backerBadges(collections: string[] | undefined | null): BackerBadge[] {
  if (!collections?.length) return [];
  const held = new Set(collections);
  return COLLECTIONS.filter((c) => c.kind === 'backer' && held.has(c.slug)).flatMap((c) => {
    // flatMap, not filter-then-map: only the lookup narrows MARKS' index signature,
    // so reading it once is what makes the result total rather than a cast.
    const mark = MARKS[c.slug];
    if (!mark) return [];
    return [
      {
        slug: c.slug,
        label: c.title,
        mark,
        // "backed by" states the relationship the tag actually asserts: the fund
        // picked the company. It says nothing about this role, which is the reading
        // a candidate would otherwise act on.
        alt: `Backed by ${c.title}`,
      },
    ];
  });
}
