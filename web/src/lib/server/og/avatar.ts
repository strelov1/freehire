// Resolves a contributor's GitHub avatar for the OG card. satori cannot fetch remote
// images itself, so we fetch it server-side and hand back a data-URI it can embed.
// Any failure (a deleted account, a timeout, a slow CDN) returns null so the card falls
// back to the shared monogram — a missing avatar must never fail the image render.
//
// Mirrors ./logo.ts, which does the same for company logos through our own proxy. The
// difference is the host: this one is GitHub's, so the URL is pinned to it.

const TIMEOUT_MS = 2500;

// The URL is read from a snapshot this repository commits, so it is trusted by
// provenance — but this function turns a string into a request the server makes, and
// that is worth bounding at the door rather than by where it came from. Compared as a
// parsed hostname, never as a prefix: `avatars.githubusercontent.com.evil.test` starts
// with the right characters and is a different host.
const AVATAR_HOST = 'avatars.githubusercontent.com';

/** A `data:` URI for the contributor's avatar, or null to signal "use the monogram". */
export async function resolveAvatar(url: string): Promise<string | null> {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return null;
  }
  if (parsed.protocol !== 'https:' || parsed.hostname !== AVATAR_HOST) return null;

  try {
    const res = await fetch(parsed, { signal: AbortSignal.timeout(TIMEOUT_MS) });
    if (!res.ok) return null;
    const bytes = Buffer.from(await res.arrayBuffer());
    if (bytes.length === 0) return null;
    const type = res.headers.get('content-type') || 'image/png';
    return `data:${type};base64,${bytes.toString('base64')}`;
  } catch {
    return null;
  }
}
