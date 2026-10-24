import type { FacetDef } from './facets';

/** The facets the job sidebar's hand-ordered groups did not show, so each still gets a
 *  group of its own under its declared label. The ordered list is presentation — which
 *  facets share a heading, which come first — and a facet missing from it must cost
 *  only its position, never its chip: a filter narrowing the feed with nothing to
 *  remove it is the untruth the summary exists to prevent. */
export function unsummarized(
  facets: readonly FacetDef[],
  shown: ReadonlySet<string>,
  exclude: readonly string[],
): FacetDef[] {
  return facets.filter((d) => !shown.has(d.param) && !exclude.includes(d.param));
}
