// The internal (session-cookie-only) API reference — see ../+page.server.ts for the
// external counterpart. Both share createScalarPageLoad; see its own comment for the
// SSR/memoization details.
import spec from '#lib/docs/generated/api-reference.internal.openapi.json' with { type: 'json' };
import { createScalarPageLoad } from '#lib/docs/scalarSsr.js';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = createScalarPageLoad(spec);
