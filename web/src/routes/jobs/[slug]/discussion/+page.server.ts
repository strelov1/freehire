import { redirect } from '@sveltejs/kit';
import { resolve } from '$app/paths';
import type { PageServerLoad } from './$types';

// The job's discussion is a tab on the job page now; this address stays for the links
// already out there (and a thread's "Back to discussion").
export const load: PageServerLoad = ({ params }) => {
  redirect(301, `${resolve('/jobs/[slug]', { slug: params.slug })}?tab=discussion`);
};
