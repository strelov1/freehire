// The external (non-session) API reference — see internal/+page.server.ts for the
// session-cookie-only counterpart. Both render their Scalar fragment at build time;
// see #lib/docs/scalarSsr for why.
import scalarHtml from '#lib/docs/generated/api-reference.openapi.json?scalar-html';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = () => ({ scalarHtml });
