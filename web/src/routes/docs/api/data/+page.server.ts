// The paid API plans' extra reference: the original-link reads and the catalogue export. Rendered at
// build time like its siblings; see #lib/docs/scalarSsr for why.
import scalarHtml from '#lib/docs/generated/api-reference.data.openapi.json?scalar-html';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = () => ({ scalarHtml });
