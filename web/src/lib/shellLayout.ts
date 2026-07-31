// Which routes render as a full-bleed, app-like shell rather than a centered document.
// Kept as a pure predicate (not inline in TopBar) so the list of such routes is
// unit-testable and lives in one place. These pages size themselves as
// `h-[calc(100dvh-3.5rem)]` — the viewport minus the header — and carry their own
// left-edge icon rail, so a header centered inside `max-w-6xl` would float above a page
// that already reaches both screen edges.

/** True on the agent chat and the CV tailoring workspace, the two full-viewport surfaces. */
export function isFullBleedRoute(pathname: string): boolean {
  return (
    pathname === '/my/assistant' ||
    pathname.startsWith('/my/assistant/') ||
    pathname.startsWith('/tailor/')
  );
}
