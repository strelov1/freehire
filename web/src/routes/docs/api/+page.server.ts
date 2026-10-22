// The external (non-session) API reference — see internal/+page.server.ts for the
// session-cookie-only counterpart. Both share createScalarPageLoad; see its own
// comment for the SSR/memoization details.
import spec from '#lib/docs/generated/api-reference.openapi.json' with { type: 'json' };
import { createScalarPageLoad } from '#lib/docs/scalarSsr.js';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = createScalarPageLoad(spec);
