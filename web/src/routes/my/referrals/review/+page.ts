import { redirect } from '@sveltejs/kit';

// Referral-offer review has moved into the unified moderator console. Keep the old
// moderator link/bookmark working by redirecting to its section there.
export const load = () => {
  redirect(308, '/moderation?tab=referrals');
};
