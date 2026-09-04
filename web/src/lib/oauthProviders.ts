// The OAuth providers this app knows how to label, and how to fetch the subset the
// server has actually enabled. Used by /signin (register, sign in, and the recovery
// steps all share one credential form) so the label map and the "unreachable endpoint
// just means no provider buttons" fallback exist in exactly one place.
import { api } from './api';

export const PROVIDER_LABELS: Record<string, string> = {
  google: 'Google',
  github: 'GitHub',
  linkedin: 'LinkedIn',
  apple: 'Apple',
};

/** The enabled OAuth providers this app has a label for, or an empty list if the
 *  endpoint is unreachable — a caller shows no provider buttons rather than failing. */
export async function loadOAuthProviders(): Promise<string[]> {
  try {
    const names = await api.oauthProviders();
    return names.filter((n) => n in PROVIDER_LABELS);
  } catch {
    return [];
  }
}
