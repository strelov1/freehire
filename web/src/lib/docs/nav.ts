// The docs navigation model, derived from the api-spec data. It owns the URL
// scheme for the per-endpoint pages (one route per endpoint) so the nav, the
// dynamic route loader, and the landing index all resolve slugs the same way.

import { OVERVIEW, GROUPS, type Endpoint } from './api-spec';

/** Anchor/segment slug. Diacritics are folded (é→e) so an accented title like
 *  "Profile & résumé" yields a clean `profile-resume`, not a lossy `profile-r-sum`. */
export function slugify(s: string): string {
  return s
    .normalize('NFKD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '');
}

/** A group's URL segment. */
export const groupSlug = (title: string) => slugify(title);

// An endpoint's URL segment within its group: the path (param names kept, so
// `/jobs` and `/jobs/{slug}` don't collide) with any leading group-name segment
// trimmed for tidier URLs, suffixed by the method to disambiguate verb pairs
// that share a path (POST vs DELETE /jobs/{slug}/save).
function baseSlug(gSlug: string, ep: Endpoint): string {
  let base = slugify(ep.path);
  if (base === gSlug) base = '';
  else if (base.startsWith(gSlug + '-')) base = base.slice(gSlug.length + 1);
  const method = ep.method.toLowerCase();
  return base ? `${base}-${method}` : `${gSlug}-${method}`;
}

export interface NavEndpoint {
  slug: string;
  href: string;
  method: Endpoint['method'];
  path: string;
  endpoint: Endpoint;
}

export interface NavGroup {
  slug: string;
  title: string;
  endpoints: NavEndpoint[];
}

/** Concept sections that live on the landing page, linked as landing anchors. */
export const CONCEPTS = [
  ...OVERVIEW.map((o) => ({ id: slugify(o.title), title: o.title })),
  { id: 'filtering-jobs', title: 'Filtering jobs' },
];

/** The grouped endpoint navigation, with a unique route per endpoint. */
export const NAV: NavGroup[] = GROUPS.map((g) => {
  const gSlug = groupSlug(g.title);
  const seen = new Set<string>();
  const endpoints = g.endpoints.map((ep) => {
    // Guard against a same-slug collision within a group (deterministic by order).
    let slug = baseSlug(gSlug, ep);
    while (seen.has(slug)) slug += '-x';
    seen.add(slug);
    return {
      slug,
      href: `/docs/api/${gSlug}/${slug}`,
      method: ep.method,
      path: ep.path,
      endpoint: ep,
    };
  });
  return { slug: gSlug, title: g.title, endpoints };
});

/** Resolve a (group, endpoint) slug pair to its nav entry, or null. */
export function findEndpoint(gSlug: string, epSlug: string): { group: NavGroup; entry: NavEndpoint } | null {
  const group = NAV.find((g) => g.slug === gSlug);
  if (!group) return null;
  const entry = group.endpoints.find((e) => e.slug === epSlug);
  return entry ? { group, entry } : null;
}

/** Every endpoint route, for prerender enumeration. */
export const allEntries = () =>
  NAV.flatMap((g) => g.endpoints.map((e) => ({ group: g.slug, endpoint: e.slug })));
