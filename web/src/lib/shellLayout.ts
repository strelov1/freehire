// Which routes render as a full-bleed, app-like shell rather than a centered document.
// Kept as a pure predicate (not inline in TopBar) so the list of such routes is
// unit-testable and lives in one place. These pages size themselves as
// `h-[calc(100dvh-3.5rem)]` — the viewport minus the header — and carry their own
// left-edge icon rail, so a header centered inside `max-w-6xl` would float above a page
// that already reaches both screen edges. `+layout.svelte` also reads this to hide the
// footer, which belongs only here: a full-viewport shell has no room for one, but a page
// that merely wants a wider header (see isWideHeaderRoute below) is still an ordinary
// scrolling document that ends in one.

/** True on the agent chat and the CV tailoring workspace, the two full-viewport surfaces. */
export function isFullBleedRoute(pathname: string): boolean {
  return (
    pathname === '/my/assistant' ||
    pathname.startsWith('/my/assistant/') ||
    pathname.startsWith('/tailor/')
  );
}

// Routes whose header should span the full width instead of centering inside
// `max-w-6xl`, without the rest of isFullBleedRoute's consequences (no footer, no
// viewport-height sizing). `/docs/api` and `/docs/api/internal` are the two cases today:
// both render the same Scalar three-column layout of sidebar, content and
// request/response examples that reads the same way every full-bleed surface does — a
// centered header floating narrower than the columns beneath it — but each stays a
// normal scrolling document (Scalar's own sidebar is `position: sticky`, not a
// height-constrained pane) and still wants the site footer at the bottom of it.

/** True for either Scalar-rendered API reference page — the one route pair that
 *  both isWideHeaderRoute and TopBar's search-box suppression need to recognize
 *  as a group, kept here as the single place that lists them. */
export function isApiReferenceRoute(pathname: string): boolean {
  return pathname === '/docs/api' || pathname === '/docs/api/internal';
}

/** True wherever the header should go edge to edge — every isFullBleedRoute plus the
 *  external and internal API references. */
export function isWideHeaderRoute(pathname: string): boolean {
  return isFullBleedRoute(pathname) || isApiReferenceRoute(pathname);
}

/** True anywhere in the signed-in account area, `/my` and everything under it. The
 *  promos that stay out of a candidate's workspace (the CLI banner, the support toast)
 *  ask this one question, so "the account area" cannot mean two things. */
export function isAccountArea(pathname: string): boolean {
  return pathname === '/my' || pathname.startsWith('/my/');
}
