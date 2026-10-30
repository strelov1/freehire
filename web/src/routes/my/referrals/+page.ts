import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// The referrer's panes were `?tab=offers` and `?tab=incoming` of this page before they
// became routes of their own, and those links are still out there — the "new referral
// request" email carried `?tab=incoming` until it moved, and a sent email cannot be fixed.
export const load: PageLoad = ({ url }) => {
  const tab = url.searchParams.get('tab');
  if (tab === 'offers') redirect(308, '/my/referrals/offers');
  if (tab === 'incoming') redirect(308, '/my/referrals/incoming');
};
