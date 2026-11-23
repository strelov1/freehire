import { resolve } from '$app/paths';
import type { Path, ResolvedPathname } from '$app/types';

/** A navigation pathname, spelled the way `page.url.pathname` spells it: leading slash
 *  included. Checked against the app's real routes, so a link to a page that does not
 *  exist is a compile error. */
export type Pathname = `/${Path}`;

/** The {@link Pathname} in the form `resolve()` takes. SvelteKit 3 reads a leading slash
 *  as a route ID and accepts a pathname only WITHOUT one — and a route ID is not always
 *  the path (`/my/assistant` is the route `/my/assistant/[[id]]`). The nav lists keep
 *  the slash because every active-state check compares them with `page.url.pathname`,
 *  so the slash comes off here, at the `resolve(toPath(href))` that makes the link.
 *  Generic so a list's literal union survives: `resolve()` relates a union argument
 *  only while it is small, and the whole `Path` union is not. */
export function toPath<P extends Path>(pathname: `/${P}`): P {
  return pathname.slice(1) as P;
}

/** Resolve a validated pathname even when it comes from a large union of static links. */
export function resolvePathname(pathname: Pathname): ResolvedPathname {
  const resolveConcretePath = resolve as (path: Path) => ResolvedPathname;
  return resolveConcretePath(pathname.slice(1) as Path);
}
