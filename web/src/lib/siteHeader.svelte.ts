/** Whether the site header is slid away (TopBar hides it on scroll down). Shared so
 *  what measures against the header's bottom edge, like the job page's pinned bar,
 *  can follow it; CSS reads the same state through `data-header-hidden` on <html>. */
export const siteHeader = $state({ hidden: false });
