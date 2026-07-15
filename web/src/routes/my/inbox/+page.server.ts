import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

// The mail inbox is a restricted rollout — moderators OR beta testers. The API 403s
// everyone else, so redirect the rest to their profile instead of rendering a page
// whose every fetch would fail. The server gate (RequireModeratorOrBeta) remains the
// real boundary.
export const load: PageServerLoad = async ({ parent }) => {
  const { user } = await parent();
  if (!(user?.role === 'moderator' || user?.beta_tester)) {
    redirect(302, '/my/profile');
  }
  return {};
};
