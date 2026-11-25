// Shared Scalar API Reference configuration, used identically by the SSR render
// (+page.server.ts, via renderApiReferenceToString) and the client-side hydration
// (ScalarReference.svelte, via createApiReference) — for both the external and
// internal reference — so the two never disagree on anything but how the spec
// reaches them: the server imports the generated file directly (no self-fetch),
// the client fetches it as an ordinary cacheable static asset rather than
// duplicating the whole spec inline in the page's own data payload.
import type { AnyApiReferenceConfiguration } from '@scalar/types/api-reference';

const SCALAR_SPEC_URL = '/api-reference.openapi.json';
export const SCALAR_INTERNAL_SPEC_URL = '/api-reference.internal.openapi.json';
export const SCALAR_DATA_SPEC_URL = '/api-reference.data.openapi.json';

const SCALAR_BASE_CONFIG = {
  layout: 'modern',
  // The site has its own theme toggle (TopBar); Scalar's own would be a second,
  // unsynced dark-mode control fighting the first.
  hideDarkModeToggle: true,
} satisfies Partial<AnyApiReferenceConfiguration>;

export function scalarConfigFromContent(spec: Record<string, unknown>): AnyApiReferenceConfiguration {
  return { ...SCALAR_BASE_CONFIG, content: spec };
}

/** `/docs/api`'s own spec by default; pass SCALAR_INTERNAL_SPEC_URL or SCALAR_DATA_SPEC_URL for `/docs/api/internal` or `/docs/api/data`. */
export function scalarConfigFromUrl(url: string = SCALAR_SPEC_URL): AnyApiReferenceConfiguration {
  return { ...SCALAR_BASE_CONFIG, url };
}
