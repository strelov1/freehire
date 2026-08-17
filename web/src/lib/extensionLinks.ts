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
