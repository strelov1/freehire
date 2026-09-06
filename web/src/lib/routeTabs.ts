// Which tab of a routed tab strip a pathname is on. Shared because the three account
// sections that navigate with one (`/my/activity`, `/my/notifications`, `/my/tracking`)
// each had their own copy of the same rule, and two of the copies matched on a bare
// prefix — which also makes `/my/tracking/listing` the List tab.
//
// The longest matching route wins, since a section's index route is a path-prefix of
// every other tab in it. A tab matches its own route or something under it, nothing
// else, and a path matching none falls back to the index — the closest ancestor a
// reader would expect to land back on.
//
// Kept free of Svelte imports so the rule stays pure and unit-testable, mirroring
// accountNav.ts.
export function activeRouteTab<T extends string>(
  pathname: string,
  tabs: readonly { id: T; href: string }[],
  fallback: T,
): T {
  let best = fallback;
  let bestLength = -1;
  for (const tab of tabs) {
    const matches = pathname === tab.href || pathname.startsWith(`${tab.href}/`);
    if (matches && tab.href.length > bestLength) {
      best = tab.id;
      bestLength = tab.href.length;
    }
  }
  return best;
}
