import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// The mail inbox is a moderator-only rollout — the API 403s everyone else, so
// redirect non-moderators to their profile instead of rendering a page whose every
// fetch would fail. The server role check (RequireRole) remains the real boundary.
export const load: PageServerLoad = async ({ parent }) => {
  const { user } = await parent();
  if (user?.role !== 'moderator') {
    redirect(302, '/my/profile');
  }
  return {};
};
