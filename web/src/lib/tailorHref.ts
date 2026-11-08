import { resolve } from '$app/paths';

/** The tailoring workspace for a vacancy. A chosen base travels as `?base=<id>`: the workspace
 *  makes its tailor request on load and again on every reload, so the choice has to live in the
 *  URL rather than in client memory. Omitted, the server uses the default base. */
export function tailorHref(slug: string, baseCvId?: string): string {
  const path = resolve('/tailor/[slug]', { slug });
  return baseCvId ? `${path}?base=${encodeURIComponent(baseCvId)}` : path;
}
