// Validates a post-login return path: only a same-origin rooted path is allowed,
// never a scheme-relative "//host", an absolute URL, or a backslash/control-char
// trick that the URL parser normalizes into one. Mirrors the backend's
// SafeReturnPath. Kept pure (a fixed base, not location) so it is unit-testable and
// identical on server and client.
const BASE = 'https://freehire.dev';

export function safeRedirect(raw: string | null): string | null {
  if (!raw || !raw.startsWith('/')) return null;
  try {
    // Resolving against a fixed origin catches every off-origin bypass: "//host",
    // "/\host" and "/\t/host" all parse to a different origin than BASE.
    const url = new URL(raw, BASE);
    if (url.origin !== BASE) return null;
    return url.pathname + url.search + url.hash;
  } catch {
    return null;
  }
}
