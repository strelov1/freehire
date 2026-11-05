// The internal (session-cookie-only) API reference — see ../+page.server.ts for the
// external counterpart. Both render their Scalar fragment at build time; see
// #lib/docs/scalarSsr for why.
import scalarHtml from '#lib/docs/generated/api-reference.internal.openapi.json?scalar-html';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = () => ({ scalarHtml });
