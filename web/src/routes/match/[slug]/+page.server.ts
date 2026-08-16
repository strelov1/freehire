import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// The standalone match-analysis page was removed; the Tailor workspace is now the sole
// surface for viewing and triggering the analysis (see job-fit-analysis's "Fit analysis
// surfaces in the tailoring workspace" requirement — the design doc's own name). Old
// links keep working via redirect.
export const load: PageServerLoad = ({ params }) => {
  redirect(308, `/tailor/${params.slug}`);
};
