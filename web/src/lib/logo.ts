// logo.dev's name endpoint resolves a company logo by name (no domain needed).
// `fallback=404` makes it return 404 — rather than a generic monogram — when it
// has no logo, so each consumer can pick its own fallback (the globe icon in the
// SPA, a monogram tile in the OG card). The token is a logo.dev publishable key,
// designed to live in a client img src; it is domain-restricted (the allowlist —
// freehire.dev + localhost for dev — is configured against the key in the logo.dev
// dashboard, since the browser attaches the page origin as the Referer).
export const LOGO_DEV_TOKEN = 'pk_fKgOEo0OT4KXvUshjPMGAQ';

/** The logo.dev image URL for a company name, or null when there is no name. */
export function logoDevUrl(name: string): string | null {
  if (!name) return null;
  return `https://img.logo.dev/name/${encodeURIComponent(name)}?token=${LOGO_DEV_TOKEN}&fallback=404`;
}
