// Resolves a company logo for the OG card. satori cannot fetch remote images
// itself, so we fetch our logo proxy server-side and hand back a data-URI it can
// embed. Any failure (404 = no logo, timeout, network) returns null so the card
// falls back to a monogram — a missing logo must never fail the image render.

import { companyLogoUrl } from '$lib/logo';

const TIMEOUT_MS = 2500;

/** A `data:` URI for the company's logo, or null to signal "use the monogram". */
export async function resolveLogo(company: string): Promise<string | null> {
  const url = companyLogoUrl(company);
  if (!url) return null;

  try {
    const res = await fetch(url, { signal: AbortSignal.timeout(TIMEOUT_MS) });
    if (!res.ok) return null;
    const bytes = Buffer.from(await res.arrayBuffer());
    if (bytes.length === 0) return null;
    const type = res.headers.get('content-type') || 'image/png';
    return `data:${type};base64,${bytes.toString('base64')}`;
  } catch {
    return null;
  }
}
