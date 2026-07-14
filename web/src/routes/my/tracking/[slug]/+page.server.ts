import { error, redirect } from '@sveltejs/kit';
import { ApiError } from '$lib/api';
import { serverApi } from '$lib/server/api';
import type { PageServerLoad } from './$types';

// The application detail page shows the mail linked to an application, so it is
// moderator-only like the rest of the inbox surface. Server-render the application
// (with its linked emails) for an instant paint; a signed-out visitor bounces to
// sign-in, a non-moderator to their tracking board, and an untracked slug is a 404.
export const load: PageServerLoad = async ({ parent, params, fetch, request }) => {
  const { user } = await parent();
  if (!user) {
    const target = `/my/tracking/${params.slug}`;
    redirect(302, `/?auth=required&redirect=${encodeURIComponent(target)}`);
  }
  if (user.role !== 'moderator') {
    redirect(302, '/my/tracking');
  }
  const application = await serverApi(fetch, request.headers.get('cookie'))
    .getTrackedApplication(params.slug)
    .catch((e) => {
      if (e instanceof ApiError && e.status === 404) error(404, 'Application not found');
      throw e;
    });
  return { application };
};
