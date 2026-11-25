// The Data plan's API reference: the slice of /docs/api a Data plan holder buys. Rendered at
// build time like its siblings; see #lib/docs/scalarSsr for why.
import scalarHtml from '#lib/docs/generated/api-reference.data.openapi.json?scalar-html';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = () => ({ scalarHtml });
