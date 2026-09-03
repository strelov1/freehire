// Where the browser extension is installed from — shared by the landing's visible
// buttons (ExtensionLandingView.svelte) and the SoftwareApplication JSON-LD
// (routes/features/extension), so `installUrl` can never point somewhere the page
// does not offer. Same reason cliLinks.ts exists.
//
// The store's own share links carry `?hl=` and `?utm_source=ext_sidebar`; both are
// dropped here — a locale pin would override the visitor's, and the tracking
// parameter describes a click that did not happen from the sidebar.
export const EXTENSION_STORE_URL =
  'https://chromewebstore.google.com/detail/freehire/ijfaechijopdlikalojadpojmpilplnj';

/** What the panel does, in three lines — the claims the extension's own landing leads
 *  with, and the ones the homepage card repeats to earn the click that lands there.
 *
 *  Shared for the same reason the store URL is: a visitor who clicks through holds a
 *  promise, and the page they arrive on has to be the one that made it. */
export const EXTENSION_CLAIMS = [
  'Reads the page itself',
  'Scores it against your CV',
  'Fills the application form',
] as const;
